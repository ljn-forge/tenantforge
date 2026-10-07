package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pgx "github.com/jackc/pgx/v5"
)

type uncertainPutStorage struct {
	Storage
	key        string
	failDelete atomic.Bool
}

func (s *uncertainPutStorage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	s.key = key
	if e := s.Storage.Put(ctx, key, r, size, ct); e != nil {
		return e
	}
	return errors.New("response lost")
}
func (s *uncertainPutStorage) Delete(ctx context.Context, key string) error {
	if s.failDelete.Load() {
		return errors.New("temporarily unavailable")
	}
	return s.Storage.Delete(ctx, key)
}
func TestUncertainUploadCleansObjectAndQueuesWithOneConnection(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.PoolConfig.MaxConns = 1 })
	s := &uncertainPutStorage{Storage: f.app.storage()}
	s.failDelete.Store(true)
	f.app.setStorage(s)
	f.multipart("/files", "uncertain.txt", "stored despite lost response", nil, 503)
	var n int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM storage_gc WHERE storage_key=$1", s.key).Scan(&n); e != nil || n != 1 {
		t.Fatal("failed upload must persist cleanup even with one request connection")
	}
	s.failDelete.Store(false)
	f.app.deleteOrQueue(s.key)
	if stream, e := s.Storage.Open(context.Background(), s.key); e == nil {
		stream.Close()
		t.Fatal("unpublished object survived cleanup")
	}
}
func TestExportProjectsFiftyOneRegisteredFields(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	m := Module{Key: "wide", Label: "Wide", LabelEN: "Wide", Actions: []string{"read", "create", "export"}}
	columns := []string{}
	data := map[string]any{}
	for i := 0; i < 51; i++ {
		key := fmt.Sprintf("field_%d", i)
		m.Fields = append(m.Fields, Field{Key: key, Label: key, LabelEN: key, Type: "string"})
		columns = append(columns, key)
		data[key] = "value"
	}
	if e := RegisterModule(m); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	f.call("POST", "/resources/wide", map[string]any{"data": data}, 200)
	job := asMap(f.call("POST", "/resources/wide/export", map[string]any{"columns": columns}, 200)["data"])
	f.awaitJob(job["id"].(string))
}
func TestEffectiveTestDatabaseValidation(t *testing.T) {
	for _, dsn := range []string{"postgres://test:fake@localhost/tenantforge_test?dbname=tenantforge", "host=localhost user=test dbname=tenantforge"} {
		c, e := pgx.ParseConfig(dsn)
		if e != nil {
			continue
		}
		if c.Database == "tenantforge_test" {
			t.Fatal("unsafe fixture routing accepted")
		}
	}
}
func TestMigrationActivatesInstalledModulesForExistingOwners(t *testing.T) {
	old := moduleRegistry
	moduleRegistry = NewRegistry()
	t.Cleanup(func() { moduleRegistry = old })
	if e := RegisterModule(Module{Key: "assets", Label: "Assets", LabelEN: "Assets", Actions: []string{"read", "create"}, Fields: []Field{{Key: "name", Label: "Name", LabelEN: "Name", Type: "string"}}}); e != nil {
		t.Fatal(e)
	}
	f := newFixture(t)
	f.sql("UPDATE roles SET grants=(SELECT jsonb_agg(g) FROM jsonb_array_elements(grants) g WHERE g->>'resource'<>'assets') WHERE tenant_id=$1 AND builtin AND name='Owner'", f.tenant)
	f.call("POST", "/roles", map[string]any{"name": "Premature assets", "grants": []Grant{{Resource: "assets", Action: "read", Scope: "self", ReadFields: []string{"name"}}}}, 403)
	owner := f.admin.Config().ConnConfig.Copy()
	if e := MigrateConfig(context.Background(), owner); e != nil {
		t.Fatal(e)
	}
	f.call("POST", "/roles", map[string]any{"name": "Installed assets", "grants": []Grant{{Resource: "assets", Action: "read", Scope: "self", ReadFields: []string{"name"}}}}, 200)
	var first, second int
	f.sql("SELECT 1")
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM audit WHERE tenant_id=$1 AND action='module_activation'", f.tenant).Scan(&first); e != nil {
		t.Fatal(e)
	}
	if e := MigrateConfig(context.Background(), owner); e != nil {
		t.Fatal(e)
	}
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM audit WHERE tenant_id=$1 AND action='module_activation'", f.tenant).Scan(&second); e != nil || first != 1 || second != first {
		t.Fatal("activation must be audited exactly once")
	}
}
func TestDeleteTerminalJobReleasesArtifactQuota(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "artifact"}}, 200)
	j := asMap(f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)["data"])
	id := j["id"].(string)
	f.awaitJob(id)
	f.call("DELETE", "/jobs/"+id, nil, 200)
	f.call("GET", "/jobs/"+id, nil, 404)
	var n int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM storage_gc").Scan(&n); e != nil || n != 1 {
		t.Fatal("artifact cleanup not durable")
	}
}
func TestDefaultAdministratorGateAndDirectUsers(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.DisableBootstrap = false })
	user := asMap(asMap(f.call("GET", "/auth/me", nil, 200)["data"])["user"])
	if user["username"] != "admin" || user["must_change_password"] != true {
		t.Fatal("default administrator was not seeded with password gate")
	}
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "blocked"}}, 403)
	f.call("GET", "/members", nil, 403)
	f.call("GET", "/files", nil, 403)
	f.call("POST", "/auth/password", map[string]any{"current_password": "Admin@123456", "new_password": "Admin@123456"}, 422)
	f.call("POST", "/auth/password", map[string]any{"current_password": "Admin@123456", "new_password": "ChangedPassword!123"}, 200)
	f.refreshCSRF()
	user = asMap(asMap(f.call("GET", "/auth/me", nil, 200)["data"])["user"])
	if user["must_change_password"] != false {
		t.Fatal("password gate not cleared")
	}
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "allowed"}}, 200)
	role := asMap(f.call("POST", "/roles", map[string]any{"name": "Reader", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}}, 200)["data"])["id"].(string)
	member := asMap(f.call("POST", "/members", map[string]any{"username": "worker.one", "email": "worker@example.test", "display_name": "Worker", "password": "MemberPassword!123", "role_ids": []string{role}}, 200)["data"])
	if member["username"] != "worker.one" || member["status"] != "active" {
		t.Fatal("direct creation response incomplete")
	}
	f.call("POST", "/members", map[string]any{"username": "worker.one", "email": "other@example.test", "display_name": "Duplicate", "password": "MemberPassword!123", "role_ids": []string{role}}, 409)
	m := f.loginAs("worker@example.test")
	if asMap(asMap(m.call("GET", "/auth/me", nil, 200)["data"])["user"])["must_change_password"] != true {
		t.Fatal("new user missing password gate")
	}
	m.call("POST", "/auth/password", map[string]any{"current_password": "MemberPassword!123", "new_password": "NewWorkerPassword!123"}, 200)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/login", map[string]any{"username": "admin", "password": "ChangedPassword!123"}, 200)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := f.app.bootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := f.admin.QueryRow(ctx, "SELECT count(*) FROM users WHERE username='admin'").Scan(&n); e != nil || n != 1 {
		t.Fatal("bootstrap overwrote existing platform")
	}
}
func TestCleanupPreservesPublishedArtifacts(t *testing.T) {
	f := newFixture(t)
	file := asMap(f.multipart("/files", "published.txt", "keep published bytes", nil, 200)["data"])
	var key string
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	tx.Exec(context.Background(), "SELECT set_config('app.tenant_id',$1,true)", f.tenant)
	e = tx.QueryRow(context.Background(), "SELECT storage_key FROM files WHERE id=$1", file["id"]).Scan(&key)
	tx.Rollback(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	// A lost COMMIT response may invoke cleanup after publication removed its reservation.
	f.app.deleteOrQueue(key)
	if stream, e := f.app.storage().Open(context.Background(), key); e != nil {
		t.Fatal("cleanup deleted committed publication without a journal")
	} else {
		stream.Close()
	}
	if e := f.app.reserveObject(context.Background(), key, f.tenant); e != nil {
		t.Fatal(e)
	}
	f.app.deleteOrQueue(key)
	if stream, e := f.app.storage().Open(context.Background(), key); e != nil {
		t.Fatal("cleanup deleted committed file after uncertain commit response")
	} else {
		stream.Close()
	}
}
func TestTenantOwnersCannotPreclaimGlobalAccounts(t *testing.T) {
	f := newFixture(t)
	f.addMember("tenant.owner@example.test", ownerGrants(), "")
	owner := f.loginAs("tenant.owner@example.test")
	owner.call("POST", "/members", map[string]any{"username": "victim", "email": "victim@example.test", "display_name": "Victim", "password": "KnownPassword!123", "role_ids": []string{}}, 403)
	var n int
	if e := f.admin.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE email='victim@example.test'").Scan(&n); e != nil || n != 0 {
		t.Fatal("tenant owner created a global account")
	}
}

type failedRow struct{ err error }

func (r failedRow) Scan(...any) error { return r.err }

type transientTenantTx struct {
	pgx.Tx
	err error
}

func (t transientTenantTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "FROM memberships m JOIN tenants t") {
		return failedRow{t.err}
	}
	return t.Tx.QueryRow(ctx, sql, args...)
}
func TestTenantOperationalFailuresRemainRetryable(t *testing.T) {
	f := newFixture(t)
	tx, e := f.app.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	want := errors.New("transient database transport failure")
	r, _ := http.NewRequest("GET", "http://worker", nil)
	q := &request{app: f.app, tx: transientTenantTx{Tx: tx, err: want}, r: r, p: Principal{ID: f.user}}
	if e = q.loadTenant(f.tenant); !errors.Is(e, want) || errorCode(e) == "forbidden" {
		t.Fatal("operational failure misclassified as permanent authorization revocation")
	}
}
func TestStartupRejectsMissingEarlierCoreMigration(t *testing.T) {
	f := newFixture(t)
	if _, e := f.admin.Exec(context.Background(), "DELETE FROM goose_db_version WHERE version_id=5"); e != nil {
		t.Fatal(e)
	}
	defer f.admin.Exec(context.Background(), "INSERT INTO goose_db_version(version_id,is_applied) VALUES(5,true)")
	app, e := New(context.Background(), f.app.Config)
	if app != nil {
		app.Close()
		t.Fatal("startup accepted an unapplied earlier core migration")
	}
	if e == nil {
		t.Fatal("migration failure missing")
	}
}
func TestConfiguredAdministratorRestartPreservesCredentials(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) {
		c.DisableBootstrap = false
		c.InitialAdminUsername = "custom.admin"
		c.InitialAdminPassword = "CustomInitial!123"
		c.InitialAdminEmail = "custom@example.test"
		c.InitialTenantName = "Configured Workspace"
	})
	f.call("POST", "/auth/password", map[string]any{"current_password": "CustomInitial!123", "new_password": "ChangedCustom!123"}, 200)
	c := f.app.Config
	c.InitialAdminUsername = "replacement"
	c.InitialAdminEmail = "replacement@example.test"
	c.InitialAdminPassword = "Replacement!123"
	app, e := New(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	app.Close()
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/login", map[string]any{"username": "custom.admin", "password": "CustomInitial!123"}, 401)
	f.call("POST", "/auth/login", map[string]any{"username": "custom.admin", "password": "ChangedCustom!123"}, 200)
	var n int
	if e = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE username='replacement'").Scan(&n); e != nil || n != 0 {
		t.Fatal("restart overwrote platform initialization")
	}
}
func TestResetCannotKeepInitialAdministratorPassword(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) { c.DisableBootstrap = false })
	f.call("POST", "/auth/reset/request", map[string]any{"email": "admin@tenantforge.local"}, 200)
	token := f.mailLinkToken("Password reset")
	f.call("POST", "/auth/reset/confirm", map[string]any{"token": token, "new_password": "Admin@123456"}, 422)
	f.call("GET", "/members", nil, 403)
	f.call("POST", "/auth/reset/confirm", map[string]any{"token": token, "new_password": "ResetAdminPassword!123"}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/login", map[string]any{"username": "admin", "password": "ResetAdminPassword!123"}, 200)
	if asMap(asMap(f.call("GET", "/auth/me", nil, 200)["data"])["user"])["must_change_password"] != false {
		t.Fatal("reset failed to clear password gate")
	}
}
func TestBootstrapRefusesOccupiedUninitializedPlatform(t *testing.T) {
	f := newFixture(t)
	if _, e := f.admin.Exec(context.Background(), "UPDATE platform_state SET initialized=false WHERE id=true"); e != nil {
		t.Fatal(e)
	}
	c := f.app.Config
	c.DisableBootstrap = false
	app, e := New(context.Background(), c)
	if app != nil {
		app.Close()
		t.Fatal("seed ran over existing users")
	}
	if e == nil {
		t.Fatal("inconsistent initialization state silently accepted")
	}
	var n int
	if e = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&n); e != nil || n != 1 {
		t.Fatal("existing accounts were changed")
	}
}
