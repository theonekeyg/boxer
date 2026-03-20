from __future__ import annotations


class BoxerError(Exception):
    """Base exception for all Boxer SDK errors."""


class BoxerAPIError(BoxerError):
    """Raised when the Boxer API returns a non-2xx response."""

    def __init__(self, message: str, status_code: int) -> None:
        super().__init__(message)
        self.status_code = status_code


class BoxerTimeoutError(BoxerAPIError):
    """Raised when the execution exceeds its wall-clock time limit (HTTP 408)."""


class BoxerOutputLimitError(BoxerAPIError):
    """Raised when the execution exceeds the output size limit (HTTP 507)."""
