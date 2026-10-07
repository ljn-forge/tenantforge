package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ljn-forge/tenantforge/internal/platform"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := platform.Run(ctx, os.Args[1:]); e != nil {
		slog.Error(e.Error())
		os.Exit(1)
	}
}
