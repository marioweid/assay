"""Safe exception types exposed by the Assay SDK."""


class AssayError(Exception):
    """Base class for safe Assay SDK failures."""


class AssayConfigurationError(AssayError):
    """Raised when local SDK configuration is missing or invalid."""


class AssayTransportError(AssayError):
    """Raised when an Assay HTTP request cannot complete."""


class AssayProtocolError(AssayError):
    """Raised when an Assay response violates the API contract."""


class AssayTimeoutError(AssayError):
    """Raised when a bounded Assay operation exceeds its deadline."""


class AssayAPIError(AssayError):
    """Represent an HTTP API failure without retaining request or response objects."""

    def __init__(
        self,
        *,
        operation: str,
        status_code: int,
        title: str | None = None,
        detail: str | None = None,
    ) -> None:
        self.operation = operation
        self.status_code = status_code
        self.title = title
        self.detail = detail
        message = f"{operation}: HTTP {status_code}"
        if title:
            message += f" {title}"
        if detail:
            message += f": {detail}"
        super().__init__(message)


class AssayImportError(AssayError):
    """Report a dataset import failure and any already committed progress."""

    def __init__(
        self,
        message: str,
        *,
        committed_items: int,
        batch_number: int | None,
    ) -> None:
        self.committed_items = committed_items
        self.batch_number = batch_number
        super().__init__(message)
