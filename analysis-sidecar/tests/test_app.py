"""Contract tests for authentication, health identity, and safe failures."""

import asyncio
import hashlib
import warnings

import httpx

from meking_analysis_sidecar.app import (
    CAPABILITIES,
    MAX_DOCUMENT_REQUEST_BYTES,
    RESOURCES,
    create_app,
)
from meking_analysis_sidecar.documents import normalize_title, normalize_warnings


def test_health_requires_bearer_and_returns_reproducibility_identity() -> None:
    async def scenario() -> None:
        transport = httpx.ASGITransport(app=create_app("test-token"))
        async with httpx.AsyncClient(
            transport=transport, base_url="http://sidecar.test"
        ) as client:
            denied = await client.get("/v1/health")
            assert denied.status_code == 401
            assert denied.json() == {
                "error": {
                    "code": "unauthorized",
                    "message": "authentication is required",
                    "retryable": False,
                }
            }

            response = await client.get(
                "/v1/health", headers={"Authorization": "Bearer test-token"}
            )
            assert response.status_code == 200
            assert response.json() == {
                "contract_version": 2,
                "api_version": "v1",
                "capabilities": list(CAPABILITIES),
                "resources": list(RESOURCES),
            }
            assert "test-token" not in response.text

    asyncio.run(scenario())


def test_sentence_endpoint_reproduces_inclusive_code_point_ranges() -> None:
    async def scenario() -> None:
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                response = await client.post(
                    "/v1/nlp/sentences",
                    headers={"Authorization": "Bearer test-token"},
                    json={
                        "contract_version": 2,
                        "documents": [
                            {
                                "id": "mixed-whitespace",
                                "text": "   Sentence with spaces. Another one!   ",
                                "language": "english",
                            },
                            {
                                "id": "unicode",
                                "text": "Café works. 再见！ Final sentence.",
                                "language": "english",
                            },
                        ],
                    },
                )

                assert response.status_code == 200
                assert response.json() == {
                    "contract_version": 2,
                    "documents": [
                        {
                            "id": "mixed-whitespace",
                            "spans": [
                                {"index": 0, "start_char": 3, "end_char": 23},
                                {"index": 1, "start_char": 25, "end_char": 36},
                            ],
                        },
                        {
                            "id": "unicode",
                            "spans": [
                                {"index": 0, "start_char": 0, "end_char": 10},
                                {"index": 1, "start_char": 12, "end_char": 30},
                            ],
                        },
                    ],
                }

    asyncio.run(scenario())


def test_sentence_endpoint_rejects_duplicate_ids_and_missing_auth() -> None:
    payload = {
        "contract_version": 2,
        "documents": [
            {"id": "same", "text": "One.", "language": "english"},
            {"id": "same", "text": "Two.", "language": "english"},
        ],
    }
    async def scenario() -> None:
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                denied = await client.post("/v1/nlp/sentences", json=payload)
                invalid = await client.post(
                    "/v1/nlp/sentences",
                    headers={"Authorization": "Bearer test-token"},
                    json=payload,
                )
                missing_version = dict(payload)
                missing_version.pop("contract_version")
                missing = await client.post(
                    "/v1/nlp/sentences",
                    headers={"Authorization": "Bearer test-token"},
                    json=missing_version,
                )

                assert denied.status_code == 401
                assert invalid.status_code == 422
                assert invalid.json()["error"]["code"] == "invalid_request"
                assert missing.status_code == 422
                assert missing.json()["error"]["code"] == "invalid_request"

    asyncio.run(scenario())


def test_unknown_route_does_not_expose_token() -> None:
    async def scenario() -> None:
        transport = httpx.ASGITransport(app=create_app("private-token"))
        async with httpx.AsyncClient(
            transport=transport, base_url="http://sidecar.test"
        ) as client:
            response = await client.get(
                "/v1/missing",
                headers={"Authorization": "Bearer private-token"},
            )
            assert response.status_code == 404
            assert "private-token" not in response.text

    asyncio.run(scenario())


def test_document_endpoint_converts_authenticated_html_stream() -> None:
    content = (
        b"<html><head><title>Atlas</title></head>"
        b"<body><h1>Beacon</h1><p>Local rich input.</p></body></html>"
    )

    async def scenario() -> None:
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                response = await client.post(
                    "/v1/documents/convert",
                    headers={"Authorization": "Bearer test-token"},
                    data={
                        "contract_version": "2",
                        "extension": ".html",
                        "media_type": "text/html",
                        "content_sha256": hashlib.sha256(content).hexdigest(),
                    },
                    files={"file": ("atlas.html", content, "text/html")},
                )

                assert response.status_code == 200
                assert response.json() == {
                    "contract_version": 2,
                    "markdown": "# Beacon\n\nLocal rich input.",
                    "title": "Atlas",
                    "warnings": [],
                }

    asyncio.run(scenario())


def test_document_endpoint_rejects_identity_digest_and_malformed_content() -> None:
    content = b"not a PDF"

    async def scenario() -> None:
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                base_data = {
                    "contract_version": "2",
                    "extension": ".pdf",
                    "media_type": "application/pdf",
                    "content_sha256": hashlib.sha256(content).hexdigest(),
                }
                mismatched = await client.post(
                    "/v1/documents/convert",
                    headers={"Authorization": "Bearer test-token"},
                    data=base_data,
                    files={"file": ("atlas.pdf", content, "application/zip")},
                )
                bad_digest = await client.post(
                    "/v1/documents/convert",
                    headers={"Authorization": "Bearer test-token"},
                    data={**base_data, "content_sha256": "0" * 64},
                    files={"file": ("atlas.pdf", content, "application/pdf")},
                )
                malformed = await client.post(
                    "/v1/documents/convert",
                    headers={"Authorization": "Bearer test-token"},
                    data=base_data,
                    files={"file": ("atlas.pdf", content, "application/pdf")},
                )

                assert mismatched.status_code == 422
                assert mismatched.json()["error"]["code"] == "invalid_document"
                assert bad_digest.status_code == 422
                assert bad_digest.json()["error"]["code"] == "invalid_document"
                assert malformed.status_code == 422
                assert malformed.json() == {
                    "error": {
                        "code": "conversion_failed",
                        "message": "the document could not be converted",
                        "retryable": False,
                    }
                }
                assert "not a PDF" not in malformed.text

    asyncio.run(scenario())


def test_document_request_size_and_metadata_normalization_are_bounded() -> None:
    async def scenario() -> None:
        transport = httpx.ASGITransport(app=create_app("test-token"))
        async with httpx.AsyncClient(
            transport=transport, base_url="http://sidecar.test"
        ) as client:
            response = await client.post(
                "/v1/documents/convert",
                headers={
                    "Authorization": "Bearer test-token",
                    "Content-Length": str(MAX_DOCUMENT_REQUEST_BYTES + 1),
                },
                content=b"x",
            )
            assert response.status_code == 413
            assert response.json()["error"]["code"] == "request_too_large"

    with warnings.catch_warnings(record=True) as captured:
        warnings.warn("  first\nwarning  ", stacklevel=1)
    assert normalize_title("  Atlas\n Beacon  ") == "Atlas Beacon"
    assert normalize_title(" \n ") is None
    assert normalize_warnings(captured) == ["first warning"]
    asyncio.run(scenario())
