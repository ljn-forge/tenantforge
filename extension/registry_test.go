package extension

import (
	"encoding/json"
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
