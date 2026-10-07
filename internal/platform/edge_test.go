package platform

import (
	"encoding/json"
	"testing"
)

func TestOptionalNullClearsAndOmittedPreserves(t *testing.T) {
	var in struct {
		Parent OptionalString `json:"parent_id"`
	}
	if e := json.Unmarshal([]byte(`{}`), &in); e != nil || in.Parent.Present {
		t.Fatal("omitted field")
	}
	if e := json.Unmarshal([]byte(`{"parent_id":null}`), &in); e != nil || !in.Parent.Present || in.Parent.Value != "" {
		t.Fatal("explicit null must clear")
	}
}
func TestGrantWildcardUnmaskRejectedAndSensitiveMaskUnion(t *testing.T) {
	if ValidateGrants([]Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"*"}, UnmaskFields: []string{"*"}}}) == nil {
		t.Fatal("wildcard sensitive unmask")
	}
	p := Principal{ID: "a", TenantID: "t", Grants: []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"customer_email"}, UnmaskFields: []string{"customer_email"}}, {Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"customer_email"}}}}
	r := Record{TenantID: "t", Data: map[string]any{"customer_email": "a@b.test"}}
	if FilterData(p, "tickets", "read", r)["customer_email"] != "a@b.test" {
		t.Fatal("unmask union")
	}
}
