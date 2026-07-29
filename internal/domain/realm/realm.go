// Package realm defines Tobari's single local isolation realm.
package realm

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	TaskUp     = "realm.up"
	TaskStatus = "realm.status"
	TaskExec   = "realm.exec"
	TaskLogs   = "realm.logs"
	TaskDown   = "realm.down"
	TaskDoctor = "realm.doctor"

	TargetKind = "realm"
	TargetID   = "realm-default"
)

// State is the secret-free persisted identity of the one managed realm.
type State struct {
	SchemaVersion    int    `json:"schema_version"`
	Root             string `json:"root"`
	RuntimeDirectory string `json:"runtime_directory"`
	PolicyDirectory  string `json:"policy_directory"`
	CredentialConfig string `json:"credential_config"`
	CredentialDir    string `json:"credential_directory"`
	AssetVersion     string `json:"asset_version"`
	ProxyEndpoint    string `json:"proxy_endpoint"`
	RecentError      string `json:"recent_error"`
}

// Validate rejects incomplete or relative state before Docker operations.
func (s State) Validate() error {
	if s.SchemaVersion != 1 {
		return fmt.Errorf("realm state schema version must be 1")
	}
	for name, value := range map[string]string{
		"root": s.Root, "runtime directory": s.RuntimeDirectory,
		"policy directory": s.PolicyDirectory, "credential config": s.CredentialConfig,
		"credential directory": s.CredentialDir,
	} {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("%s must be a canonical absolute path", name)
		}
	}
	if s.AssetVersion == "" || strings.ContainsAny(s.AssetVersion, " \t\r\n") {
		return fmt.Errorf("asset version is invalid")
	}
	if s.ProxyEndpoint != "http://tobari-gateway:8080" {
		return fmt.Errorf("proxy endpoint is invalid")
	}
	if len(s.RecentError) > 1024 || strings.IndexFunc(s.RecentError, func(r rune) bool {
		return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0 {
		return fmt.Errorf("recent error is unsafe")
	}
	return nil
}

// ComponentStatus is one exact managed container observation.
type ComponentStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health"`
}

// Status is a task-bound observation of the single realm.
type Status struct {
	Task        string            `json:"task"`
	Configured  bool              `json:"configured"`
	Running     bool              `json:"running"`
	Root        string            `json:"root"`
	Proxy       string            `json:"proxy"`
	Policy      string            `json:"policy"`
	Components  []ComponentStatus `json:"components"`
	RecentError string            `json:"recent_error"`
}

// Validate binds status to the requested task and single-realm scope.
func (s Status) Validate() error {
	if s.Task != TaskStatus && s.Task != TaskUp && s.Task != TaskDown {
		return fmt.Errorf("status task identity is invalid")
	}
	if !s.Configured {
		if s.Running || s.Root != "" || s.Proxy != "" || s.Policy != "" || len(s.Components) != 0 {
			return fmt.Errorf("unconfigured status contains realm state")
		}
		return nil
	}
	if !filepath.IsAbs(s.Root) || !filepath.IsAbs(s.Policy) || s.Proxy == "" {
		return fmt.Errorf("configured status is incomplete")
	}
	if s.Components == nil {
		return fmt.Errorf("component status is unknown")
	}
	return nil
}

// MapHostCWD maps one canonical host cwd below root into /workspace.
func MapHostCWD(root, cwd string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", fmt.Errorf("realm root must be canonical and absolute")
	}
	if cwd == "" {
		return "/workspace", nil
	}
	if !filepath.IsAbs(cwd) || filepath.Clean(cwd) != cwd {
		return "", fmt.Errorf("cwd must be canonical and absolute")
	}
	relative, err := filepath.Rel(root, cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd below root: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd must be inside the configured root")
	}
	if relative == "." {
		return "/workspace", nil
	}
	return "/workspace/" + filepath.ToSlash(relative), nil
}

// ExecRequest describes one process execution without interpreting its argv.
type ExecRequest struct {
	HostCWD     string
	CWDExplicit bool
	Command     []string
	Interactive bool
	TTY         bool
}

// Validate requires at least one exact argv element.
func (r ExecRequest) Validate() error {
	if len(r.Command) == 0 {
		return fmt.Errorf("command is required")
	}
	for _, value := range r.Command {
		if strings.IndexByte(value, 0) >= 0 {
			return fmt.Errorf("command contains a NUL byte")
		}
	}
	return nil
}

// LogRequest selects an exact component and a bounded observation.
type LogRequest struct {
	Component string
	Tail      int
}

func (r LogRequest) Validate() error {
	switch r.Component {
	case "all", "gateway", "opa", "realm":
	default:
		return fmt.Errorf("log component is invalid")
	}
	if r.Tail < 1 || r.Tail > 10_000 {
		return fmt.Errorf("log tail must be between 1 and 10000")
	}
	return nil
}
