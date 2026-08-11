//go:build !linux && !darwin

package credentialhost

import "context"

func runClaudeTerminalCommand(context.Context, ClaudeCommand) error {
	return ErrClaudeTTYRequired
}
