# Deployment and recovery

Assay is one `assayd` container and Postgres. The checked-in source Compose file is for a local
checkout; [`compose.published.yaml`](../compose.published.yaml) is the release-only, no-build file.
The [Linux quickstart](quickstart-linux.md) and [PowerShell quickstart](quickstart-powershell.md)
show their exact startup commands.

## Persistent data and secrets

Compose stores PostgreSQL data in the named `assay-pgdata` volume. `docker compose down` stops and
removes containers while keeping that volume. `docker compose down -v` deletes the volume and all
Assay data; use it only for an intentional fresh start.

Keep these outside the volume and back them up securely:

- `ASSAY_ENCRYPTION_KEY`: a stable base64 encoding of exactly 32 bytes. It encrypts stored judge API
  keys and target-endpoint secrets. A database backup without this key cannot restore those secrets.
- `ASSAY_ADMIN_TOKEN`: management and UI credential.
- Judge credentials, if configured outside Assay.

`ASSAY_POSTGRES_PASSWORD` is only the Compose database credential. It is not a judge key. Compose
uses `postgres` as the internal database host and `sslmode=disable` only inside its trusted Docker
network. Do not publish the database port; terminate HTTPS at a reverse proxy for network access.

## Back up and verify a restore

Back up before upgrades, retention changes, or any destructive operation. With the source Compose
stack running, create a custom-format dump on the host:

```bash
umask 077
mkdir -p backups
chmod 700 backups
docker compose exec -T postgres pg_dump -U assay -d assay -Fc > backups/assay-$(date +%F).dump
```

Keep the dump and encryption key in separate protected backup storage. To test a restore, use a
**new database**, never the running production database:

```bash
docker compose exec -T postgres dropdb -U assay --if-exists assay_restore_test
docker compose exec -T postgres createdb -U assay assay_restore_test
docker compose exec -T postgres pg_restore -U assay --clean --if-exists --no-owner \
  -d assay_restore_test < backups/assay-YYYY-MM-DD.dump
docker compose exec -T postgres psql -U assay -d assay_restore_test \
  -c 'SELECT count(*) FROM projects;'
```

Confirm the restored data with the same `ASSAY_ENCRYPTION_KEY` before calling the backup usable.
Remove the test database after verification with
`docker compose exec -T postgres dropdb -U assay assay_restore_test`.

## Upgrade and rollback

1. Read the release notes and take a verified backup.
2. Change `ASSAY_IMAGE` to the immutable release tag or digest that was verified for the published
   Compose path. Source users update their checkout deliberately instead.
3. Run the normal `docker compose ... up --build --force-recreate -d` command. Startup applies
   forward migrations automatically.
4. Check `docker compose ps`, `/readyz`, and the UI/API before declaring the upgrade complete.

Do **not** assume swapping an older binary safely downgrades the schema. Down migrations cannot
recover deleted source items or other lossy changes. For a failed upgrade, restore the pre-upgrade
backup into a new database, verify it, then point a compatible older deployment at that restored
database. Preserve the matching encryption key.

## Judge and tracing configuration

Tracing works with no judge. Set `ASSAY_JUDGE_BASE_URL` and `ASSAY_JUDGE_MODEL` only to enable the
built-in groundedness and correctness scorers; `ASSAY_JUDGE_API_KEY` is optional for a keyless local
endpoint. Use `ASSAY_TRACE_RETENTION_DAYS=0` to keep spans, or a positive number to expire old spans.
Retention removes spans, not trace summaries, scores, or retained score evidence.

`ASSAY_ENDPOINT=http://localhost:8080` is for host SDKs and the CLI. A container in the same Compose
network uses `http://assayd:8080`. The browser UI stores its single-user admin token in same-origin
localStorage, so use a trusted browser and origin; select Disconnect to remove it.

## Operational boundaries

- Dataset-item edits and deletion preserve immutable snapshots in existing runs. Deleting a whole
  dataset cascades its runs, so confirm that action deliberately.
- Cost/provider instrumentation, binary protobuf, OTLP/gRPC, provider auto-instrumentation, and
  session UI are not implemented.
- The embedded UI is a single-user admin-token interface, not SSO or RBAC.
- The published image remains unreleased until a maintainer approves publication and records an
  anonymous-pull smoke result. Until then, use the source path.
