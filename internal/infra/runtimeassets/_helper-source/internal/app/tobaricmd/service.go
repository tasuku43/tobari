// Package tobaricmd owns shared-cluster and named-Tobari use cases.
package tobaricmd

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type RuntimePort interface {
	CurrentDirectory(context.Context) (string, error)
	IsTerminal(io.Writer) bool
	ValidateClusterBuildIdentity(context.Context) error
	ClusterUp(context.Context) (tobari.State, error)
	LoadState(context.Context) (tobari.State, bool, error)
	InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error)
	ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error)
	ClusterDenials(context.Context, tobari.State, int) (tobari.DenialRead, error)
	ReadLearnedPolicyRules(context.Context, tobari.State) ([]tobari.LearnedPolicyRule, error)
	ReadPolicyDenyRules(context.Context, tobari.State) (tobari.PolicyDenyRuleSet, error)
	ApplyLearnedPolicyRules(
		context.Context, tobari.State, []tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
	) (tobari.PolicyActivationReceipt, error)
	ApplyPolicyDenyRules(
		context.Context, tobari.State, []tobari.LearnedPolicyRule,
		[]tobari.PolicyDenyRule, []tobari.PolicyDenyRule,
	) (tobari.PolicyActivationReceipt, error)
	ClusterDown(context.Context, tobari.State, bool) error
}

// lifecycleRuntimePort serializes shared-cluster and CWD-owned project
// lifecycle operations. It is intentionally separate from the broader
// RuntimePort so observation ports cannot acquire a
// lock they do not need.
type lifecycleRuntimePort interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	validCurrentDirectory := intent.Target.Kind == tobari.CurrentDirectoryTargetKind &&
		(intent.Target.ID == "" || intent.Target.ID == tobari.CurrentDirectoryTargetID) &&
		(intent.Target.ParentID == "" || intent.Target.ParentID == tobari.CurrentDirectoryTargetID)
	switch intent.Effect {
	case operation.EffectCreate:
		validCluster := intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ParentID == tobari.ClusterTargetID
		if !validCluster && !validCurrentDirectory {
			return fault.New(fault.KindRejected, "mutation_rejected", "cluster creation scope is not owned by Tobari", false)
		}
	case operation.EffectWrite:
		validCluster := intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ID == tobari.ClusterTargetID
		validPolicyCandidate := intent.Target.Kind == tobari.PolicyCandidateKind && intent.Target.ID != ""
		validPolicyRule := intent.Target.Kind == tobari.PolicyRuleKind && intent.Target.ID != ""
		validPolicyDecisionSet := intent.Target.Kind == tobari.PolicyDecisionSetKind &&
			intent.Target.ID == tobari.PolicyDecisionSetID
		if !validCluster && !validPolicyCandidate && !validPolicyRule && !validPolicyDecisionSet && !validCurrentDirectory {
			return fault.New(fault.KindRejected, "mutation_rejected", "mutation target is not owned by Tobari", false)
		}
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "mutation effect is not supported", false)
	}
	return nil
}

// Service coordinates validated tasks without depending on Docker.
type Service struct {
	runtime  RuntimePort
	mutator  *execution.Invoker
	selector WorkspaceSelector
}

func New(runtime RuntimePort) *Service {
	return NewWithWorkspaceSelector(runtime, nil)
}

func NewWithWorkspaceSelector(runtime RuntimePort, selector WorkspaceSelector) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{}), selector: selector}
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Tobari runtime is not configured", false)
	}
	return nil
}

// IsTerminal reports whether the injected writer is an interactive terminal.
// Terminal ownership remains in the runtime adapter; the CLI uses this only
// to decide whether to attach human progress presentation.
func (s *Service) IsTerminal(writer io.Writer) bool {
	if s == nil || portcheck.IsNil(s.runtime) {
		return false
	}
	return s.runtime.IsTerminal(writer)
}

// IsInputTerminal reports whether the injected input is an interactive
// terminal. RuntimePort deliberately keeps this capability optional so
// read-only and application test doubles do not need to model terminal
// inspection just to use the policy service.
func (s *Service) IsInputTerminal(reader io.Reader) bool {
	if s == nil || portcheck.IsNil(s.runtime) {
		return false
	}
	inputTerminal, ok := s.runtime.(interface {
		IsInputTerminal(io.Reader) bool
	})
	if !ok || portcheck.IsNil(inputTerminal) {
		return false
	}
	return inputTerminal.IsInputTerminal(reader)
}

// IsInteractive reports whether a human-facing workflow may safely read
// commands and confirmations. Both streams must be terminals; a redirected
// input or output must remain on the read-only path.
func (s *Service) IsInteractive(in io.Reader, out io.Writer) bool {
	return s.IsTerminal(out) && s.IsInputTerminal(in)
}

func (s *Service) withLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	if err := s.requireRuntime(); err != nil {
		return err
	}
	lifecycle, ok := s.runtime.(lifecycleRuntimePort)
	if !ok || portcheck.IsNil(lifecycle) {
		return fault.New(
			fault.KindInternal, "missing_runtime",
			"Tobari lifecycle lock is not configured", false,
		)
	}
	return lifecycle.WithLifecycleLock(ctx, action)
}
