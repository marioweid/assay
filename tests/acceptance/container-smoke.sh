#!/usr/bin/env bash
set -euo pipefail

if (($# != 1)); then
  echo "usage: $0 IMAGE" >&2
  exit 2
fi

IMAGE=$1
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PROJECT="assay-smoke-$(date +%s)-$$"
TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/$PROJECT.XXXXXX")
COMPOSE_FILE="$TEMP_DIR/compose.yaml"
ENV_FILE="$TEMP_DIR/.env"
umask 077

for command in base64 curl docker grep od; do
  command -v "$command" >/dev/null || {
    echo "container smoke: required command not found: $command" >&2
    exit 1
  }
done
docker info >/dev/null
docker image inspect "$IMAGE" >/dev/null || {
  echo "container smoke: image is not available locally: $IMAGE" >&2
  exit 1
}

compose() {
  docker compose --env-file "$ENV_FILE" --project-name "$PROJECT" --file "$COMPOSE_FILE" "$@"
}

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ "$PROJECT" =~ ^assay-smoke-[a-zA-Z0-9-]+$ ]]; then
    if ((status != 0)); then
      compose logs --no-color --tail 200 >&2 || true
    fi
    compose down --volumes --remove-orphans >/dev/null || status=1
  else
    echo "container smoke: refusing cleanup for unsafe project name: $PROJECT" >&2
    status=1
  fi
  unlink "$COMPOSE_FILE" 2>/dev/null || status=1
  unlink "$ENV_FILE" 2>/dev/null || status=1
  rmdir "$TEMP_DIR" 2>/dev/null || status=1
  exit "$status"
}
trap cleanup EXIT INT TERM

ADMIN_TOKEN=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
POSTGRES_PASSWORD=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64 | tr -d '\n')
printf '%s\n' \
  "ASSAY_SMOKE_IMAGE=$IMAGE" \
  "ASSAY_SMOKE_ADMIN_TOKEN=$ADMIN_TOKEN" \
  "ASSAY_SMOKE_ENCRYPTION_KEY=$ENCRYPTION_KEY" \
  "ASSAY_SMOKE_POSTGRES_PASSWORD=$POSTGRES_PASSWORD" >"$ENV_FILE"

cat >"$COMPOSE_FILE" <<'YAML'
services:
  postgres:
    image: postgres:18.6-trixie@sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280
    environment:
      POSTGRES_USER: assay
      POSTGRES_PASSWORD: ${ASSAY_SMOKE_POSTGRES_PASSWORD:?required}
      POSTGRES_DB: assay
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U assay -d assay"]
      interval: 2s
      timeout: 3s
      retries: 30
    extra_hosts:
      - "host.docker.internal:host-gateway"

  assayd:
    image: ${ASSAY_SMOKE_IMAGE:?required}
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      ASSAY_HTTP_ADDR: ":8080"
      ASSAY_DATABASE_URL: postgres://assay:${ASSAY_SMOKE_POSTGRES_PASSWORD:?required}@postgres:5432/assay?sslmode=disable
      ASSAY_ADMIN_TOKEN: ${ASSAY_SMOKE_ADMIN_TOKEN:?required}
      ASSAY_ENCRYPTION_KEY: ${ASSAY_SMOKE_ENCRYPTION_KEY:?required}
      ASSAY_UI_ENABLED: "true"
    ports:
      - "127.0.0.1::8080"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    healthcheck:
      test: ["CMD", "/assayd", "healthcheck"]
      interval: 2s
      timeout: 3s
      retries: 30
YAML

compose up --detach --wait --wait-timeout 180
compose exec --no-TTY postgres getent hosts host.docker.internal >/dev/null
ENDPOINT="http://$(compose port assayd 8080)"
INDEX=$(curl --fail --silent --show-error "$ENDPOINT/")
printf '%s' "$INDEX" | grep -E 'src="/assets/index-[A-Za-z0-9_-]+\.js"' >/dev/null
curl --fail --silent --show-error "$ENDPOINT/assay-icon.png" >/dev/null
STYLE_PATH=$(printf '%s' "$INDEX" | grep -oE '/assets/index-[A-Za-z0-9_-]+\.css' | head -1)
STYLE=$(curl --fail --silent --show-error "$ENDPOINT$STYLE_PATH")
FONT_PATH=$(printf '%s' "$STYLE" | grep -oE '/assets/[A-Za-z0-9_-]+\.woff2' | head -1)
curl --fail --silent --show-error "$ENDPOINT$FONT_PATH" >/dev/null
curl --fail --silent --show-error "$ENDPOINT/readyz" >/dev/null
DEEP_LINK=$(curl --fail --silent --show-error "$ENDPOINT/apps")
printf '%s' "$DEEP_LINK" | grep -F 'id="root"' >/dev/null
if [[ $(curl --silent --output /dev/null --write-out '%{http_code}' "$ENDPOINT/v1/projects") != "401" ]]; then
  echo "container smoke: unauthenticated API request was not rejected" >&2
  exit 1
fi
compose exec --no-TTY assayd /assayd healthcheck

cd "$ROOT"
printf 'container smoke: %s passed\n' "$IMAGE"
