package platform

import (
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const Version = "1.0.0"

type Config struct {
	PoolConfig                                                                       *pgxpool.Config
	DisableBootstrap                                                                 bool
	InitialAdminUsername, InitialAdminPassword, InitialAdminEmail, InitialTenantName string
	RequestTimeout                                                                   time.Duration
	UpstreamTimeout                                                                  time.Duration
	DatabaseURL, MigrationDatabaseURL, Listen, PublicURL, DataDir, StorageBackend    string
	MasterKey                                                                        []byte
	CookieSecure                                                                     bool
	SMTPHost, SMTPPort, SMTPFrom, SMTPUser, SMTPPassword                             string
	OIDCIssuer, OIDCClientID, OIDCClientSecret                                       string
	S3Endpoint, S3Region, S3Bucket, S3AccessKey, S3SecretKey                         string
	WebDir                                                                           string
	MetricsPublic                                                                    bool
}

func FromEnv() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("TF_DATABASE_URL"), InitialAdminUsername: env("TF_INITIAL_ADMIN_USERNAME", "admin"), InitialAdminPassword: env("TF_INITIAL_ADMIN_PASSWORD", "Admin@123456"), InitialAdminEmail: env("TF_INITIAL_ADMIN_EMAIL", "admin@tenantforge.local"), InitialTenantName: env("TF_INITIAL_TENANT_NAME", "Default Workspace"), MigrationDatabaseURL: os.Getenv("TF_MIGRATION_DATABASE_URL"), Listen: env("TF_LISTEN", "127.0.0.1:8090"), PublicURL: env("TF_PUBLIC_URL", "http://localhost:8090"), DataDir: env("TF_DATA_DIR", "./data"), StorageBackend: env("TF_STORAGE_BACKEND", "local"), CookieSecure: os.Getenv("TF_COOKIE_SECURE") == "true", SMTPHost: os.Getenv("TF_SMTP_HOST"), SMTPPort: env("TF_SMTP_PORT", "25"), SMTPFrom: env("TF_SMTP_FROM", "tenantforge@localhost"), SMTPUser: os.Getenv("TF_SMTP_USER"), SMTPPassword: os.Getenv("TF_SMTP_PASSWORD"), OIDCIssuer: os.Getenv("TF_OIDC_ISSUER"), OIDCClientID: os.Getenv("TF_OIDC_CLIENT_ID"), OIDCClientSecret: os.Getenv("TF_OIDC_CLIENT_SECRET"), S3Endpoint: os.Getenv("TF_S3_ENDPOINT"), S3Region: env("TF_S3_REGION", "us-east-1"), S3Bucket: os.Getenv("TF_S3_BUCKET"), S3AccessKey: os.Getenv("TF_S3_ACCESS_KEY"), S3SecretKey: os.Getenv("TF_S3_SECRET_KEY"), WebDir: env("TF_WEB_DIR", "web/dist"), MetricsPublic: os.Getenv("TF_METRICS_PUBLIC") == "true"}
	k, err := base64.StdEncoding.DecodeString(os.Getenv("TF_MASTER_KEY"))
	if err != nil || len(k) != 32 {
		return c, errors.New("TF_MASTER_KEY must be base64 encoded 32 bytes")
	}
	c.MasterKey = k
	return c, c.Validate()
}
func env(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("TF_DATABASE_URL is required")
	}
	if len(c.MasterKey) != 32 {
		return errors.New("TF_MASTER_KEY must contain 32 bytes")
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("TF_PUBLIC_URL must be HTTP(S)")
	}
	if c.CookieSecure && u.Scheme != "https" {
		return errors.New("secure cookies require HTTPS public URL")
	}
	if c.StorageBackend != "local" && c.StorageBackend != "s3" {
		return errors.New("TF_STORAGE_BACKEND must be local or s3")
	}
	if c.StorageBackend == "s3" && (c.S3Bucket == "" || c.S3Region == "") {
		return errors.New("S3 bucket and region required")
	}
	if c.OIDCIssuer != "" && c.OIDCClientID == "" {
		return errors.New("OIDC client ID required")
	}
	if strings.ContainsAny(c.SMTPFrom, "\r\n") || strings.ContainsAny(c.SMTPHost, "\r\n") {
		return errors.New("invalid SMTP configuration")
	}
	if _, e = strconv.Atoi(envValue(c.SMTPPort, "25")); e != nil {
		return errors.New("invalid SMTP port")
	}
	return nil
}
func envValue(v, f string) string {
	if v == "" {
		return f
	}
	return v
}
