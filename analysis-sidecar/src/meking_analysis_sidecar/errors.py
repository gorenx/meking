"""Controlled failures shared by optional analysis feature routes."""


class AnalysisError(Exception):
    """Carry a stable protocol failure without exposing its root cause."""

    def __init__(self, status: int, code: str, message: str, retryable: bool) -> None:
        super().__init__(code)
        self.status = status
        self.code = code
        self.message = message
        self.retryable = retryable
