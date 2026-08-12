//go:build !linux && !darwin

package dockerruntime

import (
	"context"
	"io"
	"os"
)

func waitHostLoginInput(context.Context, io.Reader) error {
	return errHostLoginPrompt
}

func readHostLoginInput(io.Reader, []byte) (int, error) {
	return 0, errHostLoginPrompt
}

func openHostLoginInput(io.Reader) (*os.File, error) {
	return nil, errHostLoginPrompt
}
