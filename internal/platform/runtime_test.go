package platform

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestRuntimeWaitsForHandlersBeforeClosingDependencies(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(200) })}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server, listener) }()
	go func() {
		response, e := http.Get("http://" + listener.Addr().String())
		if e == nil {
			response.Body.Close()
		}
	}()
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("runtime returned before active handler drained")
	case <-time.After(50 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not finish forced close")
	}
}
