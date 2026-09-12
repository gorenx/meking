"""Bounded noun-phrase analysis contract and worker implementation."""

from __future__ import annotations

import asyncio
import importlib.metadata
import re
import sys
import unicodedata
from collections.abc import Callable
from pathlib import Path
from typing import Annotated, Literal

import nltk
from fastapi import Depends, FastAPI, Request
from pydantic import Field, model_validator
from textblob import TextBlob

from meking_analysis_sidecar.contract import CONTRACT_VERSION, StrictModel
from meking_analysis_sidecar.errors import AnalysisError
from meking_analysis_sidecar.resources import (
    NLTK_RESOURCE_IDENTITIES,
    directory_digest,
)

TextUnitID = Annotated[str, Field(min_length=1, max_length=128)]
TextUnitText = Annotated[str, Field(max_length=1_000_000)]
Phrase = Annotated[str, Field(min_length=1, max_length=4096)]
Noun = Annotated[str, Field(min_length=1, max_length=256)]

NLTK_DATA_ROOT = Path(sys.prefix) / "nltk_data"
nltk.data.path.insert(0, str(NLTK_DATA_ROOT))


class RegexEnglishAnalyzer(StrictModel):
    """Configure the deterministic English noun-phrase policy."""

    extractor_type: Literal["regex_english"]
    exclude_nouns: Annotated[list[Noun], Field(max_length=4096)]
    max_word_length: Annotated[int, Field(ge=1, le=1_000_000)]
    word_delimiter: Annotated[str, Field(min_length=1, max_length=32)]

    @model_validator(mode="after")
    def reject_control_characters(self) -> "RegexEnglishAnalyzer":
        values = [self.word_delimiter, *self.exclude_nouns]
        if any(
            any(unicodedata.category(character) == "Cc" for character in value)
            for value in values
        ):
            raise ValueError("noun phrase configuration contains control characters")
        return self


class NounPhraseTextUnit(StrictModel):
    """One caller-owned text unit without a path or persisted graph identity."""

    id: TextUnitID
    text: TextUnitText


class NounPhraseRequest(StrictModel):
    """Versioned batch whose one analyzer applies to every input."""

    contract_version: Literal[2]
    analyzer: RegexEnglishAnalyzer
    text_units: Annotated[list[NounPhraseTextUnit], Field(min_length=1, max_length=64)]

    @model_validator(mode="after")
    def require_unique_text_unit_ids(self) -> "NounPhraseRequest":
        if len({item.id for item in self.text_units}) != len(self.text_units):
            raise ValueError("noun phrase text unit ids must be unique")
        return self


class NounPhraseResult(StrictModel):
    """Stable unique phrases aligned to one request input."""

    id: TextUnitID
    phrases: Annotated[list[Phrase], Field(max_length=100_000)]


class NounPhraseResponse(StrictModel):
    """Complete analysis results in request order."""

    contract_version: Literal[2] = CONTRACT_VERSION
    text_units: list[NounPhraseResult]


def installed_resources() -> tuple[str, ...]:
    """Return the complete locked resource set or no partial capability."""

    resources: list[str] = []
    for name, (_download, relative, expected) in sorted(
        NLTK_RESOURCE_IDENTITIES.items()
    ):
        try:
            actual = directory_digest(NLTK_DATA_ROOT / relative)
        except (OSError, RuntimeError):
            return ()
        if actual != expected:
            return ()
        resources.append(name)
    return tuple(resources)


def installed_dependencies() -> bool:
    """Require exact analysis packages before advertising the operation."""

    try:
        return (
            importlib.metadata.version("nltk") == "3.10.0"
            and importlib.metadata.version("textblob") == "0.20.0"
        )
    except importlib.metadata.PackageNotFoundError:
        return False


def register_routes(
    app: FastAPI,
    authorize: Callable[..., None],
    available: bool,
) -> None:
    """Register noun-phrase analysis without giving it app-shell ownership."""

    @app.post("/v1/nlp/noun-phrases", response_model=NounPhraseResponse)
    async def noun_phrases(
        command: NounPhraseRequest,
        request: Request,
        _authorized: None = Depends(authorize),
    ) -> NounPhraseResponse:
        if not available:
            raise AnalysisError(
                503,
                "resource_unavailable",
                "noun phrase analysis resources are not installed",
                False,
            )
        loop = asyncio.get_running_loop()
        results = await loop.run_in_executor(
            request.app.state.nlp_executor,
            extract_noun_phrases,
            command.text_units,
            command.analyzer,
        )
        return NounPhraseResponse(text_units=results)


def extract_noun_phrases(
    text_units: list[NounPhraseTextUnit],
    analyzer: RegexEnglishAnalyzer,
) -> list[NounPhraseResult]:
    """Apply the configured analyzer while returning deterministic membership."""

    nltk.corpus.brown.ensure_loaded()
    nltk.corpus.treebank.ensure_loaded()
    return [
        NounPhraseResult(
            id=item.id,
            phrases=sorted(_extract_regex_english(item.text, analyzer)),
        )
        for item in text_units
    ]


def _extract_regex_english(text: str, analyzer: RegexEnglishAnalyzer) -> set[str]:
    blob = TextBlob(text)
    proper_nouns = {token.upper() for token, tag in blob.tags if tag == "NNP"}
    excluded = {noun.upper() for noun in analyzer.exclude_nouns}
    phrases: set[str] = set()
    for noun_phrase in blob.noun_phrases:
        tokens = [token for token in re.split(r"[\s]+", noun_phrase) if token]
        cleaned = [token for token in tokens if token.upper() not in excluded]
        has_proper_nouns = any(token.upper() in proper_nouns for token in cleaned)
        has_compound_words = any(
            "-" in token
            and len(token.strip()) > 1
            and len(token.strip().split("-")) > 1
            for token in cleaned
        )
        has_valid_tokens = all(
            re.match(r"^[a-zA-Z0-9\-]+\n?$", token) for token in cleaned
        ) and all(len(token) <= analyzer.max_word_length for token in cleaned)
        if (
            has_proper_nouns or len(cleaned) > 1 or has_compound_words
        ) and has_valid_tokens:
            phrases.add(
                analyzer.word_delimiter.join(cleaned).replace("\n", "").upper()
            )
    return phrases
