"""Domain errors. Each carries the HTTP status, error code and message the API
maps it to (the Go server's ``errorMappings`` table)."""

from __future__ import annotations


class OpenFormsError(Exception):
    status: int = 500
    code: str = "internal"
    message: str = "internal server error"

    def __init__(self, message: str | None = None) -> None:
        if message is not None:
            self.message = message
        super().__init__(self.message)


class NotFound(OpenFormsError):
    status, code, message = 404, "not_found", "not found"


class Unauthenticated(OpenFormsError):
    status, code, message = 401, "unauthenticated", "authentication required"


class InvalidCredentials(OpenFormsError):
    status, code, message = 401, "invalid_credentials", "invalid email or password"


class EmailTaken(OpenFormsError):
    status, code, message = 409, "email_taken", "a user with this email already exists"


class FormNotPublic(NotFound):
    """A private form is indistinguishable from a missing one on public routes."""


class InvalidCursor(OpenFormsError):
    status, code, message = 400, "bad_request", "invalid cursor"


class Forbidden(OpenFormsError):
    status, code, message = 403, "forbidden", "you are not allowed to perform this transition"


class UnknownTransition(OpenFormsError):
    status, code, message = 422, "unknown_transition", "unknown transition"


class InvalidState(OpenFormsError):
    status, code, message = 409, "invalid_state", "transition not allowed from the current state"


class StateConflict(OpenFormsError):
    status, code, message = 409, "state_conflict", "submission state changed; reload and retry"


class NoWorkflow(OpenFormsError):
    status, code, message = 409, "no_workflow", "this submission has no workflow"


class JobNotFound(NotFound):
    message = "job not found or not failed"


class InvalidResetToken(OpenFormsError):
    status, code, message = 400, "invalid_token", "this password reset link is invalid or has expired"
