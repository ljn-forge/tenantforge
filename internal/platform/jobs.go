package platform

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"github.com/google/uuid"
)

const jobSelect = "SELECT id::text,type,status,attempts,max_attempts,error,created_at,updated_at,actor_id::text,coalesce(department_id::text,'') department_id"

func (q *request) job(id string) (map[string]any, error) {
	return q.one(jobSelect+",payload,result_key FROM jobs WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func jobPublic(row map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"id", "type", "status", "attempts", "max_attempts", "error", "created_at", "updated_at"} {
		out[key] = row[key]
	}
	return out
}
func listJobs(q *request) (any, error) {
	if e := q.require("jobs", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "jobs", "read", "j", "j.actor_id", "j.department_id", "")
	return q.sqlPage("SELECT j.id::text,j.type,j.status,j.attempts,j.max_attempts,j.error,j.created_at,j.updated_at", "FROM jobs j", where, "j.created_at DESC,j.id", b)
}
func getJob(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	r, e := q.job(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "jobs", "read", q.controlRow(r["actor_id"].(string), r["department_id"].(string))) {
		return nil, notFound()
	}
	return jobPublic(r), nil
}
func retryJob(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	r, e := q.job(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "jobs", "update", q.controlRow(r["actor_id"].(string), r["department_id"].(string))) {
		return nil, forbidden()
	}
	if r["status"] != "failed" {
		return nil, conflict()
	}
	if e = q.checkQuota("jobs", 1); e != nil {
		return nil, e
	}
	tag, e := q.tx.Exec(q.ctx(), "UPDATE jobs SET status='pending',attempts=0,error='',updated_at=now() WHERE id=$1 AND tenant_id=$2 AND status='failed'", id, q.p.TenantID)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() == 0 {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO job_leases(job_id,tenant_id,next_attempt_at) VALUES($1,$2,now()) ON CONFLICT(job_id) DO UPDATE SET lease_until=NULL,claim_token=NULL,next_attempt_at=now()", id, q.p.TenantID); e != nil {
		return nil, e
	}
	r, e = q.job(id)
	if e != nil {
		return nil, e
	}
	return jobPublic(r), q.audit("retry", "jobs", id, map[string]any{})
}
func cancelJob(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	r, e := q.job(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "jobs", "update", q.controlRow(r["actor_id"].(string), r["department_id"].(string))) {
		return nil, forbidden()
	}
	tag, e := q.tx.Exec(q.ctx(), "UPDATE jobs SET status='cancelled',updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status IN ('pending','running')", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() == 0 {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM job_leases WHERE job_id=$1 AND tenant_id=$2", id, q.p.TenantID); e != nil {
		return nil, e
	}
	r, e = q.job(id)
	if e != nil {
		return nil, e
	}
	return jobPublic(r), q.audit("cancel", "jobs", id, map[string]any{})
}

func deleteJob(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	r, e := q.job(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "jobs", "update", q.controlRow(r["actor_id"].(string), r["department_id"].(string))) {
		return nil, forbidden()
	}
	if r["status"] == "pending" || r["status"] == "running" {
		return nil, conflict()
	}
	key := stringValue(r["result_key"])
	if key != "" {
		if _, e = q.tx.Exec(q.ctx(), "INSERT INTO storage_gc(id,storage_key,tenant_id) VALUES($1,$2,$3)", uuid.NewString(), key, q.p.TenantID); e != nil {
			return nil, e
		}
	}
	tag, e := q.tx.Exec(q.ctx(), "DELETE FROM jobs WHERE tenant_id=$1 AND id=$2 AND status IN ('succeeded','failed','cancelled')", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() == 0 {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM job_leases WHERE job_id=$1", id); e != nil {
		return nil, e
	}
	return map[string]any{"deleted": true}, q.audit("delete", "jobs", id, map[string]any{})
}

type exportPayload struct {
	Module  string   `json:"module"`
	Columns []string `json:"columns"`
}

func validateColumns(module string, columns []string) error {
	m, ok := Descriptor(module)
	if module == "audit" {
		ok = true
		for _, key := range []string{"id", "actor_id", "action", "resource", "record_id", "request_id", "created_at", "changes"} {
			m.Fields = append(m.Fields, Field{Key: key})
		}
	}
	if !ok {
		return notFound()
	}
	if len(columns) == 0 || len(columns) > len(m.Fields) {
		return validation("Export columns required")
	}
	seen := map[string]bool{}
	for _, col := range columns {
		found := false
		for _, f := range m.Fields {
			if col == f.Key {
				found = true
			}
		}
		if !found || seen[col] {
			return validation("Invalid export columns")
		}
		seen[col] = true
	}
	return nil
}
func exportResources(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	var in struct {
		Columns []string `json:"columns"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	return q.enqueueExport(exportPayload{m, in.Columns})
}
func auditExport(q *request) (any, error) {
	var in struct {
		Columns []string `json:"columns"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	return q.enqueueExport(exportPayload{"audit", in.Columns})
}
func (q *request) enqueueExport(payload exportPayload) (any, error) {
	m := payload.Module
	if e := q.require(m, "export"); e != nil {
		return nil, e
	}
	if e := validateColumns(m, payload.Columns); e != nil {
		return nil, e
	}
	if e := q.checkQuota("jobs", 1); e != nil {
		return nil, e
	}
	id := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO jobs(id,tenant_id,actor_id,department_id,type,payload) VALUES($1,$2,$3,$4,'export',$5)", id, q.p.TenantID, q.p.ID, nullable(q.p.DepartmentID), jsonBytes(payload)); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO job_leases(job_id,tenant_id) VALUES($1,$2)", id, q.p.TenantID); e != nil {
		return nil, e
	}
	if e := q.audit("export", m, id, map[string]any{"columns": payload.Columns}); e != nil {
		return nil, e
	}
	j, e := q.job(id)
	if e != nil {
		return nil, e
	}
	return jobPublic(j), nil
}
func csvCell(v any) string {
	s := fmt.Sprint(v)
	if t, ok := v.(time.Time); ok {
		s = t.UTC().Format(time.RFC3339Nano)
	}
	if v == nil {
		return ""
	}
	trim := strings.TrimLeft(s, " \t\r\n")
	if trim != "" && strings.ContainsRune("=+-@", rune(trim[0])) || strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") {
		return "'" + s
	}
	return s
}
func (q *request) exportCSV(payload exportPayload) ([]byte, error) {
	if e := validateColumns(payload.Module, payload.Columns); e != nil {
		return nil, e
	}
	if e := q.require(payload.Module, "export"); e != nil {
		return nil, e
	}
	if payload.Module == "audit" {
		return q.auditCSV(payload.Columns)
	}
	b := &predicate{}
	where := b.authorized(q.p, payload.Module, "export", "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(payload.Module)
	keys := append([]string{}, payload.Columns...)
	for _, g := range q.p.Grants {
		if actionGranted(g, payload.Module, "export") {
			for _, c := range g.Conditions {
				if !slices.Contains(keys, c.Field) {
					keys = append(keys, c.Field)
				}
			}
		}
	}
	projection := []string{}
	chunks := []string{}
	for _, key := range keys {
		arg := b.arg(key)
		projection = append(projection, arg+"::text", "r.data->"+arg+"::text")
		if len(projection) == 100 {
			chunks = append(chunks, "jsonb_build_object("+strings.Join(projection, ",")+")")
			projection = nil
		}
	}
	if len(projection) > 0 {
		chunks = append(chunks, "jsonb_build_object("+strings.Join(projection, ",")+")")
	}
	queryCtx, cancel := context.WithCancel(q.ctx())
	defer cancel()
	rows, e := q.tx.Query(queryCtx, "SELECT r.tenant_id::text,r.owner_id::text,coalesce(r.department_id::text,''),("+strings.Join(chunks, " || ")+") FROM resources r WHERE "+where+" ORDER BY r.created_at DESC,r.id", b.Args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	writer.Write(payload.Columns)
	for rows.Next() {
		var tenant, owner, department string
		var raw []byte
		if e = rows.Scan(&tenant, &owner, &department, &raw); e != nil {
			return nil, e
		}
		r := Record{TenantID: tenant, OwnerID: owner, DepartmentID: department}
		if e = json.Unmarshal(raw, &r.Data); e != nil {
			return nil, e
		}
		data := FilterData(q.p, payload.Module, "export", r)
		record := []string{}
		for _, col := range payload.Columns {
			record = append(record, csvCell(data[col]))
		}
		if e = writer.Write(record); e != nil {
			return nil, e
		}
		writer.Flush()
		if buf.Len() > 16<<20 {
			cancel()
			return nil, validation("Export exceeds 16 MiB; narrow permitted scope")
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}
func downloadJob(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	j, e := q.job(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "jobs", "read", q.controlRow(j["actor_id"].(string), j["department_id"].(string))) {
		return nil, notFound()
	}
	if j["status"] != "succeeded" || j["type"] != "export" {
		return nil, conflict()
	}
	var payload exportPayload
	if e = json.Unmarshal(jsonBytes(j["payload"]), &payload); e != nil {
		return nil, e
	}
	b, e := q.exportCSV(payload)
	if e != nil {
		return nil, e
	}
	q.w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	q.w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	q.w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": payload.Module + ".csv"}))
	q.streamed = true
	_, e = q.w.Write(b)
	return nil, e
}
func parseCSVValue(f Field, s string) (any, error) {
	switch f.Type {
	case "boolean":
		v, e := strconv.ParseBool(s)
		return v, e
	case "number":
		v, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("invalid number")
		}
		return v, nil
	default:
		return s, nil
	}
}
func importResources(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	if e = q.require(m, "import"); e != nil {
		return nil, e
	}
	q.r.Body = http.MaxBytesReader(q.w, q.r.Body, (1<<20)+65536)
	if e = q.r.ParseMultipartForm(1 << 20); e != nil {
		return nil, validation("CSV import is limited to 1 MiB")
	}
	defer q.r.MultipartForm.RemoveAll()
	f, h, e := q.r.FormFile("file")
	if e != nil || h.Size > 1<<20 {
		return nil, validation("CSV file required (max 1 MiB)")
	}
	defer f.Close()
	reader := csv.NewReader(io.LimitReader(f, (1<<20)+1))
	headers, e := reader.Read()
	if e != nil {
		return nil, validation("CSV header required")
	}
	if e = validateColumns(m, headers); e != nil {
		return nil, e
	}
	module, _ := Descriptor(m)
	fields := map[string]Field{}
	for _, f := range module.Fields {
		fields[f.Key] = f
	}
	inserted := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) != len(headers) {
			return nil, validation("Invalid CSV row")
		}
		inserted++
		if inserted > 1000 {
			return nil, validation("CSV import is limited to 1000 rows")
		}
		d := map[string]any{}
		for i, key := range headers {
			if row[i] == "" && !fields[key].Required {
				continue
			}
			v, e := parseCSVValue(fields[key], row[i])
			if e != nil {
				return nil, validation("Invalid CSV field type")
			}
			d[key] = v
		}
		if _, e = q.insertResource(m, resourceInput{Data: d}, "import"); e != nil {
			return nil, e
		}
	}
	return map[string]any{"inserted": inserted}, q.audit("import", m, "", map[string]any{"inserted": inserted})
}
func (a *App) workers(ctx context.Context) {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			taskCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			a.runOneJob(taskCtx)
			a.sendOneMail(taskCtx)
			a.collectOneFile(taskCtx)
			a.DB.Exec(taskCtx, "DELETE FROM login_attempts WHERE key IN (SELECT key FROM login_attempts WHERE window_at<now()-interval '1 day' LIMIT 1000)")
			a.DB.Exec(taskCtx, "DELETE FROM oidc_flows WHERE expires_at<now()")
			a.DB.Exec(taskCtx, "DELETE FROM reset_tokens WHERE expires_at<now()")
			a.DB.Exec(taskCtx, "DELETE FROM sessions WHERE expires_at<now()")
			cancel()
		}
	}
}
func (a *App) runOneJob(ctx context.Context) {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, tenant string
	e = tx.QueryRow(ctx, `SELECT job_id::text,tenant_id::text FROM job_leases WHERE next_attempt_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &tenant)
	if e != nil {
		return
	}
	if _, e = tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenant); e != nil {
		return
	}
	var status string
	var attempts, max int
	if e = tx.QueryRow(ctx, "SELECT status,attempts,max_attempts FROM jobs WHERE id=$1 AND tenant_id=$2 FOR UPDATE", id, tenant).Scan(&status, &attempts, &max); e != nil {
		return
	}
	if status == "cancelled" || status == "succeeded" || status == "failed" || attempts >= max {
		if attempts >= max {
			tx.Exec(ctx, "UPDATE jobs SET status='failed',error='Retry limit reached',updated_at=now() WHERE id=$1 AND tenant_id=$2 AND status IN ('pending','running')", id, tenant)
		}
		tx.Exec(ctx, "DELETE FROM job_leases WHERE job_id=$1", id)
		tx.Commit(ctx)
		return
	}
	claim := randomToken()
	if _, e = tx.Exec(ctx, "UPDATE job_leases SET claim_token=$1,lease_until=now()+interval '45 seconds' WHERE job_id=$2", digest(claim), id); e != nil {
		return
	}
	if _, e = tx.Exec(ctx, "UPDATE jobs SET status='running',attempts=attempts+1,updated_at=now() WHERE id=$1 AND tenant_id=$2", id, tenant); e != nil {
		return
	}
	if e = tx.Commit(ctx); e != nil {
		return
	}
	a.executeJob(ctx, id, tenant, digest(claim))
}
func (a *App) executeJob(ctx context.Context, id, tenant, claim string) {
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ticker.C:
				var exists bool
				err := a.DB.QueryRow(execCtx, "SELECT EXISTS(SELECT FROM job_leases WHERE job_id=$1 AND claim_token=$2)", id, claim).Scan(&exists)
				if err != nil || !exists {
					cancel()
					return
				}
			}
		}
	}()
	tx, e := a.DB.Begin(execCtx)
	if e != nil {
		return
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(execCtx, "SELECT set_config('app.tenant_id',$1,true)", tenant); e != nil {
		return
	}
	var actor, status, kind string
	var raw []byte
	var attempts, max int
	if e = tx.QueryRow(execCtx, "SELECT actor_id::text,status,type,payload,attempts,max_attempts FROM jobs WHERE id=$1 AND tenant_id=$2", id, tenant).Scan(&actor, &status, &kind, &raw, &attempts, &max); e != nil || status != "running" {
		return
	}
	r, _ := http.NewRequestWithContext(execCtx, "GET", "http://worker", nil)
	q := &request{app: a, tx: tx, r: r, p: Principal{ID: actor}, requestID: id}
	e = q.loadTenant(tenant)
	var data []byte
	var payload exportPayload
	if e == nil {
		if kind != "export" {
			e = validation("Unknown job type")
		} else if e = json.Unmarshal(raw, &payload); e == nil {
			data, e = q.exportCSV(payload)
		}
	}
	key := ""
	published := false
	defer func() {
		if key != "" && !published {
			a.deleteOrQueue(key)
		}
	}()
	if e == nil {
		key = uuid.NewString()
		e = a.reserveObject(execCtx, key, tenant)
		if e == nil {
			e = a.storage().Put(execCtx, key, bytes.NewReader(data), int64(len(data)), "text/csv")
		}
	}
	tx.Rollback(context.Background())
	if execCtx.Err() != nil {
		return
	}
	finish, e2 := a.DB.Begin(ctx)
	if e2 != nil {
		return
	}
	defer finish.Rollback(ctx)
	if _, e2 = finish.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenant); e2 != nil {
		return
	}
	var valid bool
	if e2 = finish.QueryRow(ctx, "SELECT EXISTS(SELECT FROM job_leases WHERE job_id=$1 AND claim_token=$2)", id, claim).Scan(&valid); e2 != nil || !valid {
		return
	}
	r, _ = http.NewRequestWithContext(ctx, "POST", "http://worker", nil)
	q = &request{app: a, tx: finish, r: r, p: Principal{ID: actor, TenantID: tenant}, requestID: id}
	if e == nil {
		e = q.loadTenant(tenant)
		if e == nil {
			e = q.require(payload.Module, "export")
			if e == nil {
				e = q.checkQuota("storage_bytes", int64(len(data)))
			}
		}
	}
	if e != nil {
		safe := "Job execution failed"
		if errorCode(e) == "forbidden" {
			safe = "Authorization revoked"
		}
		terminal := attempts >= max || errorCode(e) == "forbidden"
		next := "pending"
		if terminal {
			next = "failed"
		}
		tag, err := finish.Exec(ctx, "UPDATE jobs SET status=$1,error=$2,updated_at=now() WHERE id=$3 AND tenant_id=$4 AND status='running' AND EXISTS(SELECT FROM job_leases WHERE job_id=$3 AND claim_token=$5)", next, safe, id, tenant, claim)
		if err != nil || tag.RowsAffected() == 0 {
			return
		}
		if terminal {
			_, e2 = finish.Exec(ctx, "DELETE FROM job_leases WHERE job_id=$1 AND claim_token=$2", id, claim)
		} else {
			_, e2 = finish.Exec(ctx, "UPDATE job_leases SET claim_token=NULL,lease_until=NULL,next_attempt_at=now()+($1*interval '5 seconds') WHERE job_id=$2 AND claim_token=$3", attempts, id, claim)
		}
		if e2 == nil {
			e2 = q.audit(next, "jobs", id, map[string]any{"type": kind})
		}
		if e2 == nil {
			finish.Commit(ctx)
		}
		return
	}
	tag, e2 := finish.Exec(ctx, "UPDATE jobs SET status='succeeded',result_key=$1,result_bytes=$5,error='',updated_at=now() WHERE id=$2 AND tenant_id=$3 AND status='running' AND EXISTS(SELECT FROM job_leases WHERE job_id=$2 AND claim_token=$4)", key, id, tenant, claim, int64(len(data)))
	if e2 == nil && tag.RowsAffected() == 1 {
		_, e2 = finish.Exec(ctx, "DELETE FROM job_leases WHERE job_id=$1 AND claim_token=$2", id, claim)
		if e2 == nil {
			e2 = q.audit("succeeded", "jobs", id, map[string]any{"type": kind})
		}
		if e2 == nil {
			_, e2 = finish.Exec(ctx, "DELETE FROM storage_gc WHERE id=$1", key)
		}
		if e2 == nil {
			e2 = finish.Commit(ctx)
			published = e2 == nil
		}
	} else {
		e2 = conflict()
	}

}
func (a *App) collectOneFile(ctx context.Context) {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, key, tenant string
	if e = tx.QueryRow(ctx, "SELECT id::text,storage_key,coalesce(tenant_id::text,'') FROM storage_gc WHERE next_attempt_at<=now() ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&id, &key, &tenant); e != nil {
		return
	}
	if e = a.collectObject(ctx, tx, id, key, tenant, 300); e != nil {
		return
	}
	tx.Commit(ctx)
}

var _ = pgx.ErrNoRows

func (q *request) auditCSV(columns []string) ([]byte, error) {
	b := &predicate{}
	where := b.authorized(q.p, "audit", "export", "a", "a.actor_id", "m.department_id", "")
	selects := []string{}
	for _, col := range columns {
		if col == "id" || col == "actor_id" {
			selects = append(selects, "a."+col+"::text")
		} else {
			selects = append(selects, "a."+col)
		}
	}
	queryCtx, cancel := context.WithCancel(q.ctx())
	defer cancel()
	rows, e := q.tx.Query(queryCtx, "SELECT "+strings.Join(selects, ",")+" FROM audit a LEFT JOIN memberships m ON m.user_id=a.actor_id AND m.tenant_id=a.tenant_id WHERE "+where+" ORDER BY a.created_at DESC,a.id", b.Args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	writer.Write(columns)
	for rows.Next() {
		vs, e := rows.Values()
		if e != nil {
			return nil, e
		}
		record := []string{}
		for i, v := range vs {
			if columns[i] == "changes" {
				v = string(jsonBytes(v))
			}
			record = append(record, csvCell(v))
		}
		if e = writer.Write(record); e != nil {
			return nil, e
		}
		writer.Flush()
		if buf.Len() > 16<<20 {
			cancel()
			return nil, validation("Export exceeds 16 MiB")
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}
