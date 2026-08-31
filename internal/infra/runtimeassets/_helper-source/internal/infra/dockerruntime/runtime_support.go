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

func (r *Runtime) verifyOwnedConfiguratorContainer(ctx context.Context, containerID string) error {
	output, err := r.runner.Output(ctx, []string{"inspect", "--format", `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},"component":{{json (index .Config.Labels "` + componentLabel + `")}}}`, containerID}, os.Environ())
	if err != nil {
		return fmt.Errorf("inspect Configurator container identity: %w: %s", err, boundedDiagnostic(output))
	}
	var observed struct {
		ID        string `json:"id"`
		Owner     string `json:"owner"`
		Component string `json:"component"`
	}
	if json.Unmarshal(output, &observed) != nil || observed.ID != containerID || observed.Owner != ownerValue || observed.Component != "configurator" {
		return fmt.Errorf("container %s is not the exact active Configurator", containerID)
	}
	return nil
}

func (r *Runtime) requireOwnedProjectContainerID(ctx context.Context, reference, projectID, role string) (string, error) {
	output, err := r.runner.Output(ctx, []string{"inspect", "--format", `{"id":{{json .Id}},"identity_ok":{{and (eq (index .Config.Labels "` + ownerLabel + `") "` + ownerValue + `") (eq (index .Config.Labels "` + componentLabel + `") "tobari") (eq (index .Config.Labels "` + projectIDLabel + `") "` + projectID + `") (eq (index .Config.Labels "` + projectRoleLabel + `") "` + role + `")}}}`, reference}, os.Environ())
	if err != nil {
		return "", fmt.Errorf("inspect Workspace container identity: %w: %s", err, boundedDiagnostic(output))
	}
	var observed struct {
		ID         string `json:"id"`
		IdentityOK bool   `json:"identity_ok"`
	}
	if json.Unmarshal(output, &observed) != nil || !observed.IdentityOK {
		return "", fmt.Errorf("container %s is not the exact selected Workspace", reference)
	}
	if _, err := exactDockerResourceID([]byte(observed.ID)); err != nil {
		return "", fmt.Errorf("selected Workspace container identity is invalid: %w", err)
	}
	return observed.ID, nil
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
