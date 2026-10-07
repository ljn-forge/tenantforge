package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ljn-forge/tenantforge/internal/platform"
)

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "version" {
		fmt.Println(platform.Version)
		return
	}
	c, e := platform.FromEnv()
	if e != nil {
		fatal(e)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch command {
	case "check-config":
		fmt.Println("Configuration valid (secrets redacted)")
	case "migrate":
		dsn := c.MigrationDatabaseURL
		if dsn == "" {
			dsn = c.DatabaseURL
		}
		if e = platform.Migrate(ctx, dsn); e != nil {
			fatal(e)
		}
		fmt.Println("Migrations applied")
	case "serve":
		app, e := platform.New(ctx, c)
		if e != nil {
			fatal(e)
		}
		defer app.Close()
		server := &http.Server{Addr: c.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server.Shutdown(shutdown)
		}()
		slog.Info("TenantForge listening", "address", c.Listen, "version", platform.Version)
		if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			fatal(fmt.Errorf("HTTP server unavailable"))
		}
	default:
		fatal(fmt.Errorf("command must be serve, migrate, check-config or version"))
	}
}
func fatal(e error) { slog.Error(e.Error()); os.Exit(1) }
