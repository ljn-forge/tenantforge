package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t                  *testing.T
	app                *App
	admin              *pgxpool.Pool
	server             *httptest.Server
	client             *http.Client
	csrf, tenant, user string
}

func newFixture(t *testing.T) *fixture { return newFixtureWithConfig(t, nil) }
func newFixtureWithConfig(t *testing.T, configure func(*Config)) *fixture {
	t.Helper()
	dsn := os.Getenv("TF_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TF_TEST_DATABASE_URL required")
	}
	testConfig, e := pgx.ParseConfig(dsn)
	if e != nil || testConfig.Database != "tenantforge_test" {
		t.Fatal("effective test database must be exactly tenantforge_test")
	}
	owner, e := pgx.ParseConfig(os.Getenv("TF_MIGRATION_DATABASE_URL"))
	if e != nil {
		t.Fatal("migration configuration required")
	}
	owner = owner.Copy()
	owner.Database = "tenantforge_test"
	ctx := context.Background()
	if e = MigrateConfig(ctx, owner); e != nil {
		t.Fatal(e)
	}
	adminConfig, e := pgxpool.ParseConfig(os.Getenv("TF_MIGRATION_DATABASE_URL"))
	if e != nil {
		t.Fatal("migration configuration invalid")
	}
	adminConfig.ConnConfig = owner.Copy()
	admin, e := pgxpool.NewWithConfig(ctx, adminConfig)
	if e != nil {
		t.Fatal("test admin unavailable")
	}
	t.Cleanup(admin.Close)
	if _, e = admin.Exec(ctx, "TRUNCATE users,tenants,mail_outbox,login_attempts,oidc_flows CASCADE"); e != nil {
		t.Fatal(e)
	}
	if _, e = admin.Exec(ctx, "UPDATE platform_state SET initialized=false"); e != nil {
		t.Fatal(e)
	}
	runtime, e := pgxpool.ParseConfig(os.Getenv("TF_DATABASE_URL"))
	if e != nil {
		t.Fatal("runtime configuration invalid")
	}
	runtime = runtime.Copy()
	runtime.ConnConfig.Database = "tenantforge_test"
	config := Config{DatabaseURL: os.Getenv("TF_DATABASE_URL"), PoolConfig: runtime, MasterKey: bytes.Repeat([]byte{7}, 32), DataDir: t.TempDir(), PublicURL: "http://localhost", StorageBackend: "local", DisableBootstrap: true}
	if configure != nil {
		configure(&config)
	}
	app, e := New(ctx, config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	f := &fixture{t: t, app: app, admin: admin, server: server, client: &http.Client{Jar: jar}}
	f.refreshCSRF()
	if config.DisableBootstrap {
		f.call("POST", "/setup", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "display_name": "Owner", "tenant_name": "First"}, 200)
	} else {
		f.call("POST", "/auth/login", map[string]any{"username": envValue(config.InitialAdminUsername, "admin"), "password": envValue(config.InitialAdminPassword, "Admin@123456")}, 200)
	}
	f.refreshCSRF()
	me := f.call("GET", "/auth/me", nil, 200)["data"].(map[string]any)
	f.tenant = me["tenants"].([]any)[0].(map[string]any)["id"].(string)
	f.user = me["user"].(map[string]any)["id"].(string)
	return f
}
func (f *fixture) request(method, path string, body io.Reader, ct string) *http.Response {
	f.t.Helper()
	r, e := http.NewRequest(method, f.server.URL+"/api/v1"+path, body)
	if e != nil {
		f.t.Fatal(e)
	}
	r.Header.Set("Content-Type", ct)
	r.Header.Set("X-CSRF-Token", f.csrf)
	r.Header.Set("X-Tenant-ID", f.tenant)
	resp, e := f.client.Do(r)
	if e != nil {
		f.t.Fatal(e)
	}
	return resp
}
func (f *fixture) call(method, path string, body any, want int) map[string]any {
	f.t.Helper()
	b, _ := json.Marshal(body)
	resp := f.request(method, path, bytes.NewReader(b), "application/json")
	defer resp.Body.Close()
	out := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != want {
		f.t.Fatalf("%s %s: got %d want %d (%s)", method, path, resp.StatusCode, want, stringValue(asMap(out["error"])["code"]))
	}
	return out
}
func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func (f *fixture) refreshCSRF() {
	f.csrf = f.call("GET", "/auth/csrf", nil, 200)["data"].(map[string]any)["csrf_token"].(string)
}
func (f *fixture) sql(query string, args ...any) {
	f.t.Helper()
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		f.t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(context.Background(), "SELECT set_config('app.tenant_id',$1,true)", f.tenant); e != nil {
		f.t.Fatal(e)
	}
	if _, e = tx.Exec(context.Background(), query, args...); e != nil {
		f.t.Fatal(e)
	}
	if e = tx.Commit(context.Background()); e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixture) addMember(email string, grants []Grant, department string) (string, string, string) {
	f.t.Helper()
	role := asMap(f.call("POST", "/roles", map[string]any{"name": email, "description": "test role", "grants": grants}, 200)["data"])["id"].(string)
	user, member := uuid.NewString(), uuid.NewString()
	h, e := hashPassword("MemberPassword!123")
	if e != nil {
		f.t.Fatal(e)
	}
	f.sql("INSERT INTO users(id,email,display_name,password_hash,username) VALUES($1,$2,$2,$3,$4)", user, email, h, userUsername(user))
	f.sql("INSERT INTO memberships(id,tenant_id,user_id,department_id) VALUES($1,$2,$3,$4)", member, f.tenant, user, nullable(department))
	f.sql("INSERT INTO member_roles(tenant_id,member_id,role_id) VALUES($1,$2,$3)", f.tenant, member, role)
	return user, member, role
}
func (f *fixture) loginAs(email string) *fixture {
	jar, _ := cookiejar.New(nil)
	other := &fixture{t: f.t, app: f.app, admin: f.admin, server: f.server, client: &http.Client{Jar: jar}, tenant: f.tenant}
	other.refreshCSRF()
	other.call("POST", "/auth/login", map[string]any{"email": email, "password": "MemberPassword!123"}, 200)
	other.refreshCSRF()
	other.tenant = ""
	me := asMap(other.call("GET", "/auth/me", nil, 200)["data"])
	other.tenant = f.tenant
	other.user = asMap(me["user"])["id"].(string)
	return other
}
