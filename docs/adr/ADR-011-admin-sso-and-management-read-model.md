# ADR-011: Core-owned admin SSO and management read models

Status: Accepted for the current P0 implementation
Date: 2026-09-02

## Decision

- Core owns `admin_identity`, one-time `admin_sso_state`, and revocable `admin_session` records in the Core MySQL schema.
- The browser completes an authorization-code flow through Core. Core validates the allow-listed redirect URI, consumes the state exactly once, resolves a pre-provisioned active admin identity, and issues a Core-audience JWT. Provider tokens are never returned to the browser.
- Provider integration is an adapter. The current production-shaped adapter is direct Enterprise WeChat OAuth; OIDC is not required for this deployment and may only be added later as a separate adapter. The development adapter is explicit, disabled in release validation, and still requires a Core identity mapping.
- Every authenticated management request resolves the current role and access scope from the Core session record. JWT claims alone are not treated as the source of current permissions, so revocation and identity disablement take effect on the next request.
- Management personnel and assignment reads are Core application queries. The handler receives a principal, while the service overwrites query scope from that principal and the MySQL adapter applies parameterized team/area/user predicates.

## Consequences

- Real provider endpoints, credentials, callback domains, and `admin_identity` provisioning remain deployment work; no secret is committed to the repository.
- The current management read surface is intentionally limited to `GET /api/v1/personnel` and `GET /api/v1/assignments`, in addition to the existing task read and lifecycle routes. Generic task CRUD is not inferred from these reads.
- A valid provider login with no Core identity mapping fails closed with a stable error. Provider availability is not allowed to bypass Core authorization.
- The state and session tables require migration `migrations/core/mysql/000005_admin_sso` before the enabled SSO path can run against MySQL.

## Verification boundary

Unit tests cover one-time state consumption, session role/scope restoration, replay rejection, handler cookie behavior, management principal authorization, and the Enterprise WeChat adapter contract. Live Enterprise WeChat calls and production identity provisioning remain unverified until deployment credentials and domains are supplied.
