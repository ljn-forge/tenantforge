package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pgx "github.com/jackc/pgx/v5"
	goose "github.com/pressly/goose/v3"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3/lock"

	"github.com/ljn-forge/tenantforge/migrations"
)

type App struct {
	metricsState *metricsState
	closeOnce    sync.Once
	LimitsDB     *pgxpool.Pool
	storageMu    sync.RWMutex
	wg           sync.WaitGroup
	DB           *pgxpool.Pool
	Config       Config
	Storage      Storage
	cancel       context.CancelFunc
	requests     atomic.Uint64
	failures     atomic.Uint64
	activityMu   sync.Mutex
	activity     map[string]struct{}
}
type request struct {
	public, tenantRequired bool
	app                    *App
	w                      http.ResponseWriter
	r                      *http.Request
	tx                     pgx.Tx
	p                      Principal
	requestID              string
	streamed               bool
	rollback               []func()
	commit                 []func()
}
type APIError struct {
	Status        int
	Code, Message string
}

func (e *APIError) Error() string                { return e.Message }
func failure(status int, code, msg string) error { return &APIError{status, code, msg} }
func validation(s string) error                  { return failure(422, "validation", s) }
func forbidden() error                           { return failure(403, "forbidden", "Access denied") }
func notFound() error                            { return failure(404, "not_found", "Record not found") }
func conflict() error                            { return failure(409, "conflict", "Record changed or conflicting reference") }
func New(ctx context.Context, c Config) (*App, error) {
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 30 * time.Second
	}
	if c.UpstreamTimeout == 0 {
		c.UpstreamTimeout = 15 * time.Second
	}
	if e := c.Validate(); e != nil {
		return nil, e
	}
	poolConfig := c.PoolConfig
	var e error
	if poolConfig == nil {
		poolConfig, e = pgxpool.ParseConfig(c.DatabaseURL)
		if e != nil {
			return nil, errors.New("database configuration invalid")
		}
	} else {
		poolConfig = poolConfig.Copy()
	}
	pool, e := pgxpool.NewWithConfig(ctx, poolConfig)
	if e != nil {
		return nil, errors.New("database configuration invalid")
	}
	if e = pool.Ping(ctx); e != nil {
		pool.Close()
		return nil, errors.New("database connection unavailable")
	}
	var bypass, super, owns bool
	e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_roles b WHERE (b.rolbypassrls OR b.rolsuper) AND pg_has_role(current_user,b.oid,'MEMBER')),r.rolsuper,EXISTS(SELECT FROM pg_tables WHERE schemaname='public' AND tablename IN ('resources','roles','jobs','audit') AND pg_has_role(current_user,tableowner,'MEMBER')) FROM pg_roles r WHERE rolname=current_user`).Scan(&bypass, &super, &owns)
	if e != nil || bypass || super || owns {
		pool.Close()
		return nil, errors.New("runtime database role must not own protected tables, be superuser or bypass RLS")
	}
	var version int
	if e = pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&version); e != nil || version != migrations.LatestVersion() {
		pool.Close()
		return nil, errors.New("database migration required")
	}
	for _, required := range migrations.Versions() {
		var applied bool
		if e = pool.QueryRow(ctx, "SELECT is_applied FROM goose_db_version WHERE version_id=$1 ORDER BY id DESC LIMIT 1", required).Scan(&applied); e != nil || !applied {
			pool.Close()
			return nil, errors.New("database migration required")
		}
	}
	storage, e := NewStorage(ctx, c)
	if e != nil {
		pool.Close()
		return nil, e
	}
	moduleRegistry.Freeze()
	migrations.FS.Freeze()
	workerCtx, cancel := context.WithCancel(context.Background())
	limitsConfig := poolConfig.Copy()
	limitsConfig.MaxConns = 2
	limitsConfig.MinConns = 0
	limits, e := pgxpool.NewWithConfig(ctx, limitsConfig)
	if e != nil {
		pool.Close()
		cancel()
		return nil, errors.New("limiter database unavailable")
	}
	a := &App{DB: pool, LimitsDB: limits, Config: c, Storage: storage, cancel: cancel}
	a.metricsState = newMetrics(a)
	if !c.DisableBootstrap {
		if e = a.bootstrap(ctx); e != nil {
			a.Close()
			return nil, e
		}
	}
	a.startWorkers(workerCtx)
	return a, nil
}
func (a *App) Close() {
	a.closeOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		a.wg.Wait()
		a.LimitsDB.Close()
		a.DB.Close()
		if local, ok := a.storage().(*localStorage); ok {
			local.Root.Close()
		}
	})
}
func Migrate(ctx context.Context, dsn string) error {
	c, e := pgx.ParseConfig(dsn)
	if e != nil {
		return errors.New("migration configuration invalid")
	}
	return MigrateConfig(ctx, c)
}

// MigrateConfig preserves the parsed effective routing settings; callers can safely override Database on a copy.
func MigrateConfig(ctx context.Context, c *pgx.ConnConfig) error {
	db := stdlib.OpenDB(*c.Copy())
	defer db.Close()
	db.SetMaxOpenConns(2)
	locker, e := lock.NewPostgresSessionLocker()
	if e != nil {
		return errors.New("migration lock configuration invalid")
	}
	migrations.FS.Freeze()
	provider, e := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker), goose.WithAllowOutofOrder(true))
	if e != nil {
		return errors.New("migration provider invalid")
	}
	if _, e = provider.Up(ctx); e != nil {
		return fmt.Errorf("migration failed (%T)", e)
	}
	return activateOwnerModules(ctx, db)
}
func activateOwnerModules(ctx context.Context, db *sql.DB) error {
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("module activation unavailable")
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, "SELECT id::text FROM tenants ORDER BY id")
	if e != nil {
		return errors.New("module activation unavailable")
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return errors.New("module activation unavailable")
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return errors.New("module activation unavailable")
	}
	grants := jsonBytes(ownerGrants())
	modules := []string{}
	for _, m := range registry() {
		modules = append(modules, m.Key)
	}
	for _, id := range ids {
		if _, e = tx.ExecContext(ctx, "SELECT set_config('app.tenant_id',$1,true)", id); e != nil {
			return errors.New("module activation unavailable")
		}
		result, e := tx.ExecContext(ctx, "UPDATE roles SET grants=$1::jsonb WHERE tenant_id=$2 AND builtin AND name='Owner' AND grants IS DISTINCT FROM $1::jsonb", grants, id)
		if e != nil {
			return errors.New("module activation unavailable")
		}
		n, e := result.RowsAffected()
		if e != nil {
			return errors.New("module activation unavailable")
		}
		if n > 0 {
			if _, e = tx.ExecContext(ctx, "INSERT INTO audit(id,tenant_id,actor_id,action,resource,record_id,request_id,changes) VALUES($1,$2,NULL,'module_activation','roles','',$3,$4)", uuid.NewString(), id, uuid.NewString(), jsonBytes(map[string]any{"modules": modules})); e != nil {
				return errors.New("module activation unavailable")
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return errors.New("module activation unavailable")
	}
	return nil
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	add := func(pattern string, public, tenant bool, h func(*request) (any, error)) {
		mux.HandleFunc(pattern, a.wrap(public, tenant, h))
	}
	add("GET /api/v1/setup/status", true, false, setupStatus)
	add("POST /api/v1/setup", true, false, setup)
	add("GET /api/v1/auth/csrf", true, false, csrf)
	add("POST /api/v1/auth/login", true, false, login)
	add("POST /api/v1/auth/logout", false, false, logout)
	add("GET /api/v1/auth/me", false, false, me)
	add("GET /api/v1/tenants", false, false, tenants)
	add("GET /api/v1/auth/security-events", false, false, securityEvents)
	add("GET /api/v1/auth/sessions", false, false, sessions)
	add("DELETE /api/v1/auth/sessions/{id}", false, false, revokeSession)
	add("POST /api/v1/auth/password", false, false, passwordChange)
	add("POST /api/v1/auth/reset/request", true, false, resetRequest)
	add("POST /api/v1/auth/reset/confirm", true, false, resetConfirm)
	add("POST /api/v1/auth/mfa/enroll", false, false, mfaEnroll)
	add("POST /api/v1/auth/mfa/confirm", false, false, mfaConfirm)
	add("POST /api/v1/auth/mfa/disable", false, false, mfaDisable)
	add("GET /api/v1/auth/oidc/start", true, false, oidcStart)
	add("GET /api/v1/auth/oidc/callback", true, false, oidcCallback)
	add("GET /api/v1/platform/tenants", false, false, platformTenants)
	add("POST /api/v1/platform/tenants", false, false, provisionTenant)
	add("PATCH /api/v1/platform/tenants/{id}", false, false, patchTenant)
	add("GET /api/v1/members", false, true, members)
	add("POST /api/v1/members", false, true, createMember)
	add("PATCH /api/v1/members/{id}", false, true, patchMember)
	add("POST /api/v1/members/{id}/transfer", false, true, transferMember)
	add("GET /api/v1/members/{id}/permissions", false, true, memberPermissions)
	add("GET /api/v1/departments", false, true, departments)
	add("POST /api/v1/departments", false, true, createDepartment)
	add("PATCH /api/v1/departments/{id}", false, true, patchDepartment)
	add("DELETE /api/v1/departments/{id}", false, true, deleteDepartment)
	add("GET /api/v1/roles", false, true, roles)
	add("POST /api/v1/roles", false, true, createRole)
	add("PATCH /api/v1/roles/{id}", false, true, patchRole)
	add("DELETE /api/v1/roles/{id}", false, true, deleteRole)
	add("POST /api/v1/roles/{id}/copy", false, true, copyRole)
	add("GET /api/v1/permissions", false, true, permissions)
	add("GET /api/v1/invitations", false, true, invitations)
	add("POST /api/v1/invitations", false, true, createInvitation)
	add("POST /api/v1/invitations/accept", false, false, acceptInvitation)
	add("GET /api/v1/resources/{module}", false, true, listResources)
	add("GET /api/v1/resources/{module}/{id}", false, true, getResource)
	add("POST /api/v1/resources/{module}", false, true, createResource)
	add("PATCH /api/v1/resources/{module}/{id}", false, true, updateResource)
	add("DELETE /api/v1/resources/{module}/{id}", false, true, deleteResource)
	add("POST /api/v1/resources/{module}/{id}/transition", false, true, transitionResource)
	add("POST /api/v1/resources/{module}/{id}/actions/{action}", false, true, operateResource)
	add("POST /api/v1/resources/{module}/export", false, true, exportResources)
	add("POST /api/v1/resources/{module}/import", false, true, importResources)
	add("GET /api/v1/files", false, true, listFiles)
	add("POST /api/v1/files", false, true, uploadFile)
	add("GET /api/v1/files/{id}/download", false, true, downloadFile)
	add("DELETE /api/v1/files/{id}", false, true, deleteFile)
	add("GET /api/v1/job-types", false, true, jobTypes)
	add("GET /api/v1/jobs", false, true, listJobs)
	add("POST /api/v1/jobs", false, true, createJob)
	add("GET /api/v1/jobs/{id}", false, true, getJob)
	add("POST /api/v1/jobs/{id}/retry", false, true, retryJob)
	add("POST /api/v1/jobs/{id}/cancel", false, true, cancelJob)
	add("DELETE /api/v1/jobs/{id}", false, true, deleteJob)
	add("GET /api/v1/jobs/{id}/download", false, true, downloadJob)
	add("GET /api/v1/audit", false, true, listAudit)
	add("POST /api/v1/audit/export", false, true, auditExport)
	add("GET /api/v1/dashboard", false, true, dashboard)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok", "version": Version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if a.DB.Ping(r.Context()) != nil {
			writeJSON(w, 503, map[string]any{"status": "unavailable"})
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ready"})
	})
	mux.HandleFunc("GET /metrics", a.metrics)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, "", notFound())
			return
		}
		path := filepath.Join(a.Config.WebDir, filepath.Clean("/"+r.URL.Path))
		if stat, e := os.Stat(path); e == nil && !stat.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.ServeFile(w, r, filepath.Join(a.Config.WebDir, "index.html"))
	})
	return a.secure(mux)
}
func (a *App) wrap(public, tenant bool, h func(*request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		observed := &observedResponseWriter{ResponseWriter: w}
		w = observed
		defer func() {
			if observed.status >= 400 {
				a.failures.Add(1)
			}
			if a.metricsState != nil {
				a.metricsState.observeHTTP(r.Pattern, r.Method, observed.status, time.Since(start))
			}
		}()
		defer func() {
			if recover() != nil {
				slog.Error("HTTP operation panic (details redacted)")
				writeError(w, w.Header().Get("X-Request-ID"), failure(500, "internal", "Service operation failed"))
			}
		}()
		ctx, cancel := context.WithTimeout(r.Context(), a.Config.RequestTimeout)
		defer cancel()
		r = r.WithContext(ctx)
		a.requests.Add(1)
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			writeError(w, id, requestError(r.Context(), failure(503, "unavailable", "Service unavailable")))
			return
		}
		req := &request{app: a, w: w, r: r, tx: tx, requestID: id, public: public, tenantRequired: tenant}
		committed := false
		defer func() {
			if !committed {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
				if req.tx != nil {
					req.tx.Rollback(rollbackCtx)
				}
				rollbackCancel()
				for _, f := range req.rollback {
					f()
				}
			}
		}()
		if !public {
			e = req.authenticate()
			if e != nil {
				writeError(w, id, requestError(r.Context(), e))
				return
			}
		}
		if req.passwordGate() != nil {
			writeError(w, id, failure(403, "password_change_required", "Change the initial password before continuing"))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if e = req.checkCSRF(); e != nil {
				writeError(w, id, requestError(r.Context(), e))
				return
			}
		}
		if tenant {
			e = req.loadTenant(r.Header.Get("X-Tenant-ID"))
			if e != nil {
				writeError(w, id, requestError(r.Context(), e))
				return
			}
		}
		defer func() {
			if r.MultipartForm != nil {
				r.MultipartForm.RemoveAll()
			}
		}()
		if r.Method != "GET" && r.Method != "HEAD" {
			if e = req.prepareBody(); e != nil {
				writeError(w, id, requestError(r.Context(), e))
				return
			}
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		data, e := h(req)
		if e == nil {
			if req.tx != nil {
				e = req.tx.Commit(r.Context())
				req.tx = nil
			}
		}
		if e != nil {
			e = requestError(r.Context(), e)
			if !req.streamed {
				writeError(w, id, e)
			}
			slog.Info("request rejected", "request_id", id, "method", r.Method, "path", r.URL.Path, "code", errorCode(e))
			return
		}
		committed = true
		for _, f := range req.commit {
			f()
		}
		if !req.streamed {
			if page, ok := data.(paged); ok {
				writeJSON(w, 200, map[string]any{"data": page.Data, "meta": page.Meta})
			} else {
				writeJSON(w, 200, map[string]any{"data": data})
			}
		}
	}
}
func (q *request) ctx() context.Context { return q.r.Context() }
func (q *request) decode(v any) error {
	q.r.Body = http.MaxBytesReader(q.w, q.r.Body, 1<<20)
	d := json.NewDecoder(q.r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return validation("Invalid JSON payload")
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return validation("Unexpected JSON content")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// requestError distinguishes transport cancellation from provider-specific query wrappers.
func requestError(ctx context.Context, e error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return e
}
func classifyError(e error) *APIError {
	var a *APIError
	if errors.As(e, &a) {
		return a
	}
	if errors.Is(e, context.Canceled) {
		return &APIError{499, "cancelled", "Request cancelled"}
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return &APIError{http.StatusGatewayTimeout, "timeout", "Request timed out"}
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return &APIError{404, "not_found", "Record not found"}
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && (p.Code == "23505" || p.Code == "23503" || p.Code == "40001" || p.Code == "40P01") {
		return &APIError{409, "conflict", "Conflicting reference or concurrent update"}
	}
	return &APIError{500, "internal", "Service operation failed"}
}
func errorCode(e error) string { return classifyError(e).Code }
func writeError(w http.ResponseWriter, id string, e error) {
	a := classifyError(e)
	writeJSON(w, a.Status, map[string]any{"error": map[string]any{"code": a.Code, "message": a.Message}, "request_id": id})
}

func (q *request) rows(sql string, args ...any) ([]map[string]any, error) {
	rows, e := q.tx.Query(q.ctx(), sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		v, e := rows.Values()
		if e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			m[f.Name] = v[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (q *request) one(sql string, args ...any) (map[string]any, error) {
	r, e := q.rows(sql, args...)
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, notFound()
	}
	q.decorateControlRow(r[0])
	return r[0], nil
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func validID(s string) bool { _, e := uuid.Parse(s); return e == nil }
func (q *request) id() (string, error) {
	id := q.r.PathValue("id")
	if !validID(id) {
		return "", notFound()
	}
	return id, nil
}
func (q *request) checkCSRF() error {
	origin := q.r.Header.Get("Origin")
	if origin != "" {
		u, e := url.Parse(q.app.Config.PublicURL)
		if e != nil || origin != u.Scheme+"://"+u.Host {
			return failure(403, "forbidden", "Invalid request origin")
		}
	}
	token := q.r.Header.Get("X-CSRF-Token")
	c, e := q.r.Cookie("tf_csrf")
	if e != nil || token == "" || digest(token) != digest(c.Value) {
		return failure(403, "forbidden", "Invalid CSRF token")
	}
	if q.p.SessionID != "" {
		var h string
		if e = q.tx.QueryRow(q.ctx(), "SELECT csrf_hash FROM sessions WHERE id=$1", q.p.SessionID).Scan(&h); e != nil || h != digest(token) {
			return failure(403, "forbidden", "Invalid CSRF token")
		}
	}
	return nil
}
func (q *request) authenticate() error {
	c, e := q.r.Cookie("tf_session")
	if e != nil {
		return failure(401, "unauthorized", "Sign in required")
	}
	fence := ""
	if !strings.HasPrefix(q.r.URL.Path, "/api/v1/auth/") {
		fence = " FOR SHARE OF s,u"
	}
	e = q.tx.QueryRow(q.ctx(), `SELECT s.id::text,u.id::text,u.platform_admin,u.must_change_password FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`+fence, digest(c.Value)).Scan(&q.p.SessionID, &q.p.ID, &q.p.PlatformAdmin, &q.p.MustChangePassword)
	if e != nil {
		return failure(401, "unauthorized", "Sign in required")
	}
	q.app.touchSession(q.p.SessionID)
	return nil
}
func (q *request) setTenant(id string) error {
	if !validID(id) {
		return forbidden()
	}
	_, e := q.tx.Exec(q.ctx(), "SELECT set_config('app.tenant_id',$1,true)", id)
	return e
}
func (q *request) loadTenant(id string) error {
	if e := q.setTenant(id); e != nil {
		return e
	}
	q.p.TenantID = id
	if q.needsInvariantLock() {
		if e := q.lockInvariant(); e != nil {
			return e
		}
	}
	var status string
	lock := " FOR SHARE OF m,t"
	e := q.tx.QueryRow(q.ctx(), `SELECT m.id::text,coalesce(m.department_id::text,''),t.status FROM memberships m JOIN tenants t ON t.id=m.tenant_id WHERE m.tenant_id=$1 AND m.user_id=$2 AND m.status='active' `+lock, id, q.p.ID).Scan(&q.p.MemberID, &q.p.DepartmentID, &status)
	if errors.Is(e, pgx.ErrNoRows) {
		return forbidden()
	}
	if e != nil {
		return e
	}
	if status != "active" {
		return forbidden()
	}
	q.p.TenantID = id
	q.p.Grants = []Grant{}
	rs, e := q.tx.Query(q.ctx(), `SELECT r.grants FROM member_roles mr JOIN roles r ON r.id=mr.role_id AND r.tenant_id=mr.tenant_id WHERE mr.member_id=$1 AND mr.tenant_id=$2 FOR SHARE OF r`, q.p.MemberID, id)
	if e != nil {
		return e
	}
	for rs.Next() {
		var raw []byte
		if e = rs.Scan(&raw); e != nil {
			rs.Close()
			return e
		}
		var gs []Grant
		if e = json.Unmarshal(raw, &gs); e != nil {
			rs.Close()
			return e
		}
		q.p.Grants = append(q.p.Grants, gs...)
	}
	rs.Close()
	if e = rs.Err(); e != nil {
		return e
	}
	q.p.Departments = map[string]string{}
	rows, e := q.rows("SELECT id::text,coalesce(parent_id::text,'') parent_id FROM departments WHERE tenant_id=$1 ORDER BY id FOR SHARE", id)
	if e != nil {
		return e
	}
	for _, d := range rows {
		q.p.Departments[d["id"].(string)] = d["parent_id"].(string)
	}
	return nil
}
func (q *request) require(res, action string) error {
	e, err := NewOperationEnforcer(q.p.Grants)
	if err != nil {
		return err
	}
	ok, err := e.Enforce(res, action)
	if err != nil {
		return err
	}
	if !ok {
		return forbidden()
	}
	return nil
}
func (q *request) controlRow(owner, dep string) Record {
	return Record{TenantID: q.p.TenantID, OwnerID: owner, DepartmentID: dep, Data: map[string]any{}}
}
func (q *request) audit(action, res, id string, changes any) error {
	if q.p.TenantID == "" {
		return nil
	}
	_, e := q.tx.Exec(q.ctx(), `INSERT INTO audit(id,tenant_id,actor_id,action,resource,record_id,request_id,changes) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.NewString(), q.p.TenantID, nullable(q.p.ID), action, res, id, q.requestID, jsonBytes(changes))
	return e
}

type paged struct {
	Data any
	Meta map[string]int
}

func (q *request) paginate(rows []map[string]any) paged {
	page, _ := strconv.Atoi(q.r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(q.r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	total := len(rows)
	start := (page - 1) * size
	if start > total || start < 0 {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return paged{rows[start:end], map[string]int{"page": page, "page_size": size, "total": total}}
}
func (q *request) checkQuota(kind string, increment int64) error {
	var quotas []byte
	if e := q.lockInvariant(); e != nil {
		return e
	}
	if e := q.tx.QueryRow(q.ctx(), "SELECT quotas FROM tenants WHERE id=$1", q.p.TenantID).Scan(&quotas); e != nil {
		return e
	}
	limits := map[string]int64{}
	if e := json.Unmarshal(quotas, &limits); e != nil {
		return e
	}
	limit, ok := limits[kind]
	if !ok {
		return nil
	}
	var used int64
	sqls := map[string]string{"records": "SELECT count(*) FROM resources WHERE tenant_id=$1", "members": "SELECT count(*) FROM memberships WHERE tenant_id=$1", "storage_bytes": "SELECT (SELECT coalesce(sum(size),0) FROM files WHERE tenant_id=$1)+(SELECT coalesce(sum(result_bytes),0) FROM jobs WHERE tenant_id=$1)+(SELECT coalesce(sum(reservation_bytes),0) FROM storage_gc WHERE tenant_id=$1)", "jobs": "SELECT count(*) FROM jobs WHERE tenant_id=$1 AND status IN ('pending','running')"}
	s, ok := sqls[kind]
	if !ok {
		return validation("Unknown quota")
	}
	if e := q.tx.QueryRow(q.ctx(), s, q.p.TenantID).Scan(&used); e != nil {
		return e
	}
	if used+increment > limit {
		return failure(429, "rate_limited", "Tenant quota exceeded")
	}
	return nil
}

var _ = time.Second

func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		if a.Config.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		defer func() {
			if recover() != nil {
				slog.Error("HTTP operation panic (details redacted)")
				writeError(w, w.Header().Get("X-Request-ID"), failure(500, "internal", "Service operation failed"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) storage() Storage     { a.storageMu.RLock(); defer a.storageMu.RUnlock(); return a.Storage }
func (a *App) setStorage(s Storage) { a.storageMu.Lock(); defer a.storageMu.Unlock(); a.Storage = s }
