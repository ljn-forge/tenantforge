package platform

import "testing"

func TestOperationScopesAndRowFields(t *testing.T) {
	p := Principal{ID: "alice", TenantID: "t1", DepartmentID: "d1", Departments: map[string]string{"d2": "d1", "d3": "d2"}, Grants: []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}, {Resource: "tickets", Action: "update", Scope: "self", WriteFields: []string{"title"}}, {Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"customer_email"}, UnmaskFields: []string{"customer_email"}}}}
	own := Record{TenantID: "t1", OwnerID: "alice", DepartmentID: "d3", Data: map[string]any{"title": "A", "customer_email": "private@example.com"}}
	other := own
	other.OwnerID = "bob"
	if !Allowed(p, "tickets", "read", other) || Allowed(p, "tickets", "update", other) {
		t.Fatal("read authority leaked into update")
	}
	if Allowed(p, "tickets", "read", Record{TenantID: "t2", OwnerID: "alice"}) {
		t.Fatal("cross tenant")
	}
	if _, ok := FilterData(p, "tickets", "read", other)["customer_email"]; ok {
		t.Fatal("row unrelated grant leaked sensitive field")
	}
	if FilterData(p, "tickets", "read", own)["customer_email"] != "private@example.com" {
		t.Fatal("own unmask denied")
	}
	if CanWrite(p, "tickets", "update", own, map[string]any{"customer_email": "x"}) {
		t.Fatal("unauthorized sensitive write")
	}
	if !CanWrite(p, "tickets", "update", own, map[string]any{"title": "x"}) {
		t.Fatal("own permitted write")
	}
}
func TestScopeVariantsConditionsAndDelegation(t *testing.T) {
	p := Principal{ID: "a", TenantID: "t", DepartmentID: "d", Departments: map[string]string{"child": "d", "deep": "child"}}
	r := Record{TenantID: "t", OwnerID: "b", DepartmentID: "deep", Data: map[string]any{"status": "open"}}
	for _, s := range []string{"department_tree", "selected_departments", "tenant"} {
		p.Grants = []Grant{{Resource: "tickets", Action: "read", Scope: s, DepartmentIDs: []string{"deep"}, ReadFields: []string{"*"}, Conditions: []Condition{{Field: "status", Op: "eq", Value: "open"}}}}
		if !Allowed(p, "tickets", "read", r) {
			t.Fatal(s)
		}
		r.Data["status"] = "closed"
		if Allowed(p, "tickets", "read", r) {
			t.Fatal("condition ignored")
		}
		r.Data["status"] = "open"
	}
	p.Grants = []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}
	if CanDelegate(p, []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"*"}}}) {
		t.Fatal("delegation escalated")
	}
	if !CanDelegate(p, p.Grants) {
		t.Fatal("identical grant denied")
	}
	if err := ValidateGrants([]Grant{{Resource: "tickets", Action: "read", Scope: "tenant", Conditions: []Condition{{Field: "password", Op: "eq", Value: "x"}}}}); err == nil {
		t.Fatal("unregistered condition allowed")
	}
}
func TestDescriptorValidation(t *testing.T) {
	if _, err := ValidateData("tickets", map[string]any{"title": "hello", "unknown": "x"}, true); err == nil {
		t.Fatal("unknown field")
	}
	if _, err := ValidateData("tickets", map[string]any{"title": "hello", "status": "bogus"}, true); err == nil {
		t.Fatal("enum")
	}
	if _, err := ValidateData("tickets", map[string]any{"title": "hello"}, true); err != nil {
		t.Fatal(err)
	}
}
func TestMemberRoleAssignmentRequiresTargetScope(t *testing.T) {
	p := Principal{ID: "a", TenantID: "t", Grants: []Grant{{Resource: "members", Action: "update", Scope: "tenant"}, {Resource: "members", Action: "assign", Scope: "self"}}}
	r := Record{TenantID: "t", OwnerID: "b"}
	if CanAssignMember(p, r, r) {
		t.Fatal("self assign leaked to another member")
	}
	if !CanAssignMember(p, Record{TenantID: "t", OwnerID: "a"}, Record{TenantID: "t", OwnerID: "a"}) {
		t.Fatal("own assignment denied")
	}
}
func TestTicketsMustStartOpen(t *testing.T) {
	if _, e := ValidateData("tickets", map[string]any{"title": "x", "status": "closed"}, true); e == nil {
		t.Fatal("closed ticket created")
	}
}
func TestDelegationEvaluatesRecipientDepartment(t *testing.T) {
	p := Principal{ID: "manager", TenantID: "t", DepartmentID: "a", Departments: map[string]string{"child": "a", "else": "b"}, Grants: []Grant{{Resource: "tickets", Action: "read", Scope: "department", ReadFields: []string{"title"}}}}
	g := p.Grants
	if CanDelegateTo(p, g, "recipient", "b") {
		t.Fatal("symbolic department grant escaped caller department")
	}
	if !CanDelegateTo(p, g, "recipient", "a") {
		t.Fatal("same department grant denied")
	}
	p.Grants = []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}
	if CanDelegateTo(p, p.Grants, "other", "a") {
		t.Fatal("self grant escaped caller owned rows")
	}
	p.Grants = []Grant{{Resource: "tickets", Action: "read", Scope: "department_tree", ReadFields: []string{"title"}}}
	if !CanDelegateTo(p, p.Grants, "other", "child") || CanDelegateTo(p, p.Grants, "other", "b") {
		t.Fatal("tree delegation containment")
	}
}
