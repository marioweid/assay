"""Ask a question, capture its context and answer, and inspect it in local Assay."""

import argparse
import os
import sys
from collections.abc import Iterator, Mapping
from contextlib import contextmanager
from dataclasses import dataclass, field

import assay
from openai import APIError, OpenAI

KNOWLEDGE = (
    assay.Chunk(id="storage", text="Assay stores traces and evaluation scores in Postgres."),
    assay.Chunk(
        id="deploy", text="Assay runs as one Go binary with an embedded web UI and Postgres."
    ),
    assay.Chunk(
        id="scoring",
        text="Groundedness checks supplied context. Correctness checks a reference answer.",
    ),
)


@dataclass(frozen=True)
class Settings:
    endpoint: str
    model: str
    admin_token: str = field(repr=False)
    openai_key: str = field(repr=False)

    @classmethod
    def from_env(cls, env: Mapping[str, str]) -> "Settings":
        """Read local endpoint and credentials without copying secrets to disk.

        Args:
            env: Process environment, optionally loaded by uv --env-file.

        Returns:
            Validated settings for this example.

        Raises:
            ValueError: A required credential is absent.
        """
        token = env.get("ASSAY_ADMIN_TOKEN", "").strip()
        key = (env.get("OPENAI_API_KEY") or env.get("ASSAY_JUDGE_API_KEY", "")).strip()
        if not token:
            raise ValueError("Set ASSAY_ADMIN_TOKEN to the token used by your local Assay server.")
        if not key:
            raise ValueError("Set OPENAI_API_KEY or ASSAY_JUDGE_API_KEY in your .env file.")
        return cls(
            endpoint=env.get("ASSAY_ENDPOINT", "http://localhost:8080").rstrip("/"),
            model=env.get("OPENAI_MODEL") or env.get("ASSAY_JUDGE_MODEL") or "gpt-5.6-luna",
            admin_token=token,
            openai_key=key,
        )


def prepare_application(client: assay.Client) -> assay.Application:
    """Reuse the example project/application, creating them on the first run.

    Args:
        client: Admin-authenticated Assay client.

    Returns:
        Application configured to score new traces for groundedness.
    """
    project = next((p for p in client.projects.list() if p.name == "python-qa-example"), None)
    if project is None:
        project = client.projects.create("python-qa-example")
    application = next(
        (a for a in client.applications.list(project.id) if a.slug == "python-qa-example"), None
    )
    if application is None:
        application = client.applications.create(
            project.id,
            "Python Q&A example",
            "python-qa-example",
            auto_score_scorers=("groundedness",),
        )
    return application


@contextmanager
def traced_session(settings: Settings) -> Iterator["TraceSession"]:
    """Initialize tracing and revoke the temporary ingestion key after flushing.

    Args:
        settings: Server connection settings and credentials.

    Yields:
        The persistent demo application whose traces can be inspected in the UI.
    """
    with assay.Client(settings.endpoint, admin_token=settings.admin_token) as client:
        application = prepare_application(client)
        key = client.keys.create(application.project_id, "temporary-example-run")
        try:
            assay.init(
                endpoint=settings.endpoint,
                api_key=key.key,
                application=application.slug,
                capture=True,
            )
            with assay.Client(
                settings.endpoint, api_key=key.key, admin_token=settings.admin_token
            ) as trace_client:
                yield TraceSession(application=application, client=trace_client)
            if not assay.flush():
                raise RuntimeError(
                    "Trace export failed. Check that the local Assay server is ready."
                )
        finally:
            assay.shutdown()
            client.keys.revoke(application.project_id, key.id)


@dataclass(frozen=True)
class TraceSession:
    application: assay.Application
    client: assay.Client


@assay.trace(name="answer-question", capture=False)
def answer_question(question: str, client: OpenAI, model: str) -> str:
    """Answer from built-in context and capture a scorable generation span.

    Args:
        question: User's question about Assay.
        client: OpenAI client used for generation.
        model: OpenAI model name.

    Returns:
        The generated answer.

    Raises:
        ValueError: The model returns an empty answer.
    """
    with assay.span("load-knowledge") as context_span:
        context_span.set_attribute("example.context_count", len(KNOWLEDGE))
        context = "\n".join(chunk.text for chunk in KNOWLEDGE)
    with assay.span("generate-answer", scorable=True) as span:
        span.set_input(question)
        span.set_context(KNOWLEDGE)
        span.set_attribute("gen_ai.provider.name", "openai")
        span.set_attribute("gen_ai.request.model", model)
        response = client.responses.create(
            model=model,
            instructions="Answer briefly using only this context. Say if it cannot answer.\n"
            + context,
            input=question,
        )
        answer = response.output_text.strip()
        if not answer:
            raise ValueError("OpenAI returned an empty answer; try another question or model.")
        span.set_output(answer)
        if response.usage is not None:
            span.set_attribute("gen_ai.usage.input_tokens", response.usage.input_tokens)
            span.set_attribute("gen_ai.usage.output_tokens", response.usage.output_tokens)
        return answer


def main() -> int:
    """Run the Q&A example and print the answer and Assay UI link."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("question", nargs="?", default="How does Assay store and evaluate traces?")
    args = parser.parse_args()
    try:
        if not args.question.strip():
            raise ValueError("Provide a non-empty question.")
        settings = Settings.from_env(os.environ)
        with (
            traced_session(settings) as session,
            OpenAI(
                api_key=settings.openai_key,
                timeout=60,
                max_retries=1,
            ) as client,
        ):
            print(answer_question(args.question, client, settings.model))
        ui_url = os.getenv("ASSAY_UI_URL", settings.endpoint).rstrip("/")
        print(f"\nTraces: {ui_url}/apps/{session.application.id}/traces")
        print("Connect with your ASSAY_ADMIN_TOKEN. Groundedness scores appear asynchronously.")
        return 0
    except APIError as error:
        print(
            f"OpenAI request failed ({type(error).__name__}). Check key, model, and network.",
            file=sys.stderr,
        )
        return 1
    except (assay.AssayError, ValueError, RuntimeError) as error:
        print(f"Example failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
