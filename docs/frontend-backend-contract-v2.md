# Frontend / Core backend contract update

Status: implemented in the current workspace on 2026-09-04.

## Flight source synchronization

Flight facts are external-source-owned. The browser does not create, arrive, depart, cancel, or edit flights. A provider adapter implements `internal/integration/flight.Provider`, normalizes upstream data, and submits a bounded batch to:

```text
POST /internal/integration/v1/flight-source/sync
X-Flight-Source-Key: <configured source key>
```

Core commits the normalized Schedule/Event records to `flight_source_inbox` in one short transaction. The idempotency key is `(provider, record_type, external_record_id)`. The API returns accepted, updated, and duplicate counts without waiting for task generation. The Worker claims records with a lease, applies them to Core flight facts, retries transient failures with backoff, and records terminal failures for diagnostics. `GET /api/v1/flights` is the fast read model used by Admin Web.

The normalized provider port and a configurable HTTP/JSON adapter are implemented. It supports contract-specific envelope, field, status and time mappings, environment-only credentials, optional mTLS, scheduled polling, reconciliation and webhook alerts. The vendor-specific contract, production credentials and production callback configuration remain pending; development seed flights remain only local acceptance fixtures.

## Position and capability dictionaries

Core exposes independent, paginated dictionaries:

```text
GET   /api/v1/positions
POST  /api/v1/positions
PATCH /api/v1/positions/{publicID}
DELETE /api/v1/positions/{publicID}  # safe delete: disable

GET   /api/v1/capabilities
POST  /api/v1/capabilities
PATCH /api/v1/capabilities/{publicID}
DELETE /api/v1/capabilities/{publicID}  # safe delete: disable
```

Dictionary codes are immutable after creation. Updates change only the display name, description, or enabled state. `DELETE` is exposed for CRUD completeness but performs the same safe disable operation for values already referenced by personnel or task-template history; `PATCH enabled=true` restores a disabled entry.

Personnel and task-template writes accept one position code and one capability code. The old array fields remain as one-item compatibility fields; Core rejects zero or multiple capability values and rejects unknown or disabled dictionary entries. Admin Web now loads the dictionaries and uses single-value selectors when creating personnel or templates.

## Task dispatch, receipt and controlled exceptions

Flight arrival is the only normal task creation trigger. Core creates a durable `pending_dispatch` task, runs the replaceable automatic dispatcher, and publishes `task.assigned.v1` only after the Assignment transaction succeeds. A task that has no usable candidate remains pending for retry or controlled operator handling; the management UI must not treat this as an employee approval step.

The employee-facing Edge commands are:

```text
POST /api/v1/tasks/{taskPublicID}/received  # receipt handshake; no refusal
POST /api/v1/tasks/{taskPublicID}/start     # begin execution after receipt
POST /api/v1/tasks/{taskPublicID}/complete  # finish execution
POST /api/v1/tasks/{taskPublicID}/exceptions # controlled exception request
```

The legacy `accept` endpoint is retained as a compatibility alias to `start`. A conflict with another flight, delay, cancellation, emergency or other external instruction is reported as an exception request. Employees and leaders can submit or report the request; only the duty manager or an administrator can approve and apply the resulting pause, cancellation, reallocation, reschedule or resume. An approved external flight source may update flight facts, but it does not bypass task-change approval for an active task; clients never write the Core task fact directly.

## Operational workbenches

The following Core read endpoints are implemented, RBAC/scope checked, and paginated where they return collections:

```text
GET /api/v1/personnel/status
GET /api/v1/personnel/{publicID}/status-history
GET /api/v1/events
GET /api/v1/audit
GET /api/v1/scopes
GET /api/v1/diagnostics
```

Admin Web pages for positions/capabilities, personnel status, events, audit, scopes, and diagnostics now consume these contracts. Audit responses are redacted and diagnostics contain counters only; raw payloads, passwords, tokens, and internal error details are not returned.

All Core and Edge list queries use `page`, `page_size`, `total`, bounded page sizes, and stable ordering. The employee task endpoint remains a complete per-employee Edge projection snapshot because it is the reconnect/recovery contract, not a management list.
