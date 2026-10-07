package platform

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestHTTPActionScopeFieldsRevocationAndDelegation(t *testing.T) {
	f := newFixture(t)
	gs := []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title", "status"}}, {Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"customer_email"}, UnmaskFields: []string{"customer_email"}}, {Resource: "tickets", Action: "update", Scope: "self", WriteFields: []string{"title"}}, {Resource: "members", Action: "update", Scope: "tenant"}, {Resource: "members", Action: "assign", Scope: "self"}}
	user, member, role := f.addMember("member@example.test", gs, "")
	own := asMap(f.call("POST", "/resources/tickets", map[string]any{"owner_id": user, "data": map[string]any{"title": "own", "customer_email": "own@example.test"}}, 200)["data"])
	other := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "other", "customer_email": "private@example.test"}}, 200)["data"])
	m := f.loginAs("member@example.test")
	data := asMap(m.call("GET", "/resources/tickets/"+own["id"].(string), nil, 200)["data"])
	if asMap(data["data"])["customer_email"] != "own@example.test" {
		t.Fatal("own field hidden")
	}
	data = asMap(m.call("GET", "/resources/tickets/"+other["id"].(string), nil, 200)["data"])
	if _, ok := asMap(data["data"])["customer_email"]; ok {
		t.Fatal("foreign sensitive field leaked")
	}
	m.call("PATCH", "/resources/tickets/"+other["id"].(string), map[string]any{"version": 1, "data": map[string]any{"title": "forbidden"}}, 403)
	m.call("PATCH", "/resources/tickets/"+own["id"].(string), map[string]any{"version": 1, "data": map[string]any{"customer_email": "no"}}, 403)
	m.call("PATCH", "/resources/tickets/"+own["id"].(string), map[string]any{"version": 1, "data": map[string]any{"title": "allowed"}}, 200)

	_ = member
	_ = role
}
func TestHTTPControlTargetAssignmentAndRoleRevocation(t *testing.T) {
	f := newFixture(t)
	gs := []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"*"}}, {Resource: "members", Action: "update", Scope: "tenant"}, {Resource: "members", Action: "assign", Scope: "self"}}
	_, _, role := f.addMember("manager@example.test", gs, "")
	_, target, _ := f.addMember("target@example.test", []Grant{}, "")
	m := f.loginAs("manager@example.test")
	m.call("PATCH", "/members/"+target, map[string]any{"role_ids": []string{}}, 403)
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "Revoked", "description": "", "grants": []Grant{}}, 200)
	m.call("GET", "/resources/tickets", nil, 403)
}
func TestHTTPScopesConditionsSQLPaginationAndCounts(t *testing.T) {
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B", "parent_id": a}, 200)["data"])["id"].(string)
	c := asMap(f.call("POST", "/departments", map[string]any{"name": "C"}, 200)["data"])["id"].(string)
	for _, dep := range []string{a, b, c} {
		for _, status := range []string{"open", "in_progress"} {
			r := asMap(f.call("POST", "/resources/tickets", map[string]any{"department_id": dep, "data": map[string]any{"title": dep}}, 200)["data"])
			if status != "open" {
				f.call("POST", "/resources/tickets/"+r["id"].(string)+"/transition", map[string]any{"version": 1, "status": status}, 200)
			}
		}
	}
	for i, tc := range []struct {
		scope string
		ids   []string
		want  int
	}{{"department", nil, 1}, {"department_tree", nil, 2}, {"selected_departments", []string{c}, 1}, {"tenant", nil, 3}, {"self", nil, 0}} {
		email := fmt.Sprintf("scope%d@example.test", i)
		f.addMember(email, []Grant{{Resource: "tickets", Action: "read", Scope: tc.scope, DepartmentIDs: tc.ids, ReadFields: []string{"title"}, Conditions: []Condition{{Field: "status", Op: "eq", Value: "open"}}}}, a)
		m := f.loginAs(email)
		page := m.call("GET", "/resources/tickets?page_size=1", nil, 200)
		if int(asMap(page["meta"])["total"].(float64)) != tc.want {
			t.Fatalf("%s count mismatch", tc.scope)
		}
		dashboard := asMap(m.call("GET", "/dashboard", nil, 200)["data"])
		if int(asMap(dashboard["counts"])["tickets"].(float64)) != tc.want {
			t.Fatal("dashboard scope mismatch")
		}
	}
}
func TestHTTPFinalOwnerDepartmentCycleAndNullClear(t *testing.T) {
	f := newFixture(t)
	list := f.call("GET", "/members", nil, 200)["data"].([]any)
	id := asMap(list[0])["id"].(string)
	f.call("PATCH", "/members/"+id, map[string]any{"role_ids": []string{}}, 422)
	f.call("PATCH", "/members/"+id, map[string]any{"status": "suspended"}, 422)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B", "parent_id": a}, 200)["data"])["id"].(string)
	f.call("PATCH", "/departments/"+a, map[string]any{"parent_id": b}, 422)
	r := asMap(f.call("PATCH", "/departments/"+b, map[string]any{"parent_id": nil}, 200)["data"])
	if r["parent_id"] != nil {
		t.Fatal("parent null did not clear")
	}
	f.call("DELETE", "/departments/"+a, nil, 200)
}
func TestRLSAndTenantReferences(t *testing.T) {
	f := newFixture(t)
	r := asMap(f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "hidden without scope"}}, 200)["data"])
	var n int
	if e := f.app.DB.QueryRow(context.Background(), "SELECT count(*) FROM resources").Scan(&n); e != nil || n != 0 {
		t.Fatal("RLS missing tenant context failed")
	}
	second := asMap(f.call("POST", "/platform/tenants", map[string]any{"name": "Second", "slug": "second", "owner_email": "owner@example.test"}, 200)["data"])["id"].(string)
	f.tenant = second
	f.call("GET", "/resources/tickets/"+r["id"].(string), nil, 404)
	f.call("POST", "/resources/tickets", map[string]any{"owner_id": uuid.NewString(), "data": map[string]any{"title": "wrong ref"}}, 422)
}
func TestRoleEditCannotExpandCurrentRecipients(t *testing.T) {
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B"}, 200)["data"])["id"].(string)
	target, _, role := f.addMember("recipient@example.test", []Grant{}, b)
	manager, _, _ := f.addMember("deptmanager@example.test", []Grant{{Resource: "roles", Action: "update", Scope: "tenant"}, {Resource: "tickets", Action: "read", Scope: "department", ReadFields: []string{"title"}}}, a)
	_ = target
	_ = manager
	m := f.loginAs("deptmanager@example.test")
	m.call("PATCH", "/roles/"+role, map[string]any{"name": "unsafe dept grant", "description": "", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "department", ReadFields: []string{"title"}}}}, 403)
}
func TestTransferRequiresResourceUpdateAndAssignment(t *testing.T) {
	f := newFixture(t)
	source, member, _ := f.addMember("source@example.test", []Grant{}, "")
	target, _, _ := f.addMember("recipient@example.test", []Grant{}, "")
	f.call("POST", "/resources/tickets", map[string]any{"owner_id": source, "data": map[string]any{"title": "transfer"}}, 200)
	f.addMember("assigner@example.test", []Grant{{Resource: "members", Action: "assign", Scope: "tenant"}, {Resource: "tickets", Action: "assign", Scope: "tenant"}}, "")
	m := f.loginAs("assigner@example.test")
	m.call("POST", "/members/"+member+"/transfer", map[string]any{"owner_id": target}, 403)
}
func TestDepartmentParentMoveChecksResultingScope(t *testing.T) {
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "A"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "B", "parent_id": a}, 200)["data"])["id"].(string)
	c := asMap(f.call("POST", "/departments", map[string]any{"name": "C"}, 200)["data"])["id"].(string)
	f.addMember("dept-editor@example.test", []Grant{{Resource: "departments", Action: "update", Scope: "department_tree"}}, a)
	m := f.loginAs("dept-editor@example.test")
	m.call("PATCH", "/departments/"+b, map[string]any{"parent_id": c}, 403)
	m.call("PATCH", "/departments/"+b, map[string]any{"parent_id": nil}, 403)
}
