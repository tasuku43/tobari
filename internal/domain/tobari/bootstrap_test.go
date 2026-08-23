package tobari

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func testAWSBootstrap() ManifestAWSBootstrap {
	return ManifestAWSBootstrap{Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start", SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json"}
}

func testEKSBootstrap(t *testing.T) ManifestEKSBootstrap {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic EKS CA"}, NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return ManifestEKSBootstrap{WorkspaceManifestName: "engineering", ClusterName: "platform", Region: "ap-northeast-1", Server: "https://abc.gr7.ap-northeast-1.eks.amazonaws.com", CertificateAuthorityData: base64.StdEncoding.EncodeToString(certificate), Namespace: "development"}
}

func TestContextBootstrapComposesEKSWithoutChangingLegacyAWSRevision(t *testing.T) {
	legacy, err := NewContextBootstrapSnapshot(1, testAWSBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	const previousAWSOnlyRevision = "sha256:84d967af35aa87cb4a18f8e8d62b5b85d1f5d0108a09fafee9bb2868fba16141"
	if legacy.Revision != previousAWSOnlyRevision {
		t.Fatalf("legacy AWS revision = %q", legacy.Revision)
	}
	composed, err := NewContextBootstrapSnapshotWithEKS(2, testAWSBootstrap(), testEKSBootstrap(t))
	if err != nil {
		t.Fatal(err)
	}
	if composed.Revision == legacy.Revision || composed.EKS == nil {
		t.Fatalf("composed snapshot = %+v", composed)
	}
	report := ManifestBootstrapReportFrom(&composed)
	if len(report.Adapters) != 2 || report.Adapters[1] != ManifestBootstrapAdapterEKS || report.EKSContext != "engineering" {
		t.Fatalf("composed report = %+v", report)
	}
	preview, err := NewContextBootstrapPreview("default", &legacy, composed)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0] != "kubernetes_eks" {
		t.Fatalf("changes = %v", preview.Changes)
	}
}

func TestContextBootstrapSemanticPreviewAndWorkspaceStates(t *testing.T) {
	first, err := NewContextBootstrapSnapshot(1, testAWSBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	changedAWS := testAWSBootstrap()
	changedAWS.RoleName = "ReadOnly"
	second, err := NewContextBootstrapSnapshot(2, changedAWS)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewContextBootstrapPreview("coding", &first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0] != "aws.sso_role_name" {
		t.Fatalf("preview changes = %+v", preview.Changes)
	}
	for _, test := range []struct {
		name    string
		applied string
		current *ManifestBootstrapSnapshot
		want    string
	}{
		{name: "none", want: WorkspaceBootstrapNotConfigured},
		{name: "legacy existing", current: &second, want: WorkspaceBootstrapNotApplied},
		{name: "current", applied: second.Revision, current: &second, want: WorkspaceBootstrapCurrent},
		{name: "older", applied: first.Revision, current: &second, want: WorkspaceBootstrapOlder},
		{name: "removed recipe", applied: first.Revision, want: WorkspaceBootstrapOlder},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := ResolveWorkspaceBootstrapReport(test.applied, test.current)
			if err != nil || report.State != test.want {
				t.Fatalf("report = %+v, error=%v, want %s", report, err, test.want)
			}
			if err := report.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContextAWSBootstrapRejectsNonAWSStartURLAndScopeExplosion(t *testing.T) {
	invalid := testAWSBootstrap()
	invalid.SSOStartURL = "https://127.0.0.1/start"
	if err := invalid.Validate(); err == nil {
		t.Fatal("private non-AWS start URL was accepted")
	}
	invalid = testAWSBootstrap()
	invalid.SSORegistrationScopes = make([]string, 17)
	for index := range invalid.SSORegistrationScopes {
		invalid.SSORegistrationScopes[index] = string(rune('a' + index))
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("unbounded registration scopes were accepted")
	}
}

func TestContextAWSBootstrapDiscoveryRequiresTypedAvailabilityAndExplicitEmptyCollection(t *testing.T) {
	t.Parallel()
	snapshot, err := NewContextBootstrapSnapshot(1, testAWSBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	valid := ManifestAWSBootstrapDiscovery{State: ManifestBootstrapDiscoveryAvailable, Candidates: []ManifestAWSBootstrapCandidate{
		{Profile: snapshot.AWS.Profile, State: ManifestBootstrapCandidateAvailable, Snapshot: &snapshot},
		{Profile: "broken", State: ManifestBootstrapCandidateUnavailable, Reason: "Referenced SSO session does not exist."},
	}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Candidates = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("absent candidate collection was accepted")
	}
	invalid = valid
	invalid.Candidates[1].Snapshot = &snapshot
	if err := invalid.Validate(); err == nil {
		t.Fatal("unavailable candidate carried authority")
	}
}
