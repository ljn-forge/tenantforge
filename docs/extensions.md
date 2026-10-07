# Extending modules

TenantForge uses validated descriptors for resources and fields. Register a module before starting the application; startup freezes the registry so authorization cannot silently change while requests are running. Duplicate/reserved keys, invalid fields/types/options/actions are rejected.

The compiling example in `examples/assets` registers an asset module and an embedded index migration. Import it in your downstream entrypoint and call `assets.Register()` before processing either migrate or serve. The stock binary does not automatically enable example modules. Inside a checkout of this module, the exported internal platform API looks like:

```go
if err := platform.RegisterModule(platform.Module{
    Key: "assets", Label: "资产", LabelEN: "Assets",
    Description: "Tenant-owned asset inventory",
    Actions: []string{"read", "create", "update", "delete", "assign", "export", "import"},
    Fields: []platform.Field{
        {Key: "name", Label: "名称", LabelEN: "Name", Type: "string", Required: true},
        {Key: "serial", Label: "序列号", LabelEN: "Serial", Type: "string", Sensitive: true},
        {Key: "active", Label: "启用", LabelEN: "Active", Type: "boolean"},
    },
}); err != nil {
    log.Fatal(err)
}
app, err := platform.New(ctx, config)
```

This integrates `/resources/assets`, the permission editor, generated resource screen, typed conditions, row/field guards, CSV import/export, dashboard and audit. The separate authorized migration command activates registered modules for protected tenant Owner roles, including existing tenants, and audits that change. Owners can then delegate explicit module grants to other roles. Custom roles do not gain permissions automatically. Run migrate with the same extension registrations as serve, even when a generic module adds no SQL schema migration.

Generic records use the existing JSONB resources table and need no schema migration. Add a migration only for a real schema/index requirement; reuse the authorization predicates and tenant references for a separate business table.

## Embedded migrations

Additional migration sources register before migration/startup using `migrations.RegisterFS(fs.FS)`. Embed a directory containing globally unique numbered Goose SQL files. Startup checks the registered migration target; keep the same sources in the migrate and serve binaries. Registration validates filenames and duplicate versions. Version numbers must be unique across the core and extension sources.

Use Goose's `-- +goose Up` syntax and `StatementBegin`/`StatementEnd` for functions. Never edit an applied migration; add a later migration instead. Test the extension against both a fresh database and a restored existing database. Apply migrations only with the owner connection.

## Business workflows

Descriptors provide bounded CRUD, fields and generic operations. A specialized workflow should add a thin handler that invokes the same principal/scope/field checks, validates its state transition and optimistic version, and commits its audit in the same transaction. Do not embed tenant-specific logic in identity or bypass authorization via raw SQL. Tickets provide the existing workflow example.

The package intentionally remains internal to the product source tree: downstream applications fork/extend the entrypoint rather than depend on a promised stable external Go SDK. REST descriptors are the frontend extension boundary.
