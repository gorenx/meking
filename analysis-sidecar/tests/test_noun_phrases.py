"""HTTP contract tests for deterministic noun-phrase analysis."""

import asyncio

import httpx

from meking_analysis_sidecar.app import CAPABILITIES, create_app


def regex_payload() -> dict[str, object]:
    return {
        "contract_version": 2,
        "analyzer": {
            "extractor_type": "regex_english",
            "exclude_nouns": [
                "stuff",
                "thing",
                "things",
                "bunch",
                "bit",
                "bits",
                "people",
                "person",
                "okay",
                "hey",
                "hi",
                "hello",
                "laughter",
                "oh",
            ],
            "max_word_length": 15,
            "word_delimiter": " ",
        },
        "text_units": [
            {
                "id": "proper",
                "text": "Microsoft launched Azure Knowledge in Seattle. Alice Smith joined Contoso Labs.",
            },
            {
                "id": "stop-words",
                "text": "People discussed the thing and said hello to a person.",
            },
        ],
    }


def test_noun_phrase_endpoint_returns_stable_baseline_membership() -> None:
    async def scenario() -> None:
        assert "noun_phrases" in CAPABILITIES
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                response = await client.post(
                    "/v1/nlp/noun-phrases",
                    headers={"Authorization": "Bearer test-token"},
                    json=regex_payload(),
                )

                assert response.status_code == 200
                assert response.json() == {
                    "contract_version": 2,
                    "text_units": [
                        {
                            "id": "proper",
                            "phrases": [
                                "ALICE SMITH",
                                "AZURE KNOWLEDGE",
                                "CONTOSO LABS",
                                "MICROSOFT",
                                "SEATTLE",
                            ],
                        },
                        {"id": "stop-words", "phrases": []},
                    ],
                }

    asyncio.run(scenario())


def test_noun_phrase_endpoint_rejects_invalid_contract_without_leaking_text() -> None:
    async def scenario() -> None:
        app = create_app("test-token")
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(app=app)
            async with httpx.AsyncClient(
                transport=transport, base_url="http://sidecar.test"
            ) as client:
                payload = regex_payload()
                payload["text_units"] = [
                    {"id": "same", "text": "private first"},
                    {"id": "same", "text": "private second"},
                ]
                duplicate = await client.post(
                    "/v1/nlp/noun-phrases",
                    headers={"Authorization": "Bearer test-token"},
                    json=payload,
                )
                denied = await client.post(
                    "/v1/nlp/noun-phrases",
                    json=regex_payload(),
                )
                invalid = regex_payload()
                invalid["analyzer"] = {
                    **invalid["analyzer"],  # type: ignore[arg-type]
                    "max_word_length": 0,
                    "private": "value",
                }
                bad_config = await client.post(
                    "/v1/nlp/noun-phrases",
                    headers={"Authorization": "Bearer test-token"},
                    json=invalid,
                )
                control = regex_payload()
                control["analyzer"] = {
                    **control["analyzer"],  # type: ignore[arg-type]
                    "word_delimiter": "\u007f",
                }
                bad_control = await client.post(
                    "/v1/nlp/noun-phrases",
                    headers={"Authorization": "Bearer test-token"},
                    json=control,
                )

                assert duplicate.status_code == 422
                assert duplicate.json()["error"]["code"] == "invalid_request"
                assert "private first" not in duplicate.text
                assert denied.status_code == 401
                assert bad_config.status_code == 422
                assert bad_config.json()["error"]["code"] == "invalid_request"
                assert "value" not in bad_config.text
                assert bad_control.status_code == 422
                assert bad_control.json()["error"]["code"] == "invalid_request"

    asyncio.run(scenario())
