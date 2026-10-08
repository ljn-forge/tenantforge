package extension

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCustomOperationsAreValidatedCopiedAndFrozen(t *testing.T) {
	r := NewRegistry()
	m := Module{Key: "assets", Label: "Assets", LabelEN: "Assets", Fields: []Field{{Key: "state", Label: "State", LabelEN: "State", Type: "enum", Options: []string{"pending", "approved"}}}, Actions: []string{"read", "approve"}, Operations: []Operation{{Key: "approve", Label: "Approve", LabelEN: "Approve", Apply: func(c OperationContext) (map[string]any, error) { return map[string]any{"state": "approved"}, nil }}}}
	if e := r.Register(m); e != nil {
		t.Fatal("validated custom operation rejected", e)
	}
	m.Operations[0].Label = "mutated"
	m.Fields[0].Options[0] = "mutated"
	view := r.Modules()
	if view[0].Operations[0].Label != "Approve" || view[0].Fields[0].Options[0] != "pending" {
		t.Fatal("registry aliases inputs")
	}
	view[0].Operations[0].Label = "mutated"
	if r.Modules()[0].Operations[0].Label != "Approve" {
		t.Fatal("registry aliases results")
	}
	b, e := json.Marshal(r.Modules())
	if e != nil || len(b) == 0 {
		t.Fatal("callbacks must never enter JSON")
	}
	r.Freeze()
	if e = r.Register(Module{}); e == nil {
		t.Fatal("late registration allowed")
	}
}

func TestJobRegistryValidatesDeepCopiesAndFreezes(t *testing.T) {
	r := NewRegistry()
	if e := r.Register(Module{Key: "assets", Label: "Assets", LabelEN: "Assets", Fields: []Field{{Key: "name", Label: "Name", LabelEN: "Name", Type: "string"}}, Actions: []string{"read"}}); e != nil {
		t.Fatal(e)
	}
	run := func(JobContext) (map[string]any, error) { return map[string]any{}, nil }
	for _, definition := range []JobDefinition{{Type: "export", Resource: "assets", Action: "read", Run: run}, {Type: "unknown", Resource: "assets", Action: "approve", Run: run}, {Type: "leak", Resource: "assets", Action: "read", Run: run, ResultFields: []Field{{Key: "token", Type: "string"}}}, {Type: "bad_enum", Resource: "assets", Action: "read", Run: run, ResultFields: []Field{{Key: "state", Type: "enum"}}}} {
		if r.RegisterJob(definition) == nil {
			t.Fatal("invalid job accepted")
		}
	}
	definition := JobDefinition{Type: "inventory", Resource: "assets", Action: "read", Run: run, ResultFields: []Field{{Key: "state", Type: "enum", Options: []string{"ready"}}}}
	if e := r.RegisterJob(definition); e != nil {
		t.Fatal(e)
	}
	definition.ResultFields[0].Options[0] = "mutated"
	stored, _ := r.Job("inventory")
	if stored.ResultFields[0].Options[0] != "ready" {
		t.Fatal("job input aliased")
	}
	stored.ResultFields[0].Options[0] = "mutated"
	again, _ := r.Job("inventory")
	if again.ResultFields[0].Options[0] != "ready" {
		t.Fatal("job view aliased")
	}
	if r.RegisterJob(definition) == nil {
		t.Fatal("duplicate accepted")
	}
	r.Freeze()
	if r.RegisterJob(JobDefinition{Type: "late", Resource: "assets", Action: "read", Run: run}) == nil {
		t.Fatal("late registration accepted")
	}
}

func TestFieldClonePreservesAbsentInputDeclaration(t *testing.T) {
	if cloneFields(nil) != nil {
		t.Fatal("nil input declaration became UI-ready empty fields")
	}
	if cloneFields([]Field{}) == nil {
		t.Fatal("explicit empty input declaration became API-only")
	}
}

func TestJobInputRegistryRejectsUnsafeOrInvalidDeclarations(t *testing.T) {
	valid := Field{Key: "category", Label: "类别", LabelEN: "Category", Type: "enum", Options: []string{"all", "open"}}
	cases := []struct {
		name   string
		fields []Field
	}{{"sensitive", []Field{{Key: "secret", Label: "Secret", LabelEN: "Secret", Type: "string", Sensitive: true}}}, {"readonly", []Field{{Key: "flag", Label: "Flag", LabelEN: "Flag", Type: "boolean", Readonly: true}}}, {"reserved", []Field{{Key: "owner_id", Label: "Owner", LabelEN: "Owner", Type: "string"}}}, {"unsupported", []Field{{Key: "nested", Label: "Nested", LabelEN: "Nested", Type: "object"}}}, {"untranslated", []Field{{Key: "name", Label: "Name", Type: "string"}}}, {"blank", []Field{{Key: "name", Label: " ", LabelEN: "Name", Type: "string"}}}, {"duplicate", []Field{valid, valid}}, {"enum_options", []Field{{Key: "state", Label: "State", LabelEN: "State", Type: "enum"}}}}
	tooMany := []Field{}
	for i := 0; i < 33; i++ {
		tooMany = append(tooMany, Field{Key: fmt.Sprintf("field_%d", i), Label: "Field", LabelEN: "Field", Type: "string"})
	}
	cases = append(cases, struct {
		name   string
		fields []Field
	}{"too_many", tooMany})
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			r := NewRegistry()
			if e := r.Register(Module{Key: "assets", Label: "Assets", LabelEN: "Assets", Fields: []Field{{Key: "name", Label: "Name", LabelEN: "Name", Type: "string"}}, Actions: []string{"read"}}); e != nil {
				t.Fatal(e)
			}
			if e := r.RegisterJob(JobDefinition{Type: "task", Resource: "assets", Action: "read", InputFields: item.fields, Run: func(JobContext) (map[string]any, error) { return nil, nil }}); e == nil {
				t.Fatal("unsafe or invalid input schema accepted")
			}
		})
	}
}
func TestJobInputDeclarationAndMetadataAreDeepCopied(t *testing.T) {
	r := NewRegistry()
	if e := r.Register(Module{Key: "assets", Label: "Assets", LabelEN: "Assets", Fields: []Field{{Key: "name", Label: "Name", LabelEN: "Name", Type: "string"}}, Actions: []string{"read"}}); e != nil {
		t.Fatal(e)
	}
	for _, definition := range []JobDefinition{{Type: "legacy", Resource: "assets", Action: "read", Run: func(JobContext) (map[string]any, error) { return nil, nil }}, {Type: "empty", Resource: "assets", Action: "read", InputFields: []Field{}, Run: func(JobContext) (map[string]any, error) { return nil, nil }}} {
		if e := r.RegisterJob(definition); e != nil {
			t.Fatal(e)
		}
	}
	d := JobDefinition{Type: "inventory", Resource: "assets", Action: "read", Label: "资产盘点", LabelEN: "Inventory", Description: "安全统计", DescriptionEN: "Safe count", InputFields: []Field{{Key: "state", Label: "状态", LabelEN: "State", Type: "enum", Options: []string{"all"}}}, Run: func(JobContext) (map[string]any, error) { return nil, nil }}
	if e := r.RegisterJob(d); e != nil {
		t.Fatal(e)
	}
	d.InputFields[0].Options[0] = "mutated"
	stored, _ := r.Job(d.Type)
	if stored.InputFields[0].Options[0] != "all" {
		t.Fatal("job input alias")
	}
	stored.InputFields[0].Options[0] = "mutated"
	views := r.Jobs()
	for _, view := range views {
		if view.Type == "legacy" && view.InputFields != nil {
			t.Fatal("legacy became UI-ready")
		}
		if view.Type == "empty" && view.InputFields == nil {
			t.Fatal("empty form became API-only")
		}
		if view.Type == "inventory" {
			if view.InputFields[0].Options[0] != "all" || view.Label != "资产盘点" || view.DescriptionEN != "Safe count" {
				t.Fatal("descriptor metadata changed")
			}
			view.InputFields[0].Options[0] = "mutated again"
		}
	}
	again, _ := r.Job("inventory")
	if again.InputFields[0].Options[0] != "all" {
		t.Fatal("job list aliases stored input schema")
	}
	r.Freeze()
	if r.RegisterJob(d) == nil {
		t.Fatal("late input registration accepted")
	}
}
