# BVS2-06 Task CRUD contract

This document records the bounded CRUD surface implemented in the current v2 slice.

## Implemented Core routes

- `GET /api/v1/tasks` lists Core task facts with `status`, `flight_public_id`, `page`, and `page_size` filters.
- `GET /api/v1/tasks/{taskPublicID}` returns the task, candidate snapshots, and the current assignment when present.
- `POST /api/v1/tasks/{taskPublicID}/confirm` is the assignment/update lifecycle operation.
- `POST /api/v1/tasks/{taskPublicID}/cancel` is the delete-like lifecycle operation. It preserves history, audit, and Outbox facts instead of hard deleting data.
- `POST /api/v1/flights/{flightPublicID}/arrival` remains the create operation for this slice. It creates the task and candidates atomically from the active template.

All Core management routes use the JWT entrypoint in normal configuration and enforce `task:read` plus server-side scope filtering. Explicit development actor headers are available only when `allow_dev_actor_headers` is enabled in a non-release configuration.

## Deliberately not implemented

Generic `POST /api/v1/tasks` and arbitrary `PATCH`/hard `DELETE` are not exposed yet. Their draft/publish semantics, template and flight binding, candidate calculation, idempotency key, and event contract remain product decisions. Adding them before those decisions would create a second task-generation path and bypass the frozen state machine.
