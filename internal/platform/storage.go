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

func envTimeout(v, f time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return f
}
