"""Bounded rich-document conversion contract and worker implementation."""

from __future__ import annotations

import asyncio
import hashlib
import importlib.metadata
import importlib.util
import re
import warnings
from collections.abc import Callable
from concurrent.futures.process import BrokenProcessPool
from typing import Annotated, Literal, TypedDict

from fastapi import Depends, FastAPI, File, Form, Request, UploadFile
from pydantic import Field

from meking_analysis_sidecar.contract import CONTRACT_VERSION, StrictModel
from meking_analysis_sidecar.errors import AnalysisError

MAX_DOCUMENT_CONTENT_BYTES = 64 << 20
MAX_DOCUMENT_MARKDOWN_CHARS = 8_000_000
MAX_DOCUMENT_WARNINGS = 64
MAX_DOCUMENT_WARNING_CHARS = 1024
MARKITDOWN_VERSION = "0.1.6"

SUPPORTED_MEDIA_TYPES = {
    ".pdf": "application/pdf",
    ".docx": (
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
    ),
    ".pptx": (
        "application/vnd.openxmlformats-officedocument.presentationml.presentation"
    ),
    ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ".html": "text/html",
    ".htm": "text/html",
}

class DocumentConversionResponse(StrictModel):
    """Complete conversion result with no source path or original bytes."""

    contract_version: Literal[2] = CONTRACT_VERSION
    markdown: Annotated[str, Field(max_length=MAX_DOCUMENT_MARKDOWN_CHARS)]
    title: str | None
    warnings: Annotated[list[str], Field(max_length=MAX_DOCUMENT_WARNINGS)]


class ConversionWorkerResult(TypedDict):
    """Pickle-safe result returned from the isolated conversion worker."""

    ok: bool
    markdown: str
    title: str | None
    warnings: list[str]


def installed_dependencies() -> bool:
    """Advertise conversion only when the locked package and all extras exist."""

    try:
        if importlib.metadata.version("markitdown") != MARKITDOWN_VERSION:
            return False
        return all(
            importlib.util.find_spec(module) is not None
            for module in (
                "lxml",
                "magika",
                "mammoth",
                "onnxruntime",
                "openpyxl",
                "pandas",
                "pdfminer",
                "pdfplumber",
                "pptx",
                "python_multipart",
            )
        )
    except (ImportError, importlib.metadata.PackageNotFoundError, ValueError):
        return False


def register_routes(
    app: FastAPI,
    authorize: Callable[..., None],
    available: bool,
) -> None:
    """Register conversion without coupling the HTTP shell to MarkItDown."""

    @app.post(
        "/v1/documents/convert",
        response_model=DocumentConversionResponse,
    )
    async def convert_document(
        request: Request,
        contract_version: Annotated[int, Form()],
        extension: Annotated[str, Form(min_length=4, max_length=5)],
        media_type: Annotated[str, Form(min_length=1, max_length=128)],
        content_sha256: Annotated[str, Form(pattern=r"^[0-9a-f]{64}$")],
        file: Annotated[UploadFile, File()],
        _authorized: None = Depends(authorize),
    ) -> DocumentConversionResponse:
        if contract_version != CONTRACT_VERSION:
            raise AnalysisError(
                422,
                "invalid_request",
                "the request does not match contract version 2",
                False,
            )
        if not available:
            raise AnalysisError(
                503,
                "resource_unavailable",
                "document conversion dependencies are not installed",
                False,
            )
        validate_document_identity(extension, media_type, file)
        content = await file.read(MAX_DOCUMENT_CONTENT_BYTES + 1)
        await file.close()
        if not content or len(content) > MAX_DOCUMENT_CONTENT_BYTES:
            raise AnalysisError(
                413,
                "request_too_large",
                "the document content exceeds the configured limit",
                False,
            )
        if not hashlib.sha256(content).hexdigest() == content_sha256:
            raise AnalysisError(
                422,
                "invalid_document",
                "the document content digest does not match",
                False,
            )

        loop = asyncio.get_running_loop()
        try:
            result = await loop.run_in_executor(
                request.app.state.document_executor,
                convert_document_bytes,
                content,
                extension,
                media_type,
                file.filename,
            )
        except BrokenProcessPool as error:
            raise AnalysisError(
                503,
                "worker_unavailable",
                "the document conversion worker is unavailable",
                True,
            ) from error
        if not result["ok"]:
            raise AnalysisError(
                422,
                "conversion_failed",
                "the document could not be converted",
                False,
            )
        return DocumentConversionResponse(
            markdown=result["markdown"],
            title=result["title"],
            warnings=result["warnings"],
        )


def validate_document_identity(
    extension: str,
    media_type: str,
    file: UploadFile,
) -> None:
    """Require the Go-validated identity to remain aligned at the boundary."""

    expected = SUPPORTED_MEDIA_TYPES.get(extension)
    filename = file.filename or ""
    if (
        expected is None
        or media_type != expected
        or file.content_type != expected
        or not filename
        or filename.strip() != filename
        or "/" in filename
        or "\\" in filename
        or not filename.lower().endswith(extension)
    ):
        raise AnalysisError(
            422,
            "invalid_document",
            "the document type metadata is invalid",
            False,
        )


def convert_document_bytes(
    content: bytes,
    extension: str,
    media_type: str,
    filename: str | None,
) -> ConversionWorkerResult:
    """Run exactly one permitted built-in converter through the stream API."""

    try:
        from io import BytesIO

        from markitdown import MarkItDown, StreamInfo
        from markitdown.converters import (
            DocxConverter,
            HtmlConverter,
            PdfConverter,
            PptxConverter,
            XlsxConverter,
        )

        converters = {
            ".pdf": PdfConverter,
            ".docx": DocxConverter,
            ".pptx": PptxConverter,
            ".xlsx": XlsxConverter,
            ".html": HtmlConverter,
            ".htm": HtmlConverter,
        }
        converter_type = converters[extension]
        markitdown = MarkItDown(enable_builtins=False, enable_plugins=False)
        markitdown.register_converter(converter_type())
        with warnings.catch_warnings(record=True) as captured:
            warnings.simplefilter("always")
            converted = markitdown.convert_stream(
                BytesIO(content),
                stream_info=StreamInfo(
                    extension=extension,
                    mimetype=media_type,
                    filename=filename,
                ),
            )
        markdown = converted.markdown
        if not isinstance(markdown, str) or len(markdown) > MAX_DOCUMENT_MARKDOWN_CHARS:
            return failed_worker_result()
        title = normalize_title(converted.title)
        converted_warnings = normalize_warnings(captured)
        return {
            "ok": True,
            "markdown": markdown,
            "title": title,
            "warnings": converted_warnings,
        }
    except Exception:  # noqa: BLE001 - never transfer dependency exceptions or tracebacks
        return failed_worker_result()


def normalize_title(title: object) -> str | None:
    """Map optional converter metadata to one safe single-line title."""

    if not isinstance(title, str):
        return None
    normalized = re.sub(r"\s+", " ", title).strip()
    return normalized or None


def normalize_warnings(captured: list[warnings.WarningMessage]) -> list[str]:
    """Return bounded messages without Python filenames or stack locations."""

    result: list[str] = []
    for warning in captured[:MAX_DOCUMENT_WARNINGS]:
        message = re.sub(r"\s+", " ", str(warning.message)).strip()
        if message:
            result.append(message[:MAX_DOCUMENT_WARNING_CHARS])
    return result


def failed_worker_result() -> ConversionWorkerResult:
    """Keep failures pickle-safe and free of dependency error details."""

    return {"ok": False, "markdown": "", "title": None, "warnings": []}
