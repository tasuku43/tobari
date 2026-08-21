package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func (r *Runtime) verifyOwned(ctx context.Context, kind, name string) error {
	args := []string{"inspect", "--format", `{{index .Config.Labels "` + ownerLabel + `"}}`, name}
	if kind == "volume" {
		args = []string{"volume", "inspect", "--format", `{{index .Labels "` + ownerLabel + `"}}`, name}
	}
	if kind == "network" {
		args = []string{"network", "inspect", "--format", `{{index .Labels "` + ownerLabel + `"}}`, name}
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) {
			return errOwnedResourceMissing
		}
		return fmt.Errorf("inspect %s %s ownership: %w: %s", kind, name, err, boundedDiagnostic(output))
	}
	if strings.TrimSpace(string(output)) != ownerValue {
		return fmt.Errorf("%s %s is not owned by Tobari", kind, name)
	}
	return nil
}

func (r *Runtime) verifyOwnedTobari(ctx context.Context, kind, name, id string) error {
	if err := r.verifyOwned(ctx, kind, name); err != nil {
		return err
	}
	var args []string
	switch kind {
	case "container":
		args = []string{"inspect", "--format", `{{index .Config.Labels "` + tobariIDLabel + `"}}`, name}
	case "volume":
		args = []string{"volume", "inspect", "--format", `{{index .Labels "` + tobariIDLabel + `"}}`, name}
	case "network":
		args = []string{"network", "inspect", "--format", `{{index .Labels "` + tobariIDLabel + `"}}`, name}
	default:
		return fmt.Errorf("unsupported resource kind %s", kind)
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != id {
		return fmt.Errorf("%s %s does not belong to the selected Tobari", kind, name)
	}
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func boundedDiagnostic(data []byte) string {
	const maximum = 4096
	data = bytes.TrimSpace(data)
	if len(data) > maximum {
		data = data[:maximum]
	}
	return string(data)
}
