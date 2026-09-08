"""Serve the chat UI and record each conversation turn in Assay."""

import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Annotated, Literal, cast

import assay
from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from openai import APIError, OpenAI
from opentelemetry.trace import get_current_span
from pydantic import BaseModel, Field, StringConstraints, model_validator

from app import Settings, TraceSession, answer_question, traced_session


class Message(BaseModel):
    role: Literal["user", "assistant"]
    content: Annotated[str, StringConstraints(strip_whitespace=True, min_length=1, max_length=4000)]


class ChatRequest(BaseModel):
    messages: list[Message] = Field(min_length=1, max_length=20)

    @model_validator(mode="after")
    def require_user_turn(self) -> "ChatRequest":
        if self.messages[-1].role != "user":
            raise ValueError("The last message must be from the user.")
        return self


class ChatReply(BaseModel):
    answer: str
    trace_url: str
    trace_exported: bool


class ChatService:
    """Generate chat replies with a shared model client and tracing session."""

    def __init__(self, session: TraceSession, client: OpenAI, model: str, ui_url: str) -> None:
        self.session = session
        self.client = client
        self.model = model
        self.traces_url = f"{ui_url.rstrip('/')}/apps/{session.application.id}/traces"

    def config(self) -> dict[str, str]:
        return {"model": self.model, "traces_url": self.traces_url}

    def respond(self, request: ChatRequest) -> ChatReply:
        """Generate a traced reply and resolve its link after synchronous export.

        Args:
            request: Bounded conversation history ending with a user message.

        Returns:
            Answer, trace link, and whether export succeeded.
        """
        started = datetime.now(UTC) - timedelta(seconds=1)
        with assay.span("chat-turn"):
            trace_id = f"{get_current_span().get_span_context().trace_id:032x}"
            transcript = "\n\n".join(f"{m.role}: {m.content}" for m in request.messages)
            answer = answer_question(transcript, self.client, self.model)
        exported = assay.flush()
        trace_url = self.traces_url
        if exported:
            traces = self.session.client.traces.list(
                application_id=self.session.application.id,
                start=started,
                limit=200,
            )
            trace = next((item for item in traces.items if item.otel_trace_id == trace_id), None)
            if trace is not None:
                trace_url += f"/{trace.id}"
        return ChatReply(answer=answer, trace_url=trace_url, trace_exported=exported)


def get_chat(request: Request) -> ChatService:
    return cast(ChatService, request.state.chat)


def configure_evaluation(session: TraceSession, target_url: str) -> None:
    """Connect offline generation to the chat API when no target is configured.

    Args:
        session: Application and authenticated management client.
        target_url: Chat API URL reachable by the Assay worker.
    """
    if session.application.target_endpoint is not None:
        return
    session.client.applications.set_endpoint(
        session.application.id,
        assay.TargetEndpoint(
            url=target_url,
            request_template={
                "messages": [{"role": "user", "content": "{{ .item.input.question }}"}]
            },
            response_mapping=assay.ResponseMapping(output="$.answer"),
            timeout_ms=150_000,
        ),
    )


ChatDependency = Annotated[ChatService, Depends(get_chat)]


def respond(request: ChatRequest, chat: ChatDependency) -> ChatReply:
    try:
        return chat.respond(request)
    except APIError as error:
        raise HTTPException(
            502, "Model request failed. Check server credentials and model."
        ) from error
    except (ValueError, assay.AssayError) as error:
        raise HTTPException(502, str(error)) from error


def create_app(service: ChatService | None = None) -> FastAPI:
    """Create the web app with either injected clients or environment-backed resources."""

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[dict[str, ChatService]]:
        if service is not None:
            yield {"chat": service}
            return
        settings = Settings.from_env(os.environ)
        with (
            traced_session(settings) as session,
            OpenAI(
                api_key=settings.openai_key,
                timeout=60,
                max_retries=1,
            ) as client,
        ):
            target_url = os.getenv("ASSAY_TARGET_URL")
            if target_url:
                configure_evaluation(session, target_url)
            yield {
                "chat": ChatService(
                    session,
                    client,
                    settings.model,
                    os.getenv("ASSAY_UI_URL", settings.endpoint),
                )
            }

    application = FastAPI(title="Assay chat example", lifespan=lifespan)
    static = Path(__file__).parent / "static"

    @application.get("/", include_in_schema=False)
    def index() -> FileResponse:
        return FileResponse(static / "index.html")

    @application.get("/healthz")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    @application.get("/api/config")
    def config(chat: ChatDependency) -> dict[str, str]:
        return chat.config()

    application.post("/api/chat")(respond)
    application.mount("/static", StaticFiles(directory=static), name="static")
    return application


app = create_app()
