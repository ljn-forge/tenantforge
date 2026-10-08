# Extending business modules

Register modules, record operations and background jobs through the public
`github.com/ljn-forge/tenantforge/extension` package before startup. The runtime
freezes a validated deep copy of registration data. Duplicate/reserved keys,
unknown actions, invalid fields and late registration are rejected.

The compiling `examples/assets` package adds assets, an approval operation and a
scoped inventory task without modifying core routes, authorization or worker
switches. Its entrypoint imports public packages only and can be copied into a
separate Go module:

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()
if err := assets.Register(); err != nil {
    log.Fatal(err)
}
if err := runtime.Run(ctx, os.Args[1:]); err != nil {
    log.Fatal(err)
}
```

Here `runtime` is `github.com/ljn-forge/tenantforge/extension/runtime`; `Run` accepts
`serve`, `migrate`, `check-config` and `version` with the standard environment
configuration. It exposes no runtime database pool. From this checkout:

```sh
python3 scripts/run-env.py --owner go run ./examples/assets/cmd migrate
python3 scripts/run-env.py go run ./examples/assets/cmd serve
```

Use the same registrations in migration and service binaries. The stock binary
does not enable example modules automatically. Migration activates registered
module grants for protected tenant Owner roles, including existing tenants, and
audits the activation. Custom roles gain no permissions automatically.

## Descriptors and record operations

```go
extension.RegisterModule(extension.Module{
    Key: "assets", Label: "资产", LabelEN: "Assets",
    Actions: []string{"read", "create", "update", "delete", "approve"},
    Fields: []extension.Field{
        {Key: "name", Label: "名称", LabelEN: "Name", Type: "string", Required: true},
        {Key: "active", Label: "启用", LabelEN: "Active", Type: "boolean"},
    },
    Operations: []extension.Operation{{
        Key: "approve", Label: "批准启用", LabelEN: "Approve activation",
        Apply: func(c extension.OperationContext) (map[string]any, error) {
            return map[string]any{"active": true}, nil
        },
    }},
})
```

The registration integrates `/resources/assets`, descriptors, permission matrix,
typed conditions, row/field guards, generated business pages and audit. Declare
`export` and `import` to enable the existing CSV pipeline.

POST `/resources/assets/{id}/actions/approve` with `{version,data:{}}`. The core
loads and authorizes the original tenant record, supplies a filtered typed
`OperationContext`, then validates the returned field patch, resulting scope,
optimistic version and audit in one short transaction. The operation requires
its own `approve` grant and writable `active` field; an update grant cannot
substitute for it. Returning a patch does not grant extra fields or ownership.
The optional `Operation.Action` permission alias defaults to `Key` and is not
serialized. Builtin ticket transitions reuse update authority while retaining
their dedicated status-input endpoint and server transition capabilities.

Callbacks contain bounded business logic, with no raw HTTP request or database
pool. Put external I/O or expensive work in a background job. A returned
`extension.Error` carries a safe machine-readable diagnostic code and retry
classification; provider error text never becomes an API message.

## Background jobs

```go
extension.RegisterJob(extension.JobDefinition{
    Type: "assets_inventory", Resource: "assets", Action: "read",
    Label: "资产盘点", LabelEN: "Asset inventory",
    Description: "统计当前权限范围内可见的资产，不修改资产数据。",
    DescriptionEN: "Count assets visible under your current access without changing records.",
    InputFields: []extension.Field{},
    ResultFields: []extension.Field{{
        Key: "count", Label: "可见资产数", LabelEN: "Visible asset count", Type: "number",
    }},
    Run: func(c extension.JobContext) (map[string]any, error) {
        count := 0
        err := c.WalkRecords(func(extension.Record) error { count++; return nil })
        return map[string]any{"count": float64(count)}, err
    },
})
```

POST `/jobs` with `{type:"assets_inventory",payload:{}}`. The core requires
`jobs:create` **and** the registered resource/action grant, a known type and a JSON
object of at most 64 KiB. Submission and execution retain scoped record checks,
live membership/permission validation and tenant quotas.

Declaring `InputFields` enables the management page's task creation form. An
explicitly empty slice, as above, declares a task accepting `{}`. A nil slice
(the zero value) retains existing API-only jobs and their custom `Validate`
contract. Nonempty input fields reuse `extension.Field`'s bilingual labels,
required values, enum options and supported types. The core rejects unknown
keys and validates declared values before invoking `Validate`; that callback
can add business restrictions. Declare at most 32 input fields, without sensitive
or readonly fields. Metadata cannot establish authority over a target record.

GET `/job-types` requires `jobs:create` and returns only UI-declared jobs whose
registered resource/action the actor holds in the current organization. The jobs
page generates a form from this catalog and posts the bounded payload; no custom
frontend route is needed. Catalog visibility does not guarantee execution if
permissions, scoped records or quotas change. The job response includes safe
`definition` labels and result field descriptors, so the details page displays
typed results with readable names. Stored input, callbacks, provider error text
and authority hashes are never returned.

Adding `jobs:create` is explicit for custom roles. The standard migration command
refreshes the protected Owner role with current platform actions and audits that
change; custom role grants are preserved. Run migration with the same registry
before serving an upgraded binary.

`JobContext` exposes `Context`, stable `ID`, `TenantID`, `ActorID`, `Payload`,
`WalkRecords` and `UpdateRecord`. Record services recheck current membership,
lease, tenant status, action, original/resulting scopes and fields; they resolve
ownership from authoritative tenant records. Owner/department values in arbitrary
payloads never establish authority. `UpdateRecord` uses an expected version and
core-managed audit. No callback gets unrestricted SQL access.

Handlers run outside long database transactions, use durable leases and support
retry/cancellation. Execution is at least once: use the stable job ID as an
idempotency key for any external effect. Cancellation stops publication of a
successful result; it cannot reverse an external effect already accepted by a
provider. Membership and grants are revalidated before completion. Generation authority is
fingerprinted; narrowed scope or changed fields stops successful publication, and
result metadata is withheld when current authority no longer matches. Export
downloads regenerate under current scopes and fields.

Results allow at most 32 explicitly registered, typed, nonsensitive fields and
16 KiB serialized metadata. Registered application code remains responsible for
the meaning of its result values. Omit credentials and sensitive business values;
use core-managed protected files for larger data. Jobs expose safe error stage
and code for diagnosis.

## Embedded migrations

Generic records use the existing JSONB resources table and need no migration.
Add one only for an actual schema/index requirement. The asset example embeds a
partial index migration using `migrations.RegisterFS(fs.FS)`.

Embed globally unique numbered Goose SQL files and register before both migrate
and serve. Registration validates filenames and duplicate versions; startup
checks every registered migration. Use `-- +goose Up` and
`StatementBegin`/`StatementEnd` for functions. Never edit an applied migration;
add a later migration and test fresh installation and upgrade. Apply migrations
with the owner connection only.

`examples/assets/assets_integration_test.go` uses the separate
`tenantforge_extension` database to verify migration idempotency, CRUD, approval,
audit, task discovery and persisted job completion. It never resets the business
database. Exercise the generated UI against production chunks with:

```sh
python3 scripts/browser-check.py --built --assets
```

This command uses only `tenantforge_browser_assets` and `.data/browser_assets`,
keeps the normal browser/recovery fixture separate, and resets its own fixtures
after checking for active connections. It starts the public example entrypoint,
not the stock binary. Vite preview is used only for local acceptance; deployment
continues to serve the built assets through the application/container.
