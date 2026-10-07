# TenantForge approved product design

The user approved a complete Go/Vue multi-tenant administration product on 2026-10-07, including functionality, roles, data and field permissions, account security, operations, extensibility and usable installation. This document concretizes those approved requirements; it does not narrow them to a prototype.

## Architecture

A modular Go application serves a same-origin Vue 3/TypeScript application and REST API. PostgreSQL is authoritative; durable jobs and outbox delivery use PostgreSQL locking rather than a mandatory additional message broker. Identity is global; tenant membership, departments, roles, files, resource records, jobs, notifications, settings and quotas are tenant-owned. Opaque server-side sessions use HttpOnly cookies. No tokens or passwords in browser storage. CSRF protects mutations. OIDC and TOTP use established libraries. Optional integrations are explicitly configured; local defaults work without paid services.

## Authorization

Each role grant binds tenant, registered resource, action, data scope, typed conditions and field access. Scopes: self (record owner), department, department tree, selected departments, entire tenant. Union only grants that authorize the requested action. Never transfer an all-read scope into an own-update grant. Field access is evaluated against the grants matching that row; sensitive fields require explicit access and can be masked. Default deny. Frontend visibility is derived from backend capabilities and is not authoritative.

All SQL reads/counts/writes and export/job execution enforce identical predicates. Tenant-aware foreign keys protect references; runtime PostgreSQL privileges and RLS protect tenant boundaries. A mutation checks existing and resulting owner/department, expected version and valid business transition, then commits its audit event atomically. Membership suspension, tenant suspension and grant changes take effect on subsequent requests/jobs. Authorization administrators cannot delegate permissions outside their own delegable authority or remove the final tenant owner. Platform administration has no implicit tenant data access.

## Complete product modules

Identity: automatic empty-only default superadministrator/default tenant initialization, username/email login/logout, mandatory temporary-password change, password recovery, bounded login attempts, TOTP enrollment/confirmation/disable, hashed one-time recovery codes, session listing/revocation, configured OIDC verified callback. The approved entry is administrator login; there is no workspace-creation wizard.

Tenancy: backend provisioning/status, direct administrator-created accounts and role/department assignments, optional membership invitations/acceptance, tenant switching, departments/positions, role templates/copy/edit, per-operation scopes/fields, effective permission inspection, tenant configuration and quotas, member suspension/transfer.

Business platform: registered module descriptors validate data; ticket lifecycle demonstrates create/assignment/transition/closure, concurrent conflict handling, protected customer fields, files, dictionaries/settings, notifications, CSV import and asynchronous export. Extension contract registers fields, actions, migrations and frontend descriptors without changing the core authorization system. Audit, jobs and files are authorized modules too.

Operations: health/readiness, structured redacted logs/request IDs, metrics, durable jobs with leases/bounded retries/cancellation and terminal-result deletion, local files and configured S3-compatible storage, SMTP outbox and local development mailbox, versioned migrations, automatic administrator provisioning, container deployment, backup/restore, upgrade verification, dependency checks, CI, English/Chinese documentation.

## Verification contract

Acceptance requires Go race/unit/integration tests on real PostgreSQL, Vue typecheck/tests/build, browser flows, cross-tenant ID tampering, role combinations, all-scope variants, field redaction/edit guards, revocation, CSRF/session/MFA/password recovery, duplicate jobs, transactional audit, optimistic conflicts, upload/download authorization, import/export authorization, migration idempotency, installation and backup/restore. Optional OIDC/S3/SMTP use reproducible local test endpoints or protocol tests and clearly documented configuration.

User confirms design and implementation in the current session. Routine implementation choices are delegated to the agent; no additional workflow approval is required to execute this design locally.
