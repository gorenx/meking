"""Stable HTTP and startup contracts shared with the Go parent process."""

from typing import Literal

from pydantic import BaseModel, ConfigDict

CONTRACT_VERSION = 2
API_VERSION = "v1"
TOKEN_ENVIRONMENT_NAME = "MEKING_ANALYSIS_TOKEN"

Capability = Literal["sentences", "noun_phrases", "markitdown"]


class StrictModel(BaseModel):
    """Reject accidental protocol drift at the process boundary."""

    model_config = ConfigDict(extra="forbid")


class HealthResponse(StrictModel):
    """Describe reproducibility and installed capabilities without local paths."""

    contract_version: Literal[2] = CONTRACT_VERSION
    api_version: Literal["v1"] = API_VERSION
    capabilities: list[Capability]
    resources: list[str]


class ErrorDetail(StrictModel):
    """Return a controlled classification instead of a Python traceback."""

    code: str
    message: str
    retryable: bool


class ErrorResponse(StrictModel):
    """Wrap every non-success response in one versioned shape."""

    error: ErrorDetail


class ReadyMessage(StrictModel):
    """Reserve child stdout for one machine-readable startup record."""

    event: Literal["ready"] = "ready"
    contract_version: Literal[2] = CONTRACT_VERSION
    port: int
    pid: int
    capabilities: list[Capability]
