// Package assets demonstrates public module, operation and background-job registration.
package assets

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/ljn-forge/tenantforge/extension"
	"github.com/ljn-forge/tenantforge/migrations"
)

//go:embed migrations/*.sql
var sqlFiles embed.FS

// Register is called once by a downstream entrypoint before migrate or serve.
func Register() error {
	if err := extension.RegisterModule(extension.Module{
		Key: "assets", Label: "资产", LabelEN: "Assets",
		Description:   "管理组织资产，批准启用并生成授权范围内的盘点结果",
		DescriptionEN: "Tenant-owned assets with guarded approval and scoped inventory",
		Actions:       []string{"read", "create", "update", "delete", "assign", "export", "import", "approve"},
		Operations:    []extension.Operation{{Key: "approve", Label: "批准启用", LabelEN: "Approve activation", Apply: approve}},
		Fields: []extension.Field{
			{Key: "name", Label: "名称", LabelEN: "Name", Type: "string", Required: true},
			{Key: "serial", Label: "序列号", LabelEN: "Serial", Type: "string", Sensitive: true},
			{Key: "active", Label: "启用", LabelEN: "Active", Type: "boolean"},
		},
	}); err != nil {
		return err
	}
	if err := extension.RegisterJob(extension.JobDefinition{
		Type: "assets_inventory", Resource: "assets", Action: "read",
		Validate:     validateInventory,
		ResultFields: []extension.Field{{Key: "count", Label: "可见资产数", LabelEN: "Visible asset count", Type: "number"}},
		Run:          inventory,
	}); err != nil {
		return err
	}
	root, err := fs.Sub(sqlFiles, "migrations")
	if err != nil {
		return err
	}
	return migrations.RegisterFS(root)
}

// The core checks the approve action, original/resulting scopes, active-field
// authority and expected version; this callback supplies only the business patch.
func approve(c extension.OperationContext) (map[string]any, error) {
	if len(c.Data) != 0 {
		return nil, errors.New("approval accepts no input fields")
	}
	return map[string]any{"active": true}, nil
}

func validateInventory(raw json.RawMessage) error {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil || len(payload) != 0 {
		return errors.New("inventory accepts an empty object only")
	}
	return nil
}

// WalkRecords rechecks assets:read and current tenant/row authority. This task
// has no external side effects and is naturally safe to retry. A task with an
// external effect must persist/use c.ID() as its idempotency key.
func inventory(c extension.JobContext) (map[string]any, error) {
	count := 0
	if err := c.WalkRecords(func(extension.Record) error { count++; return nil }); err != nil {
		return nil, err
	}
	return map[string]any{"count": float64(count)}, nil
}
