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
	// inherit the default for the remaining methods, then create.
	input := "coding\n1\n2\ne\na\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)-1) + "1\n"
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || selection.SourceAccess != tobari.ContextSourceAccessReadWrite ||
		selection.MethodPolicy.Default != tobari.PolicyPresetMethodExactReview ||
		len(selection.MethodPolicy.Overrides) != 1 || selection.MethodPolicy.Overrides[0] != (tobari.PolicyPresetMethodOverride{Method: "GET", Decision: tobari.PolicyPresetMethodAllow}) {
		t.Fatalf("wizard selection = %+v", selection)
	}
	for _, required := range []string{"Context name:", "Project source access", "Other methods (default)", "GET", "TRACE", "Workspace bootstrap", "Review & Create", "Runtime", "standard Tobari runtime", "Claude Code and Codex routine traffic", "Private and unsafe"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("wizard output lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardDoesNotInspectBootstrapOnOrdinaryPath(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, false)
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: bootstrap}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n1\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Bootstrap != nil || bootstrap.discoveryCalls != 0 {
		t.Fatalf("ordinary path inspected bootstrap: selection=%+v calls=%d", selection, bootstrap.discoveryCalls)
	}
}

func TestContextCreateWizardCanSelectAWSBootstrapProfile(t *testing.T) {
	t.Parallel()
	bootstrap := newContextCreateBootstrapFixture(t, false)
	wizard := &terminalContextCreateWizard{mode: nil, style: false, bootstrap: bootstrap}
	input := "coding\n1\n1\n2\n4\n2\n1\n1\n"
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
	input := "coding\n1\n1\n2\n4\n2\n2\n1\n"
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
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n2\n4\n2\n1\n1\n1\n"), &output)
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
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n2\n4\n1\n1\n"), &output)
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
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n2\n4\n1\n1\n"), &output); err != nil {
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
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\n1\n1\n2\n4\n2\n1\n1\n"), &output); err != nil {
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
	input := "coding\n1\n1\n1\n"
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
	input := "coding\n1\n1\n3\n"
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "Review & Create") || !strings.Contains(output.String(), "Create Context") || !strings.Contains(output.String(), "Edit settings") {
		t.Fatalf("line review was not shown before cancellation: %q", output.String())
	}
}

func TestContextCreateWizardRawUsesOneContinuousFourStepSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Ba\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadWrite || len(selection.MethodPolicy.Overrides) != 1 ||
		selection.MethodPolicy.Overrides[0].Method != "GET" || selection.MethodPolicy.Overrides[0].Decision != tobari.PolicyPresetMethodAllow {
		t.Fatalf("raw wizard selection = %+v", selection)
	}
	for _, required := range []string{
		"1 of 4 · Name", "2 of 4 · Filesystem", "3 of 4 · Network",
		"4 of 4 · Review & Create", "Continue with these settings", "Customize method policies",
		"Context name:", "Other methods (default)", "SOURCE", "Runtime", "standard Tobari runtime", "Create Context", "Edit settings",
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

func TestContextCreateWizardRawBackNavigationPreservesStagedFilesystem(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\x1b[B\rb\r\r\r\r"), io.Discard,
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
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Bair\r\r"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.PolicyPresetMethodExactReview || len(selection.MethodPolicy.Overrides) != 0 {
		t.Fatalf("reset method policy = %+v", selection.MethodPolicy)
	}
	for _, required := range []string{"METHOD", "POLICY", "SOURCE", "override", "inherited", "i inherit", "r reset defaults"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("method editor lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardRawDefaultChangePreservesOnlyExactOverrides(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\x1b[B\r\x1b[Ba\x1b[Ad\r\r"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.PolicyPresetMethodDeny || len(selection.MethodPolicy.Overrides) != 1 || selection.MethodPolicy.Decision("GET") != tobari.PolicyPresetMethodAllow || selection.MethodPolicy.Decision("POST") != tobari.PolicyPresetMethodDeny {
		t.Fatalf("default/override policy = %+v", selection.MethodPolicy)
	}
}

func TestContextCreateWizardRawEditSectionReturnsDirectlyToReview(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\r\x1b[B\r\x1b[A\r"), io.Discard)
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
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\r\x03"), io.Discard,
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
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditAWSChooserCancelPropagates(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: newContextCreateBootstrapFixture(t, true)}
	_, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditEKSChooserCancelPropagates(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false, bootstrap: newContextCreateBootstrapFixture(t, true)}
	_, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\r\x1b[B\rq"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
}

func TestContextCreateWizardRawEditNetworkBackDiscardsStagedPolicy(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: &selectorModeFake{}, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\r\x1b[B\r\x1b[Bab\x1b[A\r"), io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.MethodPolicy.Default != tobari.PolicyPresetMethodExactReview || len(selection.MethodPolicy.Overrides) != 0 {
		t.Fatalf("back navigation committed staged policy = %+v", selection.MethodPolicy)
	}
}

func TestContextCreateWizardRawReviewCanScrollWithoutChangingAction(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\x1b[6~\r"), &output)
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
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\r\x1b[B\r\r\x1b[A\r"), &output,
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
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\r\x1b[B\r\x1b[B\r\x1b[A\r"), &output,
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
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\r\r\x1b[A\r"), &output); err != nil {
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
		if _, err := wizard.Compose(context.Background(), strings.NewReader("coding\r\r\r\x1b[B\r\x1b[B\x1b[B\x1b[B\r\x1b[B\r\r\x1b[A\r"), &output); err != nil {
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
		strings.Contains(output.String(), "2 of 4 · Filesystem") {
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

func TestArgumentFreeContextCreateIsTheOnlyWizardMode(t *testing.T) {
	t.Parallel()
	empty := ParsedInputs{provided: map[string]bool{}}
	if !contextCreateInputsOmitted(empty) {
		t.Fatal("argument-free create did not select wizard mode")
	}
	for _, name := range []string{"--name", "--image", "--mode", "--source-access", "--policy-preset", "--native-readiness", "--bootstrap-aws-profile", "--format"} {
		inputs := ParsedInputs{provided: map[string]bool{name: true}}
		if contextCreateInputsOmitted(inputs) {
			t.Errorf("explicit %s unexpectedly selected wizard mode", name)
		}
	}
}
