// This entrypoint uses public packages only and can be copied to a downstream module.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ljn-forge/tenantforge/examples/assets"
	"github.com/ljn-forge/tenantforge/extension/runtime"
)

func main() {
	if err := assets.Register(); err != nil {
		slog.Error("Asset registrations rejected")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runtime.Run(ctx, os.Args[1:]); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
