# TenantForge quality upgrade implementation plan

> **For agentic workers:** Use superpowers:subagent-driven-development with independent backend/frontend ownership after the shared contract is fixed; use dispatching-parallel-agents only for disjoint files. Coordinator owns docs, examples, scripts, CI and acceptance evidence.

**Goal:** Close every reviewed quality defect and deliver the redesigned administration experience with runtime evidence.

**Architecture:** Retain PostgreSQL/RLS and existing identity protocols. Short authorization/publication transactions surround external I/O; independent workers and narrow extension interfaces share one authoritative permission implementation.

**Tech Stack:** Existing Go/pgx/Goose/Casbin, Vue/TypeScript/Element Plus; maintained Prometheus Go client.

## Backend implementation

Files: `internal/platform`, `extension`, `migrations`, `go.mod`, `go.sum`.
- [x] Reproduce PATCH department clearing using the existing HTTP fixture: create a department and ticket; PATCH `{version:1,department_id:null,data:{}}`; GET must return no department. Verify original failure before reusing OptionalString; also prove omission preserves and clear outside resulting scope rejects.
- [x] Add channel-gated storage/response-stream tests proving same-session requests and unrelated tenant writes complete while a transfer is paused. Refactor request/identity/tenant/quota/file transfer boundaries; retain all quota/revocation/publication regressions and RLS invariants.
- [x] Test a blocked job does not prevent a mail or collector callback; use separate cancellation/deadline contexts and bounded lanes with deterministic shutdown. No unbounded goroutines or queues.
- [x] Add failing custom operation/job registration tests, unknown/duplicate/action validation, freeze/deep copies, authority denial and optimistic conflict. Implement public narrow interfaces and extract the ticket workflow into the same extension mechanism.
- [x] Compute row capabilities from original records/grants, including scoped actions/fields/transitions; add authorized workload counts and optional localized module/operation descriptors. Match the design JSON exactly before frontend consumes it.
- [x] Persist redacted typed failure stage/code and check post-provider status writes; instrument duration/rejection/worker/queue metrics through a private Prometheus registry. Test parseable metrics and redaction.
- [x] Run `go test -race ./...`, real isolated PostgreSQL suite, `go vet ./...`, build and independent review. Commit only owned files after coordinator approval.

## Frontend implementation

Files: `web` only.
- [x] Add failing tests for tenant-read/self-update rows: other-owner record has no edit/delete action; matching row edit fields come only from backend capabilities. Missing capabilities default deny.
- [x] Add a tested localization/presentation layer for operations, statuses, scope summaries, relative dates and technical detail disclosure. Reuse installed Element Plus icons and components.
- [x] Replace role-card-first editing with a keyboard-accessible resource/action matrix, detail panel, safe presets and resulting-grant preview. Tests preserve multiple grants, sensitive masks and delegation limits.
- [x] Split organization views and resource subcomponents by responsibility; preserve current routes, form validation and conflict handling. Integrate custom operation metadata/endpoint.
- [x] Redesign shell, workbench, tables, drawers, account/auth/files/jobs/audit/tenant screens consistently for desktop/mobile/light/dark. Prioritize real workload and diagnosed job failures; suppress raw IDs from primary activity text.
- [x] Update browser acceptance, add mixed-record capability and department-clear flows, capture visual artifacts. Run formatting, unit/component tests, typecheck/build and independent review; commit web only after coordinator approval.

## Coordinator integration

Files: `docs`, `examples`, root scripts/config/CI.
- [x] Validate API additions, update OpenAPI and API contract; retain backward-compatible endpoints and mark added fields.
- [x] Create an external-package asset approval/job example with registered custom actions; real integration verifies permissions/version/audit/job lifecycle without modifying core.
- [x] Add deterministic end-to-end and concurrency verification, plus visual evidence. Ensure tests touch only isolated databases and retain interrupted-process safeguards.
- [x] Record upstream choices/licenses/security compatibility; update notices for changed dependencies.
- [x] Run full integration/browser/image/backup-recovery acceptance after independent spec and code-quality reviews. Resolve all findings and rehearse migration of the real snapshot with its original administrator preserved.
- [x] Integrate locally, restart the main app and verify live readiness without resetting the existing administrator.
- [x] Final delivery links screenshots and observed verification and describes any material limitation without calling incomplete work finished.
