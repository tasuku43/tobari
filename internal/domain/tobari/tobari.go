// Package tobari defines the shared enforcement cluster and CWD-owned isolation spaces.
package tobari

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	TaskClusterUp         = "cluster.up"
	TaskClusterStatus     = "cluster.status"
	TaskClusterDenials    = "cluster.denials"
	TaskClusterDown       = "cluster.down"
	TaskPolicyCandidates  = "policy.candidates"
	TaskPolicyTail        = "policy.tail"
	TaskPolicyReview      = "policy.review"
	TaskPolicyAllow       = "policy.allow"
	TaskPolicyDeny        = "policy.deny"
	TaskPolicyCompactions = "policy.compactions"
	TaskPolicyCompact     = "policy.compact"
	TaskAttach            = "tobari.attach"
	TaskList              = "tobari.list"
	TaskExec              = "tobari.exec"
	TaskLogs              = "tobari.logs"
	TaskDetach            = "tobari.detach"

	ClusterTargetKind    = "cluster"
	ClusterTargetID      = "cluster-default"
	TargetKind           = "tobari"
	ReferenceKind        = TargetKind
	PolicyCandidateKind  = "policy-candidate"
	PolicyCompactionKind = "policy-compaction"

	BuiltinImageSelector        = "builtin"
	RuntimeImageAPILabel        = "io.tobari.runtime-api"
	RuntimeImageAPI             = "1"
	RuntimeImageLifetimeLabel   = "io.tobari.runtime-lifetime-command"
	RuntimeImageLifetimeCommand = "sleep infinity"
)

var (
	namePattern           = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	idPattern             = regexp.MustCompile(`^tbr_[0-9a-f]{32}$`)
	imageReferencePattern = regexp.MustCompile(`^(?:(?:localhost|[a-z0-9]+(?:[.-][a-z0-9]+)*)(?::[0-9]{1,5})?/)?[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[0-9a-f]{64})?$`)
)

// Instance is the persisted secret-free identity and exact Docker resources of one Tobari.
type Instance struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Root       string `json:"root"`
	Container  string `json:"container"`
	Network    string `json:"network"`
	HomeVolume string `json:"home_volume"`
	Image      string `json:"image,omitempty"`
}

// ValidateName accepts a portable Docker-resource-safe display name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must match [a-z][a-z0-9-]{0,62}")
	}
	return nil
}

// ValidateID accepts only Tobari-owned opaque reference bytes.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("Tobari ID is invalid")
	}
	return nil
}

// ValidateImageSelector accepts the built-in runtime or one conservative OCI image reference.
func ValidateImageSelector(image string) error {
	if image == BuiltinImageSelector {
		return nil
	}
	if len(image) == 0 || len(image) > 255 || !imageReferencePattern.MatchString(image) {
		return fmt.Errorf("image must be %q or a portable OCI image reference", BuiltinImageSelector)
	}
	return nil
}

// ImageSelector maps schema-2 records written before image selection to the built-in runtime.
func (i Instance) ImageSelector() string {
	if i.Image == "" {
		return BuiltinImageSelector
	}
	return i.Image
}

// Validate rejects incomplete instance state before Docker operations.
func (i Instance) Validate() error {
	if err := ValidateID(i.ID); err != nil {
		return err
	}
	if err := ValidateName(i.Name); err != nil {
		return err
	}
	if !filepath.IsAbs(i.Root) || filepath.Clean(i.Root) != i.Root {
		return fmt.Errorf("root must be a canonical absolute path")
	}
	if err := ValidateImageSelector(i.ImageSelector()); err != nil {
		return err
	}
	expected := map[string]string{
		"container":   "tobari-" + i.Name,
		"network":     "tobari-" + i.Name + "-net",
		"home volume": "tobari-" + i.Name + "-home",
	}
	actual := map[string]string{
		"container": i.Container, "network": i.Network, "home volume": i.HomeVolume,
	}
	for label, value := range actual {
		if value != expected[label] {
			return fmt.Errorf("%s does not match the Tobari name", label)
		}
	}
	return nil
}

// State is the secret-free persisted identity of one cluster and its Tobari.
type State struct {
	SchemaVersion    int    `json:"schema_version"`
	RuntimeDirectory string `json:"runtime_directory"`
	// ContextName is empty only for legacy schema-2 state; infrastructure
	// interprets that value as the default Context during migration.
	ContextName      string     `json:"context_name,omitempty"`
	AgentProfile     string     `json:"agent_profile,omitempty"`
	PolicyDirectory  string     `json:"policy_directory"`
	CredentialConfig string     `json:"credential_config"`
	CredentialDir    string     `json:"credential_directory"`
	AssetVersion     string     `json:"asset_version"`
	ProxyEndpoint    string     `json:"proxy_endpoint"`
	RecentError      string     `json:"recent_error"`
	Tobari           []Instance `json:"tobari"`
}

// Validate rejects incomplete, ambiguous, or relative state.
func (s State) Validate() error {
	if s.SchemaVersion != 2 {
		return fmt.Errorf("Tobari state schema version must be 2")
	}
	for name, value := range map[string]string{
		"runtime directory": s.RuntimeDirectory,
		"policy directory":  s.PolicyDirectory, "credential config": s.CredentialConfig,
		"credential directory": s.CredentialDir,
	} {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("%s must be a canonical absolute path", name)
		}
	}
	if s.AssetVersion == "" || strings.ContainsAny(s.AssetVersion, " \t\r\n") {
		return fmt.Errorf("asset version is invalid")
	}
	if s.ContextName != "" {
		if err := ValidateName(s.ContextName); err != nil {
			return fmt.Errorf("context name: %w", err)
		}
	}
	if s.AgentProfile != "" {
		if err := ValidateName(s.AgentProfile); err != nil {
			return fmt.Errorf("agent profile: %w", err)
		}
	}
	if s.ProxyEndpoint != "http://gateway:8080" {
		return fmt.Errorf("proxy endpoint is invalid")
	}
	if s.Tobari == nil {
		return fmt.Errorf("Tobari collection is unknown")
	}
	if len(s.RecentError) > 1024 || strings.IndexFunc(s.RecentError, func(r rune) bool {
		return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0 {
		return fmt.Errorf("recent error is unsafe")
	}
	ids, names, roots := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, instance := range s.Tobari {
		if err := instance.Validate(); err != nil {
			return fmt.Errorf("invalid Tobari %q: %w", instance.Name, err)
		}
		if ids[instance.ID] || names[instance.Name] || roots[instance.Root] {
			return fmt.Errorf("Tobari IDs, names, and roots must be unique")
		}
		ids[instance.ID], names[instance.Name], roots[instance.Root] = true, true, true
	}
	return nil
}

// Find returns one exact opaque-ID match without decoding or normalizing it.
func (s State) Find(id string) (Instance, bool) {
	for _, instance := range s.Tobari {
		if instance.ID == id {
			return instance, true
		}
	}
	return Instance{}, false
}

// ComponentStatus is one exact managed container observation.
type ComponentStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health"`
}

// ClusterStatus is a task-bound observation of shared enforcement.
type ClusterStatus struct {
	Task        string            `json:"task"`
	Configured  bool              `json:"configured"`
	Running     bool              `json:"running"`
	Proxy       string            `json:"proxy"`
	Policy      string            `json:"policy"`
	TobariCount int               `json:"tobari_count"`
	Components  []ComponentStatus `json:"components"`
	RecentError string            `json:"recent_error"`
}

// Validate binds status to the requested cluster task and scope.
func (s ClusterStatus) Validate() error {
	if s.Task != TaskClusterStatus && s.Task != TaskClusterUp && s.Task != TaskClusterDown {
		return fmt.Errorf("cluster status task identity is invalid")
	}
	if !s.Configured {
		if s.Running || s.Proxy != "" || s.Policy != "" || s.TobariCount != 0 || len(s.Components) != 0 {
			return fmt.Errorf("unconfigured status contains cluster state")
		}
		return nil
	}
	if !filepath.IsAbs(s.Policy) || s.Proxy == "" || s.TobariCount < 0 || s.Components == nil {
		return fmt.Errorf("configured cluster status is incomplete")
	}
	return nil
}

// ItemStatus is one task-owned list item and opaque action reference.
type ItemStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Root      string `json:"root"`
	Image     string `json:"image"`
	Running   bool   `json:"running"`
	Container string `json:"container"`
}

// ListResult preserves the exact local collection scope, including empty.
type ListResult struct {
	Task  string       `json:"task"`
	Items []ItemStatus `json:"items"`
}

func (r ListResult) Validate() error {
	if r.Task != TaskList || r.Items == nil {
		return fmt.Errorf("Tobari list task or scope is invalid")
	}
	for _, item := range r.Items {
		if err := ValidateID(item.ID); err != nil {
			return err
		}
		if err := ValidateName(item.Name); err != nil {
			return err
		}
		if !filepath.IsAbs(item.Root) || item.Container != "tobari-"+item.Name {
			return fmt.Errorf("Tobari list item is invalid")
		}
		if err := ValidateImageSelector(item.Image); err != nil {
			return err
		}
	}
	return nil
}

// MapHostCWD maps one canonical host cwd below root into /workspace.
func MapHostCWD(root, cwd string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", fmt.Errorf("Tobari root must be canonical and absolute")
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
		return "", fmt.Errorf("cwd must be inside the selected Tobari root")
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

// LogRequest is a bounded observation of shared or per-Tobari logs.
type LogRequest struct {
	Component string
	Tail      int
}

func (r LogRequest) ValidateCluster() error {
	switch r.Component {
	case "all", "gateway", "opa":
	default:
		return fmt.Errorf("cluster log component is invalid")
	}
	return r.validateTail()
}

func (r LogRequest) ValidateTobari() error {
	if r.Component != "tobari" {
		return fmt.Errorf("Tobari log component is invalid")
	}
	return r.validateTail()
}

func (r LogRequest) validateTail() error {
	if r.Tail < 1 || r.Tail > 10_000 {
		return fmt.Errorf("log tail must be between 1 and 10000")
	}
	return nil
}
