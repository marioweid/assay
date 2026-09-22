"""Emit one captured trace using a workspace created by bootstrap.py."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import assay


ANSWER = "Assay stores traces and evaluation evidence in Postgres."


def main() -> None:
    """Send a trace, flush it, and print its OpenTelemetry trace ID."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--workspace", type=Path, required=True)
    args = parser.parse_args()
    workspace = json.loads(args.workspace.read_text(encoding="utf-8"))
    assay.init(
        endpoint=workspace["endpoint"],
        api_key=workspace["api_key"],
        application=workspace["application"],
        capture=True,
    )
    try:
        with assay.span("quickstart-request"):
            with assay.span("quickstart-answer", scorable=True) as answer:
                answer.set_messages(
                    input=[
                        {
                            "role": "user",
                            "parts": [{"type": "text", "content": "What is Assay?"}],
                        }
                    ],
                    output=[
                        {
                            "role": "assistant",
                            "parts": [{"type": "text", "content": ANSWER}],
                        }
                    ],
                )
                answer.set_context((assay.Chunk(id="docs", text=ANSWER),))
                answer.set_reference(ANSWER)
                trace_id = answer.trace_id
        if not assay.flush():
            raise RuntimeError("Assay did not flush the quickstart trace")
    finally:
        assay.shutdown()
    print(trace_id)


if __name__ == "__main__":
    main()
