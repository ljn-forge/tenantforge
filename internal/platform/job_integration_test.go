package platform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRetryRecreatesDurableLease(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	f.sql("INSERT INTO jobs(id,tenant_id,actor_id,type,status,payload) VALUES($1,$2,$3,'export','failed',$4)", id, f.tenant, f.user, jsonBytes(exportPayload{"tickets", []string{"title"}}))
	f.call("POST", "/jobs/"+id+"/retry", map[string]any{}, 200)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j := asMap(f.call("GET", "/jobs/"+id, nil, 200)["data"])
		if j["status"] == "succeeded" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("retried job never executes: missing durable lease")
}

type blockedStorage struct {
	Storage
	start   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockedStorage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	if ct == "text/csv" {
		b.once.Do(func() { close(b.start) })
		select {
		case <-b.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.Storage.Put(ctx, key, r, size, ct)
}
func TestCancelRunningExportDoesNotWaitForStorage(t *testing.T) {
	f := newFixture(t)
	s := &blockedStorage{Storage: f.app.storage(), start: make(chan struct{}), release: make(chan struct{})}
	f.app.setStorage(s)
	j := asMap(f.call("POST", "/resources/tickets/export", map[string]any{"columns": []string{"title"}}, 200)["data"])
	id := j["id"].(string)
	select {
	case <-s.start:
	case <-time.After(2 * time.Second):
		t.Fatal("worker not started")
	}
	defer close(s.release)
	client := &http.Client{Jar: f.client.Jar, Timeout: time.Second}
	r, _ := http.NewRequest("POST", f.server.URL+"/api/v1/jobs/"+id+"/cancel", bytes.NewBufferString(`{}`))
	r.Header.Set("X-CSRF-Token", f.csrf)
	r.Header.Set("X-Tenant-ID", f.tenant)
	resp, e := client.Do(r)
	if e != nil {
		t.Fatal("cancel blocked by active export")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("cancel status %d", resp.StatusCode)
	}
	j = asMap(f.call("GET", "/jobs/"+id, nil, 200)["data"])
	if j["status"] != "cancelled" {
		t.Fatal("cancellation not persisted")
	}
}

type blockedDeleteStorage struct {
	Storage
	called chan struct{}
	once   sync.Once
}

func (s *blockedDeleteStorage) Delete(ctx context.Context, key string) error {
	s.once.Do(func() { close(s.called) })
	<-ctx.Done()
	return ctx.Err()
}
func TestCleanupDeletionAndShutdownAreBounded(t *testing.T) {
	f := newFixture(t)
	storage := &blockedDeleteStorage{Storage: f.app.storage(), called: make(chan struct{})}
	f.app.setStorage(storage)
	done := make(chan struct{})
	key := uuid.NewString()
	if e := f.app.reserveObject(context.Background(), key, f.tenant); e != nil {
		t.Fatal(e)
	}
	go func() { f.app.deleteOrQueue(key); close(done) }()
	select {
	case <-storage.called:
	case <-time.After(time.Second):
		t.Fatal("cleanup not started")
	}
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("cleanup did not honor deadline")
	}
	f.app.Close()
}
