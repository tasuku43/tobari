package authbroker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const testContextID = "01912345-6789-7abc-8def-0123456789ad"
const testProjectID = "01912345-6789-7abc-8def-0123456789ab"

func activationItem(t *testing.T, states ...WorkspaceProviderProjectionState) WorkspaceActivationItem {
	t.Helper()
	providers := make([]WorkspaceProviderActivation, 0, len(states))
	for index, state := range states {
		provider := "github"
		if index > 0 {
			provider = "aws"
		}
		providers = append(providers, WorkspaceProviderActivation{Provider: provider, State: state})
	}
	item, err := NewWorkspaceActivationItem(testProjectID, "/workspace/project", "default", testContextID, providers, false)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func exhaustiveActivation(t *testing.T, items ...WorkspaceActivationItem) WorkspaceActivation {
	t.Helper()
	activation, err := NewWorkspaceActivation("default", testContextID, items)
	if err != nil {
		t.Fatal(err)
	}
	return activation
}

func configuredResult(t *testing.T, task string) Result {
	t.Helper()
	account := "octocat"
	return Result{
		Task: task, ManifestState: tobari.ManifestObservationPersisted, Provider: "github", Context: "default", WorkspaceManifestID: testContextID,
		Configured: true, AccountLabel: &account, StorageBackend: StorageBackendXDGFile,
		BrokerState: BrokerStateReady, CredentialRevision: "sha256:abcdef0123456789", Change: MutationChangeChanged,
		WorkspaceActivation: exhaustiveActivation(t, activationItem(t, WorkspaceProviderProjectionMissing)),
	}
}

func TestWorkspaceActivationTruthTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		states []WorkspaceProviderProjectionState
		want   WorkspaceActivationState
		action bool
	}{
		{name: "no configured provider", want: WorkspaceActivationNotApplicable},
		{name: "current", states: []WorkspaceProviderProjectionState{WorkspaceProviderProjectionCurrent}, want: WorkspaceActivationReady},
		{name: "missing", states: []WorkspaceProviderProjectionState{WorkspaceProviderProjectionMissing}, want: WorkspaceActivationReentryRequired, action: true},
		{name: "stale", states: []WorkspaceProviderProjectionState{WorkspaceProviderProjectionStale}, want: WorkspaceActivationReentryRequired, action: true},
		{name: "unavailable", states: []WorkspaceProviderProjectionState{WorkspaceProviderProjectionUnavailable}, want: WorkspaceActivationUnavailable},
		{name: "mixed reentry and unavailable", states: []WorkspaceProviderProjectionState{WorkspaceProviderProjectionStale, WorkspaceProviderProjectionUnavailable}, want: WorkspaceActivationUnresolved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := activationItem(t, test.states...)
			if item.State != test.want || (item.NextAction != nil) != test.action {
				t.Fatalf("item = %+v, want state %q/action %t", item, test.want, test.action)
			}
			if test.action {
				want := &WorkspaceActivationAction{WorkingDirectory: "/workspace/project", Argv: []string{"tobari", "--manifest", "default"}}
				if !reflect.DeepEqual(item.NextAction, want) {
					t.Fatalf("action = %+v, want %+v", item.NextAction, want)
				}
			}
		})
	}
}

func TestWorkspaceActivationAggregationTruthTable(t *testing.T) {
	t.Parallel()
	ready := activationItem(t, WorkspaceProviderProjectionCurrent)
	reentry := activationItem(t, WorkspaceProviderProjectionMissing)
	reentry.ProjectID = "01912345-6789-7abc-8def-0123456789ac"
	reentry.Root = "/workspace/other"
	reentry.NextAction.WorkingDirectory = reentry.Root
	unavailable := activationItem(t, WorkspaceProviderProjectionUnavailable)
	unavailable.ProjectID = "01912345-6789-7abc-8def-0123456789ae"
	unavailable.Root = "/workspace/unavailable"
	tests := []struct {
		name  string
		items []WorkspaceActivationItem
		want  WorkspaceActivationState
	}{
		{name: "zero workspace", want: WorkspaceActivationNotApplicable},
		{name: "ready", items: []WorkspaceActivationItem{ready}, want: WorkspaceActivationReady},
		{name: "reentry", items: []WorkspaceActivationItem{ready, reentry}, want: WorkspaceActivationReentryRequired},
		{name: "unavailable", items: []WorkspaceActivationItem{unavailable}, want: WorkspaceActivationUnavailable},
		{name: "mixed", items: []WorkspaceActivationItem{reentry, unavailable}, want: WorkspaceActivationUnresolved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activation := exhaustiveActivation(t, test.items...)
			if activation.State != test.want || activation.Coverage != WorkspaceActivationCoverageExhaustive {
				t.Fatalf("activation = %+v, want %q/exhaustive", activation, test.want)
			}
			if len(test.items) == 0 && activation.Guidance != "" {
				t.Fatalf("zero-workspace activation claimed guidance: %+v", activation)
			}
		})
	}
}

func TestWorkspaceActivationCoverageDistinguishesZeroFromEnumerationFailure(t *testing.T) {
	t.Parallel()
	zero := exhaustiveActivation(t)
	if zero.Coverage != WorkspaceActivationCoverageExhaustive || zero.State != WorkspaceActivationNotApplicable || len(zero.Workspaces) != 0 {
		t.Fatalf("zero eligible Workspaces = %+v", zero)
	}
	unavailable := UnavailableWorkspaceActivation("default", testContextID)
	if err := unavailable.Validate(); err != nil {
		t.Fatal(err)
	}
	if unavailable.Coverage != WorkspaceActivationCoverageUnavailable || unavailable.State != WorkspaceActivationUnavailable {
		t.Fatalf("enumeration failure = %+v", unavailable)
	}
}

func TestWorkspaceActivationRejectsInferredOrRetargetedAction(t *testing.T) {
	t.Parallel()
	item := activationItem(t, WorkspaceProviderProjectionStale)
	item.NextAction.WorkingDirectory = "/workspace/elsewhere"
	if err := item.Validate(); err == nil {
		t.Fatal("accepted action whose working directory did not match the row root")
	}
	item = activationItem(t, WorkspaceProviderProjectionStale)
	item.NextAction.Argv = []string{"tobari", "--manifest", "other"}
	if err := item.Validate(); err == nil {
		t.Fatal("accepted action whose Context did not match the row")
	}
}

func TestWorkspaceActivationConstructorsOwnOrderingAndBounds(t *testing.T) {
	t.Parallel()
	item := activationItem(t, WorkspaceProviderProjectionCurrent, WorkspaceProviderProjectionMissing)
	if item.Providers[0].Provider != "aws" || item.Providers[1].Provider != "github" {
		t.Fatalf("providers are not deterministic: %+v", item.Providers)
	}
	tooManyProviders := make([]WorkspaceProviderActivation, MaxWorkspaceActivationProviders+1)
	for index := range tooManyProviders {
		tooManyProviders[index] = WorkspaceProviderActivation{Provider: fmt.Sprintf("provider-%03d", index), State: WorkspaceProviderProjectionCurrent}
	}
	if _, err := NewWorkspaceActivationItem(testProjectID, "/workspace/project", "default", testContextID, tooManyProviders, false); err == nil {
		t.Fatal("accepted oversized provider projection collection")
	}
	item = activationItem(t, WorkspaceProviderProjectionCurrent)
	item.Root = "/" + strings.Repeat("a", MaxWorkspaceActivationRootBytes)
	if err := item.Validate(); err == nil {
		t.Fatal("accepted oversized Workspace root")
	}
	tooManyWorkspaces := make([]WorkspaceActivationItem, MaxWorkspaceActivationItems+1)
	for index := range tooManyWorkspaces {
		tooManyWorkspaces[index] = activationItem(t, WorkspaceProviderProjectionCurrent)
		tooManyWorkspaces[index].ProjectID = fmt.Sprintf("01912345-6789-7abc-8def-%012x", index)
	}
	if _, err := NewWorkspaceActivation("default", testContextID, tooManyWorkspaces); err == nil {
		t.Fatal("accepted oversized Workspace collection")
	}
}

func TestResultValidatesChangedAndNoChangeMutations(t *testing.T) {
	for _, task := range []string{TaskLogin, TaskImport} {
		result := configuredResult(t, task)
		if err := result.Validate(); err != nil {
			t.Errorf("Result.Validate(%q): %v", task, err)
		}
	}
	logout := Result{
		Task: TaskLogout, ManifestState: tobari.ManifestObservationPersisted, Provider: "github", Context: "default", WorkspaceManifestID: testContextID,
		StorageBackend: StorageBackendMacOSKeychain, BrokerState: BrokerStateReady,
		Change: MutationChangeChanged, WorkspaceActivation: exhaustiveActivation(t),
	}
	if err := logout.Validate(); err != nil {
		t.Fatalf("Result.Validate(changed logout): %v", err)
	}
	logout.Change = MutationChangeNoChange
	logout.WorkspaceActivation = NotApplicableWorkspaceActivation()
	if err := logout.Validate(); err != nil {
		t.Fatalf("Result.Validate(no-op logout): %v", err)
	}
	logout.Task = TaskImport
	if err := logout.Validate(); err == nil {
		t.Fatal("Result.Validate accepted no-change import")
	}
}

func TestResultRejectsIdentityStateAndSecretFreeTextDrift(t *testing.T) {
	cases := map[string]func(*Result){
		"task":       func(r *Result) { r.Task = "auth.rotate" },
		"provider":   func(r *Result) { r.Provider = "GitHub" },
		"context":    func(r *Result) { r.Context = "Default" },
		"context ID": func(r *Result) { r.WorkspaceManifestID = "context-id" },
		"revision":   func(r *Result) { r.CredentialRevision = "revision with spaces" },
		"backend":    func(r *Result) { r.StorageBackend = "environment" },
		"broker":     func(r *Result) { r.BrokerState = BrokerStateLocked },
		"account control": func(r *Result) {
			value := "octocat\nsecret"
			r.AccountLabel = &value
		},
		"activation scope": func(r *Result) { r.WorkspaceActivation.WorkspaceManifestID = "01912345-6789-7abc-8def-0123456789ae" },
		"change":           func(r *Result) { r.Change = "maybe" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := configuredResult(t, TaskImport)
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Result.Validate accepted invalid result")
			}
		})
	}
}

func TestStatusResultRequiresContextScopedExhaustiveWorkspaceCollection(t *testing.T) {
	t.Parallel()
	result := StatusResult{
		Task: TaskStatus, ManifestState: tobari.ManifestObservationPersisted, Context: "default", WorkspaceManifestID: testContextID,
		StorageBackend: StorageBackendXDGFile, BrokerState: BrokerStateLocked,
		Providers:           []ProviderStatus{{Provider: "github", State: ProviderCredentialUnavailable}},
		WorkspaceActivation: exhaustiveActivation(t),
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("StatusResult.Validate(): %v", err)
	}
	result.Providers = nil
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted absent providers")
	}
	result.Providers = []ProviderStatus{{Provider: "github", State: ProviderCredentialUnavailable}}
	result.WorkspaceActivation = UnavailableWorkspaceActivation("default", testContextID)
	if err := result.Validate(); err != nil {
		t.Fatalf("StatusResult.Validate(enumeration unavailable): %v", err)
	}
}

func TestStatusResultAllowsSyntheticDefaultWithoutContextAuthority(t *testing.T) {
	t.Parallel()
	result := StatusResult{
		Task: TaskStatus, ManifestState: tobari.ManifestObservationAbsent,
		Context: tobari.DefaultManifestName, StorageBackend: StorageBackendXDGFile,
		BrokerState: BrokerStateUnavailable, Providers: []ProviderStatus{},
		WorkspaceActivation: NotApplicableWorkspaceActivation(),
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("synthetic status = %v", err)
	}
	result.WorkspaceManifestID = testContextID
	if err := result.Validate(); err == nil {
		t.Fatal("synthetic status accepted Context authority")
	}
}

func TestStableAuthVocabulary(t *testing.T) {
	if MaxPrimarySecretBytes != 32*1024 {
		t.Fatalf("MaxPrimarySecretBytes = %d", MaxPrimarySecretBytes)
	}
	if CredentialCatalogTargetKind == "" || CredentialCatalogTargetID == "" {
		t.Fatal("auth credential fixed-target vocabulary is empty")
	}
}

func validStatusObservation() StatusObservation {
	digest := "sha256:" + strings.Repeat("a", 64)
	return StatusObservation{
		ManifestState: tobari.ManifestObservationPersisted, Context: "default", WorkspaceManifestID: testContextID,
		StorageBackend: StorageBackendXDGFile, BrokerState: BrokerStateReady,
		Providers: []ProviderStatus{{
			Provider: "github", State: ProviderCredentialConfigured, CredentialRevision: "revision:1",
		}},
		Workspaces: WorkspaceObservation{
			Coverage: WorkspaceActivationCoverageExhaustive,
			Workspaces: []WorkspaceProjectionObservation{{
				ProjectID: testProjectID, Root: "/workspace/project", ProjectContextID: testContextID,
				RegistryAvailable: true, RegistryProjectID: testProjectID,
				Providers: []WorkspaceProviderObservation{{
					Provider: "github", RegistryPresent: true, RegistryRevision: "revision:1",
					RegistryBindingDigest: digest, ExpectedBindingDigest: digest,
					BindingState: BrokerBindingReady, BindingProvider: "github", BindingRevision: "revision:1",
				}},
			}},
		},
	}
}

func TestStatusObservationDerivesReadyFromCorrelatedAuthorityFacts(t *testing.T) {
	t.Parallel()
	result, err := NewStatusResult("default", validStatusObservation())
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkspaceActivation.State != WorkspaceActivationReady ||
		len(result.WorkspaceActivation.Workspaces) != 1 ||
		result.WorkspaceActivation.Workspaces[0].Providers[0].State != WorkspaceProviderProjectionCurrent ||
		result.WorkspaceActivation.Workspaces[0].NextAction != nil {
		t.Fatalf("derived result = %+v", result)
	}
}

func TestObservationRejectsMismatchedOrInventedAuthority(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*StatusObservation){
		"requested Context": func(o *StatusObservation) { o.Context = "other" },
		"registry project": func(o *StatusObservation) {
			o.Workspaces.Workspaces[0].RegistryProjectID = "01912345-6789-7abc-8def-0123456789ae"
		},
		"registry digest": func(o *StatusObservation) {
			o.Workspaces.Workspaces[0].Providers[0].RegistryBindingDigest = "sha256:short"
		},
		"expected digest": func(o *StatusObservation) {
			o.Workspaces.Workspaces[0].Providers[0].ExpectedBindingDigest = "sha256:short"
		},
		"binding provider": func(o *StatusObservation) { o.Workspaces.Workspaces[0].Providers[0].BindingProvider = "aws" },
		"binding revision": func(o *StatusObservation) { o.Workspaces.Workspaces[0].Providers[0].BindingRevision = "revision:2" },
		"unobserved identity": func(o *StatusObservation) {
			o.Workspaces.Workspaces[0].Providers[0].BindingState = BrokerBindingNotObserved
		},
		"unavailable registry identity": func(o *StatusObservation) {
			o.Workspaces.Workspaces[0].RegistryAvailable = false
			o.Workspaces.Workspaces[0].Providers = nil
		},
		"duplicate project outside scope": func(o *StatusObservation) {
			duplicate := o.Workspaces.Workspaces[0]
			duplicate.ProjectContextID = "01912345-6789-7abc-8def-0123456789ae"
			o.Workspaces.Workspaces = append(o.Workspaces.Workspaces, duplicate)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observed := validStatusObservation()
			mutate(&observed)
			if _, err := NewStatusResult("default", observed); err == nil {
				t.Fatal("accepted invalid authority observation")
			}
		})
	}
}

func TestObservationRejectsDiscardedNonPersistedAndNoChangeFacts(t *testing.T) {
	t.Parallel()
	status := validStatusObservation()
	status.ManifestState = tobari.ManifestObservationAbsent
	status.WorkspaceManifestID = ""
	status.BrokerState = BrokerStateUnavailable
	status.Providers = []ProviderStatus{}
	if _, err := NewStatusResult("default", status); err == nil {
		t.Fatal("accepted non-persisted status carrying exhaustive Workspace facts")
	}
	mutation := MutationObservation{
		ManifestState: tobari.ManifestObservationPersisted, Provider: "github", Context: "default", WorkspaceManifestID: testContextID,
		StorageBackend: StorageBackendXDGFile, BrokerState: BrokerStateReady, Changed: false,
		Providers: validStatusObservation().Providers, Workspaces: validStatusObservation().Workspaces,
	}
	if _, err := NewResult(TaskLogout, "default", "github", mutation); err == nil {
		t.Fatal("accepted no-change mutation carrying Workspace change facts")
	}
	mutation.Workspaces = WorkspaceObservation{Coverage: WorkspaceActivationCoverageNotApplicable}
	if _, err := NewResult(TaskLogout, "default", "github", mutation); err == nil {
		t.Fatal("accepted no-change mutation carrying provider activation facts")
	}
}
