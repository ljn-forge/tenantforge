// Package assets demonstrates a descriptor extension without changing core authorization.
package assets

import (
	"embed"
	"io/fs"

	"github.com/ljn-forge/tenantforge/internal/platform"
	"github.com/ljn-forge/tenantforge/migrations"
)

//go:embed migrations/*.sql
var sqlFiles embed.FS

// Register is called once by a downstream entrypoint before migrate or serve.
func Register() error {
	if err := platform.RegisterModule(platform.Module{
		Key: "assets", Label: "资产", LabelEN: "Assets",
		Description: "Tenant-owned asset inventory",
		Actions:     []string{"read", "create", "update", "delete", "assign", "export", "import"},
		Fields: []platform.Field{
			{Key: "name", Label: "名称", LabelEN: "Name", Type: "string", Required: true},
			{Key: "serial", Label: "序列号", LabelEN: "Serial", Type: "string", Sensitive: true},
			{Key: "active", Label: "启用", LabelEN: "Active", Type: "boolean"},
		},
	}); err != nil {
		return err
	}
	root, err := fs.Sub(sqlFiles, "migrations")
	if err != nil {
		return err
	}
	return migrations.RegisterFS(root)
}
