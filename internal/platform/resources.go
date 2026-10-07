package platform

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type storedRecord struct {
	Record
	Module string
}

func (q *request) records(module string) ([]storedRecord, error) {
	rs, e := q.rows(`SELECT id::text,tenant_id::text,module,owner_id::text,coalesce(department_id::text,'') department_id,version,data,created_at,updated_at FROM resources WHERE tenant_id=$1 AND ($2='' OR module=$2) ORDER BY created_at DESC,id`, q.p.TenantID, module)
	if e != nil {
		return nil, e
	}
	out := []storedRecord{}
	for _, row := range rs {
		b := jsonBytes(row)
		var r storedRecord
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
func (q *request) record(module, id string, lock bool) (Record, error) {
	if !validID(id) {
		return Record{}, notFound()
	}
	sql := `SELECT id::text,tenant_id::text,owner_id::text,coalesce(department_id::text,'') department_id,version,data,created_at,updated_at FROM resources WHERE tenant_id=$1 AND module=$2 AND id=$3`
	if lock {
		sql += " FOR UPDATE"
	}
	row, e := q.one(sql, q.p.TenantID, module, id)
	if e != nil {
		return Record{}, e
	}
	var r Record
	e = json.Unmarshal(jsonBytes(row), &r)
	return r, e
}
func (q *request) module() (string, error) {
	key := q.r.PathValue("module")
	if _, ok := Descriptor(key); !ok {
		return "", notFound()
	}
	return key, nil
}
func (q *request) visible(module, action string, r Record) Record {
	r.Capabilities = capabilities(q.p, module, r)
	r.Data = FilterData(q.p, module, action, r)
	return r
}
func listResources(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	if e = q.require(m, "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, m, "read", "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(m)
	module, _ := Descriptor(m)
	if search := q.r.URL.Query().Get("q"); search != "" {
		ors := []string{}
		for _, f := range module.Fields {
			if f.Type != "string" && f.Type != "text" {
				continue
			}
			read := b.readField(q.p, m, "read", "r", "r.owner_id", "r.department_id", "r.data", f, true)
			ors = append(ors, "("+read+" AND r.data->>"+b.arg(f.Key)+" ILIKE "+b.arg("%"+search+"%")+")")
		}
		if len(ors) > 0 {
			where += " AND (" + strings.Join(ors, " OR ") + ")"
		}
	}
	for _, f := range module.Fields {
		if filter := q.r.URL.Query().Get(f.Key); filter != "" {
			where += " AND " + b.readField(q.p, m, "read", "r", "r.owner_id", "r.department_id", "r.data", f, true) + " AND r.data->>" + b.arg(f.Key) + "=" + b.arg(filter)
		}
	}
	page, e := q.sqlPage(recordSelect, "FROM resources r", where, "r.created_at DESC,r.id", b)
	if e != nil {
		return nil, e
	}
	rs, e := decodeRecords(page.Data.([]map[string]any))
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, r := range rs {
		var row map[string]any
		json.Unmarshal(jsonBytes(q.visible(m, "read", r.Record)), &row)
		out = append(out, row)
	}
	page.Data = out
	return page, nil
}
func getResource(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	r, e := q.record(m, id, false)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, m, "read", r) {
		return nil, notFound()
	}
	return q.visible(m, "read", r), nil
}

type resourceInput struct {
	Version    int            `json:"version,omitempty"`
	Owner      *string        `json:"owner_id,omitempty"`
	Department OptionalString `json:"department_id,omitempty"`
	Data       map[string]any `json:"data"`
}

func (q *request) ownerDepartment(owner string) (string, error) {
	if !validID(owner) {
		return "", validation("Invalid owner")
	}
	var dep string
	var status string
	e := q.tx.QueryRow(q.ctx(), "SELECT coalesce(m.department_id::text,''),m.status FROM memberships m WHERE m.tenant_id=$1 AND m.user_id=$2 FOR SHARE", q.p.TenantID, owner).Scan(&dep, &status)
	if e != nil || status != "active" {
		return "", validation("Owner must be an active tenant member")
	}
	return dep, nil
}
func createResource(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	var in resourceInput
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	if in.Version != 0 {
		return nil, validation("Version is assigned by server")
	}
	r, e := q.insertResource(m, in, "create")
	if e != nil {
		return nil, e
	}
	return q.visible(m, "read", r), nil
}
func (q *request) insertResource(m string, in resourceInput, action string) (Record, error) {
	if in.Data == nil {
		return Record{}, validation("Data is required")
	}
	d, e := ValidateData(m, in.Data, true)
	if e != nil {
		return Record{}, validation(e.Error())
	}
	owner := q.p.ID
	if in.Owner != nil {
		owner = *in.Owner
	}
	dep, e := q.ownerDepartment(owner)
	if e != nil {
		return Record{}, e
	}
	if in.Department.Present {
		dep = in.Department.Value
	}
	if e = q.validDepartment(dep); e != nil {
		return Record{}, e
	}
	r := Record{ID: uuid.NewString(), TenantID: q.p.TenantID, OwnerID: owner, DepartmentID: dep, Data: d}
	if !CanWrite(q.p, m, action, r, in.Data) {
		return Record{}, forbidden()
	}
	if owner != q.p.ID && !Allowed(q.p, m, "assign", r) {
		return Record{}, forbidden()
	}
	if e = q.checkQuota("records", 1); e != nil {
		return Record{}, e
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO resources(id,tenant_id,module,owner_id,department_id,data) VALUES($1,$2,$3,$4,$5,$6)", r.ID, r.TenantID, m, r.OwnerID, nullable(r.DepartmentID), jsonBytes(d)); e != nil {
		return Record{}, e
	}
	if e = q.audit(action, m, r.ID, map[string]any{"fields": safeFieldKeys(m, in.Data)}); e != nil {
		return Record{}, e
	}
	return q.record(m, r.ID, false)
}
func updateResource(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in resourceInput
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	r, e := q.updateRecord(m, id, in, "update")
	if e != nil {
		return nil, e
	}
	return q.visible(m, "read", r), nil
}
func (q *request) updateRecord(m, id string, in resourceInput, action string) (Record, error) {
	if in.Data == nil {
		return Record{}, validation("Data is required")
	}
	old, e := q.record(m, id, true)
	if e != nil {
		return Record{}, e
	}
	if !Allowed(q.p, m, action, old) {
		return Record{}, forbidden()
	}
	if old.Version != in.Version {
		return Record{}, conflict()
	}
	d, e := ValidateData(m, in.Data, false)
	if e != nil {
		return Record{}, validation(e.Error())
	}
	next := old
	next.Data = map[string]any{}
	for k, v := range old.Data {
		next.Data[k] = v
	}
	for k, v := range d {
		next.Data[k] = v
	}
	if in.Owner != nil {
		next.OwnerID = *in.Owner
		if _, e = q.ownerDepartment(next.OwnerID); e != nil {
			return Record{}, e
		}
	}
	if in.Department.Present {
		next.DepartmentID = in.Department.Value
		if e = q.validDepartment(next.DepartmentID); e != nil {
			return Record{}, e
		}
	}
	if next.OwnerID != old.OwnerID || next.DepartmentID != old.DepartmentID {
		if !Allowed(q.p, m, "assign", old) || !Allowed(q.p, m, "assign", next) {
			return Record{}, forbidden()
		}
	}
	if !CanWrite(q.p, m, action, old, d) || !CanWrite(q.p, m, action, next, d) {
		return Record{}, forbidden()
	}
	if policy, exists := policies[m]; exists && policy.Change != nil {
		if e = policy.Change(old, d); e != nil {
			return Record{}, e
		}
	}
	if _, e = q.tx.Exec(q.ctx(), `UPDATE resources SET owner_id=$1,department_id=$2,data=$3,version=version+1,updated_at=now() WHERE id=$4 AND tenant_id=$5 AND version=$6`, next.OwnerID, nullable(next.DepartmentID), jsonBytes(next.Data), id, q.p.TenantID, in.Version); e != nil {
		return Record{}, e
	}
	if e = q.audit(action, m, id, map[string]any{"fields": safeFieldKeys(m, d), "previous_version": old.Version, "version": old.Version + 1}); e != nil {
		return Record{}, e
	}
	return q.record(m, id, false)
}
func deleteResource(q *request) (any, error) {
	m, e := q.module()
	if e != nil {
		return nil, e
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	version, e := strconv.Atoi(q.r.URL.Query().Get("version"))
	if e != nil || version < 1 {
		return nil, validation("Expected version is required")
	}
	r, e := q.record(m, id, true)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, m, "delete", r) {
		return nil, forbidden()
	}
	if r.Version != version {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM resources WHERE tenant_id=$1 AND id=$2 AND version=$3", q.p.TenantID, id, version); e != nil {
		return nil, e
	}
	return map[string]any{"deleted": true}, q.audit("delete", m, id, map[string]any{"version": version})
}
func safeFieldKeys(m string, d map[string]any) []string {
	out := []string{}
	module, _ := Descriptor(m)
	for _, f := range module.Fields {
		if _, ok := d[f.Key]; ok {
			out = append(out, f.Key)
		}
	}
	return out
}
func listAudit(q *request) (any, error) {
	if e := q.require("audit", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "audit", "read", "a", "a.actor_id", "m.department_id", "")
	return q.sqlPage("SELECT a.id::text,a.actor_id::text,a.action,a.resource,a.record_id,a.request_id,a.created_at,a.changes", "FROM audit a LEFT JOIN memberships m ON m.tenant_id=a.tenant_id AND m.user_id=a.actor_id", where, "a.created_at DESC,a.id", b)
}
func dashboard(q *request) (any, error) {
	counts := map[string]int{}
	for _, m := range resourceModules() {
		b := &predicate{}
		where := b.authorized(q.p, m.Key, "read", "r", "r.owner_id", "r.department_id", "r.data") + " AND r.module=" + b.arg(m.Key)
		var count int
		if e := q.tx.QueryRow(q.ctx(), "SELECT count(*) FROM resources r WHERE "+where, b.Args...).Scan(&count); e != nil {
			return nil, e
		}
		counts[m.Key] = count
	}
	audit := []map[string]any{}
	if q.require("audit", "read") == nil {
		data, e := listAudit(q)
		if e != nil {
			return nil, e
		}
		audit = data.(paged).Data.([]map[string]any)
		if len(audit) > 10 {
			audit = audit[:10]
		}
	}
	workload, e := q.workload()
	if e != nil {
		return nil, e
	}
	return map[string]any{"counts": counts, "recent_activity": audit, "workload": workload}, nil
}
