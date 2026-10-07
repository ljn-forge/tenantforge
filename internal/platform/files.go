package platform

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const maxFileSize = 32 << 20
const fileSelect = "SELECT id::text,name,size,content_type,owner_id::text,coalesce(department_id::text,'') department_id,created_at"

func (q *request) fileRows() ([]map[string]any, error) {
	return q.rows(fileSelect+" FROM files WHERE tenant_id=$1 ORDER BY created_at DESC", q.p.TenantID)
}
func listFiles(q *request) (any, error) {
	if e := q.require("files", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "files", "read", "f", "f.owner_id", "f.department_id", "")
	return q.sqlPage("SELECT f.id::text,f.name,f.size,f.content_type,f.owner_id::text,coalesce(f.department_id::text,'') department_id,f.created_at", "FROM files f", where, "f.created_at DESC,f.id", b)
}
func uploadFile(q *request) (any, error) {
	q.r.Body = http.MaxBytesReader(q.w, q.r.Body, maxFileSize+(1<<20))
	if e := q.r.ParseMultipartForm(1 << 20); e != nil {
		return nil, validation("Invalid multipart upload or file too large")
	}
	defer q.r.MultipartForm.RemoveAll()
	f, h, e := q.r.FormFile("file")
	if e != nil {
		return nil, validation("File is required")
	}
	defer f.Close()
	if h.Size > maxFileSize || h.Size < 0 {
		return nil, validation("File too large")
	}
	owner := q.r.FormValue("owner_id")
	if owner == "" {
		owner = q.p.ID
	}
	dep, e := q.ownerDepartment(owner)
	if e != nil {
		return nil, e
	}
	ownerDepartment := dep
	if value := q.r.FormValue("department_id"); value != "" {
		dep = value
	}
	if e = q.validDepartment(dep); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "files", "create", q.controlRow(owner, dep)) {
		return nil, forbidden()
	}
	if owner != q.p.ID && !CanAssignMember(q.p, q.controlRow(q.p.ID, q.p.DepartmentID), q.controlRow(owner, ownerDepartment)) {
		return nil, forbidden()
	}
	if e = q.checkQuota("storage_bytes", h.Size); e != nil {
		return nil, e
	}
	name := filepath.Base(strings.ReplaceAll(h.Filename, "\\", "/"))
	if name == "." || name == "" || len(name) > 255 || strings.ContainsAny(name, "\r\n\x00") {
		return nil, validation("Invalid filename")
	}
	head := make([]byte, 512)
	n, e := io.ReadFull(f, head)
	if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
		return nil, e
	}
	ct := http.DetectContentType(head[:n])
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return nil, failure(503, "unavailable", "Upload could not be read")
	}
	key, id := uuid.NewString(), uuid.NewString()
	if e = q.reserveStorage(key, h.Size); e != nil {
		return nil, e
	}
	q.rollback = append(q.rollback, func() { q.app.deleteOrQueue(key) })
	if e = q.release(); e != nil {
		return nil, e
	}
	if e = q.app.storage().Put(q.ctx(), key, f, h.Size, ct); e != nil {
		return nil, failure(503, "unavailable", "File storage unavailable")
	}
	if e = q.reopen(); e != nil {
		return nil, e
	}
	actualDep, e := q.ownerDepartment(owner)
	if e != nil {
		return nil, e
	}
	if e = q.validDepartment(dep); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "files", "create", q.controlRow(owner, dep)) || owner != q.p.ID && !CanAssignMember(q.p, q.controlRow(q.p.ID, q.p.DepartmentID), q.controlRow(owner, actualDep)) {
		return nil, forbidden()
	}
	if e = q.checkQuota("storage_bytes", 0); e != nil {
		return nil, e
	}
	var reserved bool
	if e = q.tx.QueryRow(q.ctx(), "SELECT EXISTS(SELECT FROM storage_gc WHERE id=$1 AND tenant_id=$2)", key, q.p.TenantID).Scan(&reserved); e != nil {
		return nil, e
	}
	if !reserved {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO files(id,tenant_id,owner_id,department_id,name,size,content_type,storage_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, q.p.TenantID, owner, nullable(dep), name, h.Size, ct, key); e != nil {
		return nil, e
	}
	if e = q.audit("create", "files", id, map[string]any{"size": h.Size}); e != nil {
		return nil, e
	}
	if e = q.consumeReservation(key); e != nil {
		return nil, e
	}
	return q.one(fileSelect+" FROM files WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func (q *request) file(id string) (map[string]any, error) {
	return q.one(fileSelect+",storage_key FROM files WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func downloadFile(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	f, e := q.file(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "files", "read", q.controlRow(f["owner_id"].(string), f["department_id"].(string))) {
		return nil, notFound()
	}
	if e = q.release(); e != nil {
		return nil, e
	}
	stream, e := q.app.storage().Open(q.ctx(), f["storage_key"].(string))
	if e != nil {
		return nil, failure(503, "unavailable", "File storage unavailable")
	}
	defer stream.Close()
	if e = q.reopen(); e != nil {
		return nil, e
	}
	f, e = q.file(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "files", "read", q.controlRow(f["owner_id"].(string), f["department_id"].(string))) {
		return nil, notFound()
	}
	if e = q.audit("download", "files", id, map[string]any{}); e != nil {
		return nil, e
	}
	if e = q.release(); e != nil {
		return nil, e
	}
	q.streamed = true
	q.w.Header().Set("Content-Type", f["content_type"].(string))
	q.w.Header().Set("Content-Length", fmt.Sprint(f["size"]))
	q.w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f["name"].(string)}))
	_, e = io.Copy(q.w, stream)
	return nil, e
}
func deleteFile(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	f, e := q.file(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "files", "delete", q.controlRow(f["owner_id"].(string), f["department_id"].(string))) {
		return nil, forbidden()
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO storage_gc(id,storage_key,tenant_id) VALUES($1,$2,$3)", uuid.NewString(), f["storage_key"], q.p.TenantID); e != nil {
		return nil, e
	}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM files WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id); e != nil {
		return nil, e
	}
	return map[string]any{"deleted": true}, q.audit("delete", "files", id, map[string]any{})
}
