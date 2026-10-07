package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Run is the shared CLI/runtime entrypoint. It never exposes the runtime pool to extensions.
func Run(ctx context.Context, args []string) error {
	command := "serve"
	if len(args) > 1 {
		return fmt.Errorf("expected one command")
	}
	if len(args) == 1 {
		command = args[0]
	}
	if command == "version" {
		fmt.Println(Version)
		return nil
	}
	c, e := FromEnv()
	if e != nil {
		return e
	}
	switch command {
	case "check-config":
		fmt.Println("Configuration valid (secrets redacted)")
		return nil
	case "migrate":
		dsn := c.MigrationDatabaseURL
		if dsn == "" {
			dsn = c.DatabaseURL
		}
		if e = Migrate(ctx, dsn); e != nil {
			return e
		}
		fmt.Println("Migrations applied")
		return nil
	case "serve":
		a, e := New(ctx, c)
		if e != nil {
			return e
		}
		defer a.Close()
		server := &http.Server{Addr: c.Listen, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
		listener, e := net.Listen("tcp", c.Listen)
		if e != nil {
			return fmt.Errorf("HTTP server unavailable")
		}
		slog.Info("TenantForge listening", "address", c.Listen, "version", Version)
		return serveHTTP(ctx, server, listener)
	default:
		return fmt.Errorf("command must be serve, migrate, check-config or version")
	}
}

// serveHTTP waits for both the listener and in-flight handlers before callers close dependencies.
func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener) error {
	stop := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if server.Shutdown(shutdown) != nil {
				_ = server.Close()
			}
		case <-stop:
		}
	}()
	err := server.Serve(listener)
	close(stop)
	<-drained
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server unavailable")
	}
	return nil
}
