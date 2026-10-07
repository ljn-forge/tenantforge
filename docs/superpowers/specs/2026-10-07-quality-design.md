# TenantForge quality and interaction redesign

The user authorized fixing all seven review findings and taking the design to a
finished, coherent standard. Preserve default administrator sign-in, tenancy,
per-operation scopes/conditions/fields, RLS, optimistic versions and audit
atomicity. Existing accounts and the normal database must not be reset.

## Chosen approach

Use a bounded redesign of the existing modular monolith. A cosmetic patch would
leave lock contention and weak extension boundaries; rewriting the framework
would discard verified authorization/protocol code. Reuse pgx/Goose/Casbin,
Element Plus forms/tables/tree controls and its existing icon package. Compare
the existing PostgreSQL lease queue with River and Redis-backed Asynq; retain
the tested transactional queue and separate its worker lanes rather than add a
broker or replace the durable job schema. For metrics, use the maintained
Prometheus Go client instead of growing a handwritten exposition format.

Sources: [PostgreSQL locking](https://www.postgresql.org/docs/current/explicit-locking.html),
[Prometheus Go instrumentation](https://prometheus.io/docs/guides/go-application/),
[Element Plus icons](https://element-plus.org/en-US/component/icon).

## Correctness and concurrency

PATCH distinguishes omitted department (preserve), null (clear), and string
(assign). Reuse OptionalString; test resulting-scope denial as well as clearing.
Session activity must not keep an exclusive lock through a request. Tenant-wide
write locks belong only to actual quota/state invariants, not every mutation.
Body upload and object transfer must occur outside long database transactions;
bounded short transactions authorize/reserve, then reauthorize and publish.
Download authorization/audit commits before transferring the response body.
Reservations, expiry and cleanup must preserve quota correctness and uncertain
publication safety across cancellation, crashes and permission revocation. Concurrent
reservations count toward storage quota; publication converts reserved usage to
committed usage exactly once. Collection must not delete active or ambiguously
committed publications. Final tenant, membership and role revalidation shares a
short transaction synchronized with revocation and publication.

Independent bounded worker lanes handle business jobs, mail, garbage collection,
and housekeeping with their own contexts. Close waits for all lanes. Slow export
must not starve mail or collection. Typed stage/code diagnostics are persisted
and logged without raw provider errors, credentials or token-bearing values.

## Extension boundary

Add a public `extension` package with validated registration for descriptors,
record operations and background job handlers. Custom actions remain constrained
identifiers and use the same scope/field/version/audit checks as builtins.
Callbacks receive typed, narrow contexts rather than private HTTP requests or
unrestricted runtime pools. Registry validation/freeze/deep-copy protections
apply to operations and jobs. Extract ticket workflow from generic handlers.
An asset approval operation and asset background job in a separate example
package must work without changes to core routes or job switches. Job callbacks
have at-least-once execution and must use the stable job ID for idempotency.

## API additions shared by backend and frontend

Resource responses add `capabilities: {actions: string[], write_fields: string[],
transitions: string[]}`. Compute these from the original row and matching grants,
before filtering response fields. `write_fields` contains only PATCH/update
fields, never a union of custom-operation grants. Each operation uses its own
action field checks, including status writes for transitions. UI buttons and
edit forms use this authority;
missing capabilities deny mutation. Custom operations use POST
`/resources/{module}/{id}/actions/{action}` with `{version,data}`. Module metadata
may include `operations: [{key,label,label_en}]` and `description_en`; callbacks
are never serialized. The seven existing CRUD actions retain their contracts.

Jobs add optional `error_code`, `error_stage` and safe `result` metadata. POST
`/jobs` accepts `{type,payload}` only for explicitly registered job types, under
the registered resource/action grant and record scope. Callbacks access records
only through authoritative tenant-scoped services; payload owner/department
values never establish authority. Typed nonsensitive result fields are explicitly
allowlisted and serialized results are bounded. All existing endpoints
remain valid. Extend File/Job/control-row capabilities where row actions apply.

Dashboard keeps `counts` and `recent_activity` for compatibility, and adds
`workload: {open_tickets?:number, urgent_tickets?:number, running_jobs?:number,
failed_jobs?:number, members?:number}`. Omit unauthorized categories rather than
invent zeros. Counts respect the same record predicates as list endpoints.
Open/urgent classification also requires the relevant status/priority field-read
predicate; title-only readers cannot infer hidden fields through aggregates.
Activity descriptions are localized and human-readable; UUIDs and raw JSON are
secondary details, not the primary presentation. Optional actor/record display
names require their corresponding data authorization; do not leak hidden fields.

## Product and visual design

Preserve teal identity, but use a quiet charcoal/teal palette, readable 14px
body text, 8px spacing rhythm, clear headings, compact navigation with real
icons, restrained cards and consistent table/form/button states. Workbench
prioritizes actual pending/urgent work and job failures; secondary counts remain
available without making dictionary/settings totals primary KPIs. No synthetic
charts. Data tables use localized status/priority badges, readable dates,
responsive column choices and consistent action placement.

Role editing starts with a resource-by-action permission matrix. A selected cell
opens scope/field/condition details. Preserve multiple grants for the same action;
never flatten OR semantics or broaden authority through a preset. Provide safe
read-only/own-work presets with an explicit resulting-grant preview and a
plain-language authorization summary. Validation is localized and contextual.
Advanced controls remain accessible, with keyboard labels and focus handling.

Split OrganizationView into user, department and invitation views with narrowly
shared components/composables. Separate resource dialogs/details/table behavior
and centralize operation/status/scope labels and record capabilities. Do not
replace duplication with an untyped mega-component. Improve auth/account/files/
jobs/audit screens to match the same visual and interaction system.

## Acceptance

First observe regressions failing: department clearing, per-row mutation UI,
blocked stream versus unrelated operations, worker isolation, custom operation
and job, redacted persisted diagnostics. Then fix and run existing race/integration
tests. Browser tests cover matrix/presets/preview, mixed row permissions,
department clearing, localization, responsive and dark states. Capture desktop,
mobile and role-editor artifacts; add reproducible visual baselines if stable.
Verify extension migration/activation on existing tenants, Docker startup,
dependency advisories, backup/MFA recovery and safe shutdown. Document observed
results, not absolute production maturity. No external publication is authorized.
