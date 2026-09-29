#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/assay-docs-smoke.XXXXXX")
umask 077

cleanup() {
  status=$?
  trap - EXIT INT TERM
  for file in "$TEMP_DIR/.env" "$TEMP_DIR/workspace.json" "$TEMP_DIR/traces.json"; do
    [[ ! -e "$file" ]] || unlink "$file"
  done
  rmdir "$TEMP_DIR" 2>/dev/null || status=1
  exit "$status"
}
trap cleanup EXIT INT TERM

for command in node openssl uv; do
  command -v "$command" >/dev/null || {
    echo "docs smoke: required command not found: $command" >&2
    exit 1
  }
done

if [[ -e "$TEMP_DIR/.env" ]]; then
  echo "docs smoke: temporary .env unexpectedly exists" >&2
  exit 1
fi
cp "$ROOT/.env.example" "$TEMP_DIR/.env"
[[ $(openssl rand -hex 32 | tr -d '\r\n' | wc -c) -eq 64 ]] || {
  echo "docs smoke: hex credential generation failed" >&2
  exit 1
}
[[ $(openssl rand -base64 32 | tr -d '\r\n' | base64 --decode | wc -c) -eq 32 ]] || {
  echo "docs smoke: encryption-key generation failed" >&2
  exit 1
}

node - "$ROOT" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");

const root = process.argv[2];
const readme = fs.readFileSync(path.join(root, "README.md"), "utf8");
const start = "<!-- BEGIN compose.published.yaml -->\n```yaml\n";
const end = "\n```\n<!-- END compose.published.yaml -->";
const embedded = readme.split(start)[1]?.split(end)[0];
if (embedded === undefined) {
  throw new Error("docs smoke: README published Compose markers are missing");
}
const compose = fs.readFileSync(path.join(root, "compose.published.yaml"), "utf8");
if (embedded.trimEnd() !== compose.trimEnd()) {
  throw new Error("docs smoke: README published Compose differs from compose.published.yaml");
}
const files = [
  "README.md",
  "clients/python/assay/README.md",
  "examples/python-qa/README.md",
  "docs/architecture.md",
  "docs/ci-cd.md",
  "docs/quickstart-linux.md",
  "docs/quickstart-powershell.md",
  "docs/deployment.md",
  "docs/index.md",
  "docs/concepts.md",
  "docs/python-sdk.md",
  "docs/evaluations.md",
  "docs/cli.md",
  "docs/configuration.md",
  "docs/troubleshooting.md",
];
const link = /\[[^\]]+\]\(([^)]+)\)/g;
for (const relative of files) {
  const file = path.join(root, relative);
  for (const match of fs.readFileSync(file, "utf8").matchAll(link)) {
    const target = match[1].split("#", 1)[0];
    if (!target || target.includes("://") || target.startsWith("mailto:")) continue;
    if (!fs.existsSync(path.resolve(path.dirname(file), target))) {
      throw new Error(`docs smoke: broken link in ${relative}: ${target}`);
    }
  }
}
NODE

uv run --project "$ROOT/clients/python/assay" python - "$ROOT" <<'PYTHON'
import ast
import json
import re
import shlex
import sys
from pathlib import Path

from assay.cli import _build_parser

root = Path(sys.argv[1])
files = [root / "README.md", root / "clients/python/assay/README.md"]
files.extend((root / "docs").glob("*.md"))
parser = _build_parser()
checked = 0
for file in files:
    for language, source in re.findall(r"```(\w+)\n(.*?)\n```", file.read_text(), re.DOTALL):
        if language == "python":
            ast.parse(source, filename=str(file))
            checked += 1
        elif language == "json":
            json.loads(source)
            checked += 1
        elif language == "text":
            for line in source.splitlines():
                if line.startswith("assay "):
                    parser.parse_args(shlex.split(line)[1:])
                    checked += 1
print(f"docs smoke: {checked} Python/JSON/CLI examples validated without network writes")
PYTHON

if [[ -z "${ASSAY_ACCEPTANCE_ENDPOINT:-}" ]]; then
  echo "docs smoke: static checks passed (set E1 credentials for workflow smoke)"
  exit 0
fi
if [[ -z "${ASSAY_ACCEPTANCE_ADMIN_TOKEN:-}" ]]; then
  echo "docs smoke: ASSAY_ACCEPTANCE_ADMIN_TOKEN is required with an endpoint" >&2
  exit 1
fi
EXPECTED_ENDPOINT="http://127.0.0.1:${ASSAY_ACCEPTANCE_PORT:-18080}"
if [[ "$ASSAY_ACCEPTANCE_ENDPOINT" != "$EXPECTED_ENDPOINT" ]]; then
  echo "docs smoke: refusing non-disposable endpoint; expected $EXPECTED_ENDPOINT" >&2
  exit 1
fi

export ASSAY_ENDPOINT="$EXPECTED_ENDPOINT"
export ASSAY_ADMIN_TOKEN="$ASSAY_ACCEPTANCE_ADMIN_TOKEN"
SDK_PROJECT="$ROOT/clients/python/assay"
EXISTING_WORKSPACE="$TEMP_DIR/existing-workspace.json"
: >"$EXISTING_WORKSPACE"
if uv run --project "$SDK_PROJECT" python "$ROOT/examples/quickstart/bootstrap.py" \
  --output "$EXISTING_WORKSPACE"; then
  echo "docs smoke: bootstrap accepted an existing workspace" >&2
  exit 1
fi
unlink "$EXISTING_WORKSPACE"
uv run --project "$SDK_PROJECT" python "$ROOT/examples/quickstart/bootstrap.py" \
  --output "$TEMP_DIR/workspace.json"
TRACE_ID=$(uv run --project "$SDK_PROJECT" python "$ROOT/examples/quickstart/trace.py" \
  --workspace "$TEMP_DIR/workspace.json")
read -r ASSAY_API_KEY APPLICATION_ID < <(node -e '
const workspace = require(process.argv[1]);
console.log(workspace.api_key, workspace.application_id);
' "$TEMP_DIR/workspace.json")
export ASSAY_API_KEY
uv run --project "$SDK_PROJECT" assay traces list "$APPLICATION_ID" \
  --query "$TRACE_ID" >"$TEMP_DIR/traces.json"
node -e '
const traces = require(process.argv[1]);
if (!JSON.stringify(traces).includes(process.argv[2])) {
  throw new Error("docs smoke: trace query did not return the emitted OpenTelemetry ID");
}
' "$TEMP_DIR/traces.json" "$TRACE_ID"

printf 'docs smoke: workflow passed\n'
