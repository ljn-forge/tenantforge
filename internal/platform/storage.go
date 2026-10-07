package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Storage interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

var keyPattern = regexp.MustCompile(`^[a-z0-9-]{36}$`)

type localStorage struct{ Root *os.Root }

func NewStorage(ctx context.Context, c Config) (Storage, error) {
	if c.StorageBackend == "local" {
		dir := filepath.Join(c.DataDir, "storage")
		if e := os.MkdirAll(dir, 0700); e != nil {
			return nil, errors.New("storage directory unavailable")
		}
		root, e := os.OpenRoot(dir)
		if e != nil {
			return nil, errors.New("storage directory unavailable")
		}
		return &localStorage{root}, nil
	}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.S3Region), awsconfig.WithHTTPClient(&http.Client{Timeout: envTimeout(c.UpstreamTimeout, 15*time.Second)})}
	if c.S3AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.S3AccessKey, c.S3SecretKey, "")))
	}
	cfg, e := awsconfig.LoadDefaultConfig(ctx, opts...)
	if e != nil {
		return nil, errors.New("S3 configuration invalid")
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		if c.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(c.S3Endpoint)
		}
	})
	return &s3Storage{client, c.S3Bucket}, nil
}
func (s *localStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	f, e := s.Root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	n, e := io.Copy(f, io.LimitReader(r, size+1))
	closeErr := f.Close()
	if e != nil || closeErr != nil || n != size {
		s.Root.Remove(key)
		return errors.New("storage write failed")
	}
	return nil
}
func (s *localStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !keyPattern.MatchString(key) {
		return nil, errors.New("invalid storage key")
	}
	return s.Root.Open(key)
}
func (s *localStorage) Delete(ctx context.Context, key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	e := s.Root.Remove(key)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}

type s3Storage struct {
	Client *s3.Client
	Bucket string
}

func (s *s3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	_, e := s.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key), Body: r, ContentLength: aws.Int64(size), ContentType: aws.String(ct)})
	return e
}
func (s *s3Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if !keyPattern.MatchString(key) {
		return nil, errors.New("invalid storage key")
	}
	out, e := s.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	if e != nil {
		return nil, e
	}
	return out.Body, nil
}
func (s *s3Storage) Delete(ctx context.Context, key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("invalid storage key")
	}
	_, e := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String(key)})
	return e
}

// Reserve before external writes so a crash or uncertain provider response leaves a durable cleanup journal.
func (a *App) reserveObject(ctx context.Context, key, tenant string) error {
	_, e := a.LimitsDB.Exec(ctx, "INSERT INTO storage_gc(id,storage_key,tenant_id,next_attempt_at) VALUES($1,$2,$3,now()+interval '10 minutes')", key, key, tenant)
	return e
}
func (a *App) deleteOrQueue(key string) {
	if key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, e := a.LimitsDB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var id, tenant string
	// Publication deletes this locked journal atomically with metadata. A successful or uncertain committed publication therefore cannot be removed here.
	if e = tx.QueryRow(ctx, "SELECT id::text,coalesce(tenant_id::text,'') FROM storage_gc WHERE storage_key=$1 ORDER BY next_attempt_at FOR UPDATE LIMIT 1", key).Scan(&id, &tenant); e != nil {
		return
	}
	if e = a.collectObject(ctx, tx, id, key, tenant, 0); e == nil {
		tx.Commit(ctx)
	}
}
func (a *App) collectObject(ctx context.Context, tx pgx.Tx, id, key, tenant string, retrySeconds int) error {
	if tenant != "" {
		if _, e := tx.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", tenant); e != nil {
			return e
		}
		var referenced bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM files WHERE tenant_id=$1 AND storage_key=$2) OR EXISTS(SELECT FROM jobs WHERE tenant_id=$1 AND result_key=$2)", tenant, key).Scan(&referenced); e != nil {
			return e
		}
		if referenced {
			_, e := tx.Exec(ctx, "DELETE FROM storage_gc WHERE id=$1", id)
			return e
		}
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	e := a.storage().Delete(deleteCtx, key)
	cancel()
	if e == nil {
		_, e = tx.Exec(ctx, "DELETE FROM storage_gc WHERE id=$1", id)
	} else {
		_, e = tx.Exec(ctx, "UPDATE storage_gc SET attempts=attempts+1,next_attempt_at=now()+($2*interval '1 second') WHERE id=$1", id, retrySeconds)
	}
	return e
}

func envTimeout(v, f time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return f
}
