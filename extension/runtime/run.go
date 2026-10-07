// Package runtime starts an extension-enabled TenantForge deployment without exposing internal services or pools.
package runtime

import (
	"context"

	"github.com/ljn-forge/tenantforge/internal/platform"
)

// Run accepts serve, migrate, check-config or version and reads the documented TF_ environment.
// Register extension modules and jobs before calling it. Serve stops when ctx is cancelled.
func Run(ctx context.Context, args []string) error { return platform.Run(ctx, args) }
