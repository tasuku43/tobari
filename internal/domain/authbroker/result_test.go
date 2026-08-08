package authbroker

import "testing"

const testContextID = "01912345-6789-7abc-8def-0123456789ad"

func configuredResult(task string) Result {
	account := "octocat"
	return Result{
		Task: task, Provider: "github", Context: "default", ContextID: testContextID,
		Configured: true, AccountLabel: &account, StorageBackend: StorageBackendXDGFile,
		BrokerState: BrokerStateReady, CredentialRevision: "sha256:abcdef0123456789",
		WorkspaceActivation: WorkspaceActivation{State: WorkspaceActivationReentryRequired, Guidance: ContextAuthActivationGuidance},
	}
}

func TestResultValidatesConfiguredMutations(t *testing.T) {
	for _, task := range []string{TaskLogin, TaskImport} {
		result := configuredResult(task)
		if err := result.Validate(); err != nil {
			t.Errorf("Result.Validate(%q): %v", task, err)
		}
	}
	logout := Result{
		Task: TaskLogout, Provider: "github", Context: "default", ContextID: testContextID,
		StorageBackend: StorageBackendMacOSKeychain, BrokerState: BrokerStateReady,
		WorkspaceActivation: WorkspaceActivation{
			State: WorkspaceActivationReentryRequired, Guidance: ContextAuthRemovalGuidance,
		},
	}
	if err := logout.Validate(); err != nil {
		t.Fatalf("Result.Validate(logout): %v", err)
	}
}

func TestResultRejectsIdentityStateAndSecretFreeTextDrift(t *testing.T) {
	cases := map[string]func(*Result){
		"task":            func(r *Result) { r.Task = "auth.rotate" },
		"provider":        func(r *Result) { r.Provider = "GitHub" },
		"context":         func(r *Result) { r.Context = "Default" },
		"context ID":      func(r *Result) { r.ContextID = "context-id" },
		"revision":        func(r *Result) { r.CredentialRevision = "revision with spaces" },
		"backend":         func(r *Result) { r.StorageBackend = "environment" },
		"broker":          func(r *Result) { r.BrokerState = BrokerStateLocked },
		"account control": func(r *Result) { value := "octocat\nsecret"; r.AccountLabel = &value },
		"activation":      func(r *Result) { r.WorkspaceActivation.Guidance = "" },
		"activation state": func(r *Result) {
			r.WorkspaceActivation.State = WorkspaceActivationReady
			r.WorkspaceActivation.Guidance = ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := configuredResult(TaskImport)
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("Result.Validate accepted invalid result")
			}
		})
	}
	unconfigured := configuredResult(TaskImport)
	unconfigured.Configured = false
	if err := unconfigured.Validate(); err == nil {
		t.Fatal("Result.Validate accepted stale configured metadata")
	}
}

func TestStatusResultValidatesCompleteProviderCollection(t *testing.T) {
	account := "octocat"
	result := StatusResult{
		Task: TaskStatus, Context: "default", ContextID: testContextID,
		StorageBackend: StorageBackendXDGFile, BrokerState: BrokerStateLocked,
		Providers: []ProviderStatus{
			{Provider: "example-token", State: ProviderCredentialUnavailable},
			{Provider: "github", State: ProviderCredentialUnavailable},
		},
		WorkspaceActivation: WorkspaceActivation{State: WorkspaceActivationNotApplicable},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("StatusResult.Validate(): %v", err)
	}
	result.Providers = nil
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted absent providers")
	}
	result.BrokerState = BrokerStateReady
	result.Providers = []ProviderStatus{{Provider: "github", State: ProviderCredentialNotConfigured}, {Provider: "github", State: ProviderCredentialNotConfigured}}
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted duplicate providers")
	}
	result.Providers = []ProviderStatus{{Provider: "github", State: ProviderCredentialNotConfigured, CredentialRevision: "stale"}}
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted unconfigured revision")
	}
	result.Providers = []ProviderStatus{{
		Provider: "github", State: ProviderCredentialConfigured, Configured: true,
		AccountLabel: &account, CredentialRevision: "revision:2",
	}}
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted configured provider without activation guidance")
	}
	result.WorkspaceActivation = WorkspaceActivation{
		State: WorkspaceActivationReentryRequired, Guidance: ContextAuthActivationGuidance,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("StatusResult.Validate(configured): %v", err)
	}
	result.BrokerState = BrokerStateLocked
	if err := result.Validate(); err == nil {
		t.Fatal("StatusResult.Validate accepted configured provider while broker is locked")
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
