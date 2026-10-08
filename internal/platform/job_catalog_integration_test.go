package platform

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ljn-forge/tenantforge/extension"
)

func registerCatalogLegacyJob(t *testing.T) {
	t.Helper()
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	if e := moduleRegistry.RegisterJob(extension.JobDefinition{Type: "catalog_legacy", Resource: "tickets", Action: "read", Run: func(extension.JobContext) (map[string]any, error) { return map[string]any{}, nil }}); e != nil {
		t.Fatal(e)
	}
}
func TestCustomJobSubmissionRequiresJobsCreate(t *testing.T) {
	registerCatalogLegacyJob(t)
	f := newFixture(t)
	f.addMember("domain-only@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}, "")
	actor := f.loginAs("domain-only@example.test")
	actor.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{}}, 403)
}

func TestCustomJobSubmissionEnforcesJobsCreateScope(t *testing.T) {
	registerCatalogLegacyJob(t)
	f := newFixture(t)
	a := asMap(f.call("POST", "/departments", map[string]any{"name": "Creator department"}, 200)["data"])["id"].(string)
	b := asMap(f.call("POST", "/departments", map[string]any{"name": "Other department"}, 200)["data"])["id"].(string)
	_, _, role := f.addMember("scoped-creator@example.test", []Grant{{Resource: "jobs", Action: "create", Scope: "selected_departments", DepartmentIDs: []string{b}}, {Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}, a)
	actor := f.loginAs("scoped-creator@example.test")
	actor.call("GET", "/job-types", nil, 403)
	actor.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{}}, 403)
	var count int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM jobs WHERE tenant_id=$1", f.tenant).Scan(&count); e != nil || count != 0 {
		t.Fatal("out-of-scope creation persisted a job")
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "Creator scope", "grants": []Grant{{Resource: "jobs", Action: "create", Scope: "selected_departments", DepartmentIDs: []string{a}}, {Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}}, 200)
	actor.call("GET", "/job-types", nil, 200)
	actor.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{}}, 200)
}
func TestJobCatalogOwnerReturnsEmptyForAPIOnlyDefinitions(t *testing.T) {
	registerCatalogLegacyJob(t)
	f := newFixture(t)
	if got := f.call("GET", "/job-types", nil, 200)["data"].([]any); len(got) != 0 {
		t.Fatal("API-only task appeared in the catalog")
	}
}

func TestJobCatalogUpgradeActivatesOwnerWithoutChangingCustomRoles(t *testing.T) {
	registerCatalogLegacyJob(t)
	f := newFixture(t)
	custom := asMap(f.call("POST", "/roles", map[string]any{"name": "Existing custom role", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}}, 200)["data"])
	ctx := context.Background()
	var customGrants, passwordHash string
	if e := f.admin.QueryRow(ctx, "SELECT grants::text FROM roles WHERE id=$1", custom["id"]).Scan(&customGrants); e != nil {
		t.Fatal(e)
	}
	if e := f.admin.QueryRow(ctx, "SELECT password_hash FROM users WHERE id=$1", f.user).Scan(&passwordHash); e != nil {
		t.Fatal(e)
	}
	f.sql("UPDATE roles SET grants=(SELECT jsonb_agg(g) FROM jsonb_array_elements(grants) g WHERE NOT(g->>'resource'='jobs' AND g->>'action'='create')) WHERE tenant_id=$1 AND builtin AND name='Owner'", f.tenant)
	f.call("GET", "/job-types", nil, 403)
	f.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{}}, 403)
	owner := f.admin.Config().ConnConfig.Copy()
	if e := MigrateConfig(ctx, owner); e != nil {
		t.Fatal(e)
	}
	f.call("GET", "/job-types", nil, 200)
	f.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{}}, 200)
	var afterGrants, afterHash string
	if e := f.admin.QueryRow(ctx, "SELECT grants::text FROM roles WHERE id=$1", custom["id"]).Scan(&afterGrants); e != nil {
		t.Fatal(e)
	}
	if e := f.admin.QueryRow(ctx, "SELECT password_hash FROM users WHERE id=$1", f.user).Scan(&afterHash); e != nil {
		t.Fatal(e)
	}
	if customGrants != afterGrants || passwordHash != afterHash {
		t.Fatal("activation changed a custom role or existing administrator credentials")
	}
	var first, second int
	if e := f.admin.QueryRow(ctx, "SELECT count(*) FROM audit WHERE tenant_id=$1 AND action='module_activation'", f.tenant).Scan(&first); e != nil {
		t.Fatal(e)
	}
	if e := MigrateConfig(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e := f.admin.QueryRow(ctx, "SELECT count(*) FROM audit WHERE tenant_id=$1 AND action='module_activation'", f.tenant).Scan(&second); e != nil || first != 1 || second != first {
		t.Fatal("owner activation must be audited exactly once")
	}
}

func registerCatalogJobs(t *testing.T, validator func(extension.JobContext), custom func(json.RawMessage) error) {
	t.Helper()
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	run := func(c extension.JobContext) (map[string]any, error) {
		if validator != nil {
			validator(c)
		}
		return map[string]any{"count": float64(1)}, nil
	}
	definitions := []extension.JobDefinition{
		{Type: "catalog_legacy", Resource: "tickets", Action: "read", Run: func(extension.JobContext) (map[string]any, error) { return map[string]any{}, nil }},
		{Type: "catalog_empty", Resource: "tickets", Action: "read", InputFields: []Field{}, Run: func(extension.JobContext) (map[string]any, error) { return map[string]any{}, nil }},
		{Type: "catalog_typed", Resource: "tickets", Action: "read", Label: "工单统计", LabelEN: "Ticket statistics", Description: "安全汇总", DescriptionEN: "Safe aggregate", InputFields: []Field{{Key: "category", Label: "类别", LabelEN: "Category", Type: "enum", Required: true, Options: []string{"all", "open"}}, {Key: "from", Label: "起始日期", LabelEN: "From", Type: "date"}, {Key: "count", Label: "数量", LabelEN: "Count", Type: "number"}, {Key: "enabled", Label: "启用", LabelEN: "Enabled", Type: "boolean"}, {Key: "note", Label: "说明", LabelEN: "Note", Type: "string"}}, ResultFields: []Field{{Key: "count", Label: "数量", LabelEN: "Count", Type: "number"}}, Validate: custom, Run: run},
		{Type: "catalog_settings", Resource: "settings", Action: "read", InputFields: []Field{}, Run: func(extension.JobContext) (map[string]any, error) { return map[string]any{}, nil }},
	}
	for _, d := range definitions {
		if e := moduleRegistry.RegisterJob(d); e != nil {
			t.Fatal(e)
		}
	}
}
func TestJobDeclaredInputsRejectBeforeCustomValidation(t *testing.T) {
	var calls atomic.Int32
	registerCatalogJobs(t, nil, func(json.RawMessage) error { calls.Add(1); return nil })
	f := newFixture(t)
	for _, payload := range []any{map[string]any{}, map[string]any{"category": "unknown"}, map[string]any{"category": 7}, map[string]any{"category": "all", "unknown": "value"}, map[string]any{"category": "all", "from": "invalid"}, map[string]any{"category": "all", "count": "wrong"}, map[string]any{"category": "all", "enabled": "true"}, map[string]any{"category": "all", "note": strings.Repeat("x", 20001)}, []any{}, nil} {
		f.call("POST", "/jobs", map[string]any{"type": "catalog_typed", "payload": payload}, 422)
	}
	if calls.Load() != 0 {
		t.Fatal("custom validator received structurally invalid payload")
	}
	f.call("POST", "/jobs", map[string]any{"type": "catalog_empty", "payload": map[string]any{"unexpected": true}}, 422)
	f.call("POST", "/jobs", map[string]any{"type": "catalog_typed", "payload": map[string]any{"category": "all", "count": 0, "enabled": false, "from": "2026-10-08"}}, 200)
	if calls.Load() != 1 {
		t.Fatal("valid declared input did not reach custom validator")
	}
	f.call("POST", "/jobs", map[string]any{"type": "catalog_legacy", "payload": map[string]any{"legacy_arbitrary": "compatible"}}, 200)
	f.call("POST", "/jobs", map[string]any{"type": "catalog_empty", "payload": map[string]any{}}, 200)
}
func TestJobCatalogFiltersCreateAndDomainAuthority(t *testing.T) {
	registerCatalogJobs(t, nil, nil)
	f := newFixture(t)
	all := f.call("GET", "/job-types", nil, 200)["data"].([]any)
	if len(all) != 3 {
		t.Fatal("catalog included API-only job or omitted UI types")
	}
	for _, value := range all {
		row := asMap(value)
		if row["type"] == "catalog_legacy" {
			t.Fatal("API-only type discoverable")
		}
		if row["input_fields"] == nil || row["result_fields"] == nil {
			t.Fatal("catalog fields must be arrays")
		}
		for _, key := range []string{"Validate", "Run", "payload", "authority_hash"} {
			if _, found := row[key]; found {
				t.Fatal("private registry data exposed")
			}
		}
	}
	f.addMember("catalog-creator@example.test", []Grant{{Resource: "jobs", Action: "create", Scope: "self"}, {Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}, "")
	creator := f.loginAs("catalog-creator@example.test")
	types := creator.call("GET", "/job-types", nil, 200)["data"].([]any)
	if len(types) != 2 {
		t.Fatal("catalog ignored registered domain authority")
	}
	creator.call("POST", "/jobs", map[string]any{"type": "catalog_settings", "payload": map[string]any{}}, 403)
	f.addMember("catalog-domain@example.test", []Grant{{Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}, "")
	f.loginAs("catalog-domain@example.test").call("GET", "/job-types", nil, 403)
}
func TestJobPublicMetadataDoesNotBypassResultSuppression(t *testing.T) {
	registerCatalogJobs(t, nil, nil)
	f := newFixture(t)
	_, _, role := f.addMember("catalog-reader@example.test", []Grant{{Resource: "jobs", Action: "create", Scope: "self"}, {Resource: "jobs", Action: "read", Scope: "self"}, {Resource: "tickets", Action: "read", Scope: "tenant", ReadFields: []string{"title"}}}, "")
	actor := f.loginAs("catalog-reader@example.test")
	created := asMap(actor.call("POST", "/jobs", map[string]any{"type": "catalog_typed", "payload": map[string]any{"category": "all"}}, 200)["data"])
	id := created["id"].(string)
	f.awaitJob(id)
	before := asMap(actor.call("GET", "/jobs/"+id, nil, 200)["data"])
	if asMap(before["result"])["count"] != float64(1) {
		t.Fatal("authorized typed result missing")
	}
	definition := asMap(before["definition"])
	if definition["label"] != "工单统计" || definition["description_en"] != "Safe aggregate" {
		t.Fatal("registered display metadata missing")
	}
	for _, key := range []string{"input_fields", "payload", "Run", "Validate"} {
		if _, present := definition[key]; present {
			t.Fatal("non-display job metadata exposed")
		}
	}
	f.call("PATCH", "/roles/"+role, map[string]any{"name": "Narrowed catalog", "grants": []Grant{{Resource: "jobs", Action: "read", Scope: "self"}, {Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}}, 200)
	detail := asMap(actor.call("GET", "/jobs/"+id, nil, 200)["data"])
	if _, present := detail["result"]; present {
		t.Fatal("definition restored hidden result")
	}
	if asMap(detail["definition"])["label_en"] != "Ticket statistics" {
		t.Fatal("display labels unnecessarily suppressed")
	}
	list := actor.call("GET", "/jobs", nil, 200)["data"].([]any)
	if len(list) != 1 {
		t.Fatal("job row missing")
	}
	row := asMap(list[0])
	if _, present := row["result"]; present {
		t.Fatal("list disclosed suppressed result")
	}
	if asMap(row["definition"])["label"] != "工单统计" {
		t.Fatal("list definition absent")
	}
	f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)
	for _, item := range f.call("GET", "/jobs", nil, 200)["data"].([]any) {
		job := asMap(item)
		if job["type"] == "export" {
			if _, present := job["definition"]; present {
				t.Fatal("export shape changed")
			}
		}
	}
}
