# Task lifecycle and operational change contract

> 当前业务流程的完整说明见 [`docs/business-process-v2.md`](business-process-v2.md)。本文聚焦 Task/Assignment API；两者冲突时，以当前代码、OpenAPI 和该业务流程基线为准。

This document records the bounded task lifecycle implemented in the current v2 slice. A task is a Core fact driven by an external flight event; it is not a free-form record that an operator or employee can edit at will.

## Implemented Core routes

- `GET /api/v1/tasks` lists Core task facts with `status`, `flight_public_id`, `page`, and `page_size` filters.
- `GET /api/v1/tasks/{taskPublicID}` returns the task, candidate snapshots, and the current assignment when present.
- `POST /api/v1/tasks/{taskPublicID}/confirm` is the controlled assignment operation. Arrival normally invokes it through the automatic dispatcher; the management endpoint remains for a controlled supplement when automatic dispatch has no usable result.
- `POST /api/v1/tasks/{taskPublicID}/cancel` is the delete-like lifecycle operation. It preserves history, audit, and Outbox facts instead of hard deleting data.
- `POST /internal/integration/v1/flights/{flightPublicID}/arrival` remains the authenticated external-source trigger for this slice. It creates the task and candidates atomically from the active template; browser/management clients must not call it.
- `GET/POST /api/v1/task-change-requests` lists or submits a durable operational change request.
- `PATCH /api/v1/task-change-requests/{publicID}/review` is the manager/admin approval boundary. Approval, task mutation, assignment release/reservation, history, audit and Outbox are one Core transaction.
- `GET /internal/integration/v1/flight-source/health` exposes source freshness and whether the worker is operating from the pre-synced database fallback.
- `GET /api/v1/realtime/management` is a short-lived, scope-filtered SSE hint stream backed by the Core Outbox cursor.

## Automatic dispatch and employee receipt

The normal flow is:

```text
external flight arrival
  -> Core flight fact / source inbox
  -> task generated as pending_dispatch
  -> candidate snapshot and deterministic automatic selection
  -> Core Assignment confirmation
  -> Core task.assigned.v1 / Edge Inbox / Projection
  -> employee receipt acknowledgement
  -> employee starts execution
  -> employee completes
```

The current production baseline is deterministic and replaceable. Candidate generation hard-filters enabled personnel in the task's area/team, the exact required position and capability, an active primary team membership, `idle` work state, and no active assignment at the same planned time. The candidate snapshot is ranked by oldest `last_state_changed_at`, then candidate public ID. Automatic dispatch tries that order under the normal Core reservation/version transaction; a conflict invalidates only the unavailable candidate and tries the remaining snapshot candidates. If all candidates are unavailable, the task remains visible for controlled handling instead of being silently cancelled.

This is the frozen v1 scheduling rule for acceptance. Future fairness, shift load, rest time, handover and cross-flight optimization changes must be introduced as a versioned selector/rule change, with replayable scheduling tests; they must not be hidden inside a handler or bypass the assignment transaction.

Employee `POST /api/v1/tasks/{taskPublicID}/received` is a receipt handshake only. It records that the notice reached the employee, does not grant a refusal right, and does not change the work assignment. `POST /api/v1/tasks/{taskPublicID}/start` begins execution after receipt. The old `accept` command/route remains only as a compatibility alias for start and is not an approval workflow.

If an employee is already supporting another flight, or a delay/cancellation/emergency makes the assignment unsafe, the employee submits an exception Command. The exception is durable and may carry one requested action: `pause`, `reassign`, `reschedule`, `cancel` or `resume`. It never directly mutates the assignment. A leader can report/observe the problem; only a manager or admin can approve it. The approved action is rechecked against the current task version and candidate/personnel facts, then applied atomically with history, audit and Outbox. A failed application is persisted as `failed` for visible follow-up.

The three handshakes are intentionally different:

1. Core system confirmation: the machine or authorized manager confirms one candidate and reserves the employee. This is the assignment fact.
2. Projection confirmation: the Outbox/Inbox delivery moves the assignment and notification to Edge. Delivery is retried and deduplicated; Redis or an open connection is never the source of truth.
3. Employee business confirmation: `received` records that the employee saw the duty notice. It is not consent and does not provide a refusal action. `start` means the employee began the work, and `complete` closes the task. If the employee is already occupied, they must report an exception rather than reject the assignment.

The worker runs a bounded receipt-timeout scan (default 300 seconds). It locks and rechecks an unreceived assignment, releases the old reservation, then tries the remaining proposed candidates using the same eligibility rules. It emits a new assignment event when successful; when no candidate remains, the task stays assigned and the shortage is visible for manager action.

Flight data is external-only. The source adapter stages normalized schedules/events into the durable Core inbox, then the worker applies them. A provider failure records retry/alertable health state. Existing pre-synced flights remain readable through `fallback`; no browser or management user can create an arbitrary flight fact. Delays update the external schedule fact and emit a schedule-change event; changing an active task still goes through the change-request approval path.

All Core management routes use the JWT entrypoint in normal configuration and enforce `task:read` plus server-side scope filtering. Explicit development actor headers are available only when `allow_dev_actor_headers` is enabled in a non-release configuration.

## Deliberately not implemented

Generic `POST /api/v1/tasks` and arbitrary `PATCH`/hard `DELETE` are not exposed. Flight facts remain an external integration concern, and task instances are generated only from arrival plus the active template. Management clients must use the change-request workflow for operational exceptions; a manual task edit would bypass the state machine.
