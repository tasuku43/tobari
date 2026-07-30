package tobari

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// PolicyDenial is one validated, secret-free Gateway audit decision.
type PolicyDenial struct {
	Timestamp  string `json:"timestamp"`
	RequestID  string `json:"request_id"`
	Host       string `json:"host"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Reason     string `json:"reason"`
	StatusCode int    `json:"status_code"`
}

// Validate rejects audit-shaped data that cannot be safely interpreted as one
// denied HTTP boundary effect.
func (d PolicyDenial) Validate() error {
	if _, err := time.Parse(time.RFC3339Nano, d.Timestamp); err != nil {
		return fmt.Errorf("denial timestamp is invalid")
	}
	if !requestIDPattern.MatchString(d.RequestID) {
		return fmt.Errorf("denial request ID is invalid")
	}
	if len(d.Host) == 0 || len(d.Host) > 253 || containsSpaceOrControl(d.Host) {
		return fmt.Errorf("denial host is invalid")
	}
	if len(d.Method) == 0 || len(d.Method) > 32 || containsSpaceOrControl(d.Method) {
		return fmt.Errorf("denial method is invalid")
	}
	if len(d.Path) == 0 || len(d.Path) > 4096 || !strings.HasPrefix(d.Path, "/") {
		return fmt.Errorf("denial path is invalid")
	}
	if len(d.Reason) == 0 || len(d.Reason) > 1024 {
		return fmt.Errorf("denial reason is invalid")
	}
	if d.StatusCode < 400 || d.StatusCode > 599 {
		return fmt.Errorf("denial status code is invalid")
	}
	return nil
}

func containsSpaceOrControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return r <= ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0
}

// DenialReport preserves the exact local cluster scope and requested bounded
// Gateway-log window, including a valid empty result.
type DenialReport struct {
	Task            string         `json:"task"`
	PolicyDirectory string         `json:"policy"`
	WindowLines     int            `json:"window_lines"`
	Items           []PolicyDenial `json:"items"`
}

// Validate binds denial evidence to the cluster-denials task and its scope.
func (r DenialReport) Validate() error {
	if r.Task != TaskClusterDenials {
		return fmt.Errorf("denial report task identity is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("denial report policy directory is invalid")
	}
	if r.WindowLines < 1 || r.WindowLines > 10_000 {
		return fmt.Errorf("denial report window is invalid")
	}
	if r.Items == nil {
		return fmt.Errorf("denial report collection is unknown")
	}
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PolicyActivation is the confirmed result of testing and activating the
// current trusted-host policy.
type PolicyActivation struct {
	Task            string `json:"task"`
	PolicyDirectory string `json:"policy"`
	Applied         bool   `json:"applied"`
}

// Validate prevents a partial or task-mismatched activation from being
// presented as success.
func (a PolicyActivation) Validate() error {
	if a.Task != TaskPolicyApply || !a.Applied {
		return fmt.Errorf("policy activation result is incomplete")
	}
	if !filepath.IsAbs(a.PolicyDirectory) || filepath.Clean(a.PolicyDirectory) != a.PolicyDirectory {
		return fmt.Errorf("policy activation directory is invalid")
	}
	return nil
}
