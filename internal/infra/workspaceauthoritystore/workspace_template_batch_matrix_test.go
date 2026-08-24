package workspaceauthoritystore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func batchTemplateAWS() tobari.ManifestAWSBootstrap {
	return tobari.ManifestAWSBootstrap{
		Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
		SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"},
		AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
	}
}

func batchTemplateEKS(t *testing.T) tobari.ManifestEKSBootstrap {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic Batch B EKS CA"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return tobari.ManifestEKSBootstrap{
		WorkspaceManifestName: "engineering", ClusterName: "platform", Region: "ap-northeast-1",
		Server:                   "https://abc.gr7.ap-northeast-1.eks.amazonaws.com",
		CertificateAuthorityData: base64.StdEncoding.EncodeToString(certificate), Namespace: "development",
	}
}

type batchTemplateBootstrapSource struct {
	aws       tobari.ManifestAWSBootstrap
	eks       tobari.ManifestEKSBootstrap
	err       error
	lifecycle *mutationLifecycle
}

func (s *batchTemplateBootstrapSource) PrepareFinalTemplateAWSBootstrap(_ context.Context, profile string) (tobari.ManifestBootstrapSnapshot, error) {
	if s.lifecycle != nil && !s.lifecycle.held.Load() {
		return tobari.ManifestBootstrapSnapshot{}, fmt.Errorf("AWS source resolved outside lifecycle lock")
	}
	if s.err != nil {
		return tobari.ManifestBootstrapSnapshot{}, s.err
	}
	if profile != s.aws.Profile {
		return tobari.ManifestBootstrapSnapshot{}, fmt.Errorf("unexpected AWS profile %q", profile)
	}
	return tobari.NewContextBootstrapSnapshot(1, s.aws)
}

func (s *batchTemplateBootstrapSource) PrepareFinalTemplateEKSBootstrap(_ context.Context, base tobari.ManifestBootstrapSnapshot, name string) (tobari.ManifestBootstrapSnapshot, error) {
	if s.lifecycle != nil && !s.lifecycle.held.Load() {
		return tobari.ManifestBootstrapSnapshot{}, fmt.Errorf("EKS source resolved outside lifecycle lock")
	}
	if s.err != nil {
		return tobari.ManifestBootstrapSnapshot{}, s.err
	}
	if name != s.eks.WorkspaceManifestName {
		return tobari.ManifestBootstrapSnapshot{}, fmt.Errorf("unexpected EKS context %q", name)
	}
	return tobari.NewContextBootstrapSnapshotWithEKS(base.Generation, base.AWS, s.eks)
}

func TestWorkspaceTemplateBootstrapActionsResolveUnderLifecycleAndRemainAtomic(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	source := &batchTemplateBootstrapSource{aws: batchTemplateAWS(), eks: batchTemplateEKS(t), lifecycle: lifecycle}
	mutator.bootstrapSource = source

	awsConfigure := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: source.aws.Profile}
	publication, change, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsConfigure)
	if err != nil || !publication.Changed || change.AWS == nil || change.AWS.Profile != source.aws.Profile {
		t.Fatalf("AWS configure publication=%+v change=%+v err=%v", publication, change, err)
	}
	awsRefresh := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapRefresh}
	publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsRefresh)
	if err != nil || publication.Changed {
		t.Fatalf("AWS semantic no-op refresh publication=%+v err=%v", publication, err)
	}
	source.aws.RoleName = "ReadOnly"
	publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsRefresh)
	if err != nil || !publication.Changed || publication.Current.Body.CreationDefaults.Bootstrap.AWS.RoleName != "ReadOnly" {
		t.Fatalf("AWS changed-source refresh publication=%+v err=%v", publication, err)
	}

	eksConfigure := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: source.eks.WorkspaceManifestName}
	publication, change, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, eksConfigure)
	if err != nil || !publication.Changed || change.EKS == nil || change.EKS.WorkspaceManifestName != source.eks.WorkspaceManifestName || publication.Current.Body.CreationDefaults.Bootstrap.AWS.Profile != source.aws.Profile {
		t.Fatalf("EKS configure publication=%+v change=%+v err=%v", publication, change, err)
	}
	eksRefresh := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS, Action: tobari.WorkspaceTemplateBootstrapRefresh}
	publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, eksRefresh)
	if err != nil || publication.Changed {
		t.Fatalf("EKS semantic no-op refresh publication=%+v err=%v", publication, err)
	}
	source.eks.Namespace = "operations"
	publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, eksRefresh)
	if err != nil || !publication.Changed || publication.Current.Body.CreationDefaults.Bootstrap.EKS.Namespace != "operations" {
		t.Fatalf("EKS changed-source refresh publication=%+v err=%v", publication, err)
	}
	source.aws.RoleName = "Administrator"
	beforeIncompatible, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsRefresh); err == nil {
		t.Fatal("AWS replacement invalidated retained EKS authority")
	}
	afterIncompatible, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(beforeIncompatible, afterIncompatible) {
		t.Fatalf("incompatible AWS replacement changed collection: err=%v", err)
	}

	beforeDependency, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	awsRemove := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapRemove}
	if _, _, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsRemove); err == nil {
		t.Fatal("AWS removal with retained EKS reached publication")
	}
	afterDependency, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(beforeDependency, afterDependency) {
		t.Fatalf("dependency rejection changed collection: err=%v", err)
	}

	eksRemove := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS, Action: tobari.WorkspaceTemplateBootstrapRemove}
	if publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, eksRemove); err != nil || !publication.Changed || publication.Current.Body.CreationDefaults.Bootstrap.EKS != nil {
		t.Fatalf("EKS remove publication=%+v err=%v", publication, err)
	}
	if publication, _, err = mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, awsRemove); err != nil || !publication.Changed || publication.Current.Body.CreationDefaults.Bootstrap != nil {
		t.Fatalf("AWS remove publication=%+v err=%v", publication, err)
	}
}

func TestWorkspaceTemplateBootstrapUnknownSourceAndMissingDependencyAreZeroWrite(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	source := &batchTemplateBootstrapSource{aws: batchTemplateAWS(), eks: batchTemplateEKS(t), lifecycle: lifecycle, err: fmt.Errorf("host source unavailable")}
	mutator.bootstrapSource = source
	before, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	aws := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: source.aws.Profile}
	if _, _, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, aws); err == nil {
		t.Fatal("unknown AWS source reached publication")
	}
	source.err = nil
	eks := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: source.eks.WorkspaceManifestName}
	if _, _, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, eks); err == nil {
		t.Fatal("EKS without current AWS reached publication")
	}
	after, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed bootstrap changed collection: err=%v", err)
	}
}

func TestWorkspaceTemplateBootstrapAndSessionDeltasSerializeWithoutLoss(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	source := &batchTemplateBootstrapSource{aws: batchTemplateAWS(), eks: batchTemplateEKS(t), lifecycle: lifecycle}
	mutator.bootstrapSource = source
	value := "xterm-256color"
	shell := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &value}}}
	aws := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: source.aws.Profile}

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, shell)
		results <- err
	}()
	go func() {
		<-start
		_, _, err := mutator.UpdateWorkspaceTemplateBootstrapByReference(context.Background(), templateRef, aws)
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("serialized collection present=%t err=%v", present, err)
	}
	body := collection.Templates[0].Current.Body
	if len(body.SessionDefaults.ShellEnvironment) != 1 || body.CreationDefaults.Bootstrap == nil || body.CreationDefaults.Bootstrap.AWS.Profile != source.aws.Profile {
		t.Fatalf("serialized bootstrap/session authority was lost: %+v", body)
	}
}

func TestWorkspaceTemplateAllTypedChangesRetainEveryAcceptedRevision(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	templateRef, err := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	shellValue := "xterm-256color"
	aws := batchTemplateAWS()
	eks := batchTemplateEKS(t)
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	runtimeRevision := "sha256:" + strings.Repeat("b", 64)
	changes := []tobari.WorkspaceTemplateChange{
		{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &shellValue}}},
		{Kind: tobari.WorkspaceTemplateChangeGit, Git: &tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityInherit}},
		{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, AWS: &aws},
		{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS, EKS: &eks},
		{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: tobari.RuntimeRevisionRef(runtimeID, runtimeRevision)},
	}

	previous := existing.Templates[0].Current.Clone()
	initialRetained := len(existing.Templates[0].Retained)
	for index, change := range changes {
		publication, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, change)
		if err != nil {
			t.Fatalf("change %q: %v", change.Kind, err)
		}
		if err := publication.ValidateFor(templateRef, change); err != nil {
			t.Fatalf("change %q publication: %v", change.Kind, err)
		}
		if !publication.Changed || publication.Previous.Revision != previous.Revision ||
			publication.Current.Generation != previous.Generation+1 || len(publication.Template.Retained) != initialRetained+index+1 {
			t.Fatalf("change %q publication=%+v", change.Kind, publication)
		}
		previous = publication.Current.Clone()
	}

	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("final collection present=%t err=%v", present, err)
	}
	body := collection.Templates[0].Current.Body
	if len(body.SessionDefaults.ShellEnvironment) != 1 || body.SessionDefaults.GitIdentity == nil ||
		body.SessionDefaults.GitIdentity.Source != tobari.ManifestGitIdentityInherit ||
		body.CreationDefaults.Bootstrap == nil || body.CreationDefaults.Bootstrap.EKS == nil ||
		body.EntryDefaults.Runtime.RuntimeID != runtimeID || body.EntryDefaults.Runtime.Revision != runtimeRevision {
		t.Fatalf("final typed-delta body lost accepted authority: %+v", body)
	}
}

func TestWorkspaceTemplateRuntimeUnknownOrStaleObservationIsZeroWrite(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	wantedRevision := "sha256:" + strings.Repeat("b", 64)
	wantedRef := tobari.RuntimeRevisionRef(runtimeID, wantedRevision)
	resolver := &templateRuntimeRevisionFixture{binding: tobari.RuntimeBinding{
		RuntimeID: runtimeID, Name: "managed", Revision: "sha256:" + strings.Repeat("c", 64),
		Ordinal: 3, Image: "tobari-runtime-managed:cccccccccccc",
	}}
	mutator.runtimeRevision = resolver

	before, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, tobari.WorkspaceTemplateChange{
		Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: wantedRef,
	}); err == nil {
		t.Fatal("stale Runtime observation reached collection publication")
	}
	afterStale, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(before, afterStale) {
		t.Fatalf("stale Runtime observation changed collection: err=%v", err)
	}

	resolver.err = fmt.Errorf("Runtime revision observation unknown")
	if _, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, tobari.WorkspaceTemplateChange{
		Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: wantedRef,
	}); err == nil {
		t.Fatal("unknown Runtime observation reached collection publication")
	}
	afterUnknown, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(before, afterUnknown) {
		t.Fatalf("unknown Runtime observation changed collection: err=%v", err)
	}
}

func TestWorkspaceTemplatePublicationAfterCancellationReplaysAsOneRevision(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(existing.Templates[0].ID)
	value := "xterm-256color"
	change := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{
		Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &value,
	}}}

	ctx, cancel := context.WithCancel(context.Background())
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		err := realRename(source, target)
		if err == nil && target == filepath.Join(store.root, authorityFileName) {
			cancel()
		}
		return err
	}
	publication, err := mutator.UpdateWorkspaceTemplateByReference(ctx, templateRef, change)
	if err != nil || !publication.Changed {
		t.Fatalf("confirmed publication after cancellation=%+v err=%v", publication, err)
	}

	mutator.rename = realRename
	replayed, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, change)
	if err != nil || replayed.Changed || replayed.Current.Revision != publication.Current.Revision {
		t.Fatalf("same-action replay=%+v err=%v", replayed, err)
	}
	collection, _, err := store.ReadComplete(context.Background())
	if err != nil || collection.Templates[0].Current.Generation != existing.Templates[0].Current.Generation+1 ||
		len(collection.Templates[0].Retained) != len(existing.Templates[0].Retained)+1 {
		t.Fatalf("publication/replay advanced more than once: %+v err=%v", collection.Templates[0], err)
	}
}
