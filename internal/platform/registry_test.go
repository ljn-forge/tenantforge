package platform

import "testing"

func TestRegistryValidatesAndFreezesExtensions(t *testing.T) {
	r := NewRegistry()
	module := Module{Key: "assets", Label: "资产", LabelEN: "Assets", Actions: []string{"read", "create", "update", "export"}, Fields: []Field{{Key: "name", Label: "名称", LabelEN: "Name", Type: "string", Required: true}}}
	if e := r.Register(module); e != nil {
		t.Fatal(e)
	}
	if e := r.Register(module); e == nil {
		t.Fatal("duplicate")
	}
	for _, key := range []string{"auth", "roles", "x;DROP TABLE users"} {
		m := module
		m.Key = key
		if e := r.Register(m); e == nil {
			t.Fatal("reserved/invalid key")
		}
	}
	m := module
	m.Key = "other"
	m.Fields = []Field{{Key: "tenant_id", Label: "bad", LabelEN: "Bad", Type: "string"}}
	if e := r.Register(m); e == nil {
		t.Fatal("reserved envelope")
	}
	r.Freeze()
	m = module
	m.Key = "more"
	if e := r.Register(m); e == nil {
		t.Fatal("mutable running registry")
	}
}
func TestRegistryDeepCopiesInputsAndViews(t *testing.T) {
	r := NewRegistry()
	m := Module{Key: "assets", Label: "资产", LabelEN: "Assets", Actions: []string{"read"}, Fields: []Field{{Key: "status", Label: "状态", LabelEN: "Status", Type: "enum", Options: []string{"active"}}}}
	if e := r.Register(m); e != nil {
		t.Fatal(e)
	}
	r.Freeze()
	m.Actions[0] = "delete"
	m.Fields[0].Options[0] = "mutated"
	view := r.Modules()
	asset := view[len(view)-1]
	if asset.Actions[0] != "read" || asset.Fields[0].Options[0] != "active" {
		t.Fatal("input alias mutated frozen registry")
	}
	asset.Actions[0] = "update"
	asset.Fields[0].Options[0] = "second"
	again := r.Modules()
	asset = again[len(again)-1]
	if asset.Actions[0] != "read" || asset.Fields[0].Options[0] != "active" {
		t.Fatal("view alias mutated frozen registry")
	}
}
