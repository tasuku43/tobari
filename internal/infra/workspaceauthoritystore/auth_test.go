package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthBrokerFixture struct {
	mu             sync.Mutex
	statuses       map[string]authbroker.ProviderStatus
	loginCalls     int
	importCalls    int
	logoutCalls    int
	complete       bool
	state          authbroker.BrokerState
	observeErr     error
	onObserve      func()
	onInventory    func()
	afterInventory func()
	onLogin        func()
	providers      map[string]authbroker.Provider
}

func newFinalAuthBrokerFixture() *finalAuthBrokerFixture {
	return &finalAuthBrokerFixture{statuses: map[string]authbroker.ProviderStatus{}, providers: map[string]authbroker.Provider{}, complete: true, state: authbroker.BrokerStateReady}
}

func finalAuthReviewedProvider(id string) authbroker.Provider {
	if id == authbroker.BuiltinAWSProviderID {
		return authbroker.Provider{
			SchemaVersion: authbroker.ProviderSchemaVersion, ID: id, DisplayName: "AWS CLI session",
			Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "aws-sso"},
			Credential:  authbroker.Credential{Kind: authbroker.CredentialAWSSSOSession},
			WorkspaceProjections: []authbroker.WorkspaceProjection{
				{Kind: authbroker.WorkspaceProjectionEnvironment, Name: "AWS_ACCESS_KEY_ID", Template: "${HANDLE}"},
				{Kind: authbroker.WorkspaceProjectionEnvironment, Name: "AWS_SECRET_ACCESS_KEY", Template: "${HANDLE}"},
				{Kind: authbroker.WorkspaceProjectionEnvironment, Name: "AWS_SESSION_TOKEN", Template: "${HANDLE}"},
				{Kind: authbroker.WorkspaceProjectionEnvironment, Name: "AWS_EC2_METADATA_DISABLED", Template: "true"},
			},
			HeaderBindings: []authbroker.HeaderBinding{},
			SigningBindings: []authbroker.SigningBinding{{Kind: authbroker.SigningBindingAWSSigV4, AWSSigV4: &authbroker.AWSSigV4Binding{
				Target:        authbroker.AWSSigV4Target{Scheme: "https", Port: 443, DNSSuffixes: []string{"amazonaws.com"}},
				Source:        authbroker.AWSSigV4Source{AuthorizationHeader: "authorization", SecurityTokenHeader: "x-amz-security-token"},
				SecretHeaders: []string{"authorization", "x-amz-security-token"},
			}}},
		}
	}
	return authbroker.Provider{
		SchemaVersion: authbroker.ProviderSchemaVersion, ID: id, DisplayName: "Synthetic reviewed provider",
		Acquisition:          authbroker.Acquisition{Mode: authbroker.AcquisitionStdinImport},
		Credential:           authbroker.Credential{Kind: authbroker.CredentialPrimarySecret},
		WorkspaceProjections: []authbroker.WorkspaceProjection{{Kind: authbroker.WorkspaceProjectionEnvironment, Name: "SYNTHETIC_TOKEN", Template: "${HANDLE}"}},
		HeaderBindings: []authbroker.HeaderBinding{{
			Target:        authbroker.BindingTarget{Scheme: "https", Host: "api.example.com", Port: 443},
			Source:        authbroker.BindingSource{Header: "authorization", Formats: []authbroker.SourceFormat{authbroker.SourceFormatBearer}},
			Destination:   authbroker.BindingDestination{Header: "authorization", Format: authbroker.DestinationFormatBearer, SecretField: authbroker.CredentialPrimarySecret},
			SecretHeaders: []string{"authorization"},
		}},
	}
}

func (b *finalAuthBrokerFixture) WithFinalContextAuthObservation(ctx context.Context, action func(context.Context) error) error {
	return action(ctx)
}

func finalAuthKey(contextID tobari.ContextID, provider string) string {
	return string(contextID) + "\x00" + provider
}

func (b *finalAuthBrokerFixture) ResolveFinalContextProvider(_ context.Context, authority authbroker.ContextAuthenticationAuthority, provider string) (authbroker.ContextProviderAuthority, error) {
	if err := authority.ValidateFor(authority.ContextRef); err != nil || authbroker.ValidateProviderID(provider) != nil {
		return authbroker.ContextProviderAuthority{}, fmt.Errorf("invalid final provider authority: %w", err)
	}
	reviewed, present := b.providers[provider]
	if !present {
		reviewed = finalAuthReviewedProvider(provider)
	}
	target := authbroker.ContextProviderAuthority{Context: authority, Provider: reviewed}
	if err := target.ValidateFor(authority.ContextRef, provider); err != nil {
		return authbroker.ContextProviderAuthority{}, err
	}
	return target.Clone(), nil
}

func (b *finalAuthBrokerFixture) ObserveFinalContextProvider(_ context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.onObserve != nil {
		b.onObserve()
	}
	if b.observeErr != nil {
		return authbroker.ProviderStatus{}, "", "", b.observeErr
	}
	status, present := b.statuses[finalAuthKey(target.Context.ContextID, target.Provider.ID)]
	if !present {
		status = authbroker.ProviderStatus{Provider: target.Provider.ID, State: authbroker.ProviderCredentialNotConfigured}
	}
	return status, authbroker.StorageBackendXDGFile, b.state, nil
}

func (b *finalAuthBrokerFixture) ObserveFinalContextInventory(_ context.Context, authority authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.onInventory != nil {
		b.onInventory()
	}
	if b.observeErr != nil {
		return authbroker.ContextStatusObservation{}, b.observeErr
	}
	providers := make([]authbroker.ProviderStatus, 0)
	prefix := string(authority.ContextID) + "\x00"
	for key, status := range b.statuses {
		if strings.HasPrefix(key, prefix) {
			providers = append(providers, status)
		}
	}
	result, err := authbroker.NewContextStatusObservation(authority, authbroker.StorageBackendXDGFile, b.state, providers, b.complete)
	if b.afterInventory != nil {
		b.afterInventory()
	}
	return result, err
}

func (b *finalAuthBrokerFixture) rotate(contextID tobari.ContextID, provider string) authbroker.ProviderStatus {
	key := finalAuthKey(contextID, provider)
	previous := b.statuses[key]
	revision := 1
	if previous.CredentialRevision != "" {
		_, _ = fmt.Sscanf(previous.CredentialRevision, "revision-%d", &revision)
		revision++
	}
	status := authbroker.ProviderStatus{Provider: provider, State: authbroker.ProviderCredentialConfigured, CredentialRevision: fmt.Sprintf("revision-%d", revision)}
	b.statuses[key] = status
	return status
}

func (b *finalAuthBrokerFixture) LoginFinalContextProvider(_ context.Context, target authbroker.ContextProviderAuthority, _ string, _ io.Reader, _ io.Writer) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loginCalls++
	status := b.rotate(target.Context.ContextID, target.Provider.ID)
	if b.onLogin != nil {
		b.onLogin()
	}
	return status, authbroker.StorageBackendXDGFile, b.state, nil
}

func (b *finalAuthBrokerFixture) ImportFinalContextProvider(_ context.Context, target authbroker.ContextProviderAuthority, _ io.Reader) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.importCalls++
	return b.rotate(target.Context.ContextID, target.Provider.ID), authbroker.StorageBackendXDGFile, b.state, nil
}

func (b *finalAuthBrokerFixture) LogoutFinalContextProvider(_ context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logoutCalls++
	key := finalAuthKey(target.Context.ContextID, target.Provider.ID)
	_, changed := b.statuses[key]
	delete(b.statuses, key)
	return authbroker.ProviderStatus{Provider: target.Provider.ID, State: authbroker.ProviderCredentialNotConfigured}, changed, authbroker.StorageBackendXDGFile, b.state, nil
}

func finalAuthAdapterFixture(t *testing.T) (*Store, *Mutator, *FinalContextAuthAdapter, *finalAuthBrokerFixture, authbroker.ContextAuthenticationAuthority) {
	t.Helper()
	collection := storeCollectionFixture(t)
	return finalAuthAdapterFixtureFor(t, collection)
}

func finalAuthAdapterFixtureFor(t *testing.T, collection tobari.WorkspaceAuthorityCollection) (*Store, *Mutator, *FinalContextAuthAdapter, *finalAuthBrokerFixture, authbroker.ContextAuthenticationAuthority) {
	t.Helper()
	store, mutator, _, _, _ := newMutationFixture(t, &collection)
	broker := newFinalAuthBrokerFixture()
	adapter, err := NewFinalContextAuthAdapter(mutator, broker, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := tobari.ContextRef(storeContextID)
	snapshot, err := store.ReadContextAuthorityByReference(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, ref)
	if err != nil {
		t.Fatal(err)
	}
	return store, mutator, adapter, broker, authority
}

func TestFinalContextAuthNoEnvelopeDecisionRecoversEveryEffectBoundaryAndRotatesAgain(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	realRename := mutator.rename
	decisionWrites := 0
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
			decisionWrites++
			if decisionWrites == 1 {
				if err := realRename(source, target); err != nil {
					return err
				}
				return errors.New("stop after durable decision before effect")
			}
		}
		return realRename(source, target)
	}
	if _, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("pre-effect interruption passed")
	}
	if broker.loginCalls != 0 {
		t.Fatalf("pre-effect interruption calls=%d", broker.loginCalls)
	}
	mutator.rename = realRename
	first, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || first.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("first recovery=%#v calls=%d err=%v", first, broker.loginCalls, err)
	}
	second, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || second.Provider.CredentialRevision != "revision-2" || broker.loginCalls != 2 {
		t.Fatalf("later intentional rotation=%#v calls=%d err=%v", second, broker.loginCalls, err)
	}

	// An import uses the same fixed Context/provider but a distinct task and is
	// never mislabeled as the retained login terminal outcome.
	imported, err := adapter.ImportFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, strings.NewReader("synthetic"))
	if err != nil || imported.Provider.CredentialRevision != "revision-3" || broker.importCalls != 1 {
		t.Fatalf("import rotation=%#v calls=%d err=%v", imported, broker.importCalls, err)
	}
}

func TestFinalContextAuthConfirmedNoEnvelopeEffectSettlesAfterCallerCancellation(t *testing.T) {
	_, _, adapter, broker, authority := finalAuthAdapterFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	broker.onLogin = cancel
	result, err := adapter.LoginFinalContextAuth(ctx, authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || result.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("confirmed auth cancellation result=%#v calls=%d err=%v", result, broker.loginCalls, err)
	}
}

func TestFinalContextAuthIsolatedByFreshContextIdentityAndNeverAdoptsPredecessorBytes(t *testing.T) {
	_, mutator, adapter, broker, authorityA := finalAuthAdapterFixture(t)
	templateRef, err := tobari.WorkspaceTemplateRef(storeTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	snapshotB, err := mutator.CreateContextByTemplateReference(context.Background(), templateRef, "/workspace/other-project")
	if err != nil {
		t.Fatal(err)
	}
	refB, err := tobari.ContextRef(snapshotB.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorityB, err := authbroker.NewContextAuthenticationAuthority(snapshotB, refB)
	if err != nil {
		t.Fatal(err)
	}
	// A replay-capable predecessor record can carry the same credential bytes,
	// but its old identity is never a final Context lookup or migration key.
	broker.statuses["predecessor-context\x00"+authbroker.BuiltinGitHubProviderID] = authbroker.ProviderStatus{
		Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-legacy",
	}
	statusB, err := adapter.StatusFinalContextAuth(context.Background(), authorityB)
	if err != nil || len(statusB.Providers) != 0 || !statusB.InventoryComplete {
		t.Fatalf("fresh Context adopted predecessor credential: %#v, %v", statusB, err)
	}
	loginA, err := adapter.LoginFinalContextAuth(context.Background(), authorityA, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || loginA.Provider.CredentialRevision != "revision-1" {
		t.Fatalf("Context A login = %#v, %v", loginA, err)
	}
	statusB, err = adapter.StatusFinalContextAuth(context.Background(), authorityB)
	if err != nil || len(statusB.Providers) != 0 || !statusB.InventoryComplete {
		t.Fatalf("Context A credential crossed into B: %#v, %v", statusB, err)
	}
	loginB, err := adapter.LoginFinalContextAuth(context.Background(), authorityB, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || loginB.Provider.CredentialRevision != "revision-1" || loginB.Authority.ContextID == loginA.Authority.ContextID {
		t.Fatalf("Context B login = %#v, %v", loginB, err)
	}
	if _, err := adapter.LogoutFinalContextAuth(context.Background(), authorityA, authbroker.BuiltinGitHubProviderID); err != nil {
		t.Fatal(err)
	}
	statusB, err = adapter.StatusFinalContextAuth(context.Background(), authorityB)
	if err != nil || len(statusB.Providers) != 1 || statusB.Providers[0].State != authbroker.ProviderCredentialConfigured {
		t.Fatalf("Context A logout changed B: %#v, %v", statusB, err)
	}
}

func TestFinalContextAuthStatusIsZeroMutationAndRejectsBetweenPassDrift(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	lifecycle := mutator.lifecycle.(*mutationLifecycle)
	beforeAttempts := lifecycle.attempts.Load()
	changed := false
	broker.afterInventory = func() {
		if changed {
			return
		}
		changed = true
		broker.statuses[finalAuthKey(authority.ContextID, authbroker.BuiltinGitHubProviderID)] = authbroker.ProviderStatus{
			Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-drift",
		}
	}
	if _, err := adapter.StatusFinalContextAuth(context.Background(), authority); err == nil {
		t.Fatal("between-pass Broker inventory drift passed")
	}
	if lifecycle.attempts.Load() != beforeAttempts || broker.loginCalls != 0 || broker.importCalls != 0 || broker.logoutCalls != 0 {
		t.Fatalf("read-only status crossed mutation authority: attempts=%d/%d login=%d import=%d logout=%d", lifecycle.attempts.Load(), beforeAttempts, broker.loginCalls, broker.importCalls, broker.logoutCalls)
	}
	broker.afterInventory = nil
	status, err := adapter.StatusFinalContextAuth(context.Background(), authority)
	if err != nil || len(status.Providers) != 1 || status.Providers[0].CredentialRevision != "revision-drift" {
		t.Fatalf("stable status = %#v, %v", status, err)
	}
	if lifecycle.attempts.Load() != beforeAttempts {
		t.Fatalf("stable status created/acquired mutation lock: attempts=%d/%d", lifecycle.attempts.Load(), beforeAttempts)
	}
}

func TestFinalContextAuthRecoversEffectBeforeResultAndTerminalRenameUncertainty(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	realRename := mutator.rename
	writes := 0
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
			writes++
			if writes == 2 {
				return errors.New("stop after Broker effect before result receipt")
			}
		}
		return realRename(source, target)
	}
	if _, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("post-effect interruption passed")
	}
	if broker.loginCalls != 1 {
		t.Fatalf("post-effect calls=%d", broker.loginCalls)
	}
	mutator.rename = realRename
	result, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || result.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("consequence recovery=%#v calls=%d err=%v", result, broker.loginCalls, err)
	}

	// A real active->terminal rename followed by an error is classified as a
	// confirmed result-delivery interruption. Status remains zero mutation and
	// the next explicit login is a fresh rotation.
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
			if err := realRename(source, target); err != nil {
				return err
			}
			return errors.New("synthetic post-terminal rename uncertainty")
		}
		return realRename(source, target)
	}
	_, err = adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "research_auth_result_delivery_interrupted" || public.ChangeState != fault.ChangeConfirmed || broker.loginCalls != 2 {
		t.Fatalf("terminal uncertainty err=%v public=%#v calls=%d", err, public, broker.loginCalls)
	}
	beforeStatus := broker.loginCalls
	status, err := adapter.StatusFinalContextAuth(context.Background(), authority)
	if err != nil || status.Providers[0].CredentialRevision != "revision-2" || broker.loginCalls != beforeStatus {
		t.Fatalf("zero-mutation status=%#v calls=%d err=%v", status, broker.loginCalls, err)
	}
	mutator.rename = realRename
	rotated, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || rotated.Provider.CredentialRevision != "revision-3" || broker.loginCalls != 3 {
		t.Fatalf("post-reconciliation rotation=%#v calls=%d err=%v", rotated, broker.loginCalls, err)
	}
}

func TestFinalContextAuthRecoversResultReceiptBeforeTerminalRename(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
			return errors.New("stop before active decision becomes terminal")
		}
		return realRename(source, target)
	}
	_, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.ChangeState != fault.ChangeConfirmed || broker.loginCalls != 1 {
		t.Fatalf("post-result interruption err=%v public=%#v calls=%d", err, public, broker.loginCalls)
	}
	mutator.rename = realRename
	recovered, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || recovered.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("active result recovery=%#v calls=%d err=%v", recovered, broker.loginCalls, err)
	}
	rotated, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || rotated.Provider.CredentialRevision != "revision-2" || broker.loginCalls != 2 {
		t.Fatalf("later rotation=%#v calls=%d err=%v", rotated, broker.loginCalls, err)
	}
}

func TestFinalContextAuthActiveDecisionExcludesDifferentRequestDimensions(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
			if err := realRename(source, target); err != nil {
				return err
			}
			return errors.New("stop after exact login decision")
		}
		return realRename(source, target)
	}
	if _, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinAWSProviderID, "identity-center", strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("decision interruption passed")
	}
	mutator.rename = realRename
	for _, call := range []struct {
		name string
		call func() error
	}{
		{name: "provider", call: func() error {
			_, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
			return err
		}},
		{name: "method", call: func() error {
			_, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinAWSProviderID, "console", strings.NewReader(""), io.Discard)
			return err
		}},
		{name: "task", call: func() error {
			_, err := adapter.ImportFinalContextAuth(context.Background(), authority, authbroker.BuiltinAWSProviderID, strings.NewReader("synthetic"))
			return err
		}},
	} {
		t.Run(call.name, func(t *testing.T) {
			if err := call.call(); err == nil || broker.loginCalls != 0 || broker.importCalls != 0 {
				t.Fatalf("different %s request crossed active authority: err=%v login=%d import=%d", call.name, err, broker.loginCalls, broker.importCalls)
			}
		})
	}
	result, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinAWSProviderID, "identity-center", strings.NewReader(""), io.Discard)
	if err != nil || result.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("same request recovery=%#v calls=%d err=%v", result, broker.loginCalls, err)
	}
}

func TestFinalContextAuthRecoveryRejectsSameIDReviewedProviderDrift(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	original := finalAuthReviewedProvider(authbroker.BuiltinGitHubProviderID)
	broker.providers[authbroker.BuiltinGitHubProviderID] = original
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
			if err := realRename(source, target); err != nil {
				return err
			}
			return errors.New("stop after reviewed provider decision")
		}
		return realRename(source, target)
	}
	if _, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard); err == nil || broker.loginCalls != 0 {
		t.Fatalf("decision interruption err=%v calls=%d", err, broker.loginCalls)
	}
	mutator.rename = realRename
	drifted := original
	drifted.DisplayName = "Changed reviewed provider"
	broker.providers[authbroker.BuiltinGitHubProviderID] = drifted
	if _, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard); err == nil || broker.loginCalls != 0 {
		t.Fatalf("same-ID provider drift crossed durable authority: err=%v calls=%d", err, broker.loginCalls)
	}
	broker.providers[authbroker.BuiltinGitHubProviderID] = original
	result, err := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
	if err != nil || result.Provider.CredentialRevision != "revision-1" || broker.loginCalls != 1 {
		t.Fatalf("unchanged provider recovery=%#v calls=%d err=%v", result, broker.loginCalls, err)
	}
}

func TestFinalContextLogoutConvergesWithoutRepeatingEffect(t *testing.T) {
	_, mutator, adapter, broker, authority := finalAuthAdapterFixture(t)
	key := finalAuthKey(authority.ContextID, authbroker.BuiltinGitHubProviderID)
	broker.statuses[key] = authbroker.ProviderStatus{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-1"}
	realRename := mutator.rename
	writes := 0
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
			writes++
			if writes == 2 {
				return errors.New("stop after logout before result receipt")
			}
		}
		return realRename(source, target)
	}
	if _, err := adapter.LogoutFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID); err == nil || broker.logoutCalls != 1 {
		t.Fatalf("logout interruption err=%v calls=%d", err, broker.logoutCalls)
	}
	mutator.rename = realRename
	result, err := adapter.LogoutFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID)
	if err != nil || result.Provider.State != authbroker.ProviderCredentialNotConfigured || broker.logoutCalls != 1 {
		t.Fatalf("logout recovery=%#v calls=%d err=%v", result, broker.logoutCalls, err)
	}
	replayed, err := adapter.LogoutFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID)
	if err != nil || replayed.Provider.State != authbroker.ProviderCredentialNotConfigured || broker.logoutCalls != 1 {
		t.Fatalf("logout terminal replay=%#v calls=%d err=%v", replayed, broker.logoutCalls, err)
	}
}

func TestFinalContextCredentialInventoryOwnsContextDeletionPrerequisite(t *testing.T) {
	base := storeCollectionFixture(t)
	withoutWorkspace, changed, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("prepare Context-only authority: %v", err)
	}
	_, mutator, adapter, broker, authority := finalAuthAdapterFixtureFor(t, withoutWorkspace)
	broker.statuses[finalAuthKey(authority.ContextID, authbroker.BuiltinGitHubProviderID)] = authbroker.ProviderStatus{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-1"}
	broker.statuses[finalAuthKey(authority.ContextID, "removed-owner")] = authbroker.ProviderStatus{Provider: "removed-owner", State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-1"}
	if _, err := mutator.DeleteContextByReference(context.Background(), authority.ContextRef); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("configured credentials did not block Context delete: %v", err)
	}
	if _, err := adapter.LogoutFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.DeleteContextByReference(context.Background(), authority.ContextRef); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("one-of-two logout allowed Context delete: %v", err)
	}
	if _, err := adapter.LogoutFinalContextAuth(context.Background(), authority, "removed-owner"); err != nil {
		t.Fatal(err)
	}
	broker.complete = false
	broker.state = authbroker.BrokerStateLocked
	if _, err := mutator.DeleteContextByReference(context.Background(), authority.ContextRef); err == nil || errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("incomplete inventory was not fail closed: %v", err)
	}
	broker.complete = true
	broker.state = authbroker.BrokerStateReady
	result, err := mutator.DeleteContextByReference(context.Background(), authority.ContextRef)
	if err != nil || !result.Deleted {
		t.Fatalf("exhaustive empty inventory did not permit delete: %#v %v", result, err)
	}
}

func TestFinalContextCredentialInventoryAndDeleteShareLifecycleAuthority(t *testing.T) {
	base := storeCollectionFixture(t)
	withoutWorkspace, changed, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("prepare Context-only authority: %v", err)
	}
	_, mutator, adapter, broker, authority := finalAuthAdapterFixtureFor(t, withoutWorkspace)
	lifecycle := mutator.lifecycle.(*mutationLifecycle)
	inventoryEntered := make(chan struct{})
	releaseInventory := make(chan struct{})
	var inventoryOnce sync.Once
	broker.onInventory = func() {
		inventoryOnce.Do(func() { close(inventoryEntered) })
		<-releaseInventory
	}
	deleteDone := make(chan error, 1)
	go func() {
		_, deleteErr := mutator.DeleteContextByReference(context.Background(), authority.ContextRef)
		deleteDone <- deleteErr
	}()
	<-inventoryEntered
	loginDone := make(chan error, 1)
	go func() {
		_, loginErr := adapter.LoginFinalContextAuth(context.Background(), authority, authbroker.BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard)
		loginDone <- loginErr
	}()
	deadline := time.Now().Add(time.Second)
	for lifecycle.attempts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if lifecycle.attempts.Load() < 2 || !lifecycle.held.Load() || broker.loginCalls != 0 {
		t.Fatalf("credential create did not wait behind delete: attempts=%d held=%t calls=%d", lifecycle.attempts.Load(), lifecycle.held.Load(), broker.loginCalls)
	}
	close(releaseInventory)
	if err := <-deleteDone; err != nil {
		t.Fatalf("Context delete failed: %v", err)
	}
	if err := <-loginDone; err == nil || broker.loginCalls != 0 {
		t.Fatalf("credential create interleaved after absence: err=%v calls=%d", err, broker.loginCalls)
	}
}

func TestResearchAuthDecisionCannotRelabelReviewedTerminalPublication(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, _, _, _, _ = newMutationFixture(t, &existing)
	set := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])
	receipt, err := reviewedSettlementFixtureReceipt(existing, set)
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedTerminalPublication{DecisionSet: set, PreviousGeneration: existing.Generation, PreviousRevision: existing.Revision, NextGeneration: existing.Generation + 1, NextRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("9", 64)), Settlement: receipt}
	decision := effectDecision{SchemaVersion: effectDecisionSchemaVersion, Operation: "policy-apply-reviewed", Target: tobari.PolicyDecisionSetID, PreviousGeneration: existing.Generation, PreviousRevision: existing.Revision, NextGeneration: existing.Generation + 1, NextRevision: publication.NextRevision, ReviewedSet: &set, ReviewedPublication: &publication}
	if decision.validate() == nil {
		t.Fatal("malformed reviewed terminal publication passed direct decision validation")
	}
	decision.ReviewedPublication = nil
	if err := decision.validate(); err != nil {
		t.Fatalf("active reviewed decision became invalid: %v", err)
	}
}
