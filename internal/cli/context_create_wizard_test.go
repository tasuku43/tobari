package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextCreateBootstrapFixture struct {
	aws            tobari.ContextBootstrapSnapshot
	eks            *tobari.ContextBootstrapSnapshot
	discoveryCalls int
}

type driftingContextCreateBootstrapFixture struct {
	*contextCreateBootstrapFixture
	drifted bool
}

type rejectedContextCreateBootstrapFixture struct{ *contextCreateBootstrapFixture }

type emptyAWSContextCreateBootstrapFixture struct{ *contextCreateBootstrapFixture }

type emptyEKSContextCreateBootstrapFixture struct{ *contextCreateBootstrapFixture }

type contextCreateBaseFixture struct {
	base  tobari.ContextCreateBase
	calls int
}

func (f *contextCreateBaseFixture) CreationBase(_ context.Context, name string) (tobari.ContextCreateBase, error) {
	f.calls++
	if name != f.base.Name {
		return tobari.ContextCreateBase{}, tobari.ErrContextNotFound
	}
	return f.base.Clone(), nil
}

func (f *rejectedContextCreateBootstrapFixture) DiscoverAWSBootstraps(context.Context) (tobari.ContextAWSBootstrapDiscovery, error) {
	return tobari.ContextAWSBootstrapDiscovery{State: tobari.ContextBootstrapDiscoveryRejected, Reason: "Host AWS shared config has unsafe permissions.", Candidates: []tobari.ContextAWSBootstrapCandidate{}}, nil
}

func (f *emptyAWSContextCreateBootstrapFixture) DiscoverAWSBootstraps(context.Context) (tobari.ContextAWSBootstrapDiscovery, error) {
	return tobari.ContextAWSBootstrapDiscovery{State: tobari.ContextBootstrapDiscoveryAvailable, Candidates: []tobari.ContextAWSBootstrapCandidate{}}, nil
}

func (f *emptyEKSContextCreateBootstrapFixture) DiscoverEKSBootstraps(_ context.Context, aws tobari.ContextBootstrapSnapshot) (tobari.ContextEKSBootstrapDiscovery, error) {
	return tobari.ContextEKSBootstrapDiscovery{State: tobari.ContextBootstrapDiscoveryAvailable, AWSRevision: aws.Revision, Candidates: []tobari.ContextEKSBootstrapCandidate{}}, nil
}

func newContextCreateBootstrapFixture(t *testing.T, withEKS bool) *contextCreateBootstrapFixture {
	t.Helper()
	aws, err := tobari.NewContextBootstrapSnapshot(1, tobari.ContextAWSBootstrap{
		Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
		SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"},
		AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &contextCreateBootstrapFixture{aws: aws}
	if withEKS {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture"}, NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			t.Fatal(err)
		}
		ca := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		composed, err := tobari.NewContextBootstrapSnapshotWithEKS(1, aws.AWS, tobari.ContextEKSBootstrap{ContextName: "platform", ClusterName: "platform", Region: "ap-northeast-1", Server: "https://abc.gr7.ap-northeast-1.eks.amazonaws.com", CertificateAuthorityData: ca, Namespace: "development"})
		if err != nil {
			t.Fatal(err)
		}
		fixture.eks = &composed
	}
	return fixture
}

func contextCreateResetBaseFixture() tobari.ContextCreateBase {
	return tobari.ContextCreateBase{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ContextPolicyModeAdvanced,
		SourceAccess: tobari.ContextSourceAccessReadOnly, NativeReadiness: tobari.ContextNativeReadinessDisabled,
		MethodPolicy:     tobari.ContextMethodPolicy{Default: tobari.ContextMethodDeny, Overrides: []tobari.ContextMethodOverride{{Method: "GET", Decision: tobari.ContextMethodAllow}}},
		RuntimeSelection: "standard@1", ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(), GitIdentity: tobari.DefaultContextGitIdentityReport(),
	}
}

func customizedContextCreateDraft() contextCreateRawDraft {
	return contextCreateRawDraft{
		name: "standalone", policyMode: tobari.ContextPolicyModeGuided, sourceIndex: 0,
		methodDefault: tobari.ContextMethodAllow, methodOverrides: map[string]tobari.ContextMethodDecision{"POST": tobari.ContextMethodDeny},
		runtimeSelection: "standard", nativeReadiness: tobari.ContextNativeReadinessEnabled,
	}
}

func assertDraftResetToBase(t *testing.T, draft contextCreateRawDraft, base tobari.ContextCreateBase) {
	t.Helper()
	selection, err := contextCreateSelectionFromDraft(draft)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "standalone" || selection.Base == nil || selection.Base.Revision != base.Revision ||
		selection.PolicyMode != base.PolicyMode || selection.SourceAccess != base.SourceAccess ||
		selection.NativeReadiness != base.NativeReadiness || selection.RuntimeSelection != base.RuntimeSelection ||
		selection.MethodPolicy.Default != base.MethodPolicy.Default || len(selection.MethodPolicy.Overrides) != len(base.MethodPolicy.Overrides) {
		t.Fatalf("draft was not wholly reset from Base: %+v", selection)
	}
}

func TestContextCreateBaseResetLineRequiresConfirmationAndReplacesDraft(t *testing.T) {
	base := contextCreateResetBaseFixture()
	reader := &contextCreateBaseFixture{base: base}
	wizard := &terminalContextCreateWizard{
		mode: nil, style: false, baseRead: reader,
		bases: []tobari.ContextSummary{{Name: base.Name}},
	}
	draft := customizedContextCreateDraft()
	if err := wizard.editContextCreateBaseLine(context.Background(), strings.NewReader("2\n2\n"), io.Discard, &draft); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 0 || draft.base != nil || draft.methodDefault != tobari.ContextMethodAllow {
		t.Fatalf("declined Base reset changed draft: calls=%d draft=%+v", reader.calls, draft)
	}
	if err := wizard.editContextCreateBaseLine(context.Background(), strings.NewReader("2\n1\n"), io.Discard, &draft); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 {
		t.Fatalf("confirmed Base reset reads = %d", reader.calls)
	}
	assertDraftResetToBase(t, draft, base)
}

func TestContextCreateBaseResetRawRequiresExplicitReset(t *testing.T) {
	base := contextCreateResetBaseFixture()
	reader := &contextCreateBaseFixture{base: base}
	wizard := &terminalContextCreateWizard{
		mode: nil, style: false, baseRead: reader,
		bases: []tobari.ContextSummary{{Name: base.Name}},
	}
	draft := customizedContextCreateDraft()
	lineCount := 0
	keys := "\r\x1b[B\r\x1b[A\r"
	navigation, err := wizard.editContextCreateSettingsRaw(context.Background(), strings.NewReader(keys), io.Discard, &lineCount, &draft)
	if err != nil || navigation != contextCreateNavigateNext {
		t.Fatalf("raw Base reset navigation/error = %v/%v", navigation, err)
	}
	if reader.calls != 1 {
		t.Fatalf("raw Base reset reads = %d", reader.calls)
	}
	assertDraftResetToBase(t, draft, base)
}

func (f *contextCreateBootstrapFixture) DiscoverAWSBootstraps(context.Context) (tobari.ContextAWSBootstrapDiscovery, error) {
	f.discoveryCalls++
	copy := f.aws.Clone()
	return tobari.ContextAWSBootstrapDiscovery{State: tobari.ContextBootstrapDiscoveryAvailable, Candidates: []tobari.ContextAWSBootstrapCandidate{
		{Profile: "broken", State: tobari.ContextBootstrapCandidateUnavailable, Reason: "Referenced SSO session does not exist."},
		{Profile: copy.AWS.Profile, State: tobari.ContextBootstrapCandidateAvailable, Snapshot: &copy},
	}}, nil
}

func (f *contextCreateBootstrapFixture) DiscoverEKSBootstraps(_ context.Context, aws tobari.ContextBootstrapSnapshot) (tobari.ContextEKSBootstrapDiscovery, error) {
	f.discoveryCalls++
	candidates := []tobari.ContextEKSBootstrapCandidate{{ContextName: "mismatch", State: tobari.ContextBootstrapCandidateUnavailable, Reason: "Uses a different AWS profile."}}
	if f.eks != nil {
		copy := f.eks.Clone()
		candidates = append(candidates, tobari.ContextEKSBootstrapCandidate{ContextName: copy.EKS.ContextName, State: tobari.ContextBootstrapCandidateAvailable, Snapshot: &copy})
	}
	return tobari.ContextEKSBootstrapDiscovery{State: tobari.ContextBootstrapDiscoveryAvailable, AWSRevision: aws.Revision, Candidates: candidates}, nil
}

func (f *contextCreateBootstrapFixture) PrepareAWSBootstrap(context.Context, string) (tobari.ContextBootstrapSnapshot, error) {
	return f.aws.Clone(), nil
}

func (f *contextCreateBootstrapFixture) PrepareEKSBootstrap(_ context.Context, _ tobari.ContextBootstrapSnapshot, _ string) (tobari.ContextBootstrapSnapshot, error) {
	if f.eks == nil {
		return tobari.ContextBootstrapSnapshot{}, errors.New("EKS fixture is unavailable")
	}
	return f.eks.Clone(), nil
}

func (f *driftingContextCreateBootstrapFixture) PrepareAWSBootstrap(ctx context.Context, profile string) (tobari.ContextBootstrapSnapshot, error) {
	if !f.drifted {
		f.drifted = true
		updated := f.aws.AWS.Clone()
		updated.RoleName = "ReadOnly"
		snapshot, err := tobari.NewContextBootstrapSnapshot(1, updated)
		if err != nil {
			return tobari.ContextBootstrapSnapshot{}, err
		}
		f.aws = snapshot
	}
	return f.contextCreateBootstrapFixture.PrepareAWSBootstrap(ctx, profile)
}

func TestContextCreateWizardCollectsNameFilesystemAndEveryMethodDecision(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	// name, filesystem=read-write, customize, default=exact-review, GET=allow,
	// inherit the default for the remaining methods, choose standard@1,
	// continue without Workspace bootstrap, then create.
	input := "coding\n1\n2\ne\na\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)-1) + "1\n1\n1\n"
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || selection.SourceAccess != tobari.ContextSourceAccessReadWrite ||
		selection.MethodPolicy.Default != tobari.ContextMethodExactReview ||
		len(selection.MethodPolicy.Overrides) != 1 || selection.MethodPolicy.Overrides[0] != (tobari.ContextMethodOverride{Method: "GET", Decision: tobari.ContextMethodAllow}) {
		t.Fatalf("wizard selection = %+v", selection)
	}
	for _, required := range []string{"Context name:", "Project source access", "Other methods (default)", "GET", "TRACE", "Workspace bootstrap", "Host configuration is read only after Configure from host is selected.", "Review & Create", "Ready Runtime revision", "standard@1", "Claude Code and Codex routine traffic", "Private and unsafe"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("wizard output lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardDoesNotInspectBootstrapOnOrdinaryPath(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, false)
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: bootstrap}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n1\n1\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Bootstrap != nil || bootstrap.discoveryCalls != 0 {
		t.Fatalf("ordinary path inspected bootstrap: selection=%+v calls=%d", selection, bootstrap.discoveryCalls)
	}
}

func TestContextCreateWizardSeedPreservesSuppliedValuesAndSkipsTheirInitialStages(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	seed := contextCreateWizardSeed{
		Selection: contextCreateSelection{
			Name:             "sre3",
			SourceAccess:     tobari.ContextSourceAccessReadOnly,
			RuntimeSelection: "frontend@4",
			MethodPolicy: tobari.ContextMethodPolicy{
				Default:   tobari.ContextMethodExactReview,
				Overrides: []tobari.ContextMethodOverride{},
			},
		},
		NameProvided:     true,
		FilesystemFilled: true,
		RuntimeProvided:  true,
		BootstrapFilled:  true,
	}
	var output bytes.Buffer
	selection, err := wizard.ComposeSeeded(context.Background(), strings.NewReader("1\n1\n"), &output, seed)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "sre3" || selection.SourceAccess != tobari.ContextSourceAccessReadOnly || selection.RuntimeSelection != "frontend@4" {
		t.Fatalf("seeded wizard selection = %+v", selection)
	}
	for _, skipped := range []string{"Context name:", "Ready Runtime revision", "Tobari · Create Context · Workspace bootstrap"} {
		if strings.Contains(output.String(), skipped) {
			t.Errorf("seeded wizard replayed %q: %q", skipped, output.String())
		}
	}
	if !strings.Contains(output.String(), "Tobari · Create Context · Network access") || !strings.Contains(output.String(), "Review & Create") {
		t.Fatalf("seeded wizard omitted required stages: %q", output.String())
	}
}

func TestContextCreateWizardStageNavigationSkipsPrefilledStages(t *testing.T) {
	t.Parallel()
	seed := contextCreateWizardSeed{NameProvided: true, FilesystemFilled: true, RuntimeProvided: true}
	if got := firstContextCreateStep(seed); got != contextCreateStepNetwork {
		t.Fatalf("first partial step = %v, want Network", got)
	}
	if got := nextContextCreateStep(contextCreateStepNetwork, seed); got != contextCreateStepBootstrap {
		t.Fatalf("next partial step = %v, want Bootstrap", got)
	}
	if got, ok := previousContextCreateStep(contextCreateStepBootstrap, seed); !ok || got != contextCreateStepNetwork {
		t.Fatalf("previous partial step = (%v, %t), want Network", got, ok)
	}
}

func TestContextCreateWizardOffersOnlyReadyRuntimeRevisions(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false, runtimes: []tobari.RuntimeSummary{
		{ID: "018bcfe5-687b-7000-8000-000000000071", Name: "frontend", Kind: tobari.RuntimeKindManaged, Ready: true, Head: 4, Revision: "sha256:" + strings.Repeat("a", 64), SourcePath: "/config/runtimes/frontend/source"},
		{ID: "018bcfe5-687b-7000-8000-000000000072", Name: "backend", Kind: tobari.RuntimeKindManaged, SourcePath: "/config/runtimes/backend/source"},
	}}
	draft := contextCreateRawDraft{name: "coding", runtimeSelection: tobari.StandardRuntimeName}
	var output bytes.Buffer
	if err := wizard.editContextCreateRuntimeLine(context.Background(), strings.NewReader("2\n"), &output, &draft); err != nil {
		t.Fatal(err)
	}
	if draft.runtimeSelection != "frontend@4" {
		t.Fatalf("Runtime selection = %q", draft.runtimeSelection)
	}
	if strings.Contains(output.String(), "backend") {
		t.Fatalf("unready Runtime was offered: %q", output.String())
	}
}

func TestContextCreateWizardCanSelectAWSBootstrapProfile(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, false)
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: bootstrap}
	input := "coding\n1\n1\n1\n2\n2\n1\n1\n"
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" || selection.Bootstrap == nil {
		t.Fatalf("AWS bootstrap profile = %q", selection.AWSBootstrapProfile)
	}
}

func TestContextCreateWizardCanComposeAWSAndEKSBootstrap(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, true)
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: bootstrap}
	input := "coding\n1\n1\n1\n2\n2\n2\n1\n"
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" || selection.EKSBootstrapContext != "platform" {
		t.Fatalf("composed bootstrap = %+v", selection)
	}
}

func TestContextCreateWizardKeepsDraftAndRequiresReviewAfterSemanticDrift(t *testing.T) {
	t.Parallel()
	fixture := &driftingContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: fixture}
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n2\n2\n1\n1\n1\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Bootstrap == nil || selection.Bootstrap.AWS.RoleName != "ReadOnly" || !strings.Contains(output.String(), "changed during review") {
		t.Fatalf("drifted selection/output = %+v / %q", selection, output.String())
	}
}

func TestContextCreateWizardCanExplicitlyContinueWithoutRejectedOptionalBootstrap(t *testing.T) {
	t.Parallel()
	fixture := &rejectedContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: fixture}
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n2\n1\n1\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Bootstrap != nil || !strings.Contains(output.String(), "unsafe permissions") || !strings.Contains(output.String(), "help config bootstrap aws") {
		t.Fatalf("rejected optional bootstrap = %+v / %q", selection, output.String())
	}
}

func TestContextCreateWizardLineReportsKnownEmptyBootstrapCandidates(t *testing.T) {
	t.Parallel()
	t.Run("AWS", func(t *testing.T) {
		fixture := &emptyAWSContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
		wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: fixture}
		var output bytes.Buffer
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n2\n1\n1\n"), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "No compatible IAM Identity Center profiles were found.") {
			t.Fatalf("line AWS empty discovery was not explicit: %q", output.String())
		}
	})
	t.Run("EKS", func(t *testing.T) {
		fixture := &emptyEKSContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
		wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: fixture}
		var output bytes.Buffer
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n2\n2\n1\n1\n"), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "No compatible Amazon EKS contexts were found.") {
			t.Fatalf("line EKS empty discovery was not explicit: %q", output.String())
		}
	})
}

func TestContextCreateWizardFallsBackToBoundedLineModeWhenRawModeIsUnavailable(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{enterErr: errors.New("raw mode unavailable")}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	input := "coding\n1\n1\n1\n1\n1\n"
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || mode.entered != 1 || mode.restored != 0 {
		t.Fatalf("line fallback selection/mode = %+v / %d/%d", selection, mode.entered, mode.restored)
	}
	if strings.Contains(output.String(), "\x1b[") || !strings.Contains(output.String(), "Context name:") {
		t.Fatalf("line fallback output = %q", output.String())
	}
}

func TestContextCreateWizardLineReviewCanCancelTheCompleteSelection(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "coding\n1\n1\n1\n1\n3\n"
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "Review & Create") || !strings.Contains(output.String(), "Create Context") || !strings.Contains(output.String(), "Edit settings") {
		t.Fatalf("line review was not shown before cancellation: %q", output.String())
	}
}

func TestContextCreateWizardRawUsesOneContinuousSixStepSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Ba\r\r\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadWrite || len(selection.MethodPolicy.Overrides) != 1 ||
		selection.MethodPolicy.Overrides[0].Method != "GET" || selection.MethodPolicy.Overrides[0].Decision != tobari.ContextMethodAllow {
		t.Fatalf("raw wizard selection = %+v", selection)
	}
	for _, required := range []string{
		"1 of 6 · Name", "2 of 6 · Filesystem", "3 of 6 · Network",
		"4 of 6 · Runtime", "5 of 6 · Workspace bootstrap", "6 of 6 · Review & Create", "Continue with these settings", "Customize method policies",
		"Context name:", "Other methods (default)", "SOURCE", "Ready Runtime revision", "standard@1", "Create Context", "Edit settings",
	} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("raw wizard output lacks %q: %q", required, output.String())
		}
	}
	if strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("alternate screen entry/exit count is not one: %q", output.String())
	}
	if strings.Index(output.String(), selectorAlternateScreenEnter) > strings.Index(output.String(), "Context name:") {
		t.Fatalf("name prompt appeared before the full-screen session: %q", output.String())
	}
}

func TestContextCreateWizardRawPrefilledNameStartsAtFirstOmittedStage(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	seed := contextCreateWizardSeed{
		Selection: contextCreateSelection{
			Name:             "sre3",
			SourceAccess:     tobari.ContextSourceAccessReadWrite,
			RuntimeSelection: tobari.StandardRuntimeName,
			MethodPolicy: tobari.ContextMethodPolicy{
				Default:   tobari.ContextMethodExactReview,
				Overrides: []tobari.ContextMethodOverride{},
			},
		},
		NameProvided: true,
	}
	var output bytes.Buffer
	selection, err := wizard.ComposeSeeded(
		context.Background(), strings.NewReader("\r\r\r\r\r"), &output, seed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "sre3" {
		t.Fatalf("prefilled raw selection = %+v", selection)
	}
	if strings.Contains(output.String(), "1 of 6 · Name") || strings.Contains(output.String(), "Context name:") {
		t.Fatalf("prefilled Name stage was replayed: %q", output.String())
	}
	for _, required := range []string{"2 of 6 · Filesystem", "3 of 6 · Network", "4 of 6 · Runtime", "5 of 6 · Workspace bootstrap", "6 of 6 · Review & Create"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("prefilled raw flow lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardRawRuntimeStepSelectsOnlyReadyRevision(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, runtimes: []tobari.RuntimeSummary{
		{ID: "018bcfe5-687b-7000-8000-000000000071", Name: "frontend", Kind: tobari.RuntimeKindManaged, Ready: true, Head: 4, Revision: "sha256:" + strings.Repeat("a", 64), SourcePath: "/config/runtimes/frontend/source"},
		{ID: "018bcfe5-687b-7000-8000-000000000072", Name: "backend", Kind: tobari.RuntimeKindManaged, SourcePath: "/config/runtimes/backend/source"},
	}}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.RuntimeSelection != "frontend@4" {
		t.Fatalf("Runtime selection = %q", selection.RuntimeSelection)
	}
	if !strings.Contains(output.String(), "4 of 6 · Runtime") ||
		!strings.Contains(output.String(), "standard@1") ||
		!strings.Contains(output.String(), "frontend@4") ||
		strings.Contains(output.String(), "backend") {
		t.Fatalf("Runtime step output = %q", output.String())
	}
}

func TestContextCreateWizardRawBackFromRuntimeReturnsToNetworkAndPreservesDraft(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\x1b[B\r\rb\r\r\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadOnly || selection.RuntimeSelection != tobari.StandardRuntimeName {
		t.Fatalf("selection after Runtime Back = %+v", selection)
	}
	if strings.Count(output.String(), "3 of 6 · Network") < 2 {
		t.Fatalf("Runtime Back did not return to Network: %q", output.String())
	}
}

func TestContextCreateWizardRawBackFromBootstrapReturnsToRuntimeWithoutHostRead(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, false)
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: bootstrap}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\rb\r\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Bootstrap != nil || bootstrap.discoveryCalls != 0 {
		t.Fatalf("Bootstrap Back inspected host or changed selection: %+v calls=%d", selection, bootstrap.discoveryCalls)
	}
	if strings.Count(output.String(), "4 of 6 · Runtime") < 2 {
		t.Fatalf("Bootstrap Back did not return to Runtime: %q", output.String())
	}
}

func TestContextCreateReviewMirrorsFilesystemNetworkRuntimeOrder(t *testing.T) {
	t.Parallel()
	lines := strings.Join(contextCreateReviewLines(false, contextCreateSelection{
		Name: "coding", RuntimeSelection: tobari.StandardRuntimeName,
		SourceAccess: tobari.ContextSourceAccessReadWrite,
		MethodPolicy: tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview},
	}), "\n")
	filesystem := strings.Index(lines, "Filesystem")
	network := strings.Index(lines, "Network")
	runtime := strings.Index(lines, "Runtime")
	bootstrap := strings.Index(lines, "Workspace bootstrap")
	if filesystem < 0 || network <= filesystem || runtime <= network || bootstrap <= runtime {
		t.Fatalf("review section order is not Filesystem, Network, Runtime, Bootstrap: %q", lines)
	}
	if !strings.Contains(lines, "standard@1") || !strings.Contains(lines, "ready") {
		t.Fatalf("review Runtime is not exact and ready: %q", lines)
	}
}

func TestContextCreateReviewDoesNotPresentDisabledNativeReadinessAsAllowed(t *testing.T) {
	t.Parallel()
	lines := strings.Join(contextCreateReviewLines(false, contextCreateSelection{
		Name: "coding", RuntimeSelection: tobari.StandardRuntimeName,
		SourceAccess:    tobari.ContextSourceAccessReadWrite,
		NativeReadiness: tobari.ContextNativeReadinessDisabled,
		MethodPolicy:    tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview},
	}), "\n")
	if !strings.Contains(lines, "Claude/Codex routine      not pre-authorized") || strings.Contains(lines, "Claude/Codex routine      allow") {
		t.Fatalf("disabled native readiness review = %q", lines)
	}
}

func TestContextCreateWizardRawBackNavigationPreservesStagedFilesystem(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\x1b[B\rb\r\r\r\r\r"), io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadOnly {
		t.Fatalf("source access after back navigation = %q", selection.SourceAccess)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
}

func TestContextCreateWizardRawMethodEditorExposesInheritanceAndReset(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Bair\r\r\r\r"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.ContextMethodExactReview || len(selection.MethodPolicy.Overrides) != 0 {
		t.Fatalf("reset method policy = %+v", selection.MethodPolicy)
	}
	for _, required := range []string{"METHOD", "POLICY", "SOURCE", "override", "inherited", "i inherit", "r reset defaults"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("method editor lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardMethodRowsRemainAlignedWhenStyled(t *testing.T) {
	t.Parallel()

	draft := &contextCreateRawDraft{
		name: "coding", sourceIndex: 0, methodSelected: 1,
		methodDefault:   tobari.ContextMethodExactReview,
		methodOverrides: map[string]tobari.ContextMethodDecision{"GET": tobari.ContextMethodAllow},
	}
	var output bytes.Buffer
	lineCount := 0
	navigation, err := editContextCreateMethodPolicyRaw(context.Background(), strings.NewReader("\r"), &output, &lineCount, true, draft)
	if err != nil {
		t.Fatal(err)
	}
	if navigation != contextCreateNavigateNext {
		t.Fatalf("method editor navigation = %v, want next", navigation)
	}
	if lineCount == 0 {
		t.Fatal("method editor rendered no lines")
	}
	visible := stripANSIStyles(output.String())
	for _, want := range []string{
		fmt.Sprintf("❯ %-25s %-15s %s", "GET", "allow", "override"),
		fmt.Sprintf("  %-25s %-15s %s", "HEAD", "exact review", "inherited"),
	} {
		if !strings.Contains(visible, want) {
			t.Errorf("styled method editor lacks aligned row %q: %q", want, visible)
		}
	}
}

func TestContextCreateWizardRawDefaultChangePreservesOnlyExactOverrides(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Ba\x1b[Ad\r\r\r\r"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.ContextMethodDeny || len(selection.MethodPolicy.Overrides) != 1 || selection.MethodPolicy.Decision("GET") != tobari.ContextMethodAllow || selection.MethodPolicy.Decision("POST") != tobari.ContextMethodDeny {
		t.Fatalf("default/override policy = %+v", selection.MethodPolicy)
	}
}

func TestContextCreateWizardRawEditSectionReturnsDirectlyToReview(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\x1b[B\r\x1b[B\r\x1b[A\r"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadOnly {
		t.Fatalf("section-local edit source access = %q", selection.SourceAccess)
	}
}

func TestContextCreateWizardRawEditNameControlCCancelsWithoutSelection(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\r\x03"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if selection.Name != "" || selection.SourceAccess != "" || len(selection.MethodPolicy.Overrides) != 0 || selection.Bootstrap != nil {
		t.Fatalf("canceled wizard returned selection = %+v", selection)
	}
}

func TestContextCreateWizardRawEditNetworkCancelPropagates(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	_, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\x1b[B\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditAWSChooserCancelPropagates(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: newContextCreateBootstrapFixture(t, true)}
	_, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditEKSChooserCancelPropagates(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: newContextCreateBootstrapFixture(t, true)}
	_, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\x1b[B\r\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditNetworkBackDiscardsStagedPolicy(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[B\r\x1b[B\x1b[B\r\x1b[B\r\x1b[Bab\x1b[A\r"), io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.ContextMethodExactReview || len(selection.MethodPolicy.Overrides) != 0 {
		t.Fatalf("back navigation committed staged policy = %+v", selection.MethodPolicy)
	}
}

func TestContextCreateWizardRawReviewCanScrollWithoutChangingAction(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\r\r\x1b[6~\r"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || !strings.Contains(output.String(), "PgUp/PgDn scroll") || !strings.Contains(output.String(), "Workspace bootstrap") {
		t.Fatalf("scrolling review output = %q", output.String())
	}
}

func TestContextCreateWizardRawCollectsAWSProfileWithoutLeavingSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false, bootstrap: newContextCreateBootstrapFixture(t, false)}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\x1b[B\r\x1b[B\r\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" {
		t.Fatalf("AWS profile = %q", selection.AWSBootstrapProfile)
	}
	if !strings.Contains(output.String(), "AWS profile") || !strings.Contains(output.String(), "account 123456789012") ||
		strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("AWS profile was not collected inside one full-screen session: %q", output.String())
	}
}

func TestContextCreateWizardRawCollectsAWSAndEKSInsideOneSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false, bootstrap: newContextCreateBootstrapFixture(t, true)}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\r\x1b[B\r\x1b[B\r\x1b[B\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" || selection.EKSBootstrapContext != "platform" {
		t.Fatalf("composed bootstrap = %+v", selection)
	}
	if !strings.Contains(output.String(), "Amazon EKS") || strings.Count(output.String(), selectorAlternateScreenEnter) != 1 || strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("EKS bootstrap left the continuous session: %q", output.String())
	}
}

func TestContextCreateWizardRawReportsKnownEmptyBootstrapCandidates(t *testing.T) {
	t.Parallel()
	t.Run("AWS", func(t *testing.T) {
		fixture := &emptyAWSContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
		wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: fixture}
		var output bytes.Buffer
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\r\x1b[B\r\r\r"), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "No compatible IAM Identity Center profiles were found.") {
			t.Fatalf("raw AWS empty discovery was not explicit: %q", output.String())
		}
	})
	t.Run("EKS", func(t *testing.T) {
		fixture := &emptyEKSContextCreateBootstrapFixture{contextCreateBootstrapFixture: newContextCreateBootstrapFixture(t, false)}
		wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: fixture}
		var output bytes.Buffer
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\r\x1b[B\r\x1b[B\r\r\r"), &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "No compatible Amazon EKS contexts were found.") {
			t.Fatalf("raw EKS empty discovery was not explicit: %q", output.String())
		}
	})
}

func TestContextCreateWizardRawCancelRestoresTerminalBeforeSelection(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader("coding\rq"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if mode.entered != 1 || mode.restored != 1 ||
		strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("terminal cleanup entered/restored = %d/%d, output %q", mode.entered, mode.restored, output.String())
	}
}

func TestContextCreateWizardRawKeepsInvalidNameInsideFirstStep(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader("INVALID\r\x03"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "name must match [a-z][a-z0-9-]{0,62}") ||
		strings.Contains(output.String(), "2 of 6 · Filesystem") {
		t.Fatalf("invalid name escaped the first step: %q", output.String())
	}
}

type oneTimeoutReader struct {
	remaining io.Reader
	timedOut  bool
}

func (r *oneTimeoutReader) Read(value []byte) (int, error) {
	if !r.timedOut {
		r.timedOut = true
		return 0, nil
	}
	return r.remaining.Read(value)
}

func TestContextCreateWizardRawDoesNotRedrawAnUnchangedTextStepOnPollingTimeout(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	input := &oneTimeoutReader{remaining: strings.NewReader("\x03")}
	_, err := wizard.Compose(context.Background(), input, &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if count := strings.Count(output.String(), "Context name:"); count != 1 {
		t.Fatalf("unchanged name step rendered %d times after one timeout: %q", count, output.String())
	}
}

func TestContextCreateWizardRejectsInvalidNameBeforeCompositionAndCancelsWithoutSelection(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "../outside\nsafe\nq\n"
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "Use a valid portable Context name.") {
		t.Fatalf("invalid name was not explained: %q", output.String())
	}
}

func TestContextCreateDirectInputCompletenessDistinguishesPartialCompositionFromFormat(t *testing.T) {
	t.Parallel()
	empty := ParsedInputs{provided: map[string]bool{}}
	if contextCreateDirectInputsComplete(empty) || contextCreateCompositionInputProvided(empty) {
		t.Fatal("argument-free create unexpectedly selected direct or prefilled mode")
	}
	for _, name := range []string{"--name", "--runtime", "--mode", "--source-access", "--native-readiness", "--bootstrap-aws-profile"} {
		inputs := ParsedInputs{provided: map[string]bool{name: true}}
		if contextCreateDirectInputsComplete(inputs) || !contextCreateCompositionInputProvided(inputs) {
			t.Errorf("partial %s was not classified as an incomplete composition", name)
		}
	}
	formatOnly := ParsedInputs{provided: map[string]bool{"--format": true}}
	if contextCreateDirectInputsComplete(formatOnly) || contextCreateCompositionInputProvided(formatOnly) {
		t.Fatal("format-only input was treated as a supplied Context boundary")
	}
	complete := ParsedInputs{provided: map[string]bool{
		"--name": true, "--runtime": true, "--mode": true,
		"--source-access": true, "--native-readiness": true,
	}}
	if !contextCreateDirectInputsComplete(complete) || !contextCreateCompositionInputProvided(complete) {
		t.Fatal("complete direct group was not recognized")
	}
}
