#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
COMPOSE_FILE="$ROOT/tests/acceptance/compose.yaml"
PORT=${ASSAY_ACCEPTANCE_PORT:-18080}
EXPECTED_ENDPOINT="http://127.0.0.1:$PORT"
PROJECT="assay-acceptance-$(date +%s)-$$"
IMAGE="$PROJECT-assayd"
FIXTURE_IMAGE="$PROJECT-fixtures"
KEEP=${ASSAY_ACCEPTANCE_KEEP:-0}
umask 077

if [[ "$KEEP" != "0" && "$KEEP" != "1" ]]; then
  echo "acceptance: ASSAY_ACCEPTANCE_KEEP must be 0 or 1" >&2
  exit 1
fi
for command in base64 docker head od pnpm; do
  command -v "$command" >/dev/null || {
    echo "acceptance: required command not found: $command" >&2
    exit 1
  }
done
docker info >/dev/null
if [[ "$PORT" == "8080" || "${ASSAY_ACCEPTANCE_ENDPOINT:-$EXPECTED_ENDPOINT}" != "$EXPECTED_ENDPOINT" ]]; then
  echo "acceptance: refusing non-disposable endpoint; expected $EXPECTED_ENDPOINT" >&2
  exit 1
fi
ENV_FILE=$(mktemp "${TMPDIR:-/tmp}/$PROJECT.XXXXXX.env")

cleanup() {
  status=$?
  cleanup_failed=0
  trap - EXIT INT TERM
  if [[ ! "$PROJECT" =~ ^assay-acceptance-[a-zA-Z0-9-]+$ ]]; then
    echo "acceptance: refusing cleanup for unsafe project name: $PROJECT" >&2
    exit 1
  fi
  if ((status == 0 && KEEP == 1)); then
    printf 'acceptance: stack preserved at %s; endpoint: %s; credentials: %s\n' \
      "$PROJECT" "$EXPECTED_ENDPOINT" "$ENV_FILE" >&2
    exit 0
  fi
  if ((status != 0)); then
    docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" \
      --file "$COMPOSE_FILE" logs --no-color --tail 200 >&2 ||
      echo "acceptance: failed to capture container logs" >&2
  fi
  if ! docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" \
    --file "$COMPOSE_FILE" down --volumes --remove-orphans >/dev/null; then
    echo "acceptance: failed to remove disposable containers and volumes" >&2
    cleanup_failed=1
  fi
  if ! docker image rm "$IMAGE" "$FIXTURE_IMAGE" >/dev/null; then
    echo "acceptance: failed to remove disposable images" >&2
    cleanup_failed=1
  fi
  if [[ -e "$ENV_FILE" ]] && ! unlink "$ENV_FILE"; then
    echo "acceptance: failed to remove temporary credentials" >&2
    cleanup_failed=1
  fi
  if ((status == 0 && cleanup_failed != 0)); then
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

ADMIN_TOKEN=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
POSTGRES_PASSWORD=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
printf '%s\n' \
  "ASSAY_ACCEPTANCE_ADMIN_TOKEN=$ADMIN_TOKEN" \
  "ASSAY_ACCEPTANCE_ENCRYPTION_KEY=$ENCRYPTION_KEY" \
  "ASSAY_ACCEPTANCE_POSTGRES_PASSWORD=$POSTGRES_PASSWORD" \
  "ASSAY_ACCEPTANCE_PORT=$PORT" \
  "ASSAY_ACCEPTANCE_IMAGE=$IMAGE" \
  "ASSAY_ACCEPTANCE_FIXTURE_IMAGE=$FIXTURE_IMAGE" >"$ENV_FILE"
chmod 600 "$ENV_FILE"

cd "$ROOT"
docker build --file assayd/Dockerfile --tag "$IMAGE" .
docker build --file tests/acceptance/Dockerfile.fixtures --tag "$FIXTURE_IMAGE" .
docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" --file "$COMPOSE_FILE" \
  up --detach --wait --wait-timeout 180

export ASSAY_ACCEPTANCE_ENDPOINT="$EXPECTED_ENDPOINT"
export ASSAY_ACCEPTANCE_ADMIN_TOKEN="$ADMIN_TOKEN"
export ASSAY_ACCEPTANCE_PROJECT="$PROJECT"
(
  cd web
  if [[ -z "${PLAYWRIGHT_CHROMIUM_EXECUTABLE:-}" ]]; then
    pnpm exec playwright install chromium
  fi
  pnpm test:e2e
)

./tests/acceptance/docs-smoke.sh

if [[ -f clients/python/assay/tests/test_product_acceptance.py ]]; then
  command -v uv >/dev/null || {
    echo "acceptance: uv is required for SDK acceptance" >&2
    exit 1
  }
  uv run --project clients/python/assay pytest -q \
    clients/python/assay/tests/test_product_acceptance.py
fi
