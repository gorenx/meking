"""Pinned analysis resource identities shared by setup and runtime checks."""

from __future__ import annotations

import hashlib
from pathlib import Path

PUNKT_TAB_SHA256 = "51a7e26f8409ec375b64202d92aca8b2a600588daffbe219b9be7ec6a4ae50b2"

NLTK_RESOURCE_IDENTITIES = {
    "nltk:averaged_perceptron_tagger_eng": (
        "averaged_perceptron_tagger_eng",
        "taggers/averaged_perceptron_tagger_eng",
        "7e5d9b33a66c85c56c5f00e28dcd1f9bd1478231ff58638f6c501e2f7917fcce",
    ),
    "nltk:brown": (
        "brown",
        "corpora/brown",
        "b1e8f1d1cc2acffea1d815699fa1768dedaef67c0adc0c47fe42437e816281ef",
    ),
    "nltk:punkt": (
        "punkt",
        "tokenizers/punkt",
        "f1165b325f548f7bf6b93e425c0c0b2fc047b0533497a7d97f1d028a704b50d8",
    ),
    "nltk:punkt_tab": (
        "punkt_tab",
        "tokenizers/punkt_tab",
        PUNKT_TAB_SHA256,
    ),
    "nltk:treebank": (
        "treebank",
        "corpora/treebank",
        "efefad2a7f7cc05b5678af6575a5168f30f3b65f48781b4cd16ca2e05998e9ac",
    ),
}


def directory_digest(root: Path) -> str:
    """Hash file names, lengths, and contents without using local paths."""

    digest = hashlib.sha256()
    files = sorted(path for path in root.rglob("*") if path.is_file())
    if not files:
        raise RuntimeError("analysis resource directory is empty")
    for path in files:
        relative = path.relative_to(root).as_posix().encode("utf-8")
        content = path.read_bytes()
        digest.update(len(relative).to_bytes(8, "big"))
        digest.update(relative)
        digest.update(len(content).to_bytes(8, "big"))
        digest.update(content)
    return digest.hexdigest()
