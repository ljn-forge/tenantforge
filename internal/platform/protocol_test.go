package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

func TestS3SignedStorageProtocol(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	signed := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			w.WriteHeader(403)
			return
		}
		signed++
		switch r.Method {
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			objects[r.URL.Path] = b
			w.Header().Set("ETag", `"testetag"`)
		case "GET":
			b, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write(b)
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	storage, e := NewStorage(ctx, Config{StorageBackend: "s3", S3Endpoint: server.URL, S3Region: "us-east-1", S3Bucket: "testbucket", S3AccessKey: "test-only-key", S3SecretKey: "test-only-secret"})
	if e != nil {
		t.Fatal(e)
	}
	key := uuid.NewString()
	if e = storage.Put(ctx, key, bytes.NewReader([]byte("hello")), 5, "text/plain"); e != nil {
		t.Fatal("S3 put failed")
	}
	r, e := storage.Open(ctx, key)
	if e != nil {
		t.Fatal("S3 get failed")
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "hello" {
		t.Fatal("S3 content mismatch")
	}
	if e = storage.Delete(ctx, key); e != nil {
		t.Fatal("S3 delete failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if signed != 3 {
		t.Fatal("unsigned S3 operation")
	}
}
func TestOIDCVerifiedIssuerNoncePKCEAndBinding(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	jwk := jose.JSONWebKey{Key: key, KeyID: "test-key", Algorithm: "RS256", Use: "sig"}
	signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jwk}, nil)
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	nonce, challenge, mode := "", "", ""
	var provider *httptest.Server
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(w, 200, map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/jwks", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			writeJSON(w, 200, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk.Public()}})
		case "/token":
			r.ParseForm()
			mu.Lock()
			n, c, m := nonce, challenge, mode
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != c {
				writeJSON(w, 400, map[string]any{"error": "invalid_grant"})
				return
			}
			issuer, audience := provider.URL, "test-client"
			if m == "nonce" {
				n = "wrong"
			}
			if m == "issuer" {
				issuer = "http://wrong-issuer.test"
			}
			if m == "audience" {
				audience = "other-client"
			}
			token, err := jwt.Signed(signer).Claims(jwt.Claims{Issuer: issuer, Subject: "provider-subject", Audience: jwt.Audience{audience}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}).Claims(map[string]any{"nonce": n, "email": "owner@example.test", "email_verified": true, "name": "Owner"}).Serialize()
			if err != nil {
				w.WriteHeader(500)
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "fake-token-for-test", "token_type": "Bearer", "expires_in": 3600, "id_token": token})
		default:
			w.WriteHeader(404)
		}
	}))
	defer provider.Close()
	f := newFixtureWithConfig(t, func(c *Config) {
		c.OIDCIssuer = provider.URL
		c.OIDCClientID = "test-client"
		c.OIDCClientSecret = "test-only-secret"
	})
	client := &http.Client{Jar: f.client.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := func(m string) string {
		t.Helper()
		resp, e := client.Get(f.server.URL + "/api/v1/auth/oidc/start")
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 302 {
			t.Fatal("OIDC start failed")
		}
		location, e := url.Parse(resp.Header.Get("Location"))
		if e != nil {
			t.Fatal(e)
		}
		params := location.Query()
		if params.Get("code_challenge_method") != "S256" {
			t.Fatal("PKCE missing")
		}
		mu.Lock()
		nonce = params.Get("nonce")
		challenge = params.Get("code_challenge")
		mode = m
		mu.Unlock()
		return params.Get("state")
	}
	callback := func(state string, want int) {
		t.Helper()
		resp, e := client.Get(f.server.URL + "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(state) + "&code=test-code")
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("OIDC callback got %d want %d", resp.StatusCode, want)
		}
	}
	state := start("")
	callback("tampered-state", 401)
	callback(state, 302)
	callback(state, 401)
	for _, m := range []string{"nonce", "issuer", "audience"} {
		state = start(m)
		callback(state, 401)
	}
	var count int
	if e = f.app.DB.QueryRow(context.Background(), "SELECT count(*) FROM oidc_identities WHERE user_id=$1", f.user).Scan(&count); e != nil || count != 1 {
		t.Fatal("issuer/subject binding missing")
	}
}
func TestSMTPOutboxDeliveredToLocalMailpit(t *testing.T) {
	f := newFixtureWithConfig(t, func(c *Config) {
		c.SMTPHost = "127.0.0.1"
		c.SMTPPort = "1039"
		c.SMTPFrom = "tenantforge@example.test"
	})
	f.call("POST", "/auth/reset/request", map[string]any{"email": "owner@example.test"}, 200)
	deadline := time.Now().Add(4 * time.Second)
	sent := false
	for time.Now().Before(deadline) {
		var status string
		if e := f.app.DB.QueryRow(context.Background(), "SELECT status FROM mail_outbox ORDER BY created_at DESC LIMIT 1").Scan(&status); e == nil && status == "sent" {
			sent = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sent {
		t.Fatal("SMTP outbox delivery failed; isolated Mailpit localhost1039 required")
	}
	resp, e := http.Get("http://127.0.0.1:8029/api/v1/messages")
	if e != nil {
		t.Fatal("Mailpit readback unavailable")
	}
	defer resp.Body.Close()
	var messages struct {
		Messages []struct {
			Subject string `json:"Subject"`
		} `json:"messages"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&messages); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, m := range messages.Messages {
		if m.Subject == "Password reset" {
			found = true
		}
	}
	if !found {
		t.Fatal("sent reset absent in Mailpit readback")
	}
}
func TestS3RealMultipartUploadUsesSeekableBodyAndRetries(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	puts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "PUT":
			puts++
			if puts == 1 {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(503)
				w.Write([]byte(`<Error><Code>SlowDown</Code><Message>Try again</Message></Error>`))
				return
			}
			b, _ := io.ReadAll(r.Body)
			objects[r.URL.Path] = b
			w.Header().Set("ETag", `"test"`)
		case "GET":
			w.Write(objects[r.URL.Path])
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	f := newFixtureWithConfig(t, func(c *Config) {
		c.StorageBackend = "s3"
		c.S3Endpoint = server.URL
		c.S3Region = "us-east-1"
		c.S3Bucket = "test"
		c.S3AccessKey = "fake-local-key"
		c.S3SecretKey = "fake-local-secret"
	})
	row := asMap(f.multipart("/files", "real.txt", "seekable multipart body", nil, 200)["data"])
	resp := f.request("GET", "/files/"+row["id"].(string)+"/download", nil, "")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "seekable multipart body" {
		t.Fatal("actual S3 multipart upload/download failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if puts != 2 {
		t.Fatal("transient provider failure was not retried")
	}
}
func TestOIDCUpstreamWithoutHeadersIsBoundedByRequestContext(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer provider.Close()
	f := newFixtureWithConfig(t, func(c *Config) {
		c.OIDCIssuer = provider.URL
		c.OIDCClientID = "test"
		c.RequestTimeout = 200 * time.Millisecond
		c.UpstreamTimeout = time.Second
	})
	client := &http.Client{Jar: f.client.Jar, Timeout: 2 * time.Second}
	started := time.Now()
	resp, e := client.Get(f.server.URL + "/api/v1/auth/oidc/start")
	if e != nil {
		t.Fatal("handler did not return within deadline")
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || time.Since(started) > time.Second {
		t.Fatal("upstream request lacked bounded context")
	}
}
