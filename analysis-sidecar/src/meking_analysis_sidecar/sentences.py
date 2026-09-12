"""Sentence analysis contract, resources, route, and worker implementation."""

from __future__ import annotations

import asyncio
import sys
from collections.abc import Callable
from pathlib import Path
from typing import Annotated, Literal

import nltk
from fastapi import Depends, FastAPI, Request
from pydantic import Field, model_validator

from meking_analysis_sidecar.contract import CONTRACT_VERSION, StrictModel
from meking_analysis_sidecar.errors import AnalysisError
from meking_analysis_sidecar.resources import PUNKT_TAB_SHA256, directory_digest

DocumentID = Annotated[str, Field(min_length=1, max_length=128)]
Language = Annotated[str, Field(pattern=r"^[a-z][a-z_]{1,31}$")]
DocumentText = Annotated[str, Field(max_length=1_000_000)]

NLTK_DATA_ROOT = Path(sys.prefix) / "nltk_data"
nltk.data.path.insert(0, str(NLTK_DATA_ROOT))


class SentenceDocument(StrictModel):
    """One caller-owned document sent as text rather than a readable path."""

    id: DocumentID
    text: DocumentText
    language: Language


class SentenceRequest(StrictModel):
    """Versioned bounded batch accepted by the sentence operation."""

    contract_version: Literal[2]
    documents: Annotated[list[SentenceDocument], Field(min_length=1, max_length=64)]

    @model_validator(mode="after")
    def require_unique_document_ids(self) -> "SentenceRequest":
        if len({document.id for document in self.documents}) != len(self.documents):
            raise ValueError("sentence document ids must be unique")
        return self


class SentenceSpan(StrictModel):
    """Inclusive Python code-point offsets for one sentence."""

    index: Annotated[int, Field(ge=0)]
    start_char: Annotated[int, Field(ge=0)]
    end_char: Annotated[int, Field(ge=0)]


class SentenceDocumentResult(StrictModel):
    """Ordered spans aligned to one request document identity."""

    id: DocumentID
    spans: list[SentenceSpan]


class SentenceResponse(StrictModel):
    """Complete sentence results in the same order as the request batch."""

    contract_version: Literal[2] = CONTRACT_VERSION
    documents: list[SentenceDocumentResult]


def installed_resources() -> tuple[str, ...]:
    """Return only Sentence resources whose content matches the lock."""

    root = NLTK_DATA_ROOT / "tokenizers" / "punkt_tab"
    try:
        digest = directory_digest(root)
    except (OSError, RuntimeError):
        return ()
    if digest != PUNKT_TAB_SHA256:
        return ()
    return ("nltk:punkt_tab",)


def register_routes(
    app: FastAPI,
    authorize: Callable[..., None],
    available: bool,
) -> None:
    """Register the Sentence feature without coupling the app shell to NLP."""

    @app.post("/v1/nlp/sentences", response_model=SentenceResponse)
    async def sentences(
        command: SentenceRequest,
        request: Request,
        _authorized: None = Depends(authorize),
    ) -> SentenceResponse:
        if not available:
            raise AnalysisError(
                503,
                "resource_unavailable",
                "sentence analysis resources are not installed",
                False,
            )
        loop = asyncio.get_running_loop()
        documents = await loop.run_in_executor(
            request.app.state.nlp_executor,
            split_sentence_documents,
            command.documents,
        )
        return SentenceResponse(documents=documents)


def split_sentence_documents(
    documents: list[SentenceDocument],
) -> list[SentenceDocumentResult]:
    """Reproduce the baseline NLTK sentence chunker in a worker process."""

    results: list[SentenceDocumentResult] = []
    for document in documents:
        sentences = nltk.sent_tokenize(document.text.strip(), language=document.language)
        spans: list[SentenceSpan] = []
        start_char = 0
        for index, sentence in enumerate(sentences):
            end_char = start_char + len(sentence) - 1
            spans.append(
                SentenceSpan(index=index, start_char=start_char, end_char=end_char)
            )
            start_char = end_char + 1
        for index, span in enumerate(spans):
            sentence = sentences[index]
            actual_start = document.text.find(sentence, span.start_char)
            delta = actual_start - span.start_char
            if delta > 0:
                span.start_char += delta
                span.end_char += delta
                if index < len(spans) - 1:
                    spans[index + 1].start_char += delta
                    spans[index + 1].end_char += delta
        results.append(SentenceDocumentResult(id=document.id, spans=spans))
    return results
