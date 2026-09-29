# Configuration reference

[Documentation](index.md) / Configuration

The server reads process environment at startup. Docker Compose reads `.env` for interpolation and
passes the configured values into the container. The Python SDK/CLI reads its own process environment;
it does **not** automatically source Docker's `.env`. Recreate containers after changing settings.

Start with [`.env.example`](../.env.example), or use the quickstart to generate a minimal private file.
Never commit real `.env` files, project keys, judge keys or workspace bootstrap JSON.

## Authentication modes

| Setting | Default | Effect |
|---|---|---|
| `ASSAY_LOCAL_MODE` | `false` | `true` permits management/UI access without an admin token |
| `ASSAY_ADMIN_TOKEN` | required in normal mode | Bearer token for management and UI; optional only in local mode |

### Local mode deliberately removes an authentication boundary

```dotenv
ASSAY_LOCAL_MODE=true
```

The UI discovers the mode from `GET /v1/server-info`, connects without a token, removes a stale stored
admin token and displays **Local mode**. There is no dummy credential. Discovery failures show a
retry state instead of silently assuming local access.

**Anyone who can reach the server can administer it in local mode.** That includes reading traces,
creating/revoking keys, changing evaluation targets and deleting data. Trust every peer on the
Docker network. Project keys still scope ingestion and explicitly keyed trace/session requests,
but are not a defense against a client who can omit credentials and use local management access.
Explicit invalid credentials are not silently treated as anonymous admin requests.

The server also restricts Host values to `localhost`, loopback IPs and the checked-in Compose service
name `assayd`. API requests with a browser Origin must exactly match the request's scheme/host/port;
cross-site and same-site-but-not-same-origin Fetch Metadata requests are rejected. Forwarded headers
do not bypass these checks. Public UI pages remain navigable from documentation links.

These are browser/DNS-rebinding mitigations, **not network authentication**: a non-browser client
can supply headers. Keep `127.0.0.1` Compose bindings. Do not use local mode with public reverse
proxies, tunnels, shared hosts or untrusted Docker networks. Custom hostnames are intentionally not
supported in local mode; use normal token mode for those deployments.

To disable it, set `ASSAY_LOCAL_MODE=false`, configure a real admin token, recreate the server,
and reload the browser. Set the same mode/credential in any host CLI environment.

## Server settings

| Variable | Default / requirement | Purpose |
|---|---|---|
| `ASSAY_HTTP_ADDR` | `:8080` | Native server listen address; Compose fixes the internal port to 8080 |
| `ASSAY_DATABASE_URL` | required | PostgreSQL URL; Compose overrides it with its internal `postgres` host |
| `ASSAY_ENCRYPTION_KEY` | required in every mode | Base64 encoding of exactly 32 bytes; encrypts stored judge/target secrets |
| `ASSAY_UI_ENABLED` | `true` | Serve the embedded browser UI at `/` |
| `ASSAY_AUTO_CREATE_APPS` | `false` | Allow ingest to create an unknown application slug in the key's project |
| `ASSAY_WORKER_CONCURRENCY` | `GOMAXPROCS`; blank uses default | Number of worker jobs processed concurrently; positive integer |
| `ASSAY_JOB_MAX_ATTEMPTS` | `3` | Positive maximum job attempts |
| `ASSAY_TRACE_RETENTION_DAYS` | `0` | Zero keeps spans; positive values expire old span partitions |
| `ASSAY_LOG_FORMAT` | `json` | `json` or `text` |

Keep the encryption key stable across restarts, upgrades and restores. Local mode does not eliminate
it: secrets already stored in the database still depend on it. Retention deletes spans, not all trace
summaries, scores or retained evaluation evidence. It is not a comprehensive privacy-deletion policy.

## Compose settings

| Variable | Default / requirement | Purpose |
|---|---|---|
| `ASSAY_HTTP_PORT` | `8080` | Host HTTP port, bound to loopback |
| `ASSAY_POSTGRES_PASSWORD` | required | Generated URL-safe database password, separate from other credentials |
| `ASSAY_POSTGRES_USER` | `assay` in source Compose | Database user |
| `ASSAY_POSTGRES_DB` | `assay` in source Compose | Database name |
| `ASSAY_POSTGRES_PORT` | `5432` in source Compose | Loopback-only host database port; published Compose keeps Postgres private |
| `ASSAY_IMAGE` | published Compose only | Verified server v0.1.0 digest in `.env.example`; pin or update deliberately |

`docker-compose.yml` builds from source. `compose.published.yaml` never builds; do not combine them.
The published file uses the fixed database user/name `assay`. A Compose `.env` is not automatically
an environment file for arbitrary additional application services.

For a fresh alternative HTTP port, set `ASSAY_HTTP_PORT=18080`, recreate the server, then use
`http://localhost:18080` in the browser and host SDK. Do not change the container's port mapping
or `ASSAY_HTTP_ADDR` independently of its healthcheck.

## SDK and CLI settings

| Variable / parameter | Used by | Meaning |
|---|---|---|
| `ASSAY_ENDPOINT` | tracing / CLI | Assay base URL; no `/v1/traces` suffix |
| `ASSAY_API_KEY` | tracing / CLI | Project key; always required by tracing |
| `ASSAY_APPLICATION` | tracing | Existing application **slug** |
| `ASSAY_ADMIN_TOKEN` | CLI | Admin credential for token mode |
| `ASSAY_LOCAL_MODE` | management Client / CLI | Allow management without credentials when the server also enables it |
| `Client(endpoint, local_mode=...)` | management Client | Explicit boolean overrides the environment |
| `Client(..., api_key=..., admin_token=...)` | management Client | Credentials are explicit constructor arguments; not automatically read from env |
| `init(capture=True)` | tracing | Opt into decorator input/return capture; default false |
| `init(max_capture_bytes=...)` | tracing | Per-attribute capture cap; default 65,536 bytes |

The CLI reads credentials from the environment. The management Client reads the local-mode flag
when omitted, but endpoint and credentials are supplied to its constructor. `assay.init()` reads
its three tracing variables; explicit arguments override them. No SDK setting can turn off
server-side token authentication remotely.

## Judges and target endpoints

| Variable | Purpose |
|---|---|
| `ASSAY_JUDGE_BASE_URL` | OpenAI-compatible judge endpoint, typically ending in `/v1` |
| `ASSAY_JUDGE_MODEL` | Actual model name available at that endpoint |
| `ASSAY_JUDGE_API_KEY` | Provider credential, optional only for a keyless endpoint |

Tracing requires none of these. Scorer-level settings override project settings, which override
global defaults. Evaluation target endpoints are configured on the **application**, not through
`ASSAY_ENDPOINT`. Targets generate answers; judges evaluate answers; Assay stores the evidence.

The optional demo chat has separate `ASSAY_CHAT_*` cookie/origin/signing settings; see its
[README](../examples/python-qa/README.md). Local mode does not remove the demo's signed transcript
capability or make its evaluation endpoint public.

## URL cheat sheet

| Caller | Assay URL |
|---|---|
| Browser / host Python / host CLI | `http://localhost:8080` |
| Another service on the same Compose network | `http://assayd:8080` |
| Assay reaching a host-local judge | Typically `http://host.docker.internal:11434/v1` |

A separate Compose project is a different network by default. Connecting it to Assay's network
also grants it local-mode management access. Review that trust decision explicitly.

**Next:** [Troubleshooting](troubleshooting.md) · [Deployment and recovery](deployment.md)
