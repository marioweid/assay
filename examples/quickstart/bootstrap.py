"""Create a project, ingest key, and application for the quickstart."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
from uuid import uuid4

import assay


def main() -> None:
    """Create an isolated workspace and write its one-time ingest key privately."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        args.output.touch(mode=0o600, exist_ok=False)
    except FileExistsError as error:
        raise SystemExit(f"workspace already exists: {args.output}") from error
    endpoint = os.environ.get("ASSAY_ENDPOINT", "http://localhost:8080")
    admin_token = os.environ["ASSAY_ADMIN_TOKEN"]
    suffix = uuid4().hex[:12]
    try:
        with assay.Client(endpoint, admin_token=admin_token) as client:
            project_id = _create_workspace(client, suffix, endpoint, args.output)
    except Exception:
        args.output.unlink()
        raise
    print(f"Created workspace for project {project_id}: {args.output}")


def _create_workspace(
    client: assay.Client, suffix: str, endpoint: str, output: Path
) -> str:
    """Create and persist a workspace, removing the project on a setup failure."""
    project_id: str | None = None
    try:
        project = client.projects.create(f"quickstart-{suffix}")
        project_id = project.id
        key = client.keys.create(project.id, "quickstart-ingest")
        application = client.applications.create(
            project.id, "Assay quickstart", f"quickstart-{suffix}"
        )
        workspace = {
            "endpoint": endpoint,
            "project_id": project.id,
            "application_id": application.id,
            "application": application.slug,
            "api_key": key.key,
        }
        output.write_text(json.dumps(workspace, indent=2) + "\n", encoding="utf-8")
        return project.id
    except Exception:
        if project_id is not None:
            try:
                client.projects.delete(project_id)
            except Exception as cleanup_error:
                raise RuntimeError(
                    f"quickstart setup failed; delete project {project_id} in the UI"
                ) from cleanup_error
        raise
    print(f"Created {application.slug}; private workspace: {args.output}")


if __name__ == "__main__":
    main()
