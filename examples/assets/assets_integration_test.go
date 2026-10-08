package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ljn-forge/tenantforge/internal/platform"
)

// This test provisions/resets only its own named extension fixture database.
func TestRegisteredModuleWorksThroughHTTPAndEmbeddedMigration(t *testing.T) {
	if os.Getenv("TF_TEST_DATABASE_URL") == "" {
		t.Skip("isolated PostgreSQL integration environment required")
	}
	ctx := context.Background()
	c, err := platform.FromEnv()
	if err != nil {
		t.Fatal("integration configuration invalid")
	}
	c.DisableBootstrap = true
	testConfig, err := pgxpool.ParseConfig(os.Getenv("TF_TEST_DATABASE_URL"))
	if err != nil || testConfig.ConnConfig.Database != "tenantforge_test" {
		t.Fatal("effective integration database must be tenantforge_test")
	}
	owner, err := pgxpool.ParseConfig(c.MigrationDatabaseURL)
	if err != nil {
		t.Fatal("owner integration connection required")
	}
	owner = owner.Copy()
	owner.ConnConfig.Database = "postgres"
	cluster, err := pgxpool.NewWithConfig(ctx, owner)
	if err != nil {
		t.Fatal("integration cluster unavailable")
	}
	defer cluster.Close()
	var exists bool
	if err = cluster.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_database WHERE datname='tenantforge_extension')").Scan(&exists); err != nil {
		t.Fatal("database probe failed")
	}
	if !exists {
		if _, err = cluster.Exec(ctx, "CREATE DATABASE tenantforge_extension OWNER tenantforge_owner"); err != nil {
			t.Fatal("extension fixture creation failed")
		}
	}
	owner = owner.Copy()
	owner.ConnConfig.Database = "tenantforge_extension"
	if err = Register(); err != nil {
		t.Fatal(err)
	}
	if err = platform.MigrateConfig(ctx, owner.ConnConfig.Copy()); err != nil {
		t.Fatal(err)
	}
	if err = platform.MigrateConfig(ctx, owner.ConnConfig.Copy()); err != nil {
		t.Fatal("extension migration is not idempotent")
	}
	admin, err := pgxpool.NewWithConfig(ctx, owner.Copy())
	if err != nil {
		t.Fatal("extension fixture connection failed")
	}
	defer admin.Close()
	if _, err = admin.Exec(ctx, "TRUNCATE users,tenants,mail_outbox,login_attempts,oidc_flows CASCADE; UPDATE platform_state SET initialized=false"); err != nil {
		t.Fatal("extension fixture reset failed")
	}
	var indexExists bool
	if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT FROM pg_indexes WHERE indexname='resources_asset_name')").Scan(&indexExists); err != nil || !indexExists {
		t.Fatal("embedded extension migration not applied")
	}
	runtimeConfig, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		t.Fatal("runtime URL invalid")
	}
	runtimeConfig = runtimeConfig.Copy()
	runtimeConfig.ConnConfig.Database = "tenantforge_extension"
	c.PoolConfig, c.DataDir, c.SMTPHost = runtimeConfig, t.TempDir(), ""
	app, err := platform.New(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf, tenant := "", ""
	call := func(method, path string, body any) map[string]any {
		t.Helper()
		encoded, _ := json.Marshal(body)
		r, err := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("X-Tenant-ID", tenant)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal("extension HTTP operation failed")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s %s returned %d", method, path, response.StatusCode)
		}
		var envelope map[string]any
		if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatal("invalid extension response")
		}
		return envelope
	}
	csrf = call("GET", "/auth/csrf", nil)["data"].(map[string]any)["csrf_token"].(string)
	call("POST", "/setup", map[string]any{"email": "assets-owner@example.test", "password": "ExtensionFixture-Only!2026", "display_name": "Assets owner", "tenant_name": "Extension fixture"})
	csrf = call("GET", "/auth/csrf", nil)["data"].(map[string]any)["csrf_token"].(string)
	me := call("GET", "/auth/me", nil)["data"].(map[string]any)
	tenant = me["tenants"].([]any)[0].(map[string]any)["id"].(string)
	created := call("POST", "/resources/assets", map[string]any{"data": map[string]any{"name": "Laptop", "serial": "FIXTURE-001", "active": false}})["data"].(map[string]any)
	id := created["id"].(string)
	detail := call("GET", "/resources/assets/"+id, nil)["data"].(map[string]any)
	if detail["data"].(map[string]any)["name"] != "Laptop" {
		t.Fatal("registered module did not use the authorized resource pipeline")
	}
	listed := call("GET", "/resources/assets?q=Laptop", nil)["data"].([]any)
	if len(listed) != 1 {
		t.Fatal("registered module SQL search/paging mismatch")
	}
	approved := call("POST", "/resources/assets/"+id+"/actions/approve", map[string]any{"version": 1, "data": map[string]any{}})["data"].(map[string]any)
	if approved["version"] != float64(2) || approved["data"].(map[string]any)["active"] != true {
		t.Fatal("registered approval did not persist its guarded field patch")
	}
	audit := call("GET", "/audit", nil)["data"].([]any)
	foundApproval := false
	for _, item := range audit {
		entry := item.(map[string]any)
		if entry["resource"] == "assets" && entry["action"] == "approve" && entry["record_id"] == id {
			foundApproval = true
		}
	}
	if !foundApproval {
		t.Fatal("approval did not commit a core-managed audit")
	}
	catalog := call("GET", "/job-types", nil)["data"].([]any)
	if len(catalog) != 1 {
		t.Fatal("public extension task was not discoverable")
	}
	task := catalog[0].(map[string]any)
	if task["type"] != "assets_inventory" || task["label_en"] != "Asset inventory" || len(task["input_fields"].([]any)) != 0 {
		t.Fatal("public extension task descriptor differs from its declared UI contract")
	}
	job := call("POST", "/jobs", map[string]any{"type": "assets_inventory", "payload": map[string]any{}})["data"].(map[string]any)
	deadline := time.Now().Add(8 * time.Second)
	for {
		job = call("GET", "/jobs/"+job["id"].(string), nil)["data"].(map[string]any)
		if job["status"] == "succeeded" {
			break
		}
		if job["status"] == "failed" || time.Now().After(deadline) {
			t.Fatal("registered background inventory did not complete")
		}
		time.Sleep(20 * time.Millisecond)
	}
	result := job["result"].(map[string]any)
	if result["count"] != float64(1) {
		t.Fatal("registered task did not walk its authorized records")
	}
	presentation := job["definition"].(map[string]any)
	if presentation["label"] != "资产盘点" || presentation["result_fields"].([]any)[0].(map[string]any)["label_en"] != "Visible asset count" {
		t.Fatal("persisted job did not retain safe bilingual result presentation")
	}
	for _, private := range []string{"payload", "authority_hash", "authority_resource", "authority_action"} {
		if _, exposed := job[private]; exposed {
			t.Fatal("extension job response exposed private execution data")
		}
	}

}
