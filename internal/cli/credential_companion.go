package cli

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/infra/companionruntime"
)

// IsCredentialCompanionArg0 recognizes only the exact private process name.
// It is deliberately absent from Catalog and every public help surface.
func IsCredentialCompanionArg0(value string) bool {
	return value == companionruntime.PrivateArg0
}

// RunCredentialCompanionContext is the private composition seam used only by
// the self-spawned companion process. Invalid bootstrap and runtime failures
// are intentionally secret-free and silent.
func RunCredentialCompanionContext(ctx context.Context, args []string, input io.Reader) int {
	if ctx == nil || len(args) != 0 || input == nil {
		return ExitUsage
	}
	if err := companionruntime.Run(ctx, input); err != nil {
		return ExitUnavailable
	}
	return ExitOK
}
