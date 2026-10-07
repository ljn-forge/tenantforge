package platform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// release ends the current authorization/reservation phase before any external I/O.
func (q *request) release() error {
	if q.tx == nil {
		return nil
	}
	e := q.tx.Commit(q.ctx())
	q.tx = nil
	return e
}

// reopen always revalidates the cookie, CSRF, membership and grants after external work.
func (q *request) reopen() error {
	tx, e := q.app.DB.Begin(q.ctx())
	if e != nil {
		return e
	}
	q.tx = tx
	if !q.public {
		if e = q.authenticate(); e != nil {
			return e
		}
		if e = q.passwordGate(); e != nil {
			return e
		}
	}
	if q.r.Method != "GET" && q.r.Method != "HEAD" {
		if e = q.checkCSRF(); e != nil {
			return e
		}
	}
	if q.tenantRequired {
		return q.loadTenant(q.r.Header.Get("X-Tenant-ID"))
	}
	return nil
}
func (q *request) prepareBody() error {
	if q.r.Method == "GET" || q.r.Method == "HEAD" {
		return nil
	}
	if e := q.release(); e != nil {
		return e
	}
	if strings.HasPrefix(q.r.Header.Get("Content-Type"), "multipart/form-data") {
		size := int64(2 << 20)
		if q.r.URL.Path == "/api/v1/files" {
			size = maxFileSize + (1 << 20)
		}
		q.r.Body = http.MaxBytesReader(q.w, q.r.Body, size)
		if e := q.r.ParseMultipartForm(1 << 20); e != nil {
			return validation("Invalid multipart upload or file too large")
		}
	} else {
		q.r.Body = http.MaxBytesReader(q.w, q.r.Body, 1<<20)
		b, e := io.ReadAll(q.r.Body)
		if e != nil {
			return validation("Invalid request body")
		}
		q.r.Body = io.NopCloser(bytes.NewReader(b))
	}
	return q.reopen()
}
func (q *request) reserveStorage(key string, size int64) error {
	if e := q.checkQuota("storage_bytes", size); e != nil {
		return e
	}
	expires := time.Now().Add(10 * time.Minute)
	if d, ok := q.ctx().Deadline(); ok && d.Add(time.Minute).After(expires) {
		expires = d.Add(time.Minute)
	}
	_, e := q.tx.Exec(q.ctx(), "INSERT INTO storage_gc(id,storage_key,tenant_id,reservation_bytes,next_attempt_at) VALUES($1,$2,$3,$4,$5)", key, key, q.p.TenantID, size, expires)
	return e
}
func (a *App) touchSession(id string) {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	if a.activity == nil {
		a.activity = map[string]struct{}{}
	}
	if len(a.activity) < 4096 {
		a.activity[id] = struct{}{}
	}
}
func (a *App) flushSessionActivity(ctx context.Context) error {
	a.activityMu.Lock()
	ids := make([]string, 0, len(a.activity))
	for id := range a.activity {
		ids = append(ids, id)
	}
	a.activity = map[string]struct{}{}
	a.activityMu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	_, e := a.DB.Exec(ctx, "UPDATE sessions SET last_seen_at=now() WHERE id=ANY($1::uuid[])", ids)
	return e
}

// Invariants use a separate row, acquired before membership/grant fences to avoid lock upgrades.
func (q *request) lockInvariant() error {
	id := q.p.TenantID
	if id == "" {
		id = q.r.Header.Get("X-Tenant-ID")
	}
	if !validID(id) {
		return forbidden()
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO tenant_quota_locks(tenant_id) VALUES($1) ON CONFLICT DO NOTHING", id); e != nil {
		return e
	}
	_, e := q.tx.Exec(q.ctx(), "SELECT tenant_id FROM tenant_quota_locks WHERE tenant_id=$1 FOR UPDATE", id)
	return e
}
func (q *request) needsInvariantLock() bool {
	if q.r.Method == "GET" || q.r.Method == "HEAD" {
		return false
	}
	path := q.r.URL.Path
	if path == "/" {
		return true
	}
	for _, prefix := range []string{"/api/v1/members", "/api/v1/roles", "/api/v1/departments", "/api/v1/invitations"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if q.r.Method == "POST" {
		if path == "/api/v1/files" || path == "/api/v1/jobs" {
			return true
		}
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 4 && parts[2] == "resources" {
			return true
		}
		if len(parts) == 5 && parts[2] == "resources" && (parts[4] == "import" || parts[4] == "export") {
			return true
		}
		if len(parts) == 5 && parts[2] == "jobs" && parts[4] == "retry" {
			return true
		}
		if path == "/api/v1/audit/export" {
			return true
		}
	}
	return false
}

func (q *request) passwordGate() error {
	if !q.p.MustChangePassword {
		return nil
	}
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/auth/csrf", "/api/v1/auth/password", "/api/v1/auth/logout"} {
		if q.r.URL.Path == path {
			return nil
		}
	}
	return failure(403, "password_change_required", "Change the initial password before continuing")
}
