package authbroker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const ContextCredentialTargetKind = "research-context-credential"
const MaxContextProviderInventory = 64

type ContextAuthDecisionAuthority struct {
	Task              string                         `json:"task"`
	Context           ContextAuthenticationAuthority `json:"context"`
	Provider          string                         `json:"provider"`
	ProviderAuthority Provider                       `json:"provider_authority"`
	LoginMethod       string                         `json:"login_method,omitempty"`
	Previous          ProviderStatus                 `json:"previous"`
}

func (d ContextAuthDecisionAuthority) Validate() error {
	if d.Task != TaskLogin && d.Task != TaskImport && d.Task != TaskLogout {
		return fmt.Errorf("final Context authentication task is invalid")
	}
	if err := d.Context.ValidateFor(d.Context.ContextRef); err != nil {
		return err
	}
	if err := ValidateProviderID(d.Provider); err != nil {
		return err
	}
	if err := d.ProviderAuthority.Validate(); err != nil || d.ProviderAuthority.ID != d.Provider {
		return fmt.Errorf("final Context authentication reviewed provider authority is invalid: %w", err)
	}
	if err := d.Previous.Validate(); err != nil {
		return fmt.Errorf("final Context authentication precondition is invalid: %w", err)
	}
	if d.Previous.Provider != d.Provider {
		return fmt.Errorf("final Context authentication precondition provider is invalid")
	}
	if d.Task == TaskLogin {
		if d.Provider == BuiltinAWSProviderID {
			if d.LoginMethod != "identity-center" && d.LoginMethod != "console" {
				return fmt.Errorf("final AWS authentication login method is invalid")
			}
		} else if d.LoginMethod != "" {
			return fmt.Errorf("final Context authentication login method is not applicable")
		}
	} else if d.LoginMethod != "" {
		return fmt.Errorf("final Context authentication decision has an unexpected login method")
	}
	return nil
}

func (d ContextAuthDecisionAuthority) Digest() (tobari.SemanticDigest, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(sum[:])), nil
}

func (d ContextAuthDecisionAuthority) Reference() (string, error) {
	digest, err := d.Digest()
	if err != nil {
		return "", err
	}
	return "research-auth:" + d.Task + ":" + string(d.Context.ContextID) + ":" + d.Provider + ":" + string(digest), nil
}

func (d ContextAuthDecisionAuthority) ValidateReference(reference string) error {
	exact, err := d.Reference()
	if err != nil || exact != reference {
		return fmt.Errorf("final Context authentication decision reference is invalid")
	}
	return nil
}

// ContextAuthenticationAuthority is the complete final-only authority for one
// research authentication task. The Context reference is retained unchanged;
// ContextID is used only as the value of the frozen private Broker wire token.
// Runtime comes from the current immutable Template revision so a container
// login helper never consults a predecessor Manifest.
type ContextAuthenticationAuthority struct {
	ContextRef       string                     `json:"context_ref"`
	ContextID        tobari.ContextID           `json:"context_id"`
	TemplateRef      string                     `json:"template_ref"`
	TemplateID       tobari.WorkspaceTemplateID `json:"template_id"`
	TemplateRevision tobari.SemanticDigest      `json:"template_revision"`
	Runtime          tobari.RuntimeBinding      `json:"runtime"`
}

func NewContextAuthenticationAuthority(snapshot tobari.ContextAuthoritySnapshot, contextRef string) (ContextAuthenticationAuthority, error) {
	if err := snapshot.Validate(); err != nil {
		return ContextAuthenticationAuthority{}, err
	}
	contextID, err := tobari.ParseContextRef(contextRef)
	if err != nil || contextID != snapshot.Context.ID {
		return ContextAuthenticationAuthority{}, fmt.Errorf("authentication Context reference does not match final authority")
	}
	templateRef, err := tobari.WorkspaceTemplateRef(snapshot.Template.ID)
	if err != nil {
		return ContextAuthenticationAuthority{}, err
	}
	authority := ContextAuthenticationAuthority{
		ContextRef: contextRef, ContextID: contextID,
		TemplateRef: templateRef, TemplateID: snapshot.Template.ID,
		TemplateRevision: snapshot.Template.Current.Revision,
		Runtime:          snapshot.Template.Current.Body.EntryDefaults.Runtime,
	}
	return authority, authority.ValidateFor(contextRef)
}

func (a ContextAuthenticationAuthority) ValidateFor(contextRef string) error {
	contextID, err := tobari.ParseContextRef(contextRef)
	if err != nil || a.ContextRef != contextRef || a.ContextID != contextID {
		return fmt.Errorf("authentication authority does not preserve the exact Context reference")
	}
	if err := a.TemplateID.Validate(); err != nil {
		return err
	}
	templateRef, err := tobari.WorkspaceTemplateRef(a.TemplateID)
	if err != nil || a.TemplateRef != templateRef {
		return fmt.Errorf("authentication authority Template reference is invalid")
	}
	if err := a.TemplateRevision.Validate(); err != nil {
		return err
	}
	return a.Runtime.Validate()
}

// ContextProviderAuthority joins one complete final Context with one reviewed
// provider definition. The provider projection remains installation-owned;
// the Template Runtime binding selects any reviewed container login helper.
type ContextProviderAuthority struct {
	Context  ContextAuthenticationAuthority `json:"context"`
	Provider Provider                       `json:"provider"`
}

func (a ContextProviderAuthority) Clone() ContextProviderAuthority {
	a.Provider = cloneProvider(a.Provider)
	return a
}

// DecisionProvider returns the unique JSON-stable representation retained by
// the durable auth decision. Provider contains an omitempty union field, so a
// plain deep clone would otherwise distinguish an empty slice from the exact
// decoded journal representation.
func (a ContextProviderAuthority) DecisionProvider() (Provider, error) {
	if err := a.ValidateFor(a.Context.ContextRef, a.Provider.ID); err != nil {
		return Provider{}, err
	}
	encoded, err := json.Marshal(a.Provider)
	if err != nil {
		return Provider{}, err
	}
	var canonical Provider
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return Provider{}, err
	}
	if err := canonical.Validate(); err != nil {
		return Provider{}, err
	}
	return canonical, nil
}

func (a ContextProviderAuthority) ValidateFor(contextRef, provider string) error {
	if err := a.Context.ValidateFor(contextRef); err != nil {
		return err
	}
	if err := a.Provider.Validate(); err != nil {
		return err
	}
	if a.Provider.ID != provider {
		return fmt.Errorf("authentication provider authority does not match the request")
	}
	return nil
}

// ContextMutationObservation is the secret-free exact result of one final
// Context credential mutation. DecisionRef is private recovery correlation and
// is never part of the public schema projection.
type ContextMutationObservation struct {
	Authority      ContextAuthenticationAuthority
	Decision       ContextAuthDecisionAuthority
	Provider       ProviderStatus
	StorageBackend StorageBackend
	BrokerState    BrokerState
	Changed        bool
	DecisionRef    string
}

func (o ContextMutationObservation) ValidateFor(task, contextRef, provider string) error {
	if task != TaskLogin && task != TaskImport && task != TaskLogout {
		return fmt.Errorf("final Context authentication task is invalid")
	}
	if err := o.Authority.ValidateFor(contextRef); err != nil {
		return err
	}
	if err := o.Provider.Validate(); err != nil {
		return fmt.Errorf("final Context authentication provider result is invalid: %w", err)
	}
	if o.Provider.Provider != provider {
		return fmt.Errorf("final Context authentication provider result does not match the request")
	}
	if err := o.StorageBackend.Validate(); err != nil {
		return err
	}
	if o.BrokerState != BrokerStateReady {
		return fmt.Errorf("final Context authentication mutation requires a ready Broker")
	}
	if task == TaskLogin || task == TaskImport {
		if !o.Changed || o.Provider.State != ProviderCredentialConfigured {
			return fmt.Errorf("final Context credential create did not confirm a changed configured credential")
		}
	} else if o.Provider.State != ProviderCredentialNotConfigured {
		return fmt.Errorf("final Context logout did not confirm credential absence")
	}
	if err := o.Decision.Validate(); err != nil || o.Decision.Task != task || o.Decision.Context != o.Authority || o.Decision.Provider != provider {
		return fmt.Errorf("final Context authentication decision authority is invalid: %w", err)
	}
	if err := o.Decision.ValidateReference(o.DecisionRef); err != nil {
		return err
	}
	return nil
}

// ContextResult is the schema-v2 task-owned mutation result prepared for the
// later atomic public cutover. Raw ContextID and private decision receipts are
// deliberately absent from JSON.
type ContextResult struct {
	Task               string                         `json:"task"`
	ContextRef         string                         `json:"context_ref"`
	Provider           string                         `json:"provider"`
	Configured         bool                           `json:"configured"`
	AccountLabel       *string                        `json:"account_label"`
	StorageBackend     StorageBackend                 `json:"storage_backend"`
	BrokerState        BrokerState                    `json:"broker_state"`
	CredentialRevision string                         `json:"credential_revision"`
	Change             MutationChange                 `json:"change"`
	Authority          ContextAuthenticationAuthority `json:"-"`
	Decision           ContextAuthDecisionAuthority   `json:"-"`
	DecisionRef        string                         `json:"-"`
}

func NewContextResult(task, contextRef, provider string, observation ContextMutationObservation) (ContextResult, error) {
	if err := observation.ValidateFor(task, contextRef, provider); err != nil {
		return ContextResult{}, err
	}
	change := MutationChangeChanged
	if !observation.Changed {
		change = MutationChangeNoChange
	}
	result := ContextResult{
		Task: task, ContextRef: contextRef,
		Provider: provider, Configured: observation.Provider.State == ProviderCredentialConfigured,
		AccountLabel: observation.Provider.AccountLabel, StorageBackend: observation.StorageBackend,
		BrokerState: observation.BrokerState, CredentialRevision: observation.Provider.CredentialRevision,
		Change: change, Authority: observation.Authority, Decision: observation.Decision, DecisionRef: observation.DecisionRef,
	}
	return result, result.ValidateFor(task, contextRef, provider)
}

func (r ContextResult) ValidateFor(task, contextRef, provider string) error {
	if r.Task != task || r.ContextRef != contextRef || r.Provider != provider {
		return fmt.Errorf("final Context authentication result does not match the request")
	}
	if err := r.Authority.ValidateFor(contextRef); err != nil {
		return fmt.Errorf("final Context authentication result authority is invalid: %w", err)
	}
	status := ProviderStatus{Provider: r.Provider, AccountLabel: r.AccountLabel, CredentialRevision: r.CredentialRevision}
	if r.Configured {
		status.State = ProviderCredentialConfigured
	} else {
		status.State = ProviderCredentialNotConfigured
	}
	observation := ContextMutationObservation{Authority: r.Authority, Decision: r.Decision, Provider: status, StorageBackend: r.StorageBackend, BrokerState: r.BrokerState, Changed: r.Change == MutationChangeChanged, DecisionRef: r.DecisionRef}
	if err := r.Change.Validate(); err != nil {
		return err
	}
	return observation.ValidateFor(task, contextRef, provider)
}

// ContextStatusObservation is one bounded exhaustive read of every reviewed
// provider for one final Context. An unsafe or incomplete Broker store is an
// error, never an empty provider collection.
type ContextStatusObservation struct {
	Authority         ContextAuthenticationAuthority
	StorageBackend    StorageBackend
	BrokerState       BrokerState
	Providers         []ProviderStatus
	InventoryComplete bool
	InventoryDigest   tobari.SemanticDigest
}

func (o ContextStatusObservation) ValidateFor(contextRef string) error {
	if err := o.Authority.ValidateFor(contextRef); err != nil {
		return err
	}
	if err := o.StorageBackend.Validate(); err != nil {
		return err
	}
	if err := o.BrokerState.Validate(); err != nil {
		return err
	}
	if o.Providers == nil || len(o.Providers) > MaxContextProviderInventory {
		return fmt.Errorf("final Context authentication provider collection is unknown")
	}
	if o.InventoryComplete != (o.BrokerState == BrokerStateReady) {
		return fmt.Errorf("final Context authentication inventory coverage does not match Broker state")
	}
	if !o.InventoryComplete && len(o.Providers) != 0 {
		return fmt.Errorf("incomplete final Context authentication inventory carries provider claims")
	}
	seen := make(map[string]struct{}, len(o.Providers))
	previous := ""
	for _, provider := range o.Providers {
		if err := provider.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[provider.Provider]; duplicate || previous != "" && provider.Provider <= previous {
			return fmt.Errorf("final Context authentication providers are duplicated or unsorted")
		}
		seen[provider.Provider] = struct{}{}
		previous = provider.Provider
	}
	digest, err := contextProviderInventoryDigest(o.Authority.ContextID, o.Providers)
	if err != nil || o.InventoryDigest != digest {
		return fmt.Errorf("final Context authentication inventory receipt is invalid")
	}
	return nil
}

func NewContextStatusObservation(authority ContextAuthenticationAuthority, backend StorageBackend, state BrokerState, providers []ProviderStatus, complete bool) (ContextStatusObservation, error) {
	copyProviders := append([]ProviderStatus{}, providers...)
	sort.Slice(copyProviders, func(i, j int) bool { return copyProviders[i].Provider < copyProviders[j].Provider })
	digest, err := contextProviderInventoryDigest(authority.ContextID, copyProviders)
	if err != nil {
		return ContextStatusObservation{}, err
	}
	result := ContextStatusObservation{Authority: authority, StorageBackend: backend, BrokerState: state, Providers: copyProviders, InventoryComplete: complete, InventoryDigest: digest}
	return result, result.ValidateFor(authority.ContextRef)
}

func contextProviderInventoryDigest(contextID tobari.ContextID, providers []ProviderStatus) (tobari.SemanticDigest, error) {
	encoded, err := json.Marshal(struct {
		SchemaVersion int              `json:"schema_version"`
		ContextID     tobari.ContextID `json:"context_id"`
		Providers     []ProviderStatus `json:"providers"`
	}{SchemaVersion: 1, ContextID: contextID, Providers: providers})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(sum[:])), nil
}

type ContextStatusResult struct {
	Task              string                         `json:"task"`
	ContextRef        string                         `json:"context_ref"`
	StorageBackend    StorageBackend                 `json:"storage_backend"`
	BrokerState       BrokerState                    `json:"broker_state"`
	Providers         []ProviderStatus               `json:"providers"`
	Authority         ContextAuthenticationAuthority `json:"-"`
	InventoryComplete bool                           `json:"-"`
	InventoryDigest   tobari.SemanticDigest          `json:"-"`
}

func NewContextStatusResult(contextRef string, observation ContextStatusObservation) (ContextStatusResult, error) {
	if err := observation.ValidateFor(contextRef); err != nil {
		return ContextStatusResult{}, err
	}
	result := ContextStatusResult{Task: TaskStatus, ContextRef: contextRef, StorageBackend: observation.StorageBackend, BrokerState: observation.BrokerState, Providers: append([]ProviderStatus{}, observation.Providers...), Authority: observation.Authority, InventoryComplete: observation.InventoryComplete, InventoryDigest: observation.InventoryDigest}
	return result, result.ValidateFor(contextRef)
}

func (r ContextStatusResult) ValidateFor(contextRef string) error {
	if r.Task != TaskStatus || r.ContextRef != contextRef {
		return fmt.Errorf("final Context authentication status does not match the request")
	}
	return (ContextStatusObservation{Authority: r.Authority, StorageBackend: r.StorageBackend, BrokerState: r.BrokerState, Providers: r.Providers, InventoryComplete: r.InventoryComplete, InventoryDigest: r.InventoryDigest}).ValidateFor(contextRef)
}

func (r ContextStatusResult) CredentialsAbsent() bool {
	if r.ValidateFor(r.ContextRef) != nil || r.BrokerState != BrokerStateReady {
		return false
	}
	for _, provider := range r.Providers {
		if provider.State != ProviderCredentialNotConfigured {
			return false
		}
	}
	return true
}
