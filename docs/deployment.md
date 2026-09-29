# Deployment and recovery

[Documentation](index.md) / Deployment

Assay runs as one `assayd` container plus PostgreSQL. The source Compose file builds the checkout;
[`compose.published.yaml`](../compose.published.yaml) is a separate, no-build release template.
The first [public server image](https://github.com/marioweid/assay/releases/tag/v0.1.0) is
`ghcr.io/marioweid/assay:0.1.0`, independently versioned from the Python SDK. CI anonymously pulled
and smoke-tested this immutable multi-architecture digest:

```text
ghcr.io/marioweid/assay@sha256:ca095432a1cc199e33b5239959faa8b13364841c9c3cd4712c65a15410be297f
```

Publication was approved before manual UI signoff and representative large-database migration
sizing. Do not treat public availability as approval to upgrade an existing database without a
backup and a deliberate migration plan.

## Choose the access model before starting

### Private workstation: local mode

Follow the [Linux](quickstart-linux.md) or [PowerShell](quickstart-powershell.md) quickstart with:

```dotenv
ASSAY_LOCAL_MODE=true
```

No admin token is required. Project ingestion keys, the database password and encryption key remain
required. Keep the checked-in loopback bindings and trust every service on the Docker network.
Any caller that can reach a local-mode server can manage and delete its data without credentials.
Host/Origin checks mitigate browser attacks; they do not authenticate native network clients.

### Shared/network deployment: token mode

Use `ASSAY_LOCAL_MODE=false` (the default), a separately generated strong `ASSAY_ADMIN_TOKEN`, a
private database password, and a stable base64-encoded 32-byte encryption key. Put HTTPS and access
controls in front of Assay. Do not expose a local-mode server through a reverse proxy or tunnel.

The UI is a single-administrator interface, not SSO, user accounts or RBAC. Its admin token is kept
in same-origin localStorage; use a trusted browser/origin and select **Disconnect** to remove it.
Local mode stores no replacement token and removes a stale stored admin credential on discovery.

The source Compose database port is loopback-only; published Compose does not publish it.
Do not make PostgreSQL publicly reachable. The internal URL uses `sslmode=disable` only on the
trusted Docker network. For a database outside that trust boundary, configure verified TLS and
network access controls appropriate to your deployment.

## Start and verify

For a **new** Linux/macOS deployment without a checkout, create a fresh private directory. The
Compose file is pinned to the verified server tag, uses the public image and keeps Postgres private:

```bash
set -euo pipefail
mkdir assay-deploy
cd assay-deploy
curl --fail --silent --show-error --output compose.yaml \
  https://raw.githubusercontent.com/marioweid/assay/v0.1.0/compose.published.yaml
(
  set -euo pipefail
  umask 077
  set -C
  password=$(openssl rand -hex 32)
  admin_token=$(openssl rand -hex 32)
  encryption_key=$(openssl rand -base64 32)
  image='ghcr.io/marioweid/assay@sha256:'
  image+='ca095432a1cc199e33b5239959faa8b13364841c9c3cd4712c65a15410be297f'
  printf '%s\n' "ASSAY_IMAGE=$image" "ASSAY_POSTGRES_PASSWORD=$password" \
    "ASSAY_ADMIN_TOKEN=$admin_token" "ASSAY_ENCRYPTION_KEY=$encryption_key" >.env
)
docker compose -p assay-published up -d
docker compose -p assay-published ps
curl --fail http://localhost:8080/readyz
```

The admin token stays in `.env`; enter it in the UI at `http://localhost:8080`. For trusted local
mode instead, explicitly set `ASSAY_LOCAL_MODE=true` before first boot; any caller able to reach the
server can then administer it without a token. Project ingestion keys remain required. On Windows,
use the [PowerShell quickstart](quickstart-powershell.md) to generate secrets; add the pinned
`ASSAY_IMAGE` and run published Compose under a **new** project name, not an existing source stack.

For source development instead, use the [source quickstart](quickstart-linux.md), which builds the
checkout with `docker compose up --build -d`. **Do not combine** source and published Compose. Recreate
services after changing `.env`; restarting a container does not update its environment. Startup
applies forward database migrations. Before using an existing database, follow the upgrade procedure
below and preserve its original secrets.

Readiness proves the server/database are available, not that your judge or evaluation target works.
Verify the UI, create a disposable project key, send a synthetic trace and confirm it appears.
Test a small evaluation separately if you configured a judge.

## Persistent data and secrets

Compose keeps PostgreSQL in a named `assay-pgdata` volume (normally prefixed by the Compose project).
`docker compose down` preserves it. **`docker compose down -v` deletes it and all Assay data.**
Never use that command as a repair for startup/credential problems.

Back up these outside the volume, with restricted access:

- `ASSAY_ENCRYPTION_KEY`: needed to decrypt stored judge/target secrets. A database dump without
  the matching key is not a complete backup.
- Database connection credentials and Compose configuration.
- Admin token for normal mode, plus external judge/target credentials where applicable.
- Project keys used by senders, stored in their normal secret store; raw keys cannot be recovered
  from the database's hashes. You can create replacement keys after recovery.

Preserve existing secrets. Changing `ASSAY_POSTGRES_PASSWORD` in `.env` does not rotate the password
inside an existing PostgreSQL volume. Regenerating `ASSAY_ENCRYPTION_KEY` does not re-encrypt stored
secrets and makes existing ciphertext unreadable.

## Back up and verify a restore

The following commands use **Bash** and the published database/user `assay`. In the directory
containing the Compose file, choose the **same project and file** used at startup. Keep the selection
in the same shell for backup, restore and upgrade. Verify `ps` lists the intended services **before**
dumping a database:

```bash
# Fresh no-checkout deployment above (compose.yaml):
ASSAY_COMPOSE=(docker compose -p assay-published)
# If using the published file in the checkout, replace the assignment with:
# ASSAY_COMPOSE=(docker compose -p assay-published -f compose.published.yaml)
# If using source Compose, replace it with the exact project you started:
# ASSAY_COMPOSE=(docker compose)  # or: docker compose -p YOUR_PROJECT
"${ASSAY_COMPOSE[@]}" ps
```

Substitute your configured database name/user. Use a private backup directory, and do not overwrite
a previous backup:

```bash
set -euo pipefail
umask 077
mkdir -p backups
chmod 700 backups
BACKUP="backups/assay-$(date +%Y%m%d-%H%M%S).dump"
(set -o noclobber; "${ASSAY_COMPOSE[@]}" exec -T postgres pg_dump -U assay -d assay -Fc > "$BACKUP")
```

Check the command's exit code; a file alone does not prove a successful dump. Store dumps and the
matching encryption key in separate protected backup storage. Test a restore into a **new database**,
never the active database:

```bash
RESTORE_DB="assay_restore_$(date +%Y%m%d%H%M%S)"
"${ASSAY_COMPOSE[@]}" exec -T postgres createdb -U assay "$RESTORE_DB"
"${ASSAY_COMPOSE[@]}" exec -T postgres pg_restore -U assay --single-transaction --no-owner \
  -d "$RESTORE_DB" < "$BACKUP"
"${ASSAY_COMPOSE[@]}" exec -T postgres psql -U assay -d "$RESTORE_DB" \
  -c 'SELECT count(*) FROM projects;'
```

A row count is only an initial check. Point a compatible isolated server at the restored database,
using the matching encryption key, and verify representative traces, runs and encrypted configuration.
Do not enable workers against live external targets unintentionally during recovery tests.
After verification, deliberately remove **only** that restore-test database:

```bash
"${ASSAY_COMPOSE[@]}" exec -T postgres dropdb -U assay "$RESTORE_DB"
```

## Upgrade and rollback

1. Read release notes and migration changes. Schedule downtime if a migration may lock large tables.
2. Take a verified backup and preserve the matching encryption key/configuration.
3. Update the source checkout deliberately, or pin a verified immutable image tag/digest.
4. Reuse the `ASSAY_COMPOSE` selection above **in the same shell**, reselecting it after a new login.
   For source builds, run `"${ASSAY_COMPOSE[@]}" up --build -d`. For published images, run
   `"${ASSAY_COMPOSE[@]}" pull` then `"${ASSAY_COMPOSE[@]}" up -d`.
5. Watch startup/migration logs; check readiness, auth mode, UI/API and a synthetic trace.

Do **not** assume swapping to an older binary downgrades the schema safely. Down migrations cannot
recover deleted source data or other lossy changes. For a failed upgrade, restore the pre-upgrade
backup into a new database, verify it, and point a compatible older deployment at that database.
Keep the failed database intact until diagnosis/recovery is complete.

## Retention, judges and limits

Tracing needs no judge. Evaluation needs a reachable OpenAI-compatible judge; see
[evaluations](evaluations.md) and [configuration](configuration.md). Assay's endpoint, the model
provider, and an application's evaluation target are three different URLs.

`ASSAY_TRACE_RETENTION_DAYS=0` keeps spans; a positive value expires old span partitions. Retention
removes spans, not all trace summaries, scores or retained evaluation evidence. It is not a complete
privacy-erasure mechanism. Dataset-item edits/deletion preserve old run snapshots; deleting a whole
dataset cascades its runs.

Sessions, JSON OTLP/HTTP, manual instrumentation, typed management and the CLI are implemented in
this checkout. Binary protobuf, OTLP/gRPC, automatic provider instrumentation, SSO and RBAC are not.
Local mode and session SDK features are available in published `assay-sdk` 0.4.0 or the checkout;
0.3.0 does not include them. Server image v0.1.0 and SDK 0.4.0 are independent releases.

**Next:** [Configuration reference](configuration.md) · [Troubleshooting](troubleshooting.md)
