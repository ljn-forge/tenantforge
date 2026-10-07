package platform

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/oauth2"
)

const userSelect = "SELECT id::text,username,email,display_name,mfa_enabled,must_change_password,created_at FROM users"

func emailValid(e string) bool {
	if len(e) > 254 || strings.ContainsAny(e, "\r\n") {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}
func setupStatus(q *request) (any, error) {
	var b bool
	e := q.tx.QueryRow(q.ctx(), "SELECT initialized FROM platform_state WHERE id=true").Scan(&b)
	return map[string]any{"initialized": b, "version": Version, "oidc_enabled": q.app.Config.OIDCIssuer != ""}, e
}
func csrf(q *request) (any, error) {
	token := randomToken()
	if cookie, e := q.r.Cookie("tf_csrf"); e == nil {
		if decoded, e := base64.RawURLEncoding.DecodeString(cookie.Value); e == nil && len(decoded) == 32 {
			token = cookie.Value
		}
	}
	if c, e := q.r.Cookie("tf_session"); e == nil {
		_, e = q.tx.Exec(q.ctx(), "UPDATE sessions SET csrf_hash=$1 WHERE token_hash=$2 AND expires_at>now()", digest(token), digest(c.Value))
		if e != nil {
			return nil, e
		}
	}
	http.SetCookie(q.w, &http.Cookie{Name: "tf_csrf", Value: token, Path: "/", HttpOnly: true, Secure: q.app.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 86400})
	return map[string]any{"csrf_token": token}, nil
}
func (q *request) newSession(userID string) error {
	token := randomToken()
	csrfToken := ""
	if c, e := q.r.Cookie("tf_csrf"); e == nil {
		csrfToken = c.Value
	}
	_, e := q.tx.Exec(q.ctx(), `INSERT INTO sessions(id,user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,now()+interval '7 days')`, uuid.NewString(), userID, digest(token), digest(csrfToken))
	if e != nil {
		return e
	}
	http.SetCookie(q.w, &http.Cookie{Name: "tf_session", Value: token, Path: "/", HttpOnly: true, Secure: q.app.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 86400})
	return nil
}
func setup(q *request) (any, error) {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		TenantName  string `json:"tenant_name"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailValid(in.Email) || strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.TenantName) == "" || len(in.DisplayName) > 120 || len(in.TenantName) > 120 {
		return nil, validation("Valid email, display name and tenant name required")
	}
	h, e := hashPassword(in.Password)
	if e != nil {
		return nil, validation(e.Error())
	}
	var initialized bool
	if e = q.tx.QueryRow(q.ctx(), "SELECT initialized FROM platform_state WHERE id=true FOR UPDATE").Scan(&initialized); e != nil {
		return nil, e
	}
	if initialized {
		return nil, conflict()
	}
	id := uuid.NewString()
	_, e = q.tx.Exec(q.ctx(), "INSERT INTO users(id,email,display_name,password_hash,platform_admin,username) VALUES($1,$2,$3,$4,true,$5)", id, in.Email, in.DisplayName, h, userUsername(id))
	if e != nil {
		return nil, e
	}
	q.p.ID = id
	q.p.PlatformAdmin = true
	if _, e = q.tx.Exec(q.ctx(), "UPDATE platform_state SET initialized=true WHERE id=true"); e != nil {
		return nil, e
	}
	if _, e = q.createTenant(in.TenantName, "", id, nil); e != nil {
		return nil, e
	}
	if e = q.newSession(id); e != nil {
		return nil, e
	}
	if e = q.securityEvent(id, "login", map[string]any{}); e != nil {
		return nil, e
	}
	return q.one(userSelect+" WHERE id=$1", id)
}
func (q *request) rateLimit(key string, max int) error {
	var n int
	err := q.app.LimitsDB.QueryRow(q.ctx(), `INSERT INTO login_attempts(key,attempts) VALUES($1,1) ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN login_attempts.window_at<now()-interval '15 minutes' THEN 1 ELSE login_attempts.attempts+1 END,window_at=CASE WHEN login_attempts.window_at<now()-interval '15 minutes' THEN now() ELSE login_attempts.window_at END RETURNING attempts`, digest(key)).Scan(&n)
	if err != nil {
		return err
	}
	if n > max {
		return failure(429, "rate_limited", "Too many attempts; try again later")
	}
	return nil
}
func login(q *request) (any, error) {
	var in struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
		Recovery string `json:"recovery_code"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Username != "" {
		in.Email = strings.ToLower(strings.TrimSpace(in.Username))
	}
	if e := q.rateLimit("login-ip:"+clientIP(q.r), 100); e != nil {
		return nil, e
	}
	if e := q.rateLimit("login:"+in.Email, 20); e != nil {
		return nil, e
	}
	var id, h string
	var mfa bool
	e := q.tx.QueryRow(q.ctx(), "SELECT id::text,password_hash,mfa_enabled FROM users WHERE email=$1 OR username=$1 FOR UPDATE", in.Email).Scan(&id, &h, &mfa)
	if e != nil || !verifyPassword(h, in.Password) {
		return nil, failure(401, "unauthorized", "Invalid credentials")
	}
	if mfa {
		if in.TOTP == "" && in.Recovery == "" {
			return nil, failure(403, "mfa_required", "Second factor required")
		}
		if e = q.verifyFactor(id, in.TOTP, in.Recovery); e != nil {
			return nil, e
		}
	}
	if e = q.newSession(id); e != nil {
		return nil, e
	}
	if e = q.securityEvent(id, "login", map[string]any{}); e != nil {
		return nil, e
	}
	return q.one(userSelect+" WHERE id=$1", id)
}
func logout(q *request) (any, error) {
	_, e := q.tx.Exec(q.ctx(), "DELETE FROM sessions WHERE id=$1", q.p.SessionID)
	http.SetCookie(q.w, &http.Cookie{Name: "tf_session", Path: "/", Value: "", HttpOnly: true, Secure: q.app.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if e == nil {
		e = q.securityEvent(q.p.ID, "session_revoked", map[string]any{})
	}
	return map[string]any{"revoked": true}, e
}
func ownTenants(q *request) ([]map[string]any, error) {
	return q.rows(`SELECT t.id::text,t.name,t.slug,t.status,t.created_at,t.quotas FROM tenants t JOIN memberships m ON m.tenant_id=t.id WHERE m.user_id=$1 AND m.status='active' ORDER BY t.created_at`, q.p.ID)
}
func tenants(q *request) (any, error) {
	b := &predicate{}
	where := "m.user_id=" + b.arg(q.p.ID) + "::uuid AND m.status='active'"
	if search := q.r.URL.Query().Get("q"); search != "" {
		where += " AND t.name ILIKE " + b.arg("%"+search+"%")
	}
	return q.sqlPage("SELECT t.id::text,t.name,t.slug,t.status,t.created_at,t.quotas", "FROM tenants t JOIN memberships m ON m.tenant_id=t.id", where, "t.created_at,t.id", b)
}
func me(q *request) (any, error) {
	u, e := q.one(userSelect+" WHERE id=$1", q.p.ID)
	if e != nil {
		return nil, e
	}
	ts, e := ownTenants(q)
	if e != nil {
		return nil, e
	}
	gs := []Grant{}
	if id := q.r.Header.Get("X-Tenant-ID"); id != "" {
		if e = q.loadTenant(id); e != nil {
			return nil, e
		}
		gs = q.p.Grants
	}
	return map[string]any{"user": u, "tenants": ts, "permissions": gs, "modules": registry(), "platform_admin": q.p.PlatformAdmin}, nil
}
func sessions(q *request) (any, error) {
	b := &predicate{}
	where := "s.user_id=" + b.arg(q.p.ID) + "::uuid AND s.expires_at>now()"
	current := b.arg(q.p.SessionID)
	where += " AND " + current + "::uuid IS NOT NULL"
	return q.sqlPage("SELECT s.id::text,s.created_at,s.last_seen_at,s.expires_at,(s.id="+current+"::uuid) AS current", "FROM sessions s", where, "s.created_at DESC,s.id", b)
}
func revokeSession(q *request) (any, error) {
	id, e := q.id()
	if e != nil {
		return nil, e
	}
	tag, e := q.tx.Exec(q.ctx(), "DELETE FROM sessions WHERE id=$1 AND user_id=$2", id, q.p.ID)
	if e == nil && tag.RowsAffected() == 0 {
		return nil, notFound()
	}
	if e == nil {
		e = q.securityEvent(q.p.ID, "session_revoked", map[string]any{})
	}
	return map[string]any{"revoked": true}, e
}
func passwordChange(q *request) (any, error) {
	if e := q.rateLimit("password-change:"+q.p.ID, 10); e != nil {
		return nil, e
	}
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
		TOTP    string `json:"totp"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	var h string
	var mfa bool
	if e := q.tx.QueryRow(q.ctx(), "SELECT password_hash,mfa_enabled FROM users WHERE id=$1 FOR UPDATE", q.p.ID).Scan(&h, &mfa); e != nil {
		return nil, e
	}
	if !verifyPassword(h, in.Current) {
		return nil, failure(401, "unauthorized", "Invalid credentials")
	}
	if mfa {
		if e := q.verifyFactor(q.p.ID, in.TOTP, ""); e != nil {
			return nil, e
		}
	}
	if q.p.MustChangePassword && verifyPassword(h, in.New) {
		return nil, validation("Choose a different password from the initial password")
	}
	newHash, e := hashPassword(in.New)
	if e != nil {
		return nil, validation(e.Error())
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE users SET password_hash=$1,must_change_password=false WHERE id=$2", newHash, q.p.ID); e != nil {
		return nil, e
	}
	_, e = q.tx.Exec(q.ctx(), "DELETE FROM sessions WHERE user_id=$1 AND id<>$2", q.p.ID, q.p.SessionID)
	if e == nil {
		e = q.securityEvent(q.p.ID, "password_changed", map[string]any{})
	}
	return map[string]any{"changed": true}, e
}
func resetRequest(q *request) (any, error) {
	var in struct {
		Email string `json:"email"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if e := q.rateLimit("reset-ip:"+clientIP(q.r), 20); e != nil {
		return nil, e
	}
	if e := q.rateLimit("reset:"+in.Email, 5); e != nil {
		return nil, e
	}
	var id, email string
	e := q.tx.QueryRow(q.ctx(), "SELECT id::text,email FROM users WHERE email=$1", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&id, &email)
	if e == pgx.ErrNoRows {
		return map[string]any{"sent": true}, nil
	}
	if e != nil {
		return nil, e
	}
	token := randomToken()
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM reset_tokens WHERE user_id=$1", id); e != nil {
		return nil, e
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO reset_tokens(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '30 minutes')", digest(token), id); e != nil {
		return nil, e
	}
	if e = q.enqueueMail(email, "Password reset", "Open "+strings.TrimRight(q.app.Config.PublicURL, "/")+"/reset?token="+token); e != nil {
		return nil, e
	}
	return map[string]any{"sent": true}, nil
}
func resetConfirm(q *request) (any, error) {
	var in struct {
		Token string `json:"token"`
		New   string `json:"new_password"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if e := q.rateLimit("reset-confirm:"+clientIP(q.r), 20); e != nil {
		return nil, e
	}
	h, e := hashPassword(in.New)
	if e != nil {
		return nil, validation(e.Error())
	}
	var id string
	if e = q.tx.QueryRow(q.ctx(), "DELETE FROM reset_tokens WHERE token_hash=$1 AND expires_at>now() RETURNING user_id::text", digest(in.Token)).Scan(&id); e != nil {
		return nil, validation("Invalid or expired reset token")
	}
	var currentHash string
	var mustChange bool
	if e = q.tx.QueryRow(q.ctx(), "SELECT password_hash,must_change_password FROM users WHERE id=$1 FOR UPDATE", id).Scan(&currentHash, &mustChange); e != nil {
		return nil, e
	}
	if mustChange && verifyPassword(currentHash, in.New) {
		return nil, validation("Choose a different password from the initial password")
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE users SET password_hash=$1,must_change_password=false WHERE id=$2", h, id); e != nil {
		return nil, e
	}
	_, e = q.tx.Exec(q.ctx(), "DELETE FROM sessions WHERE user_id=$1", id)
	if e == nil {
		e = q.securityEvent(id, "password_reset", map[string]any{})
	}
	return map[string]any{"changed": true}, e
}
func (q *request) verifyFactor(id, code, recovery string) error {
	var secretCipher string
	var last int64
	if e := q.tx.QueryRow(q.ctx(), "SELECT mfa_secret,last_totp FROM users WHERE id=$1 FOR UPDATE", id).Scan(&secretCipher, &last); e != nil {
		return e
	}
	if recovery != "" {
		tag, e := q.tx.Exec(q.ctx(), "DELETE FROM recovery_codes WHERE user_id=$1 AND code_hash=$2", id, digest(recovery))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return failure(403, "mfa_required", "Invalid second factor")
		}
		return nil
	}
	secret, e := q.app.Config.open(secretCipher)
	if e != nil {
		return failure(403, "mfa_required", "Invalid second factor")
	}
	now := time.Now().UTC()
	step := now.Unix() / 30
	validStep := int64(-1)
	for offset := int64(-1); offset <= 1; offset++ {
		s := step + offset
		if s <= last {
			continue
		}
		ok, e := totp.ValidateCustom(code, secret, time.Unix(s*30, 0), totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if e == nil && ok {
			validStep = s
			break
		}
	}
	if validStep < 0 {
		return failure(403, "mfa_required", "Invalid or already used second factor")
	}
	_, e = q.tx.Exec(q.ctx(), "UPDATE users SET last_totp=$1 WHERE id=$2", validStep, id)
	return e
}
func mfaEnroll(q *request) (any, error) {
	if e := q.rateLimit("mfa-enroll:"+q.p.ID, 10); e != nil {
		return nil, e
	}
	var in struct {
		Password string `json:"password"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	var hash, email string
	var enabled bool
	if e := q.tx.QueryRow(q.ctx(), "SELECT password_hash,email,mfa_enabled FROM users WHERE id=$1 FOR UPDATE", q.p.ID).Scan(&hash, &email, &enabled); e != nil {
		return nil, e
	}
	if enabled {
		return nil, conflict()
	}
	if !verifyPassword(hash, in.Password) {
		return nil, failure(401, "unauthorized", "Invalid credentials")
	}
	key, e := totp.Generate(totp.GenerateOpts{Issuer: "TenantForge", AccountName: email, Period: 30, SecretSize: 20})
	if e != nil {
		return nil, e
	}
	sealed, e := q.app.Config.seal(key.Secret())
	if e != nil {
		return nil, e
	}
	_, e = q.tx.Exec(q.ctx(), "UPDATE users SET mfa_pending=$1 WHERE id=$2", sealed, q.p.ID)
	if e == nil {
		e = q.securityEvent(q.p.ID, "mfa_enrollment_started", map[string]any{})
	}
	return map[string]any{"secret": key.Secret(), "otpauth_url": key.URL()}, e
}
func mfaConfirm(q *request) (any, error) {
	var in struct {
		TOTP string `json:"totp"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if e := q.rateLimit("mfa-confirm:"+q.p.ID, 10); e != nil {
		return nil, e
	}
	var pending string
	var enabled bool
	if e := q.tx.QueryRow(q.ctx(), "SELECT mfa_pending,mfa_enabled FROM users WHERE id=$1 FOR UPDATE", q.p.ID).Scan(&pending, &enabled); e != nil {
		return nil, e
	}
	if enabled || pending == "" {
		return nil, conflict()
	}
	secret, e := q.app.Config.open(pending)
	if e != nil || !totp.Validate(in.TOTP, secret) {
		return nil, failure(403, "mfa_required", "Invalid second factor")
	}
	if _, e = q.tx.Exec(q.ctx(), "UPDATE users SET mfa_secret=mfa_pending,mfa_pending='',mfa_enabled=true,last_totp=-1 WHERE id=$1", q.p.ID); e != nil {
		return nil, e
	}
	codes := []string{}
	if _, e = q.tx.Exec(q.ctx(), "DELETE FROM recovery_codes WHERE user_id=$1", q.p.ID); e != nil {
		return nil, e
	}
	for i := 0; i < 10; i++ {
		code := randomToken()[:16]
		codes = append(codes, code)
		if _, e = q.tx.Exec(q.ctx(), "INSERT INTO recovery_codes(user_id,code_hash) VALUES($1,$2)", q.p.ID, digest(code)); e != nil {
			return nil, e
		}
	}
	return map[string]any{"recovery_codes": codes}, q.securityEvent(q.p.ID, "mfa_enabled", map[string]any{"recovery_codes_count": len(codes)})
}
func mfaDisable(q *request) (any, error) {
	var in struct {
		Password string `json:"password"`
		TOTP     string `json:"totp"`
		Recovery string `json:"recovery_code"`
	}
	if e := q.decode(&in); e != nil {
		return nil, e
	}
	if e := q.rateLimit("mfa-disable:"+q.p.ID, 10); e != nil {
		return nil, e
	}
	var h string
	if e := q.tx.QueryRow(q.ctx(), "SELECT password_hash FROM users WHERE id=$1", q.p.ID).Scan(&h); e != nil {
		return nil, e
	}
	if !verifyPassword(h, in.Password) {
		return nil, failure(401, "unauthorized", "Invalid credentials")
	}
	if e := q.verifyFactor(q.p.ID, in.TOTP, in.Recovery); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "UPDATE users SET mfa_enabled=false,mfa_secret='',mfa_pending='',last_totp=-1 WHERE id=$1", q.p.ID); e != nil {
		return nil, e
	}
	if _, e := q.tx.Exec(q.ctx(), "DELETE FROM recovery_codes WHERE user_id=$1", q.p.ID); e != nil {
		return nil, e
	}
	_, e := q.tx.Exec(q.ctx(), "DELETE FROM sessions WHERE user_id=$1 AND id<>$2", q.p.ID, q.p.SessionID)
	if e == nil {
		e = q.securityEvent(q.p.ID, "mfa_disabled", map[string]any{})
	}
	return map[string]any{"disabled": true}, e
}
func (q *request) oidcConfig() (*oidc.Provider, *oauth2.Config, error) {
	c := q.app.Config
	if c.OIDCIssuer == "" {
		return nil, nil, failure(404, "not_found", "OIDC is not configured")
	}
	q.r = q.r.WithContext(oidc.ClientContext(q.ctx(), &http.Client{Timeout: c.UpstreamTimeout}))
	provider, e := oidc.NewProvider(q.ctx(), c.OIDCIssuer)
	if e != nil {
		return nil, nil, failure(503, "unavailable", "Identity provider unavailable")
	}
	return provider, &oauth2.Config{ClientID: c.OIDCClientID, ClientSecret: c.OIDCClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimRight(c.PublicURL, "/") + "/api/v1/auth/oidc/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}, nil
}
func oidcStart(q *request) (any, error) {
	if e := q.release(); e != nil {
		return nil, e
	}
	_, config, e := q.oidcConfig()
	if e != nil {
		return nil, e
	}
	if e = q.reopen(); e != nil {
		return nil, e
	}
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	sealed, e := q.app.Config.seal(verifier)
	if e != nil {
		return nil, e
	}
	if _, e = q.tx.Exec(q.ctx(), "INSERT INTO oidc_flows(state_hash,nonce,verifier_cipher,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')", digest(state), nonce, sealed); e != nil {
		return nil, e
	}
	http.SetCookie(q.w, &http.Cookie{Name: "tf_oidc", Value: state, Path: "/api/v1/auth/oidc", HttpOnly: true, Secure: q.app.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	q.streamed = true
	http.Redirect(q.w, q.r, config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
	return nil, nil
}
func oidcCallback(q *request) (any, error) {
	state := q.r.URL.Query().Get("state")
	c, e := q.r.Cookie("tf_oidc")
	if e != nil || state == "" || digest(c.Value) != digest(state) {
		return nil, failure(401, "unauthorized", "Invalid identity flow")
	}
	var nonce, sealed string
	if e = q.tx.QueryRow(q.ctx(), "DELETE FROM oidc_flows WHERE state_hash=$1 AND expires_at>now() RETURNING nonce,verifier_cipher", digest(state)).Scan(&nonce, &sealed); e != nil {
		return nil, failure(401, "unauthorized", "Expired identity flow")
	}
	if e = q.release(); e != nil {
		return nil, e
	}
	verifier, e := q.app.Config.open(sealed)
	if e != nil {
		return nil, e
	}
	provider, config, e := q.oidcConfig()
	if e != nil {
		return nil, e
	}
	token, e := config.Exchange(q.ctx(), q.r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if e != nil {
		return nil, failure(401, "unauthorized", "Identity exchange rejected")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, failure(401, "unauthorized", "Identity token missing")
	}
	verified, e := provider.Verifier(&oidc.Config{ClientID: config.ClientID}).Verify(q.ctx(), raw)
	if e != nil || verified.Nonce != nonce {
		return nil, failure(401, "unauthorized", "Identity token rejected")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if e = verified.Claims(&claims); e != nil || !claims.EmailVerified || !emailValid(claims.Email) {
		return nil, failure(401, "unauthorized", "Verified email required")
	}
	if e = q.reopen(); e != nil {
		return nil, e
	}
	claims.Email = strings.ToLower(claims.Email)
	var userID string
	e = q.tx.QueryRow(q.ctx(), "SELECT user_id::text FROM oidc_identities WHERE issuer=$1 AND subject=$2", verified.Issuer, verified.Subject).Scan(&userID)
	if e == pgx.ErrNoRows {
		var existing string
		err := q.tx.QueryRow(q.ctx(), "SELECT id::text FROM users WHERE email=$1", claims.Email).Scan(&existing)
		if err == nil {
			if e = q.authenticate(); e != nil || q.p.ID != existing {
				return nil, failure(409, "conflict", "Sign in with password before linking this identity")
			}
			userID = existing
		} else if err == pgx.ErrNoRows {
			userID = uuid.NewString()
			if claims.Name == "" {
				claims.Name = claims.Email
			}
			if _, e = q.tx.Exec(q.ctx(), "INSERT INTO users(id,email,display_name,username) VALUES($1,$2,$3,$4)", userID, claims.Email, claims.Name, userUsername(userID)); e != nil {
				return nil, e
			}
		} else {
			return nil, err
		}
		if _, e = q.tx.Exec(q.ctx(), "INSERT INTO oidc_identities(issuer,subject,user_id) VALUES($1,$2,$3)", verified.Issuer, verified.Subject, userID); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	var mfa bool
	if e = q.tx.QueryRow(q.ctx(), "SELECT mfa_enabled FROM users WHERE id=$1", userID).Scan(&mfa); e != nil {
		return nil, e
	}
	if mfa {
		return nil, failure(403, "mfa_required", "Use password and second factor for this account")
	}
	if e = q.securityEvent(userID, "oidc_login", map[string]any{}); e != nil {
		return nil, e
	}
	if e = q.newSession(userID); e != nil {
		return nil, e
	}
	http.SetCookie(q.w, &http.Cookie{Name: "tf_oidc", Path: "/api/v1/auth/oidc", MaxAge: -1, HttpOnly: true})
	q.streamed = true
	http.Redirect(q.w, q.r, "/", http.StatusFound)
	return nil, nil
}
func (q *request) enqueueMail(to, subject, body string) error {
	if !emailValid(to) || strings.ContainsAny(subject, "\r\n") {
		return validation("Invalid mail address")
	}
	b, _ := json.Marshal(map[string]string{"subject": subject, "body": body})
	cipher, e := q.app.Config.seal(string(b))
	if e != nil {
		return e
	}
	_, e = q.tx.Exec(q.ctx(), "INSERT INTO mail_outbox(id,recipient,payload_cipher) VALUES($1,$2,$3)", uuid.NewString(), to, cipher)
	return e
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (q *request) securityEvent(user, action string, changes any) error {
	_, e := q.tx.Exec(q.ctx(), "INSERT INTO security_events(id,user_id,action,request_id,changes) VALUES($1,$2,$3,$4,$5)", uuid.NewString(), user, action, q.requestID, jsonBytes(changes))
	return e
}
func securityEvents(q *request) (any, error) {
	b := &predicate{}
	where := "s.user_id=" + b.arg(q.p.ID) + "::uuid"
	return q.sqlPage("SELECT s.id::text,s.action,s.request_id,s.changes,s.created_at", "FROM security_events s", where, "s.created_at DESC,s.id", b)
}
