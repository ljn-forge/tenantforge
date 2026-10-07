package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestIdentityMFARecoveryReplayAndSecurityAudit(t *testing.T) {
	f := newFixture(t)
	enroll := asMap(f.call("POST", "/auth/mfa/enroll", map[string]any{"password": "LongPassword!123"}, 200)["data"])
	secret := enroll["secret"].(string)
	code, e := totp.GenerateCode(secret, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	confirm := asMap(f.call("POST", "/auth/mfa/confirm", map[string]any{"totp": code}, 200)["data"])
	codes := confirm["recovery_codes"].([]any)
	if len(codes) != 10 {
		t.Fatal("recovery count")
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123"}, 403)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "totp": code}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "totp": code}, 403)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "recovery_code": codes[0]}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "recovery_code": codes[0]}, 403)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123", "recovery_code": codes[1]}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/mfa/disable", map[string]any{"password": "LongPassword!123", "recovery_code": codes[2]}, 200)
	events := f.call("GET", "/auth/security-events", nil, 200)["data"].([]any)
	if len(events) < 6 {
		t.Fatal("persistent security events missing")
	}
	for _, v := range events {
		b := string(jsonBytes(v))
		if strings.Contains(b, secret) || strings.Contains(b, codes[0].(string)) {
			t.Fatal("security audit secret leak")
		}
	}
}
func (f *fixture) mailLinkToken(subject string) string {
	f.t.Helper()
	var cipher string
	if e := f.app.DB.QueryRow(context.Background(), "SELECT payload_cipher FROM mail_outbox ORDER BY created_at DESC").Scan(&cipher); e != nil {
		f.t.Fatal("mail outbox unavailable")
	}
	plain, e := f.app.Config.open(cipher)
	if e != nil {
		f.t.Fatal("outbox decrypt failed")
	}
	var mail map[string]string
	if e = json.Unmarshal([]byte(plain), &mail); e != nil {
		f.t.Fatal(e)
	}
	if mail["subject"] != subject {
		f.t.Fatal("unexpected mail subject")
	}
	idx := strings.Index(mail["body"], "http")
	if idx < 0 {
		f.t.Fatal("mail link missing")
	}
	link, e := url.Parse(strings.TrimSpace(mail["body"][idx:]))
	if e != nil {
		f.t.Fatal("invalid mail link")
	}
	return link.Query().Get("token")
}
func TestIdentityResetOutboxSessionsAndCSRF(t *testing.T) {
	f := newFixture(t)
	saved := f.csrf
	f.csrf = "wrong"
	f.call("POST", "/auth/password", map[string]any{"current_password": "LongPassword!123", "new_password": "NewPassword!123"}, 403)
	f.csrf = saved
	f.call("POST", "/auth/reset/request", map[string]any{"email": "nobody@example.test"}, 200)
	out := f.call("POST", "/auth/reset/request", map[string]any{"email": "owner@example.test"}, 200)
	if strings.Contains(string(jsonBytes(out)), "token") {
		t.Fatal("reset token returned")
	}
	token := f.mailLinkToken("Password reset")
	f.call("POST", "/auth/reset/confirm", map[string]any{"token": token, "new_password": "ChangedPassword!123"}, 200)
	f.call("GET", "/auth/me", nil, 401)
	f.call("POST", "/auth/reset/confirm", map[string]any{"token": token, "new_password": "ChangedPassword!123"}, 422)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "ChangedPassword!123"}, 200)
	f.refreshCSRF()
	list := f.call("GET", "/auth/sessions", nil, 200)["data"].([]any)
	if len(list) != 1 {
		t.Fatal("password reset failed to revoke sessions")
	}
	id := asMap(list[0])["id"].(string)
	f.call("DELETE", "/auth/sessions/"+id, nil, 200)
	f.call("GET", "/auth/me", nil, 401)
}
func TestInvitationAcceptanceAndPendingRoleProtection(t *testing.T) {
	f := newFixture(t)
	user, _, _ := f.addMember("new@example.test", []Grant{}, "")
	f.sql("DELETE FROM memberships WHERE tenant_id=$1 AND user_id=$2", f.tenant, user)
	role := asMap(f.call("POST", "/roles", map[string]any{"name": "invited", "description": "", "grants": []Grant{{Resource: "tickets", Action: "read", Scope: "self", ReadFields: []string{"title"}}}}, 200)["data"])["id"].(string)
	f.call("POST", "/invitations", map[string]any{"email": "new@example.test", "role_ids": []string{role}}, 200)
	token := f.mailLinkToken("Tenant invitation")
	f.call("DELETE", "/roles/"+role, nil, 409)
	f.call("POST", "/invitations/accept", map[string]any{"token": token}, 403)
	m := f.loginAs("new@example.test")
	m.call("POST", "/invitations/accept", map[string]any{"token": token}, 200)
	m.call("POST", "/invitations/accept", map[string]any{"token": token}, 422)
}
func TestSingleConnectionIdentityRateLimitDoesNotDeadlock(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) {
		c.PoolConfig.MaxConns = 1
	})
	f.client.Timeout = time.Second
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "LongPassword!123"}, 200)
	f.refreshCSRF()
	f.call("POST", "/auth/reset/request", map[string]any{"email": " owner@example.test "}, 200)
}
func TestCSRFReadsPreserveOtherTabToken(t *testing.T) {
	f := newFixture(t)
	original := f.csrf
	f.call("GET", "/auth/csrf", nil, 200)
	f.csrf = original
	f.call("POST", "/resources/tickets", map[string]any{"data": map[string]any{"title": "same session other tab"}}, 200)
}
func TestResetLimiterNormalizesWhitespaceVariants(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		f.call("POST", "/auth/reset/request", map[string]any{"email": strings.Repeat(" ", i) + "owner@example.test" + strings.Repeat(" ", i)}, 200)
	}
	f.call("POST", "/auth/reset/request", map[string]any{"email": "\towner@example.test\t"}, 429)
}
