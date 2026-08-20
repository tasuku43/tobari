//go:build !darwin && !linux

package dockerruntime

import (
	"context"
	"io"
)

func (osCommandRunner) RunInteractive(
	ctx context.Context,
	args, environment []string,
	in io.Reader,
	out, errOut io.Writer,
	_ bool,
) error {
	return osCommandRunner{}.Run(ctx, args, environment, in, out, errOut)
}
