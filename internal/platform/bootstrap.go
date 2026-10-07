package platform

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

func userUsername(id string) string { return "user_" + strings.ReplaceAll(id, "-", "") }

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,39}$`)

func validUsername(s string) bool { return usernamePattern.MatchString(s) }
func (a *App) bootstrap(ctx context.Context) error {
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return failure(503, "unavailable", "Platform initialization unavailable")
	}
	defer tx.Rollback(ctx)
	var initialized bool
	if e = tx.QueryRow(ctx, "SELECT initialized FROM platform_state WHERE id=true FOR UPDATE").Scan(&initialized); e != nil {
		return e
	}
	if initialized {
		return nil
	}
	var occupied bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM users) OR EXISTS(SELECT FROM tenants)").Scan(&occupied); e != nil {
		return e
	}
	if occupied {
		return errors.New("existing platform requires initialization state repair")
	}
	username := strings.ToLower(envValue(a.Config.InitialAdminUsername, "admin"))
	password := envValue(a.Config.InitialAdminPassword, "Admin@123456")
	email := strings.ToLower(envValue(a.Config.InitialAdminEmail, "admin@tenantforge.local"))
	tenantName := envValue(a.Config.InitialTenantName, "Default Workspace")
	if !validUsername(username) || !emailValid(email) || strings.TrimSpace(tenantName) == "" {
		return validation("Initial administrator username, email and tenant configuration invalid")
	}
	h, e := hashPassword(password)
	if e != nil {
		return validation("Initial administrator password must contain at least 12 characters")
	}
	id := uuid.NewString()
	if _, e = tx.Exec(ctx, "INSERT INTO users(id,username,email,display_name,password_hash,platform_admin,must_change_password) VALUES($1,$2,$3,'Administrator',$4,true,true)", id, username, email, h); e != nil {
		return e
	}
	r, _ := http.NewRequestWithContext(ctx, "POST", "http://bootstrap", nil)
	q := &request{app: a, tx: tx, r: r, p: Principal{ID: id, PlatformAdmin: true}, requestID: uuid.NewString()}
	if _, e = q.createTenant(tenantName, "", id, nil); e != nil {
		return e
	}
	if e = q.securityEvent(id, "initial_admin_created", map[string]any{}); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE platform_state SET initialized=true WHERE id=true"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func createMember(q *request) (any, error) {
	if !q.p.PlatformAdmin {
		return nil, forbidden()
	}
	var in struct {
		Username    string   `json:"username"`
		Email       string   `json:"email"`
		DisplayName string   `json:"display_name"`
		Password    string   `json:"password"`
		Department  string   `json:"department_id"`
		Position    string   `json:"position"`
		Roles       []string `json:"role_ids"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !validUsername(in.Username) || !emailValid(in.Email) || in.DisplayName == "" || len(in.DisplayName) > 120 || len(in.Position) > 120 || in.Roles == nil {
		return nil, validation("Valid username, email, display name and role IDs are required")
	}
	if e := q.validDepartment(in.Department); e != nil {
		return nil, e
	}
	id, memberID := uuid.NewString(), uuid.NewString()
	record := q.controlRow(id, in.Department)
	if !Allowed(q.p, "members", "create", record) || !Allowed(q.p, "members", "assign", record) {
		return nil, forbidden()
	}
	if e := q.validateRolesFor(in.Roles, id, in.Department); e != nil {
		return nil, e
	}
	if e := q.checkQuota("members", 1); e != nil {
		return nil, e
	}
	h, e := hashPassword(in.Password)
	if e != nil {
		return nil, validation(e.Error())
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO users(id,username,email,display_name,password_hash,must_change_password) VALUES($1,$2,$3,$4,$5,true)", id, in.Username, in.Email, in.DisplayName, h); e != nil {
		return nil, e
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO memberships(id,tenant_id,user_id,department_id,position) VALUES($1,$2,$3,$4,$5)", memberID, q.p.TenantID, id, nullable(in.Department), in.Position); e != nil {
		return nil, e
	}
	if e = q.replaceRoles(memberID, in.Roles); e != nil {
		return nil, e
	}
	if e = q.audit("create", "members", memberID, map[string]any{"role_count": len(in.Roles)}); e != nil {
		return nil, e
	}
	if e = q.securityEvent(id, "account_created", map[string]any{}); e != nil {
		return nil, e
	}
	return q.member(memberID)
}
