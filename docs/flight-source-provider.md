# Flight source provider runbook

The Worker can poll a real AODB or airline HTTP/JSON endpoint through
`internal/integration/flight.HTTPJSONProvider`. Core receives only normalized
`Schedule` and `Event` values; provider-specific field names and status values
stay in configuration.

## Required deployment inputs

The actual AODB/airline contract must provide:

- an absolute HTTPS base URL and separate schedule/event paths;
- the query parameter names used for the UTC polling window;
- one credential mode: `bearer`, `api_key`, `basic`, or `none`;
- JSON envelope paths and field paths for schedules and lifecycle events;
- source status mapping to `arrived`, `departed`, `cancelled`, or `delayed`;
- the source timestamp format and timezone convention.

Use `configs/config.v2.yaml` as the shape of the configuration. The file does
not contain a real secret. Set `sync.flight_source.enabled=true` only after
replacing the placeholder endpoint and mapping values with the signed source
contract.

Credentials are read from the environment variables named by:

- `sync.flight_source.auth.token_env` for bearer/API-key credentials;
- `sync.flight_source.auth.username_env` and `password_env` for basic auth;
- `sync.flight_source.alert.webhook_url_env` for the alert gateway URL.

The Worker supports optional mTLS through the nested `tls` block. Certificate
files are deployment secrets and are not committed to the repository.

## Runtime behavior

Each polling cycle:

1. fetches schedules and lifecycle events for the configured UTC window;
2. validates and maps the response, then stages it into Core
   `flight_source_inbox` with the provider/external-record idempotency key;
3. lets the normal Worker loop apply the inbox asynchronously with lease,
   retry and failed-record semantics;
4. compares the upstream schedule set with Core flight facts and persists a
   `flight_source_reconciliation` report;
5. sends a generic JSON alert for source failure, reconciliation failure, or
   a non-pending reconciliation mismatch when the webhook notifier is enabled.

`pending` reconciliation means the source record is present in the durable
Inbox but has not been applied yet. It is observable and is not raised as a
data mismatch until the apply window is exhausted.

## Isolated acceptance

After Docker Desktop is available, run:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/acceptance-backend.ps1
```

The script uses a separate Compose project, ports, and volumes; it applies
Core/Edge migrations, asserts Core `000010` through `000014` and Edge
`000007`, waits for both APIs, and runs the SQL/HTTP at-least-once probe. It
does not call `down -v` or delete volumes, so an interrupted run remains
inspectable.
