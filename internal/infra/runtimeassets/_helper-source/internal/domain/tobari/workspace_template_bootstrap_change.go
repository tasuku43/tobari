package tobari

import (
	"fmt"
	"strings"
)

type WorkspaceTemplateBootstrapAction string

const (
	WorkspaceTemplateBootstrapConfigure WorkspaceTemplateBootstrapAction = "configure"
	WorkspaceTemplateBootstrapRefresh   WorkspaceTemplateBootstrapAction = "refresh"
	WorkspaceTemplateBootstrapRemove    WorkspaceTemplateBootstrapAction = "remove"
)

// WorkspaceTemplateBootstrapRequest is the closed user action used to derive
// one bootstrap delta from the exact current Template while the installation
// lifecycle lock is held. It carries no Template name or filesystem path.
type WorkspaceTemplateBootstrapRequest struct {
	Kind     WorkspaceTemplateChangeKind
	Action   WorkspaceTemplateBootstrapAction
	Selector string
}

func (r WorkspaceTemplateBootstrapRequest) Validate() error {
	if r.Kind != WorkspaceTemplateChangeBootstrapAWS && r.Kind != WorkspaceTemplateChangeBootstrapEKS {
		return fmt.Errorf("Template bootstrap request kind is invalid")
	}
	switch r.Action {
	case WorkspaceTemplateBootstrapConfigure:
		if r.Kind == WorkspaceTemplateChangeBootstrapAWS {
			return ValidateContextAWSBootstrapProfileName(r.Selector)
		}
		return ValidateContextEKSBootstrapName(r.Selector)
	case WorkspaceTemplateBootstrapRefresh, WorkspaceTemplateBootstrapRemove:
		if r.Selector != "" {
			return fmt.Errorf("Template bootstrap refresh/remove cannot carry a selector")
		}
		return nil
	default:
		return fmt.Errorf("Template bootstrap action is invalid")
	}
}

func ValidateContextEKSBootstrapName(value string) error {
	if !eksBootstrapNamePattern.MatchString(value) || strings.Contains(value, "..") {
		return fmt.Errorf("EKS bootstrap context name is invalid")
	}
	return nil
}
