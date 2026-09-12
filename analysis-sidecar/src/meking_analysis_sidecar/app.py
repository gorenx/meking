"""Authenticated loopback HTTP application for optional analysis operations."""

from __future__ import annotations

import hmac
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from concurrent.futures import ProcessPoolExecutor
from typing import Any

from fastapi import Depends, FastAPI, Header, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from meking_analysis_sidecar.contract import (
    CONTRACT_VERSION,
    ErrorDetail,
    ErrorResponse,
    HealthResponse,
)
from meking_analysis_sidecar.errors import AnalysisError
from meking_analysis_sidecar.documents import (
    installed_dependencies,
    register_routes as register_document_routes,
)
from meking_analysis_sidecar.noun_phrases import (
    installed_dependencies as noun_phrase_dependencies_installed,
    installed_resources as installed_noun_phrase_resources,
    register_routes as register_noun_phrase_routes,
)
from meking_analysis_sidecar.sentences import (
    installed_resources as installed_sentence_resources,
    register_routes as register_sentence_routes,
)

MAX_REQUEST_BYTES = 1 << 20
MAX_DOCUMENT_REQUEST_BYTES = (64 << 20) + (1 << 20)
MAX_NLP_WORKERS = max(1, min(4, os.cpu_count() or 1))
MAX_DOCUMENT_WORKERS = max(1, min(2, os.cpu_count() or 1))
SENTENCE_RESOURCES = installed_sentence_resources()
NOUN_PHRASE_RESOURCES = installed_noun_phrase_resources()
RESOURCES = tuple(sorted(set(SENTENCE_RESOURCES + NOUN_PHRASE_RESOURCES)))
CAPABILITIES: tuple[str, ...] = tuple(
    sorted(
        capability
        for capability, available in (
            ("sentences", bool(SENTENCE_RESOURCES)),
            (
                "noun_phrases",
                bool(NOUN_PHRASE_RESOURCES) and noun_phrase_dependencies_installed(),
            ),
            ("markitdown", installed_dependencies()),
        )
        if available
    )
)


def create_app(bearer_token: str) -> FastAPI:
    """Create one run-scoped app whose token is never stored in response state."""

    if not bearer_token.strip():
        raise ValueError("analysis bearer token is required")

    @asynccontextmanager
    async def lifespan(application: FastAPI) -> AsyncIterator[None]:
        nlp_executor = ProcessPoolExecutor(max_workers=MAX_NLP_WORKERS)
        document_executor = ProcessPoolExecutor(max_workers=MAX_DOCUMENT_WORKERS)
        application.state.nlp_executor = nlp_executor
        application.state.document_executor = document_executor
        try:
            yield
        finally:
            document_executor.shutdown(wait=True, cancel_futures=True)
            nlp_executor.shutdown(wait=True, cancel_futures=True)

    app = FastAPI(
        title="Meking Analysis Sidecar",
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        lifespan=lifespan,
    )

    def authorize(authorization: str | None = Header(default=None)) -> None:
        expected = "Bearer " + bearer_token
        if authorization is None or not hmac.compare_digest(authorization, expected):
            raise AnalysisError(401, "unauthorized", "authentication is required", False)

    @app.exception_handler(AnalysisError)
    async def analysis_error_handler(_request: Request, error: AnalysisError) -> JSONResponse:
        payload = ErrorResponse(
            error=ErrorDetail(
                code=error.code,
                message=error.message,
                retryable=error.retryable,
            )
        )
        return JSONResponse(status_code=error.status, content=payload.model_dump())

    @app.exception_handler(RequestValidationError)
    async def validation_error_handler(
        _request: Request, _error: RequestValidationError
    ) -> JSONResponse:
        payload = ErrorResponse(
            error=ErrorDetail(
                code="invalid_request",
                message="the request does not match contract version 2",
                retryable=False,
            )
        )
        return JSONResponse(status_code=422, content=payload.model_dump())

    @app.exception_handler(Exception)
    async def internal_error_handler(_request: Request, _error: Exception) -> JSONResponse:
        payload = ErrorResponse(
            error=ErrorDetail(
                code="analysis_failed",
                message="the analysis operation failed",
                retryable=False,
            )
        )
        return JSONResponse(status_code=500, content=payload.model_dump())

    @app.middleware("http")
    async def reject_oversized_request(request: Request, call_next: Any) -> JSONResponse:
        limit = (
            MAX_DOCUMENT_REQUEST_BYTES
            if request.url.path == "/v1/documents/convert"
            else MAX_REQUEST_BYTES
        )
        content_length = request.headers.get("content-length")
        if content_length is not None:
            try:
                oversized = int(content_length) > limit
            except ValueError:
                oversized = True
            if oversized:
                payload = ErrorResponse(
                    error=ErrorDetail(
                        code="request_too_large",
                        message="the analysis request is too large",
                        retryable=False,
                    )
                )
                return JSONResponse(status_code=413, content=payload.model_dump())
        return await call_next(request)

    @app.get("/v1/health", response_model=HealthResponse)
    async def health(_authorized: None = Depends(authorize)) -> HealthResponse:
        return HealthResponse(
            capabilities=list(CAPABILITIES),
            resources=sorted(RESOURCES),
        )

    register_sentence_routes(app, authorize, "sentences" in CAPABILITIES)
    register_noun_phrase_routes(app, authorize, "noun_phrases" in CAPABILITIES)
    register_document_routes(app, authorize, "markitdown" in CAPABILITIES)

    return app
