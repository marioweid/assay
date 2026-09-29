#!/usr/bin/env bash
# Exercise the opt-in local mode only on an isolated disposable PostgreSQL/Assay stack.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"
PROJECT="assay-acceptance-local-$(date +%s)-$$"
IMAGE="$PROJECT-assayd"
FIXTURE="$PROJECT-fixtures"
ENV_FILE=$(mktemp "${TMPDIR:-/tmp}/$PROJECT.XXXXXX.env")
OVERRIDE=$(mktemp "${TMPDIR:-/tmp}/$PROJECT.XXXXXX.yaml")
BASE="$ROOT/tests/acceptance/compose.yaml"
umask 077

cleanup() {
  status=$?
  trap - EXIT
  if [[ "$PROJECT" =~ ^assay-acceptance-local-[0-9-]+$ ]]; then
    if ! docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" \
      -f "$BASE" -f "$OVERRIDE" down --volumes --remove-orphans; then
      status=1
    fi
    if ! docker image rm "$IMAGE" "$FIXTURE"; then
      status=1
    fi
  fi
  unlink "$ENV_FILE"
  unlink "$OVERRIDE"
  exit "$status"
}
trap cleanup EXIT

PASSWORD=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
KEY=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
printf '%s\n' "ASSAY_ACCEPTANCE_ADMIN_TOKEN=synthetic-unused-local-test-token" \
  "ASSAY_ACCEPTANCE_ENCRYPTION_KEY=$KEY" \
  "ASSAY_ACCEPTANCE_POSTGRES_PASSWORD=$PASSWORD" \
  'ASSAY_ACCEPTANCE_PORT=18081' \
  "ASSAY_ACCEPTANCE_IMAGE=$IMAGE" \
  "ASSAY_ACCEPTANCE_FIXTURE_IMAGE=$FIXTURE" > "$ENV_FILE"
# Never modify or start the source/persistent Compose project.
printf '%s\n' 'services:' '  assayd:' '    environment:' \
  '      ASSAY_LOCAL_MODE: "true"' '      ASSAY_ADMIN_TOKEN: ""' > "$OVERRIDE"

docker info >/dev/null
docker build -f assayd/Dockerfile --tag "$IMAGE" .
docker build -f tests/acceptance/Dockerfile.fixtures --tag "$FIXTURE" .
docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" \
  -f "$BASE" -f "$OVERRIDE" up -d --wait --wait-timeout 180
export ASSAY_ACCEPTANCE_ENDPOINT=http://127.0.0.1:18081
export ASSAY_ACCEPTANCE_PROJECT="$PROJECT"
export ASSAY_LOCAL_MODE=true
uv run --project clients/python/assay pytest -q \
  clients/python/assay/tests/test_local_mode_acceptance.py
(cd web && corepack pnpm exec playwright test --config playwright.config.ts e2e/local-mode-real.spec.ts)
