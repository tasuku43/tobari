package authbroker

import (
	"fmt"
	"regexp"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

var observationDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// BrokerBindingObservationState preserves the exact result of one bounded
// binding_status observation without deciding the user-facing activation state.
type BrokerBindingObservationState string

const (
	BrokerBindingNotObserved BrokerBindingObservationState = "not_observed"
	BrokerBindingReady       BrokerBindingObservationState = "ready"
	BrokerBindingMissing     BrokerBindingObservationState = "missing"
	BrokerBindingStale       BrokerBindingObservationState = "stale"
	BrokerBindingUnavailable BrokerBindingObservationState = "unavailable"
)

func (s BrokerBindingObservationState) Validate() error {
	switch s {
	case BrokerBindingNotObserved, BrokerBindingReady, BrokerBindingMissing,
		BrokerBindingStale, BrokerBindingUnavailable:
		return nil
	default:
		return fmt.Errorf("Broker binding observation state is invalid: %q", s)
	}
}

// WorkspaceProviderObservation carries only secret-free authority facts. The
// application joins these facts to the Context provider grant collection and
// decides current/missing/stale/unavailable semantics.
type WorkspaceProviderObservation struct {
	Provider              string
	RegistryPresent       bool
	RegistryRevision      string
	RegistryBindingDigest string
	ExpectedBindingDigest string
	BindingState          BrokerBindingObservationState
	BindingProvider       string
	BindingRevision       string
}

// WorkspaceProjectionObservation is one logical project and its stored
// registry/broker facts. ProjectContextID is authoritative eligibility; the
// display Context name is never used for selection.
type WorkspaceProjectionObservation struct {
	ProjectID         string
	Root              string
	ProjectContextID  string
	Incomplete        bool
	RegistryAvailable bool
	RegistryProjectID string
	Providers         []WorkspaceProviderObservation
}

// WorkspaceObservation is a bounded project enumeration. Unavailable coverage
// carries no rows; exhaustive coverage may be empty.
type WorkspaceObservation struct {
	Coverage   WorkspaceActivationCoverage
	Workspaces []WorkspaceProjectionObservation
}

// MutationObservation is the secret-free authoritative runtime receipt. The
// application owns task identity, change semantics, and Workspace activation.
type MutationObservation struct {
	ManifestState       tobari.ManifestObservationState
	Provider            string
	Context             string
	WorkspaceManifestID string
	Configured          bool
	AccountLabel        *string
	StorageBackend      StorageBackend
	BrokerState         BrokerState
	CredentialRevision  string
	Changed             bool
	Providers           []ProviderStatus
	Workspaces          WorkspaceObservation
}

// StatusObservation is the secret-free runtime snapshot used by auth status.
type StatusObservation struct {
	ManifestState       tobari.ManifestObservationState
	Context             string
	WorkspaceManifestID string
	StorageBackend      StorageBackend
	BrokerState         BrokerState
	Providers           []ProviderStatus
	Workspaces          WorkspaceObservation
}

// NewResult validates the authoritative mutation observation against the
// task-bound Context/provider before deriving public semantics.
func NewResult(task, requestedContext, requestedProvider string, observed MutationObservation) (Result, error) {
	if requestedContext != "" && observed.Context != requestedContext {
		return Result{}, fmt.Errorf("authentication mutation Context does not match the request")
	}
	if observed.Provider != requestedProvider {
		return Result{}, fmt.Errorf("authentication mutation provider does not match the request")
	}
	activation := NotApplicableWorkspaceActivation()
	change := MutationChangeChanged
	if !observed.Changed {
		if len(observed.Providers) != 0 || observed.Workspaces.Coverage != WorkspaceActivationCoverageNotApplicable || len(observed.Workspaces.Workspaces) != 0 {
			return Result{}, fmt.Errorf("no-change mutation observation carries activation change facts")
		}
		change = MutationChangeNoChange
	} else {
		var err error
		activation, err = workspaceActivationFromObservation(
			observed.Context, observed.WorkspaceManifestID, observed.Providers, observed.Workspaces,
		)
		if err != nil {
			return Result{}, err
		}
	}
	result := Result{
		Task: task, ManifestState: observed.ManifestState, Provider: observed.Provider,
		Context: observed.Context, WorkspaceManifestID: observed.WorkspaceManifestID, Configured: observed.Configured,
		AccountLabel: observed.AccountLabel, StorageBackend: observed.StorageBackend,
		BrokerState: observed.BrokerState, CredentialRevision: observed.CredentialRevision,
		Change: change, WorkspaceActivation: activation,
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// NewStatusResult validates the runtime scope and derives the public Workspace
// activation collection independently from presentation.
func NewStatusResult(requestedContext string, observed StatusObservation) (StatusResult, error) {
	if requestedContext != "" && observed.Context != requestedContext {
		return StatusResult{}, fmt.Errorf("authentication status Context does not match the request")
	}
	activation := NotApplicableWorkspaceActivation()
	var err error
	if observed.ManifestState == tobari.ManifestObservationPersisted {
		activation, err = workspaceActivationFromObservation(
			observed.Context, observed.WorkspaceManifestID, observed.Providers, observed.Workspaces,
		)
		if err != nil {
			return StatusResult{}, err
		}
	} else if observed.Workspaces.Coverage != WorkspaceActivationCoverageNotApplicable || len(observed.Workspaces.Workspaces) != 0 {
		return StatusResult{}, fmt.Errorf("non-persisted auth status observation carries Workspace authority")
	}
	result := StatusResult{
		Task: TaskStatus, ManifestState: observed.ManifestState, Context: observed.Context,
		WorkspaceManifestID: observed.WorkspaceManifestID, StorageBackend: observed.StorageBackend,
		BrokerState: observed.BrokerState, Providers: append(make([]ProviderStatus, 0, len(observed.Providers)), observed.Providers...),
		WorkspaceActivation: activation,
	}
	if err := result.Validate(); err != nil {
		return StatusResult{}, err
	}
	return result, nil
}

func workspaceActivationFromObservation(
	contextName, contextID string,
	statuses []ProviderStatus,
	observed WorkspaceObservation,
) (WorkspaceActivation, error) {
	if observed.Coverage == WorkspaceActivationCoverageUnavailable {
		if len(observed.Workspaces) != 0 {
			return WorkspaceActivation{}, fmt.Errorf("unavailable Workspace observation carries rows")
		}
		return UnavailableWorkspaceActivation(contextName, contextID), nil
	}
	if observed.Coverage != WorkspaceActivationCoverageExhaustive {
		return WorkspaceActivation{}, fmt.Errorf("persisted Workspace observation coverage is invalid")
	}
	if len(statuses) > MaxWorkspaceActivationProviders || len(observed.Workspaces) > MaxWorkspaceActivationItems {
		return WorkspaceActivation{}, fmt.Errorf("Workspace observation exceeds its collection bound")
	}
	statusByProvider := make(map[string]ProviderStatus, len(statuses))
	for _, status := range statuses {
		if err := status.Validate(); err != nil {
			return WorkspaceActivation{}, err
		}
		if _, duplicate := statusByProvider[status.Provider]; duplicate {
			return WorkspaceActivation{}, fmt.Errorf("provider observation %q is duplicated", status.Provider)
		}
		statusByProvider[status.Provider] = status
	}
	items := make([]WorkspaceActivationItem, 0, len(observed.Workspaces))
	seenProjects := make(map[string]struct{}, len(observed.Workspaces))
	for _, workspace := range observed.Workspaces {
		if err := tobari.ValidateWorkspaceID(workspace.ProjectID); err != nil {
			return WorkspaceActivation{}, err
		}
		if err := tobari.ValidateCanonicalRoot(workspace.Root); err != nil {
			return WorkspaceActivation{}, err
		}
		if !contextIDPattern.MatchString(workspace.ProjectContextID) {
			return WorkspaceActivation{}, fmt.Errorf("Workspace observation Context ID is invalid")
		}
		if _, duplicate := seenProjects[workspace.ProjectID]; duplicate {
			return WorkspaceActivation{}, fmt.Errorf("Workspace observation project %q is duplicated", workspace.ProjectID)
		}
		seenProjects[workspace.ProjectID] = struct{}{}
		if workspace.RegistryAvailable {
			if workspace.RegistryProjectID != workspace.ProjectID {
				return WorkspaceActivation{}, fmt.Errorf("Workspace registry project identity does not match its logical project")
			}
		} else if workspace.RegistryProjectID != "" || len(workspace.Providers) != 0 {
			return WorkspaceActivation{}, fmt.Errorf("unavailable Workspace registry claims identity or provider facts")
		}
		if len(workspace.Providers) > MaxWorkspaceActivationProviders {
			return WorkspaceActivation{}, fmt.Errorf("Workspace provider observation exceeds its collection bound")
		}
		if workspace.ProjectContextID != contextID {
			continue
		}
		providerFacts := make(map[string]WorkspaceProviderObservation, len(workspace.Providers))
		for _, fact := range workspace.Providers {
			if err := ValidateProviderID(fact.Provider); err != nil {
				return WorkspaceActivation{}, err
			}
			if err := fact.BindingState.Validate(); err != nil {
				return WorkspaceActivation{}, err
			}
			if _, duplicate := providerFacts[fact.Provider]; duplicate {
				return WorkspaceActivation{}, fmt.Errorf("Workspace provider observation %q is duplicated", fact.Provider)
			}
			if fact.RegistryPresent {
				if !revisionPattern.MatchString(fact.RegistryRevision) || !observationDigestPattern.MatchString(fact.RegistryBindingDigest) {
					return WorkspaceActivation{}, fmt.Errorf("Workspace registry provider observation is invalid")
				}
			} else if fact.RegistryRevision != "" || fact.RegistryBindingDigest != "" {
				return WorkspaceActivation{}, fmt.Errorf("absent Workspace registry provider carries stored authority")
			}
			if fact.ExpectedBindingDigest != "" && !observationDigestPattern.MatchString(fact.ExpectedBindingDigest) {
				return WorkspaceActivation{}, fmt.Errorf("expected Workspace binding digest is invalid")
			}
			if fact.BindingState == BrokerBindingNotObserved {
				if fact.BindingProvider != "" || fact.BindingRevision != "" {
					return WorkspaceActivation{}, fmt.Errorf("unobserved Broker binding carries request identity")
				}
			} else if fact.BindingProvider != fact.Provider || fact.BindingRevision != fact.RegistryRevision {
				return WorkspaceActivation{}, fmt.Errorf("Broker binding observation does not match its provider/revision request")
			}
			providerFacts[fact.Provider] = fact
		}
		providerIDs := make(map[string]struct{}, len(statuses)+len(providerFacts))
		for provider := range statusByProvider {
			providerIDs[provider] = struct{}{}
		}
		for provider := range providerFacts {
			providerIDs[provider] = struct{}{}
		}
		if len(providerIDs) > MaxWorkspaceActivationProviders {
			return WorkspaceActivation{}, fmt.Errorf("Workspace provider join exceeds its collection bound")
		}
		providers := make([]WorkspaceProviderActivation, 0, len(providerIDs))
		for provider := range providerIDs {
			status, installed := statusByProvider[provider]
			fact, projected := providerFacts[provider]
			state := WorkspaceProviderProjectionNotApplicable
			switch {
			case !workspace.RegistryAvailable:
				if installed && status.State != ProviderCredentialNotConfigured {
					state = WorkspaceProviderProjectionUnavailable
				}
			case !installed:
				state = WorkspaceProviderProjectionStale
			case status.State == ProviderCredentialUnavailable:
				state = WorkspaceProviderProjectionUnavailable
			case status.State == ProviderCredentialNotConfigured:
				if projected && fact.RegistryPresent {
					state = WorkspaceProviderProjectionStale
				}
			case status.State == ProviderCredentialConfigured:
				switch {
				case !projected || !fact.RegistryPresent:
					state = WorkspaceProviderProjectionMissing
				case fact.ExpectedBindingDigest == "":
					state = WorkspaceProviderProjectionUnavailable
				case fact.RegistryRevision != status.CredentialRevision || fact.RegistryBindingDigest != fact.ExpectedBindingDigest:
					state = WorkspaceProviderProjectionStale
				default:
					switch fact.BindingState {
					case BrokerBindingReady:
						state = WorkspaceProviderProjectionCurrent
					case BrokerBindingMissing:
						state = WorkspaceProviderProjectionMissing
					case BrokerBindingStale:
						state = WorkspaceProviderProjectionStale
					default:
						state = WorkspaceProviderProjectionUnavailable
					}
				}
			}
			providers = append(providers, WorkspaceProviderActivation{Provider: provider, State: state})
		}
		item, err := NewWorkspaceActivationItem(
			workspace.ProjectID, workspace.Root, contextName, contextID, providers,
			workspace.Incomplete || !workspace.RegistryAvailable,
		)
		if err != nil {
			return WorkspaceActivation{}, err
		}
		items = append(items, item)
	}
	return NewWorkspaceActivation(contextName, contextID, items)
}
