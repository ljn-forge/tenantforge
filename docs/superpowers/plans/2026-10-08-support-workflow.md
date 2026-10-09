# Internal Support Workflow Implementation Plan

> **For agentic workers:** Use test-driven development and bounded parallel tasks with separate file ownership; review specification before quality review.

**Goal:** Make TenantForge's business value tangible through a complete internal support-ticket reference workflow.

**Architecture:** Reuse the builtin ticket resource and its authorization/state graph. Append-only tenant-isolated activity accompanies common writes, while typed frontend handling uses server capabilities. Existing platform components remain reusable and example-specific behavior stays in ticket-focused files.

**Tech Stack:** Go, PostgreSQL/Goose, Vue/Element Plus, Vitest, Playwright; no new dependencies.

## Task 1: Ticket service and security (backend owner)

Files: internal/platform/ticket_workflow.go, new support_workflow.go, resources.go, authz.go, capabilities.go, app.go, extension/types.go, new migrations/011_ticket_events.sql; internal/platform/support_workflow_integration_test.go and support_workflow_test.go.

- [x] Add failing integration assertions for POST assignment with only assign+read (no update), required resolution through transition and PATCH, correct reply/note visibility/count under explicit field grants, stale and closed replies, current scope and cross-tenant boundaries. Use existing newFixture and a second restricted client; use only tenantforge_test.
- [x] Run `python3 scripts/run-env.py --owner go test -race -count=1 ./internal/platform -run 'TestSupport'`; confirm route/validation/visibility failures reflect missing behavior.
- [x] Add the additive capabilities/fields/actions, common-write activity hook, RLS migration and three endpoints to match the complete spec exactly. Reuse q.record, updateRecord, CanWrite/Allowed/FilterData and q.sqlPage. Never copy authorization into another unchecked path.
- [x] Run focused and full real database race suites, formatting, vet and compilation. Assert audit rollback and required event writes; verify known existing tests retain behavior.
- [x] Commit only backend/migrations/extension paths and report contract deviations immediately.

## Task 2: Handling interface and role templates (frontend owner)

Files: web/src/api/types.ts, focused support API/composable/components, ResourcesView.vue and resources.ts only as required, RoleEditor/RolesView/new support role helper, relevant styles/presentation, web/tests/support*.test.ts. Excludes all e2e files.

- [x] Add failing component tests for valid capabilities-only composer kinds, required summary, POST expected version/kind/body, stale context/close clearing, retry/conflict/double-submit, role template sensitivity preview and existing draft safety.
- [x] Run `npm test --prefix web`; confirm the targeted assertions fail before behavior implementation.
- [x] Reuse existing detail wrapper and state invalidation; provide queue controls, immutable activity rendering/paging, assignment using dedicated action and supported scoped directory choices, reply/note composer and resolution input. Exact requests and types come from the canonical amended contract.
- [x] Add new-role templates with explicit sensitive grants and no automatic persistence. Preserve generic pages and existing acceptance labels.
- [x] Run frontend tests, format, type/build and `python3 scripts/check-web-budget.py`; commit only owned web paths.

## Task 3: Business explanation, contract and persisted acceptance (root owner)

Files: docs/api-contract.md, docs/openapi.json, README.md, README.zh-CN.md, docs/support-desk.md, docs/security.md and installation/operations references; web/e2e/support.spec.ts; docs/assets/support-*; docs/verification.md; tooling only when justified by isolation.

- [x] Record and self-review exact protocol before consumers implement; source parity stays checked by `python3 -m unittest discover -s tests -p test_api_contract.py`.
- [x] Create role/account/department business acceptance with ordinary APIs, then employee UI for start/reply/note/resolve/close/reopen and tenant/field guards. Run against built chunks in the existing isolated browser fixture; screenshots contain acceptance data only.
- [x] Explain target user and reuse benefits first in both READMEs; provide an explicit administrator-to-agent tutorial and mapping from workflow to framework building blocks. Distinguish stored communication from outbound delivery.
- [ ] Review implementation and run make check, real DB race, Python contract/tooling checks, production browser (stock and opt-in assets), recovery and final image build. Fresh tests/restore use only declared isolated databases.
- [ ] Back up actual local app and record private fingerprints, then fast-forward main, rebuild/start and prove readiness/admin login/original data preservation. Commit observed evidence, no fabricated timings or remote CI claims.
