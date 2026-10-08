# Extension task usability and delivery performance

The authorized quality work continues from 482332e. Existing administrator credentials and tenant data are preserved. No external publication is in scope.

## Concrete gaps

- Registered extension tasks are callable through POST /jobs, but administrators cannot discover/configure/start them in the management interface.
- Safe typed task results currently appear only as raw JSON with technical keys.
- Global Element Plus registration loads the entire component library and stylesheet on the first page.

## Design and implementation order

1. Extend the existing public job registry with bilingual titles/descriptions and optional `InputFields []Field`. Nil input fields retain API-only compatibility; explicitly empty fields mean a launchable task with an empty payload. Clone nil distinctly from empty. Reject unsupported/duplicate/sensitive/readonly input fields at registration and validate declared payloads using existing field validation before invoking the custom validator. Existing 64 KiB bounds remain.
2. Add GET /api/v1/job-types, requiring jobs:create. Return only UI-declared definitions for which the current actor also has the registered resource/action grant. The response is an envelope array of `{type,label,label_en,description?,description_en?,resource,action,input_fields,result_fields}`. Discovery is permission-filtered, not an execution guarantee: submission and execution retain live authorization, record scopes, field controls and quotas.
3. Public job responses optionally include `definition: {label,label_en,description?,description_en?,result_fields}` for registered extension jobs. No callbacks, payloads or provider errors are exposed. Result values retain the existing authority fingerprint suppression.
4. Reuse FieldInput, existing field validation, epoch invalidation, ActionButton and request handling to build a task creation dialog. Clear stale tenant/task state, prevent duplicate submissions, preserve validation failures and refresh after success. Display typed results with bilingual field names outside technical JSON. Use registry titles in list/details.
5. Compare official on-demand component resolution against explicit imports; select a maintained, minimal build-time integration. Keep explicit service imports and their CSS. Measure the same production entry resources before/after; verify actual built browser behavior, themes, keyboard/mobile and no missing component/service styles.
6. Extend the public assets example, canonical API contract/OpenAPI and security/extension documentation. Add meaningful failing registry/auth/payload/UI tests before implementation; run database suites sequentially. Review independently, then merge and rebuild the running local app with existing data.

## Reuse decision

Task metadata/forms extend the existing descriptor/FieldInput/validation mechanism. A new JSON-schema renderer or workflow framework would duplicate the platform's types and authority handling. Load optimization follows Element Plus's official on-demand import guidance (https://element-plus.org/en-US/guide/quickstart.html), compared with explicit registration; dependency versions, license, maintenance, audit and bundle results must be recorded before acceptance.

## Acceptance

Unauthorized types are not discoverable; direct POST still enforces authority; malformed/unknown/required/enum/type-invalid payloads cannot enqueue; legacy API-only jobs retain behavior. UI launch and typed-result display work under an installed extension, with context changes unable to publish stale state. Built application retains login, role, resource, jobs, service dialogs and both themes. Production bundle measurements report actual compressed bytes, not only warning limits. Existing local account/password/data are preserved.

## Final verification

The typed extension task workflow and production loading budget passed. The final built browser run passed all four cases in 37.0s. A pending organization-switch reload race exposed by acceptance was reproduced with a stalled request, fixed with a user-bound validated selection intent, and independently reviewed; all 61 frontend tests passed. The final source was merged locally, rebuilt and started healthy on port 8090. All ten preserved-data comparisons passed; protected Owner grants were intentionally refreshed and audited. No external publication was performed.
