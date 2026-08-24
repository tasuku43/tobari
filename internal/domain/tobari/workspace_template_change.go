package tobari

import "fmt"

type WorkspaceTemplateChangeKind string

const (
	WorkspaceTemplateChangeShell        WorkspaceTemplateChangeKind = "shell"
	WorkspaceTemplateChangeGit          WorkspaceTemplateChangeKind = "git"
	WorkspaceTemplateChangeBootstrapAWS WorkspaceTemplateChangeKind = "bootstrap_aws"
	WorkspaceTemplateChangeBootstrapEKS WorkspaceTemplateChangeKind = "bootstrap_eks"
	WorkspaceTemplateChangeRuntime      WorkspaceTemplateChangeKind = "runtime"
)

// WorkspaceTemplateChange is one closed reviewed delta. It deliberately does
// not carry a complete Template body: the owner-only mutator applies it to the
// exact current revision under the installation lifecycle lock, preventing
// concurrent configuration tasks from reverting unrelated accepted fields.
type WorkspaceTemplateChange struct {
	Kind               WorkspaceTemplateChangeKind
	Shell              []ManifestShellEnvironmentSetting
	Git                *ManifestGitIdentitySetting
	AWS                *ManifestAWSBootstrap
	EKS                *ManifestEKSBootstrap
	RuntimeRevisionRef string
}

func (c WorkspaceTemplateChange) Validate() error {
	switch c.Kind {
	case WorkspaceTemplateChangeShell:
		if len(c.Shell) == 0 || c.Git != nil || c.AWS != nil || c.EKS != nil || c.RuntimeRevisionRef != "" {
			return fmt.Errorf("Template shell change dimensions are invalid")
		}
		_, err := ApplyContextShellEnvironmentSettings(nil, c.Shell)
		return err
	case WorkspaceTemplateChangeGit:
		if c.Git == nil || len(c.Shell) != 0 || c.AWS != nil || c.EKS != nil || c.RuntimeRevisionRef != "" {
			return fmt.Errorf("Template Git change dimensions are invalid")
		}
		return c.Git.Validate(true)
	case WorkspaceTemplateChangeBootstrapAWS:
		if len(c.Shell) != 0 || c.Git != nil || c.EKS != nil || c.RuntimeRevisionRef != "" {
			return fmt.Errorf("Template AWS bootstrap change dimensions are invalid")
		}
		if c.AWS != nil {
			return c.AWS.Validate()
		}
		return nil
	case WorkspaceTemplateChangeBootstrapEKS:
		if len(c.Shell) != 0 || c.Git != nil || c.AWS != nil || c.RuntimeRevisionRef != "" {
			return fmt.Errorf("Template EKS bootstrap change dimensions are invalid")
		}
		if c.EKS != nil {
			return c.EKS.Validate()
		}
		return nil
	case WorkspaceTemplateChangeRuntime:
		if len(c.Shell) != 0 || c.Git != nil || c.AWS != nil || c.EKS != nil {
			return fmt.Errorf("Template Runtime change dimensions are invalid")
		}
		_, _, err := ParseRuntimeRevisionRef(c.RuntimeRevisionRef)
		return err
	default:
		return fmt.Errorf("Template change kind is invalid")
	}
}

func (c WorkspaceTemplateChange) Clone() WorkspaceTemplateChange {
	result := c
	result.Shell = cloneShellSettings(c.Shell)
	if c.Git != nil {
		value := cloneGitIdentity(*c.Git)
		result.Git = &value
	}
	if c.AWS != nil {
		value := c.AWS.Clone()
		result.AWS = &value
	}
	if c.EKS != nil {
		value := *c.EKS
		result.EKS = &value
	}
	return result
}

// ApplyWorkspaceTemplateChange derives one complete next body from the exact
// current body. resolvedRuntime is required only for Runtime changes and must
// correlate to the unchanged runtime-revision reference.
func ApplyWorkspaceTemplateChange(
	current WorkspaceTemplateBody,
	change WorkspaceTemplateChange,
	resolvedRuntime *RuntimeBinding,
) (WorkspaceTemplateBody, error) {
	if err := current.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	if err := change.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	next := current.Clone()
	switch change.Kind {
	case WorkspaceTemplateChangeShell:
		settings, err := ApplyContextShellEnvironmentSettings(next.SessionDefaults.ShellEnvironment, change.Shell)
		if err != nil {
			return WorkspaceTemplateBody{}, err
		}
		next.SessionDefaults.ShellEnvironment = settings
	case WorkspaceTemplateChangeGit:
		if change.Git.Source == ManifestGitIdentityDefault {
			next.SessionDefaults.GitIdentity = nil
		} else {
			value := cloneGitIdentity(*change.Git)
			next.SessionDefaults.GitIdentity = &value
		}
	case WorkspaceTemplateChangeBootstrapAWS:
		if change.AWS == nil {
			if next.CreationDefaults.Bootstrap != nil && next.CreationDefaults.Bootstrap.EKS != nil {
				return WorkspaceTemplateBody{}, fmt.Errorf("AWS bootstrap cannot be removed while EKS bootstrap exists")
			}
			next.CreationDefaults.Bootstrap = nil
			break
		}
		generation := uint64(1)
		var eks *ManifestEKSBootstrap
		if next.CreationDefaults.Bootstrap != nil {
			generation = next.CreationDefaults.Bootstrap.Generation + 1
			if next.CreationDefaults.Bootstrap.EKS != nil {
				value := *next.CreationDefaults.Bootstrap.EKS
				eks = &value
			}
		}
		var snapshot ManifestBootstrapSnapshot
		var err error
		if eks == nil {
			snapshot, err = NewContextBootstrapSnapshot(generation, change.AWS.Clone())
		} else {
			snapshot, err = NewContextBootstrapSnapshotWithEKS(generation, change.AWS.Clone(), *eks)
		}
		if err != nil {
			return WorkspaceTemplateBody{}, err
		}
		if current := next.CreationDefaults.Bootstrap; current != nil && current.Revision == snapshot.Revision {
			snapshot = current.Clone()
		}
		next.CreationDefaults.Bootstrap = &snapshot
	case WorkspaceTemplateChangeBootstrapEKS:
		currentBootstrap := next.CreationDefaults.Bootstrap
		if currentBootstrap == nil {
			if change.EKS == nil {
				break
			}
			return WorkspaceTemplateBody{}, fmt.Errorf("EKS bootstrap requires AWS bootstrap")
		}
		generation := currentBootstrap.Generation + 1
		var snapshot ManifestBootstrapSnapshot
		var err error
		if change.EKS == nil {
			snapshot, err = NewContextBootstrapSnapshot(generation, currentBootstrap.AWS.Clone())
		} else {
			snapshot, err = NewContextBootstrapSnapshotWithEKS(generation, currentBootstrap.AWS.Clone(), *change.EKS)
		}
		if err != nil {
			return WorkspaceTemplateBody{}, err
		}
		if currentBootstrap.Revision == snapshot.Revision {
			snapshot = currentBootstrap.Clone()
		}
		next.CreationDefaults.Bootstrap = &snapshot
	case WorkspaceTemplateChangeRuntime:
		if resolvedRuntime == nil {
			return WorkspaceTemplateBody{}, fmt.Errorf("Template Runtime change requires exact resolved revision authority")
		}
		if err := resolvedRuntime.Validate(); err != nil {
			return WorkspaceTemplateBody{}, err
		}
		runtimeID, revision, err := ParseRuntimeRevisionRef(change.RuntimeRevisionRef)
		if err != nil || resolvedRuntime.RuntimeID != runtimeID || resolvedRuntime.Revision != revision {
			return WorkspaceTemplateBody{}, fmt.Errorf("resolved Runtime does not match the unchanged revision reference")
		}
		next.EntryDefaults.Runtime = *resolvedRuntime
	}
	if err := next.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	return next, nil
}

func cloneShellSettings(values []ManifestShellEnvironmentSetting) []ManifestShellEnvironmentSetting {
	result := make([]ManifestShellEnvironmentSetting, len(values))
	for index, value := range values {
		result[index] = value
		if value.Value != nil {
			text := *value.Value
			result[index].Value = &text
		}
	}
	return result
}

func cloneGitIdentity(value ManifestGitIdentitySetting) ManifestGitIdentitySetting {
	result := value
	if value.Name != nil {
		text := *value.Name
		result.Name = &text
	}
	if value.Email != nil {
		text := *value.Email
		result.Email = &text
	}
	return result
}
