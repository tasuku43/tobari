package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// FinalContextCredentialAuthority is the narrow closed-Broker boundary. It
// accepts only complete final Context/provider authority and has no name,
// predecessor Manifest, default, or quarantine lookup method.
type FinalContextCredentialAuthority interface {
	WithFinalContextAuthObservation(context.Context, func(context.Context) error) error
	ResolveFinalContextProvider(context.Context, authbroker.ContextAuthenticationAuthority, string) (authbroker.ContextProviderAuthority, error)
	ObserveFinalContextProvider(context.Context, authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error)
	ObserveFinalContextInventory(context.Context, authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error)
	LoginFinalContextProvider(context.Context, authbroker.ContextProviderAuthority, string, io.Reader, io.Writer) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error)
	ImportFinalContextProvider(context.Context, authbroker.ContextProviderAuthority, io.Reader) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error)
	LogoutFinalContextProvider(context.Context, authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error)
}

// FinalContextAuthAdapter shares Mutator's lifecycle lock, stage, and durable
// active/terminal decision. It remains dormant until the atomic WP11 cutover.
type FinalContextAuthAdapter struct {
	mutator  *Mutator
	broker   FinalContextCredentialAuthority
	lifetime context.Context
}

func NewFinalContextAuthAdapter(mutator *Mutator, broker FinalContextCredentialAuthority, lifetime context.Context) (*FinalContextAuthAdapter, error) {
	if mutator == nil || mutator.store == nil || mutator.lifecycle == nil || broker == nil || lifetime == nil {
		return nil, fmt.Errorf("final Context research authentication authorities are required")
	}
	if mutator.researchAuth != nil {
		return nil, fmt.Errorf("final Context research authentication authority is already configured")
	}
	mutator.researchAuth = broker
	adapter := &FinalContextAuthAdapter{mutator: mutator, broker: broker, lifetime: lifetime}
	mutator.credentialAbsence = adapter
	return adapter, nil
}

func (a *FinalContextAuthAdapter) IsInputTerminal(input io.Reader) bool {
	if terminal, ok := a.broker.(interface{ IsInputTerminal(io.Reader) bool }); ok {
		return terminal.IsInputTerminal(input)
	}
	return false
}

func (a *FinalContextAuthAdapter) IsTerminal(output io.Writer) bool {
	if terminal, ok := a.broker.(interface{ IsTerminal(io.Writer) bool }); ok {
		return terminal.IsTerminal(output)
	}
	return false
}

func (a *FinalContextAuthAdapter) StatusFinalContextAuth(ctx context.Context, authority authbroker.ContextAuthenticationAuthority) (result authbroker.ContextStatusObservation, resultErr error) {
	if a == nil || a.mutator == nil || a.broker == nil {
		return result, fmt.Errorf("final Context research authentication adapter is unavailable")
	}
	resultErr = a.broker.WithFinalContextAuthObservation(ctx, func(observationContext context.Context) error {
		before, present, err := a.mutator.store.ReadComplete(observationContext)
		if err != nil || !present {
			return fmt.Errorf("read final Context authentication authority before status: %w", err)
		}
		if err := a.confirmCurrentAuthorityCollection(before, authority); err != nil {
			return err
		}
		first, err := a.broker.ObserveFinalContextInventory(observationContext, authority)
		if err != nil {
			return err
		}
		if err := first.ValidateFor(authority.ContextRef); err != nil {
			return err
		}
		after, present, err := a.mutator.store.ReadComplete(observationContext)
		if err != nil || !present {
			return fmt.Errorf("read final Context authentication authority after status: %w", err)
		}
		if err := a.confirmCurrentAuthorityCollection(after, authority); err != nil {
			return err
		}
		second, err := a.broker.ObserveFinalContextInventory(observationContext, authority)
		if err != nil {
			return err
		}
		if err := second.ValidateFor(authority.ContextRef); err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(first, second) {
			return fmt.Errorf("final Context authentication status changed during observation")
		}
		result = second
		return nil
	})
	return result, resultErr
}

func (a *FinalContextAuthAdapter) LoginFinalContextAuth(ctx context.Context, authority authbroker.ContextAuthenticationAuthority, provider, method string, input io.Reader, errOut io.Writer) (authbroker.ContextMutationObservation, error) {
	return a.mutate(ctx, authbroker.TaskLogin, authority, provider, method, func(effectContext context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error) {
		status, backend, state, err := a.broker.LoginFinalContextProvider(effectContext, target, method, input, errOut)
		return status, true, backend, state, err
	})
}

func (a *FinalContextAuthAdapter) ImportFinalContextAuth(ctx context.Context, authority authbroker.ContextAuthenticationAuthority, provider string, secret io.Reader) (authbroker.ContextMutationObservation, error) {
	return a.mutate(ctx, authbroker.TaskImport, authority, provider, "", func(effectContext context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error) {
		status, backend, state, err := a.broker.ImportFinalContextProvider(effectContext, target, secret)
		return status, true, backend, state, err
	})
}

func (a *FinalContextAuthAdapter) LogoutFinalContextAuth(ctx context.Context, authority authbroker.ContextAuthenticationAuthority, provider string) (authbroker.ContextMutationObservation, error) {
	return a.mutate(ctx, authbroker.TaskLogout, authority, provider, "", func(effectContext context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error) {
		return a.broker.LogoutFinalContextProvider(effectContext, target)
	})
}

type finalContextCredentialEffect func(context.Context, authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error)

func (a *FinalContextAuthAdapter) mutate(ctx context.Context, task string, authority authbroker.ContextAuthenticationAuthority, provider, method string, mutate finalContextCredentialEffect) (result authbroker.ContextMutationObservation, resultErr error) {
	if a == nil || a.mutator == nil || a.broker == nil || mutate == nil {
		return result, fmt.Errorf("final Context research authentication adapter is unavailable")
	}
	if err := authority.ValidateFor(authority.ContextRef); err != nil {
		return result, err
	}
	operation := map[string]string{authbroker.TaskLogin: "research-auth-login", authbroker.TaskImport: "research-auth-import", authbroker.TaskLogout: "research-auth-logout"}[task]
	if operation == "" {
		return result, fmt.Errorf("final Context research authentication task is invalid")
	}
	requestMatches := func(decision effectDecision) bool {
		return decision.AuthDecision != nil && decision.AuthDecision.Task == task && decision.AuthDecision.Provider == provider && decision.AuthDecision.LoginMethod == method && decision.AuthDecision.Context == authority
	}
	committed, resultErr := a.mutator.effectfulMutate(ctx, operation, authority.ContextRef, requestMatches, func(current tobari.WorkspaceAuthorityCollection, recovering bool) (effectPlan, error) {
		if err := a.confirmCurrentAuthorityCollection(current, authority); err != nil {
			return effectPlan{}, err
		}
		target, err := a.broker.ResolveFinalContextProvider(ctx, authority, provider)
		if err != nil {
			return effectPlan{}, err
		}
		previous, _, brokerState, err := a.broker.ObserveFinalContextProvider(ctx, target)
		if err != nil {
			return effectPlan{}, err
		}
		if brokerState != authbroker.BrokerStateReady || previous.State == authbroker.ProviderCredentialUnavailable {
			return effectPlan{}, fmt.Errorf("final Context credential precondition is unavailable")
		}
		target = target.Clone()
		providerAuthority, err := target.DecisionProvider()
		if err != nil {
			return effectPlan{}, err
		}
		decision := authbroker.ContextAuthDecisionAuthority{Task: task, Context: authority, Provider: provider, ProviderAuthority: providerAuthority, LoginMethod: method, Previous: previous}
		if recovering {
			active, present, readErr := a.mutator.readEffectDecision()
			if readErr != nil || !present || active.AuthDecision == nil {
				return effectPlan{}, fmt.Errorf("final Context credential recovery decision is unavailable: %w", readErr)
			}
			decision = *active.AuthDecision
			if !reflect.DeepEqual(providerAuthority, decision.ProviderAuthority) {
				return effectPlan{}, fmt.Errorf("final Context reviewed provider authority changed after the durable decision")
			}
		}
		if err := decision.Validate(); err != nil {
			return effectPlan{}, err
		}
		var observed authbroker.ContextMutationObservation
		return effectPlan{
			next: current.Clone(), decision: effectDecision{AuthDecision: &decision},
			effect: func(effectContext context.Context) error {
				value, err := a.applyDecision(effectContext, target, decision, mutate)
				if err == nil {
					observed = value
				}
				return err
			},
			finalizeDecision: func(durable effectDecision) (effectDecision, error) {
				if err := observed.ValidateFor(task, authority.ContextRef, provider); err != nil {
					return effectDecision{}, err
				}
				copyResult := observed
				durable.AuthResult = &copyResult
				return durable, nil
			},
		}, nil
	})
	if resultErr != nil {
		if committed.AuthResult != nil {
			return authbroker.ContextMutationObservation{}, fault.WithClassification(
				fault.New(fault.KindUnavailable, "research_auth_result_delivery_interrupted", "Research authentication changed, but result delivery was interrupted; inspect exact Context authentication status.", false, fault.NextAction{Command: "auth status", Reason: "Read the exact Context credential inventory before choosing another mutation."}),
				fault.PhaseMutation, fault.ChangeConfirmed,
			)
		}
		return result, resultErr
	}
	if committed.AuthResult == nil {
		return result, fmt.Errorf("final Context credential mutation has no confirmed terminal receipt")
	}
	result = *committed.AuthResult
	return result, result.ValidateFor(task, authority.ContextRef, provider)
}

func (a *FinalContextAuthAdapter) applyDecision(ctx context.Context, target authbroker.ContextProviderAuthority, decision authbroker.ContextAuthDecisionAuthority, mutate finalContextCredentialEffect) (authbroker.ContextMutationObservation, error) {
	current, backend, state, err := a.broker.ObserveFinalContextProvider(ctx, target)
	if err != nil {
		return authbroker.ContextMutationObservation{}, err
	}
	if state != authbroker.BrokerStateReady {
		return authbroker.ContextMutationObservation{}, fmt.Errorf("final Context credential observation is unavailable")
	}
	decisionRef, err := decision.Reference()
	if err != nil {
		return authbroker.ContextMutationObservation{}, err
	}
	if !reflect.DeepEqual(current, decision.Previous) {
		if finalContextAuthConsequence(decision.Task, decision.Previous, current) {
			return authbroker.ContextMutationObservation{Authority: decision.Context, Decision: decision, Provider: current, StorageBackend: backend, BrokerState: state, Changed: true, DecisionRef: decisionRef}, nil
		}
		return authbroker.ContextMutationObservation{}, fmt.Errorf("final Context credential changed outside the durable decision")
	}
	if decision.Task == authbroker.TaskLogout && current.State == authbroker.ProviderCredentialNotConfigured {
		return authbroker.ContextMutationObservation{Authority: decision.Context, Decision: decision, Provider: current, StorageBackend: backend, BrokerState: state, Changed: false, DecisionRef: decisionRef}, nil
	}
	status, changed, backend, state, effectErr := mutate(ctx, target)
	if effectErr == nil {
		observation := authbroker.ContextMutationObservation{Authority: decision.Context, Decision: decision, Provider: status, StorageBackend: backend, BrokerState: state, Changed: changed, DecisionRef: decisionRef}
		return observation, observation.ValidateFor(decision.Task, decision.Context.ContextRef, decision.Provider)
	}
	settlementContext, cancel := context.WithTimeout(a.lifetime, 10*time.Second)
	defer cancel()
	observed, observedBackend, observedState, observeErr := a.broker.ObserveFinalContextProvider(settlementContext, target)
	if observeErr == nil && observedState == authbroker.BrokerStateReady && finalContextAuthConsequence(decision.Task, decision.Previous, observed) {
		observation := authbroker.ContextMutationObservation{Authority: decision.Context, Decision: decision, Provider: observed, StorageBackend: observedBackend, BrokerState: observedState, Changed: true, DecisionRef: decisionRef}
		return observation, observation.ValidateFor(decision.Task, decision.Context.ContextRef, decision.Provider)
	}
	return authbroker.ContextMutationObservation{}, fault.WithClassification(fault.New(fault.KindUnavailable, "research_auth_mutation_interrupted", "Research authentication mutation may be incomplete; inspect exact Context authentication status.", false, fault.NextAction{Command: "auth status", Reason: "Read the exact Context credential inventory before another mutation."}), fault.PhaseMutation, fault.ChangePartial)
}

func finalContextAuthConsequence(task string, previous, current authbroker.ProviderStatus) bool {
	if current.Provider != previous.Provider {
		return false
	}
	if task == authbroker.TaskLogout {
		return current.State == authbroker.ProviderCredentialNotConfigured
	}
	return current.State == authbroker.ProviderCredentialConfigured && (previous.State != authbroker.ProviderCredentialConfigured || current.CredentialRevision != previous.CredentialRevision)
}

func (a *FinalContextAuthAdapter) confirmCurrentAuthority(ctx context.Context, authority authbroker.ContextAuthenticationAuthority) error {
	snapshot, err := a.mutator.store.ReadContextAuthorityByReference(ctx, authority.ContextRef)
	if err != nil {
		return err
	}
	current, err := authbroker.NewContextAuthenticationAuthority(snapshot, authority.ContextRef)
	if err != nil || current != authority {
		return fmt.Errorf("final Context authentication authority changed after review")
	}
	return nil
}

func (a *FinalContextAuthAdapter) confirmCurrentAuthorityCollection(collection tobari.WorkspaceAuthorityCollection, authority authbroker.ContextAuthenticationAuthority) error {
	snapshot, err := snapshotForContext(collection, authority.ContextID)
	if err != nil {
		return err
	}
	current, err := authbroker.NewContextAuthenticationAuthority(snapshot, authority.ContextRef)
	if err != nil || current != authority {
		return fmt.Errorf("final Context authentication authority changed after review")
	}
	return nil
}

// ConfirmContextCredentialAbsent is the Context-delete prerequisite. The
// caller holds the same installation lifecycle lock, so no credential create
// can interleave between this exhaustive check and delete publication.
func (a *FinalContextAuthAdapter) ConfirmContextCredentialAbsent(ctx context.Context, contextID tobari.ContextID) error {
	contextRef, err := tobari.ContextRef(contextID)
	if err != nil {
		return err
	}
	snapshot, err := a.mutator.store.ReadContextAuthorityByReference(ctx, contextRef)
	if err != nil {
		return err
	}
	authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, contextRef)
	if err != nil {
		return err
	}
	status, err := a.broker.ObserveFinalContextInventory(ctx, authority)
	if err != nil {
		return err
	}
	if err := status.ValidateFor(contextRef); err != nil || !status.InventoryComplete || status.BrokerState != authbroker.BrokerStateReady {
		return errors.Join(fmt.Errorf("final Context credential absence is not exhaustively confirmed"), err)
	}
	for _, provider := range status.Providers {
		if provider.State != authbroker.ProviderCredentialNotConfigured {
			return tobari.ErrContextBindingProtected
		}
	}
	return nil
}
