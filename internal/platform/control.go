package platform

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
)

func permissions(q *request) (any, error) {
	return map[string]any{"modules": registry(), "scopes": Scopes}, nil
}
func (q *request) createTenant(name, slug, ownerID string, quotas map[string]int64) (map[string]any, error) {
	id := uuid.NewString()
	if slug == "" {
		slug = "tenant-" + id[:8]
	}
	if strings.TrimSpace(name) == "" || len(name) > 120 || len(slug) > 80 {
		return nil, validation("Tenant name and slug required")
	}
	for _, c := range slug {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return nil, validation("Slug uses lowercase letters, digits and hyphens")
		}
	}
	if quotas == nil {
		quotas = map[string]int64{"records": 100000, "members": 1000, "storage_bytes": 1 << 30, "jobs": 1000}
	}
	if e := validateQuotas(quotas); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO tenants(id,name,slug,quotas) VALUES($1,$2,$3,$4)", id, name, slug, jsonBytes(quotas)); e != nil {
		return nil, e
	}
	if e := q.setTenant(id); e != nil {
		return nil, e
	}
	q.p.TenantID = id
	roleID, memberID := uuid.NewString(), uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO roles(id,tenant_id,name,description,builtin,grants,created_by) VALUES($1,$2,'Owner','Protected tenant owner',true,$3,$4)", roleID, id, jsonBytes(ownerGrants()), q.p.ID); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO memberships(id,tenant_id,user_id) VALUES($1,$2,$3)", memberID, id, ownerID); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO member_roles(tenant_id,member_id,role_id) VALUES($1,$2,$3)", id, memberID, roleID); e != nil {
		return nil, e
	}
	if e := q.audit("create", "tenants", id, map[string]any{"name": name}); e != nil {
		return nil, e
	}
	return q.one("SELECT id::text,name,slug,status,created_at,quotas FROM tenants WHERE id=$1", id)
}
func validateQuotas(qs map[string]int64) error {
	for k, v := range qs {
		if !slices.Contains([]string{"records", "members", "storage_bytes", "jobs"}, k) || v < 0 {
			return validation("Invalid tenant quota")
		}
	}
	return nil
}
func platformTenants(q *request) (any, error) {
	if !q.p.PlatformAdmin {
		return nil, forbidden()
	}
	b := &predicate{}
	where := "true"
	if search := q.r.URL.Query().Get("q"); search != "" {
		v := b.arg("%" + search + "%")
		where = "(t.name ILIKE " + v + " OR t.slug ILIKE " + v + ")"
	}
	return q.sqlPage("SELECT t.id::text,t.name,t.slug,t.status,t.quotas,t.created_at", "FROM tenants t", where, "t.created_at,t.id", b)
}
func provisionTenant(q *request) (any, error) {
	if !q.p.PlatformAdmin {
		return nil, forbidden()
	}
	var in struct {
		Name   string           `json:"name"`
		Slug   string           `json:"slug"`
		Email  string           `json:"owner_email"`
		Quotas map[string]int64 `json:"quotas"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !emailValid(email) {
		return nil, validation("Owner email required")
	}
	ownerID, e := q.ensureInvitedUser(email)
	if e != nil {
		return nil, e
	}
	return q.createTenant(in.Name, in.Slug, ownerID, in.Quotas)
}
func (q *request) ensureInvitedUser(email string) (string, error) {
	var id string
	e := q.tx.QueryRow(q.ctx(), "SELECT id::text FROM users WHERE email=$1", email).Scan(&id)
	if e == nil {
		return id, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	id = uuid.NewString()
	h, e := hashPassword(randomToken())
	if e != nil {
		return "", e
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO users(id,email,display_name,password_hash,username) VALUES($1,$2,$2,$3,$4)", id, email, h, userUsername(id)); e != nil {
		return "", e
	}
	token := randomToken()
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO reset_tokens(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')", digest(token), id); e != nil {
		return "", e
	}
	e = q.enqueueMail(email, "Set up your TenantForge account", "Set your password: "+strings.TrimRight(q.app.Config.PublicURL, "/")+"/reset?token="+token)
	return id, e
}
func patchTenant(q *request) (any, error) {
	if !q.p.PlatformAdmin {
		return nil, forbidden()
	}
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Name   *string          `json:"name"`
		Status *string          `json:"status"`
		Quotas map[string]int64 `json:"quotas"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	if in.Status != nil && !slices.Contains([]string{"active", "suspended"}, *in.Status) {
		return nil, validation("Invalid status")
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 120) {
		return nil, validation("Invalid tenant name")
	}
	if e = validateQuotas(in.Quotas); e != nil {
		return nil, e
	}
	var quota any
	if in.Quotas != nil {
		quota = jsonBytes(in.Quotas)
	}
	tag, e := q.tx.Exec(q.ctx(), "UPDATE tenants SET name=coalesce($1,name),status=coalesce($2,status),quotas=coalesce($3,quotas) WHERE id=$4", in.Name, in.Status, quota, id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() == 0 {
		return nil, notFound()
	}
	if e = q.setTenant(id); e != nil {
		return nil, e
	}
	q.p.TenantID = id
	if e = q.audit("update", "tenants", id, map[string]any{"fields": []string{"name", "status", "quotas"}}); e != nil {
		return nil, e
	}
	return q.one("SELECT id::text,name,slug,status,quotas,created_at FROM tenants WHERE id=$1", id)
}

const memberSelect = `SELECT m.id::text,m.user_id::text,u.username,u.email,u.display_name,coalesce(m.department_id::text,'') department_id,m.position,m.status,coalesce((SELECT jsonb_agg(mr.role_id::text) FROM member_roles mr WHERE mr.member_id=m.id AND mr.tenant_id=m.tenant_id),'[]') role_ids FROM memberships m JOIN users u ON u.id=m.user_id`

func members(q *request) (any, error) {
	if e := q.require("members", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "members", "read", "m", "m.user_id", "m.department_id", "")
	if search := q.r.URL.Query().Get("q"); search != "" {
		v := b.arg("%" + search + "%")
		where += " AND (u.email ILIKE " + v + " OR u.display_name ILIKE " + v + " OR m.position ILIKE " + v + ")"
	}
	selectSQL := strings.TrimSuffix(memberSelect, " FROM memberships m JOIN users u ON u.id=m.user_id")
	return q.sqlPage(selectSQL, "FROM memberships m JOIN users u ON u.id=m.user_id", where, "u.email,m.id", b)
}
func (q *request) member(id string) (map[string]any, error) {
	return q.one(memberSelect+" WHERE m.tenant_id=$1 AND m.id=$2", q.p.TenantID, id)
}
func (q *request) validateRoles(ids []string) error {
	for _, id := range ids {
		if !validID(id) {
			return validation("Invalid role ID")
		}
		var raw []byte
		if e := q.tx.QueryRow(q.ctx(), "SELECT grants FROM roles WHERE id=$1 AND tenant_id=$2 FOR SHARE", id, q.p.TenantID).Scan(&raw); e != nil {
			return validation("Role does not belong to tenant")
		}
		var gs []Grant
		if e := json.Unmarshal(raw, &gs); e != nil {
			return e
		}
		if !CanDelegate(q.p, gs) {
			return forbidden()
		}
	}
	return nil
}
func (q *request) validDepartment(id string) error {
	if id == "" {
		return nil
	}
	if !validID(id) {
		return validation("Invalid department")
	}
	var exists bool
	if e := q.tx.QueryRow(q.ctx(), "SELECT EXISTS(SELECT FROM departments WHERE tenant_id=$1 AND id=$2)", q.p.TenantID, id).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return validation("Department does not belong to tenant")
	}
	return nil
}
func (q *request) replaceRoles(member string, roles []string) error {
	if _, e := q.tx.Exec(q.ctx(), "DELETE FROM member_roles WHERE tenant_id=$1 AND member_id=$2", q.p.TenantID, member); e != nil {
		return e
	}
	for _, id := range roles {
		if _, e := q.tx.Exec(q.ctx(), "INSERT INTO member_roles(tenant_id,member_id,role_id) VALUES($1,$2,$3)", q.p.TenantID, member, id); e != nil {
			return e
		}
	}
	return nil
}
func (q *request) guardFinalOwner(member string, newStatus string, roleIDs []string) error {
	if e := q.lockInvariant(); e != nil {
		return e
	}
	var isOwner bool
	if e := q.tx.QueryRow(q.ctx(), `SELECT EXISTS(SELECT FROM member_roles mr JOIN roles r ON r.id=mr.role_id AND r.tenant_id=mr.tenant_id WHERE mr.tenant_id=$1 AND mr.member_id=$2 AND r.builtin AND r.name='Owner')`, q.p.TenantID, member).Scan(&isOwner); e != nil {
		return e
	}
	if !isOwner {
		return nil
	}
	keeps := newStatus != "suspended"
	if roleIDs != nil {
		var ownerRole string
		if e := q.tx.QueryRow(q.ctx(), "SELECT id::text FROM roles WHERE tenant_id=$1 AND builtin AND name='Owner'", q.p.TenantID).Scan(&ownerRole); e != nil {
			return e
		}
		keeps = keeps && slices.Contains(roleIDs, ownerRole)
	}
	if keeps {
		return nil
	}
	var n int
	if e := q.tx.QueryRow(q.ctx(), `SELECT count(DISTINCT m.id) FROM memberships m JOIN member_roles mr ON mr.member_id=m.id AND mr.tenant_id=m.tenant_id JOIN roles r ON r.id=mr.role_id AND r.tenant_id=mr.tenant_id WHERE m.tenant_id=$1 AND m.status='active' AND r.builtin AND r.name='Owner' AND m.id<>$2`, q.p.TenantID, member).Scan(&n); e != nil {
		return e
	}
	if n == 0 {
		return validation("The final active tenant owner is protected")
	}
	return nil
}
func patchMember(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Department OptionalString `json:"department_id"`
		Position   *string        `json:"position"`
		Roles      []string       `json:"role_ids"`
		Status     *string        `json:"status"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	r, e := q.member(id)
	if e != nil {
		return nil, e
	}
	old := q.controlRow(r["user_id"].(string), r["department_id"].(string))
	if !Allowed(q.p, "members", "update", old) {
		return nil, forbidden()
	}
	next := old
	if in.Department.Present {
		if e = q.validDepartment(in.Department.Value); e != nil {
			return nil, e
		}
		next.DepartmentID = in.Department.Value
	}
	if !Allowed(q.p, "members", "update", next) {
		return nil, forbidden()
	}
	status := r["status"].(string)
	if in.Status != nil {
		status = *in.Status
		if !slices.Contains([]string{"active", "suspended"}, status) {
			return nil, validation("Invalid member status")
		}
	}
	if in.Position != nil && len(*in.Position) > 120 {
		return nil, validation("Position too long")
	}
	existingIDs := []string{}
	if e = json.Unmarshal(jsonBytes(r["role_ids"]), &existingIDs); e != nil {
		return nil, e
	}
	if in.Roles != nil || in.Department.Present {
		if e = q.validateRolesFor(existingIDs, old.OwnerID, old.DepartmentID); e != nil {
			return nil, e
		}
	}
	if in.Department.Present && in.Roles == nil {
		if e = q.validateRolesFor(existingIDs, old.OwnerID, next.DepartmentID); e != nil {
			return nil, e
		}
	}
	if in.Roles != nil {
		if !CanAssignMember(q.p, old, next) {
			return nil, forbidden()
		}
		if e = q.validateRolesFor(in.Roles, old.OwnerID, next.DepartmentID); e != nil {
			return nil, e
		}
	}
	if e = q.guardFinalOwner(id, status, in.Roles); e != nil {
		return nil, e
	}
	if status == "suspended" {
		var count int
		if e = q.tx.QueryRow(q.ctx(), `SELECT (SELECT count(*) FROM resources WHERE tenant_id=$1 AND owner_id=$2)+(SELECT count(*) FROM files WHERE tenant_id=$1 AND owner_id=$2)`, q.p.TenantID, old.OwnerID).Scan(&count); e != nil {
			return nil, e
		}
		if count > 0 {
			return nil, validation("Transfer owned records and files before suspending member")
		}
	}
	var dep any
	if in.Department.Present {
		dep = nullable(in.Department.Value)
	} else {
		dep = nullable(old.DepartmentID)
	}
	_, e = q.tx.Exec(q.ctx(), "UPDATE memberships SET department_id=$1,position=coalesce($2,position),status=$3 WHERE tenant_id=$4 AND id=$5", dep, in.Position, status, q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	if in.Roles != nil {
		if e = q.replaceRoles(id, in.Roles); e != nil {
			return nil, e
		}
	}
	if e = q.audit("update", "members", id, map[string]any{"fields": []string{"department_id", "position", "status", "role_ids"}}); e != nil {
		return nil, e
	}
	return q.member(id)
}
func transferMember(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Owner string `json:"owner_id"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	m, e := q.member(id)
	if e != nil {
		return nil, e
	}
	old := q.controlRow(m["user_id"].(string), m["department_id"].(string))
	if !Allowed(q.p, "members", "assign", old) {
		return nil, forbidden()
	}
	dep, e := q.ownerDepartment(in.Owner)
	if e != nil {
		return nil, e
	}
	next := q.controlRow(in.Owner, dep)
	if !Allowed(q.p, "members", "assign", next) {
		return nil, forbidden()
	}
	records, e := q.recordsLocked(old.OwnerID)
	if e != nil {
		return nil, e
	}
	recordIDs := []string{}
	for _, r := range records {
		if r.OwnerID != old.OwnerID {
			continue
		}
		recordIDs = append(recordIDs, r.ID)
		n := r
		n.OwnerID = in.Owner
		if !Allowed(q.p, r.Module, "assign", r.Record) || !Allowed(q.p, r.Module, "assign", n.Record) || !Allowed(q.p, r.Module, "update", r.Record) || !Allowed(q.p, r.Module, "update", n.Record) {
			return nil, forbidden()
		}
	}
	fs, e := q.rows(fileSelect+" FROM files WHERE tenant_id=$1 AND owner_id=$2 FOR UPDATE", q.p.TenantID, old.OwnerID)
	if e != nil {
		return nil, e
	}
	fileIDs := []string{}
	for _, f := range fs {
		if f["owner_id"].(string) == old.OwnerID {
			fileIDs = append(fileIDs, f["id"].(string))
			if !Allowed(q.p, "files", "delete", q.controlRow(old.OwnerID, stringValue(f["department_id"]))) || !Allowed(q.p, "files", "create", q.controlRow(in.Owner, stringValue(f["department_id"]))) {
				return nil, forbidden()
			}
		}
	}
	tag, e := q.tx.Exec(q.ctx(), "UPDATE resources SET owner_id=$1,version=version+1,updated_at=now() WHERE tenant_id=$2 AND id::text=ANY($3::text[])", in.Owner, q.p.TenantID, recordIDs)
	if e != nil {
		return nil, e
	}
	for _, r := range records {
		if r.Module == "tickets" && r.OwnerID == old.OwnerID {
			n := r.Record
			n.OwnerID = in.Owner
			if e = q.ticketChangeEvents(r.Record, n, nil); e != nil {
				return nil, e
			}
		}
	}
	files, e := q.tx.Exec(q.ctx(), "UPDATE files SET owner_id=$1 WHERE tenant_id=$2 AND id::text=ANY($3::text[])", in.Owner, q.p.TenantID, fileIDs)
	if e != nil {
		return nil, e
	}
	if e = q.audit("transfer", "members", id, map[string]any{"owner_id": in.Owner, "records": tag.RowsAffected(), "files": files.RowsAffected()}); e != nil {
		return nil, e
	}
	return map[string]any{"transferred": tag.RowsAffected() + files.RowsAffected()}, nil
}
func memberPermissions(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	m, e := q.member(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "members", "read", q.controlRow(m["user_id"].(string), m["department_id"].(string))) || q.require("roles", "read") != nil {
		return nil, forbidden()
	}
	rs, e := q.rows("SELECT r.grants FROM roles r JOIN member_roles mr ON mr.role_id=r.id AND mr.tenant_id=r.tenant_id WHERE mr.tenant_id=$1 AND mr.member_id=$2", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	gs := []Grant{}
	for _, r := range rs {
		var g []Grant
		if e = json.Unmarshal(jsonBytes(r["grants"]), &g); e != nil {
			return nil, e
		}
		gs = append(gs, g...)
	}
	return gs, nil
}
func departments(q *request) (any, error) {
	if e := q.require("departments", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "departments", "read", "d", "d.created_by", "d.id", "")
	if search := q.r.URL.Query().Get("q"); search != "" {
		where += " AND d.name ILIKE " + b.arg("%"+search+"%")
	}
	return q.sqlPage("SELECT d.id::text,d.name,d.parent_id::text,d.created_by::text _cap_owner", "FROM departments d", where, "d.name,d.id", b)
}
func createDepartment(q *request) (any, error) {
	var in struct {
		Name   string         `json:"name"`
		Parent OptionalString `json:"parent_id"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 {
		return nil, validation("Department name required")
	}
	parent := ""
	if in.Parent.Present {
		parent = in.Parent.Value
	}
	if e := q.validDepartment(parent); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "departments", "create", q.controlRow(q.p.ID, parent)) {
		return nil, forbidden()
	}
	id := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO departments(id,tenant_id,name,parent_id,created_by) VALUES($1,$2,$3,$4,$5)", id, q.p.TenantID, in.Name, nullable(parent), q.p.ID); e != nil {
		return nil, e
	}
	if e := q.audit("create", "departments", id, map[string]any{"name": in.Name}); e != nil {
		return nil, e
	}
	return q.one("SELECT id::text,name,parent_id::text,created_by::text _cap_owner FROM departments WHERE id=$1 AND tenant_id=$2", id, q.p.TenantID)
}
func patchDepartment(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Name   *string        `json:"name"`
		Parent OptionalString `json:"parent_id"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	depRow, e := q.departmentRecord(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "departments", "update", depRow) {
		return nil, forbidden()
	}
	if e = q.lockInvariant(); e != nil {
		return nil, e
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 120) {
		return nil, validation("Department name required")
	}
	old, e := q.one("SELECT parent_id::text FROM departments WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	parent := stringValue(old["parent_id"])
	if in.Parent.Present {
		parent = in.Parent.Value
		if e = q.validDepartment(parent); e != nil {
			return nil, e
		}
		seen := map[string]bool{}
		for p := parent; p != ""; {
			if p == id || seen[p] {
				return nil, validation("Department hierarchy cycle")
			}
			seen[p] = true
			var next string
			if e = q.tx.QueryRow(q.ctx(), "SELECT coalesce(parent_id::text,'') FROM departments WHERE tenant_id=$1 AND id=$2", q.p.TenantID, p).Scan(&next); e != nil {
				return nil, e
			}
			p = next
		}
	}
	nextPrincipal := q.p
	nextPrincipal.Departments = map[string]string{}
	for child, p := range q.p.Departments {
		nextPrincipal.Departments[child] = p
	}
	nextPrincipal.Departments[id] = parent
	if !Allowed(nextPrincipal, "departments", "update", depRow) {
		return nil, forbidden()
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE departments SET name=coalesce($1,name),parent_id=$2 WHERE tenant_id=$3 AND id=$4", in.Name, nullable(parent), q.p.TenantID, id); e != nil {
		return nil, e
	}
	if e = q.audit("update", "departments", id, map[string]any{"fields": []string{"name", "parent_id"}}); e != nil {
		return nil, e
	}
	return q.one("SELECT id::text,name,parent_id::text,created_by::text _cap_owner FROM departments WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func deleteDepartment(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	depRow, e := q.departmentRecord(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "departments", "delete", depRow) {
		return nil, forbidden()
	}
	tag, e := q.tx.Exec(q.ctx(), "DELETE FROM departments WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() == 0 {
		return nil, notFound()
	}
	return map[string]any{"deleted": true}, q.audit("delete", "departments", id, map[string]any{})
}

const roleSelect = "SELECT id::text,name,description,builtin,grants,created_by::text _cap_owner FROM roles"

func roles(q *request) (any, error) {
	if e := q.require("roles", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "roles", "read", "r", "r.created_by", "NULL::uuid", "")
	if search := q.r.URL.Query().Get("q"); search != "" {
		v := b.arg("%" + search + "%")
		where += " AND (r.name ILIKE " + v + " OR r.description ILIKE " + v + ")"
	}
	return q.sqlPage(strings.TrimSuffix(roleSelect, " FROM roles"), "FROM roles r", where, "r.builtin DESC,r.name,r.id", b)
}

type roleInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Grants      []Grant `json:"grants"`
}

func (q *request) validateRole(in roleInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || len(in.Description) > 2000 || in.Grants == nil {
		return validation("Role name and grants required")
	}
	if e := ValidateGrants(in.Grants); e != nil {
		return validation(e.Error())
	}
	for _, g := range in.Grants {
		for _, d := range g.DepartmentIDs {
			if e := q.validDepartment(d); e != nil {
				return e
			}
		}
	}
	if !CanDelegate(q.p, in.Grants) {
		return forbidden()
	}
	return nil
}
func createRole(q *request) (any, error) {
	var in roleInput
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "roles", "create", q.controlRow(q.p.ID, "")) {
		return nil, forbidden()
	}
	if e := q.validateRole(in); e != nil {
		return nil, e
	}
	return q.insertRole(in)
}
func (q *request) insertRole(in roleInput) (any, error) {
	id := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO roles(id,tenant_id,name,description,grants,created_by) VALUES($1,$2,$3,$4,$5,$6)", id, q.p.TenantID, in.Name, in.Description, jsonBytes(in.Grants), q.p.ID); e != nil {
		return nil, e
	}
	if e := q.audit("create", "roles", id, map[string]any{"name": in.Name, "grant_count": len(in.Grants)}); e != nil {
		return nil, e
	}
	return q.one(roleSelect+" WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func patchRole(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in roleInput
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	roleRow, e := q.roleRecord(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "roles", "update", roleRow) {
		return nil, forbidden()
	}
	if e = q.validateRole(in); e != nil {
		return nil, e
	}
	var builtin bool
	var raw []byte
	if e = q.tx.QueryRow(q.ctx(), "SELECT builtin,grants FROM roles WHERE tenant_id=$1 AND id=$2 FOR UPDATE", q.p.TenantID, id).Scan(&builtin, &raw); e != nil {
		return nil, e
	}
	if builtin {
		return nil, validation("Builtin roles are protected")
	}
	var old []Grant
	if e = json.Unmarshal(raw, &old); e != nil {
		return nil, e
	}
	if !CanDelegate(q.p, old) {
		return nil, forbidden()
	}
	if e = q.validateRoleRecipients(id, old); e != nil {
		return nil, e
	}
	if e = q.validateRoleRecipients(id, in.Grants); e != nil {
		return nil, e
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE roles SET name=$1,description=$2,grants=$3 WHERE tenant_id=$4 AND id=$5", in.Name, in.Description, jsonBytes(in.Grants), q.p.TenantID, id); e != nil {
		return nil, e
	}
	if e = q.audit("update", "roles", id, map[string]any{"grant_count": len(in.Grants)}); e != nil {
		return nil, e
	}
	return q.one(roleSelect+" WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
}
func deleteRole(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	roleRow, e := q.roleRecord(id)
	if e != nil {
		return nil, e
	}
	if !Allowed(q.p, "roles", "delete", roleRow) {
		return nil, forbidden()
	}
	r, e := q.one(roleSelect+" WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	if r["builtin"].(bool) {
		return nil, validation("Builtin roles are protected")
	}
	var gs []Grant
	if e = json.Unmarshal(jsonBytes(r["grants"]), &gs); e != nil {
		return nil, e
	}
	if !CanDelegate(q.p, gs) {
		return nil, forbidden()
	}
	var pending bool
	if e = q.tx.QueryRow(q.ctx(), "SELECT EXISTS(SELECT FROM invitations WHERE tenant_id=$1 AND role_ids ? $2 AND accepted_at IS NULL AND expires_at>now())", q.p.TenantID, id).Scan(&pending); e != nil {
		return nil, e
	}
	if pending {
		return nil, conflict()
	}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM roles WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id); e != nil {
		return nil, e
	}
	return map[string]any{"deleted": true}, q.audit("delete", "roles", id, map[string]any{})
}
func copyRole(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	var in struct {
		Name string `json:"name"`
	}
	if e = q.decode(&in); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "roles", "create", q.controlRow(q.p.ID, "")) {
		return nil, forbidden()
	}
	r, e := q.one(roleSelect+" WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id)
	if e != nil {
		return nil, e
	}
	var gs []Grant
	if e = json.Unmarshal(jsonBytes(r["grants"]), &gs); e != nil {
		return nil, e
	}
	ri := roleInput{in.Name, r["description"].(string), gs}
	if e = q.validateRole(ri); e != nil {
		return nil, e
	}
	return q.insertRole(ri)
}
func invitations(q *request) (any, error) {
	if e := q.require("invitations", "read"); e != nil {
		return nil, e
	}
	b := &predicate{}
	where := b.authorized(q.p, "invitations", "read", "i", "i.created_by", "i.department_id", "")
	if search := q.r.URL.Query().Get("q"); search != "" {
		where += " AND i.email ILIKE " + b.arg("%"+search+"%")
	}
	return q.sqlPage("SELECT i.id::text,i.created_by::text _cap_owner,i.email,coalesce(i.department_id::text,'') department_id,i.role_ids,i.expires_at,i.accepted_at,CASE WHEN i.accepted_at IS NOT NULL THEN 'accepted' WHEN i.expires_at<now() THEN 'expired' ELSE 'pending' END status,i.created_at", "FROM invitations i", where, "i.created_at DESC,i.id", b)
}
func createInvitation(q *request) (any, error) {
	var in struct {
		Email      string   `json:"email"`
		Department string   `json:"department_id"`
		Roles      []string `json:"role_ids"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailValid(in.Email) || in.Roles == nil {
		return nil, validation("Email and role IDs required")
	}
	if e := q.validDepartment(in.Department); e != nil {
		return nil, e
	}
	if !Allowed(q.p, "invitations", "create", q.controlRow(q.p.ID, in.Department)) {
		return nil, forbidden()
	}
	if e := q.validateRoles(in.Roles); e != nil {
		return nil, e
	}
	if e := q.checkQuota("members", 1); e != nil {
		return nil, e
	}
	recipient, e := q.ensureInvitedUser(in.Email)
	if e != nil {
		return nil, e
	}
	if e = q.validateRolesFor(in.Roles, recipient, in.Department); e != nil {
		return nil, e
	}
	id, token := uuid.NewString(), randomToken()
	if _, e := q.tx.Exec(q.ctx(), `INSERT INTO invitations(id,tenant_id,email,department_id,role_ids,token_hash,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6,now()+interval '7 days',$7)`, id, q.p.TenantID, in.Email, nullable(in.Department), jsonBytes(in.Roles), digest(token), q.p.ID); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO invitation_tokens(token_hash,tenant_id,expires_at) VALUES($1,$2,now()+interval '7 days')", digest(token), q.p.TenantID); e != nil {
		return nil, e
	}
	if e := q.enqueueMail(in.Email, "Tenant invitation", "Accept after signing in: "+strings.TrimRight(q.app.Config.PublicURL, "/")+"/invite?token="+token); e != nil {
		return nil, e
	}
	if e := q.audit("create", "invitations", id, map[string]any{}); e != nil {
		return nil, e
	}
	return q.one("SELECT id::text,created_by::text _cap_owner,email,department_id::text,role_ids,expires_at,CASE WHEN accepted_at IS NOT NULL THEN 'accepted' WHEN expires_at<now() THEN 'expired' ELSE 'pending' END status,created_at FROM invitations WHERE id=$1 AND tenant_id=$2", id, q.p.TenantID)
}
func acceptInvitation(q *request) (any, error) {
	var in struct {
		Token string `json:"token"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	var tenantID string
	// Invitation lookup is intentionally narrow: an unguessable hashed token is resolved by a SECURITY DEFINER function.
	if e := q.tx.QueryRow(q.ctx(), "SELECT resolve_invitation($1)", digest(in.Token)).Scan(&tenantID); e != nil || tenantID == "" {
		return nil, validation("Invalid or expired invitation")
	}
	if e := q.setTenant(tenantID); e != nil {
		return nil, e
	}
	q.p.TenantID = tenantID
	if e := q.lockInvariant(); e != nil {
		return nil, e
	}
	var id, email, dep, inviter string
	var rolesRaw []byte
	var expires time.Time
	var accepted *time.Time
	if e := q.tx.QueryRow(q.ctx(), "SELECT id::text,email,coalesce(department_id::text,''),role_ids,expires_at,accepted_at,coalesce(created_by::text,'') FROM invitations WHERE token_hash=$1 AND tenant_id=$2 FOR UPDATE", digest(in.Token), tenantID).Scan(&id, &email, &dep, &rolesRaw, &expires, &accepted, &inviter); e != nil || accepted != nil || expires.Before(time.Now()) {
		return nil, validation("Invalid or expired invitation")
	}
	var actualEmail, status string
	var verified bool
	if e := q.tx.QueryRow(q.ctx(), "SELECT email,email_verified FROM users WHERE id=$1", q.p.ID).Scan(&actualEmail, &verified); e != nil {
		return nil, e
	}
	if actualEmail != email || !verified {
		return nil, forbidden()
	}
	if e := q.tx.QueryRow(q.ctx(), "SELECT status FROM tenants WHERE id=$1 FOR SHARE", tenantID).Scan(&status); e != nil || status != "active" {
		return nil, forbidden()
	}
	if e := q.checkQuota("members", 1); e != nil {
		return nil, e
	}
	mid := uuid.NewString()
	if _, e := q.tx.Exec(q.ctx(), "INSERT INTO memberships(id,tenant_id,user_id,department_id) VALUES($1,$2,$3,$4)", mid, tenantID, q.p.ID, nullable(dep)); e != nil {
		return nil, e
	}
	var roleIDs []string
	if e := json.Unmarshal(rolesRaw, &roleIDs); e != nil {
		return nil, e
	}
	recipientPrincipal := q.p
	q.p = Principal{ID: inviter}
	if e := q.loadTenant(tenantID); e != nil {
		return nil, forbidden()
	}
	if !Allowed(q.p, "invitations", "create", q.controlRow(inviter, dep)) {
		return nil, forbidden()
	}
	if e := q.validateRolesFor(roleIDs, recipientPrincipal.ID, dep); e != nil {
		return nil, e
	}
	q.p = recipientPrincipal
	q.p.TenantID = tenantID
	if e := q.replaceRoles(mid, roleIDs); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "UPDATE invitations SET accepted_at=now() WHERE tenant_id=$1 AND id=$2", tenantID, id); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "DELETE FROM invitation_tokens WHERE token_hash=$1", digest(in.Token)); e != nil {
		return nil, e
	}
	return map[string]any{"tenant_id": tenantID, "member_id": mid}, q.audit("accept", "invitations", id, map[string]any{})
}
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

var _ = fmt.Sprint

func (q *request) recordsLocked(owner string) ([]storedRecord, error) {
	rs, e := q.rows(recordSelect+" FROM resources r WHERE r.tenant_id=$1 AND r.owner_id=$2 FOR UPDATE", q.p.TenantID, owner)
	if e != nil {
		return nil, e
	}
	return decodeRecords(rs)
}

func (q *request) validateRolesFor(ids []string, target, department string) error {
	for _, id := range ids {
		var raw []byte
		if e := q.tx.QueryRow(q.ctx(), "SELECT grants FROM roles WHERE tenant_id=$1 AND id=$2 FOR SHARE", q.p.TenantID, id).Scan(&raw); e != nil {
			return validation("Role does not belong to tenant")
		}
		var gs []Grant
		if e := json.Unmarshal(raw, &gs); e != nil {
			return e
		}
		if !CanDelegateTo(q.p, gs, target, department) {
			return forbidden()
		}
	}
	return nil
}

func (q *request) departmentRecord(id string) (Record, error) {
	var owner string
	e := q.tx.QueryRow(q.ctx(), "SELECT coalesce(created_by::text,'') FROM departments WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id).Scan(&owner)
	return q.controlRow(owner, id), e
}
func (q *request) roleRecord(id string) (Record, error) {
	var owner string
	e := q.tx.QueryRow(q.ctx(), "SELECT coalesce(created_by::text,'') FROM roles WHERE tenant_id=$1 AND id=$2", q.p.TenantID, id).Scan(&owner)
	return q.controlRow(owner, ""), e
}

func (q *request) validateRoleRecipients(role string, gs []Grant) error {
	rs, e := q.rows("SELECT m.user_id::text,coalesce(m.department_id::text,'') department_id FROM memberships m JOIN member_roles mr ON mr.member_id=m.id AND mr.tenant_id=m.tenant_id WHERE m.tenant_id=$1 AND mr.role_id=$2", q.p.TenantID, role)
	if e != nil {
		return e
	}
	pending, e := q.rows("SELECT u.id::text user_id,coalesce(i.department_id::text,'') department_id FROM invitations i JOIN users u ON u.email=i.email WHERE i.tenant_id=$1 AND i.role_ids ? $2 AND i.accepted_at IS NULL AND i.expires_at>now()", q.p.TenantID, role)
	if e != nil {
		return e
	}
	rs = append(rs, pending...)
	for _, r := range rs {
		if !CanDelegateTo(q.p, gs, r["user_id"].(string), r["department_id"].(string)) {
			return forbidden()
		}
	}
	return nil
}
