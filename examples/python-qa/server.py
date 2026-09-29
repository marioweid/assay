"""Serve a capability-bound chat and record each conversation turn in Assay."""

from __future__ import annotations

import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager, nullcontext
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Annotated, cast

import assay
from fastapi import Depends, FastAPI, Header, HTTPException, Query, Request, Response
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from openai import APIError, OpenAI
from pydantic import BaseModel, StringConstraints

from app import Settings, TraceSession, answer_question, traced_session, uses_assay_context
from chat_sessions import COOKIE_LIFETIME, COOKIE_NAME, ChatIdentity, SessionSigner, captured_turn

Question = Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=4000)]


class ChatRequest(BaseModel):
    message: Question


class ChatReply(BaseModel):
    answer: str
    trace_url: str
    trace_exported: bool


class ChatTurn(BaseModel):
    question: str | None
    answer: str | None
    trace_url: str
    status: str


class SessionView(BaseModel):
    session_id: str
    browser_id: str
    turns: list[ChatTurn]
    next_cursor: str | None


class ChatService:
    """Generate bounded replies using server-owned identities and persisted Assay turns."""

    def __init__(
        self,
        session: TraceSession,
        client: OpenAI,
        model: str,
        ui_url: str,
        signer: SessionSigner,
        *,
        secure_cookie: bool = False,
    ) -> None:
        self.session = session
        self.client = client
        self.model = model
        self.signer = signer
        self.secure_cookie = secure_cookie
        self.traces_url = f"{ui_url.rstrip('/')}/apps/{session.application.id}/traces"

    def config(self) -> dict[str, str]:
        return {"model": self.model, "traces_url": self.traces_url}

    def history(self, identity: ChatIdentity, cursor: str | None) -> SessionView:
        """Read a bounded page of root-only turns for the verified cookie's session."""
        try:
            page = self.session.client.sessions.turns(
                self.session.application.id, identity.session_id, limit=50, cursor=cursor
            )
        except assay.AssayAPIError as error:
            if error.status_code != 404 or cursor is not None:
                raise
            return SessionView(
                session_id=identity.session_id,
                browser_id=identity.browser_id,
                turns=[],
                next_cursor=None,
            )
        turns = []
        for turn in page.items:
            question, answer = captured_turn(turn.attributes)
            turns.append(
                ChatTurn(
                    question=question,
                    answer=answer,
                    trace_url=f"{self.traces_url}/{turn.id}",
                    status=turn.status,
                )
            )
        return SessionView(
            session_id=identity.session_id,
            browser_id=identity.browser_id,
            turns=turns,
            next_cursor=page.next_cursor,
        )

    def respond(self, question: str, identity: ChatIdentity | None) -> ChatReply:
        """Trace only the current turn on the root; child model spans may retain history."""
        started = datetime.now(UTC) - timedelta(seconds=1)
        history = self._recent_history(identity) if identity is not None else ""
        prompt = (history + f"\n\nuser: {question}").strip()
        scope = (
            assay.session(
                identity.session_id,
                pseudonymous_user_id=identity.browser_id,
                conversation_id=identity.session_id,
            )
            if identity is not None
            else nullcontext()
        )
        with scope, assay.span("chat-turn") as turn:
            turn.set_input(question)
            trace_id = turn.trace_id
            answer = answer_question(
                prompt, self.client, self.model, scorable=uses_assay_context(question)
            )
            turn.set_output(answer)
        exported = assay.flush()
        trace_url = self.traces_url
        if exported:
            traces = self.session.client.traces.list(
                application_id=self.session.application.id, start=started, limit=200
            )
            trace = next((item for item in traces.items if item.otel_trace_id == trace_id), None)
            if trace is not None:
                trace_url += f"/{trace.id}"
        return ChatReply(answer=answer, trace_url=trace_url, trace_exported=exported)

    def _recent_history(self, identity: ChatIdentity) -> str:
        try:
            turns = self.session.client.sessions.recent(
                self.session.application.id, identity.session_id
            )
        except assay.AssayAPIError as error:
            if error.status_code != 404:
                raise
            return ""
        pairs = []
        for turn in turns:
            question, answer = captured_turn(turn.attributes)
            if question is not None and answer is not None:
                pairs.append(f"user: {question}\nassistant: {answer}")
        return "\n\n".join(pairs)[-20_000:]


def get_chat(request: Request) -> ChatService:
    return cast(ChatService, request.state.chat)


ChatDependency = Annotated[ChatService, Depends(get_chat)]


def _origin(request: Request) -> None:
    configured = os.getenv("ASSAY_CHAT_ORIGIN")
    port = os.getenv("ASSAY_CHAT_PORT", "8090")
    allowed = (
        {configured} if configured else {f"http://localhost:{port}", f"http://127.0.0.1:{port}"}
    )
    if request.headers.get("origin") not in allowed:
        raise HTTPException(403, "A trusted chat origin is required.")


def _identity(request: Request, chat: ChatService) -> ChatIdentity:
    identity = chat.signer.verify(request.cookies.get(COOKIE_NAME))
    if identity is None:
        raise HTTPException(401, "Chat session expired. Reload to start a new chat.")
    return identity


def _cookie(response: Response, chat: ChatService, identity: ChatIdentity) -> None:
    response.set_cookie(
        COOKIE_NAME,
        chat.signer.issue(identity),
        max_age=int(COOKIE_LIFETIME.total_seconds()),
        httponly=True,
        samesite="lax",
        secure=chat.secure_cookie,
        path="/",
    )


def configure_evaluation(session: TraceSession, target_url: str, token: str) -> None:
    """Set a protected evaluation target for the worker's stateless test cases."""
    endpoint = session.application.target_endpoint
    if (
        endpoint is not None
        and endpoint.url == target_url
        and endpoint.headers.get("X-Assay-Example-Key") == token
    ):
        return
    session.client.applications.set_endpoint(
        session.application.id,
        assay.TargetEndpoint(
            url=target_url,
            headers={"X-Assay-Example-Key": token},
            request_template={"message": "{{ .item.input.question }}"},
            response_mapping=assay.ResponseMapping(output="$.answer"),
            timeout_ms=150_000,
        ),
    )


def respond(request: Request, payload: ChatRequest, chat: ChatDependency) -> ChatReply:
    _origin(request)
    identity = _identity(request, chat)
    try:
        return chat.respond(payload.message, identity)
    except APIError as error:
        raise HTTPException(502, "Model provider failed. Check model and connection.") from error
    except (ValueError, assay.AssayError) as error:
        raise HTTPException(502, str(error)) from error


def evaluate(
    payload: ChatRequest,
    chat: ChatDependency,
    key: Annotated[str | None, Header(alias="X-Assay-Example-Key")] = None,
) -> ChatReply:
    if not chat.signer.valid_evaluation_token(key):
        raise HTTPException(401, "Evaluation credential required.")
    try:
        return chat.respond(payload.message, None)
    except (APIError, ValueError, assay.AssayError) as error:
        raise HTTPException(
            502, "Evaluation generation failed. Check the model and Assay."
        ) from error


@asynccontextmanager
async def lifespan(application: FastAPI) -> AsyncIterator[dict[str, ChatService]]:
    service = cast(ChatService | None, application.state.injected_service)
    if service is not None:
        yield {"chat": service}
        return
    settings = Settings.from_env(os.environ)
    secret = os.getenv("ASSAY_CHAT_SIGNING_SECRET", "")
    try:
        signer = SessionSigner(bytes.fromhex(secret))
    except ValueError as error:
        raise ValueError(
            "Set ASSAY_CHAT_SIGNING_SECRET to 64 hex characters (32 random bytes)."
        ) from error
    with (
        traced_session(settings) as session,
        OpenAI(
            api_key=settings.openai_key,
            base_url=settings.base_url,
            timeout=120,
            max_retries=1,
        ) as client,
    ):
        target_url = os.getenv("ASSAY_TARGET_URL")
        if target_url:
            configure_evaluation(session, target_url, signer.evaluation_token())
        yield {
            "chat": ChatService(
                session,
                client,
                settings.model,
                os.getenv("ASSAY_UI_URL", settings.endpoint),
                signer,
                secure_cookie=os.getenv("ASSAY_CHAT_SECURE_COOKIE") == "true",
            )
        }


def index() -> FileResponse:
    return FileResponse(Path(__file__).parent / "static" / "index.html")


def health() -> dict[str, str]:
    return {"status": "ok"}


def config(chat: ChatDependency) -> dict[str, str]:
    return chat.config()


def current_session(
    request: Request,
    response: Response,
    chat: ChatDependency,
    cursor: Annotated[str | None, Query(max_length=4096)] = None,
) -> SessionView:
    identity = chat.signer.verify(request.cookies.get(COOKIE_NAME))
    if identity is None:
        identity = chat.signer.create()
        _cookie(response, chat, identity)
    try:
        return chat.history(identity, cursor)
    except assay.AssayError as error:
        raise HTTPException(502, "Unable to restore chat history from Assay.") from error


def new_session(request: Request, response: Response, chat: ChatDependency) -> SessionView:
    _origin(request)
    identity = chat.signer.rotate(_identity(request, chat))
    _cookie(response, chat, identity)
    return SessionView(
        session_id=identity.session_id,
        browser_id=identity.browser_id,
        turns=[],
        next_cursor=None,
    )


def create_app(service: ChatService | None = None) -> FastAPI:
    """Create the web app with an injected service or persistent env-backed resources."""
    application = FastAPI(title="Assay chat example", lifespan=lifespan)
    application.state.injected_service = service
    application.add_api_route("/", index, methods=["GET"], include_in_schema=False)
    application.add_api_route("/healthz", health, methods=["GET"])
    application.add_api_route("/api/config", config, methods=["GET"])
    application.add_api_route("/api/session", current_session, methods=["GET"])
    application.add_api_route("/api/session", new_session, methods=["POST"])
    application.add_api_route("/api/chat", respond, methods=["POST"])
    application.add_api_route("/api/evaluate", evaluate, methods=["POST"])
    application.mount(
        "/static", StaticFiles(directory=Path(__file__).parent / "static"), name="static"
    )
    return application


app = create_app()
