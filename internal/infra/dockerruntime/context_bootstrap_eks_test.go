package dockerruntime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticEKSCA(t *testing.T) string {
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
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func syntheticEKSConfig(t *testing.T) string {
	t.Helper()
	return `apiVersion: v1
kind: Config
preferences: {}
clusters:
- name: arn:aws:eks:ap-northeast-1:123456789012:cluster/platform
  cluster:
    certificate-authority-data: ` + syntheticEKSCA(t) + `
    server: https://ABC.gr7.ap-northeast-1.eks.amazonaws.com
contexts:
- name: engineering
  context:
    cluster: arn:aws:eks:ap-northeast-1:123456789012:cluster/platform
    user: arn:aws:eks:ap-northeast-1:123456789012:cluster/platform
    namespace: development
current-context: engineering
users:
- name: arn:aws:eks:ap-northeast-1:123456789012:cluster/platform
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      args:
      - --region
      - ap-northeast-1
      - eks
      - get-token
      - --cluster-name
      - platform
      - --output
      - json
      command: aws
      env:
      - name: AWS_PROFILE
        value: engineering
      interactiveMode: IfAvailable
      provideClusterInfo: false
`
}

func TestParseHostEKSBootstrapAcceptsOnlyReviewedAWSExec(t *testing.T) {
	result, err := parseHostEKSBootstrap([]byte(syntheticEKSConfig(t)), "engineering", "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if result.ClusterName != "platform" || result.Region != "ap-northeast-1" || result.Server != "https://abc.gr7.ap-northeast-1.eks.amazonaws.com" || result.Namespace != "development" {
		t.Fatalf("EKS bootstrap = %+v", result)
	}
	encoded, err := encodeProjectEKSConfig("engineering", result)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if document["current-context"] != "engineering" || strings.Contains(string(encoded), "\"token\":") || strings.Contains(string(encoded), "client-key") {
		t.Fatalf("projected kubeconfig = %s", encoded)
	}
}

func TestParseHostEKSBootstrapRejectsCredentialAndExecutableWidening(t *testing.T) {
	base := syntheticEKSConfig(t)
	cases := map[string]string{
		"profile mismatch":  base,
		"arbitrary command": strings.Replace(base, "command: aws", "command: helper", 1),
		"role argument":     strings.Replace(base, "      - --output\n", "      - --role\n      - arn:aws:iam::123456789012:role/Admin\n      - --output\n", 1),
		"proxy":             strings.Replace(base, "    server:", "    proxy-url: https://proxy.example.com\n    server:", 1),
		"token":             strings.Replace(base, "    exec:\n", "    token: secret\n    exec:\n", 1),
		"outer field":       strings.Replace(base, "- name: engineering\n  context:", "- name: engineering\n  user: {}\n  context:", 1),
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			profile := "engineering"
			if name == "profile mismatch" {
				profile = "other"
			}
			if _, err := parseHostEKSBootstrap([]byte(source), "engineering", profile); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestDiscoverEKSBootstrapsSeparatesCompatibleAndProfileMismatchCandidates(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig)
	base, err := runtime.PrepareContextAWSBootstrap(context.Background(), "engineering")
	if err != nil {
		t.Fatal(err)
	}
	home := runtime.hostHomeDirectory
	directory := filepath.Join(home, ".kube")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	source := syntheticEKSConfig(t)
	mismatch := strings.ReplaceAll(source, "- name: engineering\n  context:", "- name: engineering\n  context:")
	mismatch = strings.Replace(mismatch, "contexts:\n", "contexts:\n- name: mismatch\n  context:\n    cluster: arn:aws:eks:ap-northeast-1:123456789012:cluster/platform\n    user: mismatch-user\n", 1)
	mismatch = strings.Replace(mismatch, "users:\n", "users:\n- name: mismatch-user\n  user:\n    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      args: [--region, ap-northeast-1, eks, get-token, --cluster-name, platform, --output, json]\n      command: aws\n      env:\n      - name: AWS_PROFILE\n        value: other\n      interactiveMode: IfAvailable\n      provideClusterInfo: false\n", 1)
	if err := os.WriteFile(filepath.Join(directory, "config"), []byte(mismatch), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.DiscoverContextEKSBootstraps(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, candidate := range result.Candidates {
		states[candidate.WorkspaceManifestName] = candidate.State
	}
	if states["engineering"] != tobari.ManifestBootstrapCandidateAvailable || states["mismatch"] != tobari.ManifestBootstrapCandidateUnavailable {
		t.Fatalf("EKS candidate states = %+v (%+v)", states, result)
	}
}

func TestDiscoverEKSBootstrapsRejectsDuplicateWholeFileWithoutPartialCandidates(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig)
	base, err := runtime.PrepareContextAWSBootstrap(context.Background(), "engineering")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(runtime.hostHomeDirectory, ".kube")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	source := syntheticEKSConfig(t)
	source = strings.Replace(source, "contexts:\n", "contexts:\n- name: engineering\n  context:\n    cluster: duplicate\n    user: duplicate\n", 1)
	if err := os.WriteFile(filepath.Join(directory, "config"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.DiscoverContextEKSBootstraps(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ManifestBootstrapDiscoveryRejected || len(result.Candidates) != 0 {
		t.Fatalf("duplicate discovery = %+v", result)
	}
}

func TestContextEKSBootstrapComposesOnceAndEnforcesAWSDependency(t *testing.T) {
	runtime := newProjectStateRuntime(t)
	writeSyntheticHostAWSConfig(t, runtime, syntheticAWSSharedConfig)
	kubeDirectory := filepath.Join(runtime.hostHomeDirectory, ".kube")
	if err := os.Mkdir(kubeDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kubeDirectory, "config"), []byte(syntheticEKSConfig(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultManifestName, "engineering", "", false); err != nil {
		t.Fatal(err)
	}
	configured, err := runtime.ConfigureContextEKSBootstrap(context.Background(), tobari.DefaultManifestName, "engineering", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Bootstrap.EKSContext != "engineering" || len(configured.Bootstrap.Adapters) != 2 {
		t.Fatalf("configured bootstrap = %+v", configured.Bootstrap)
	}
	if _, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultManifestName, "", "", true); err != tobari.ErrContextBootstrapDependency {
		t.Fatalf("remove AWS with EKS error = %v", err)
	}
	if _, err := runtime.ConfigureContextAWSBootstrap(context.Background(), tobari.DefaultManifestName, "other", "", false); err != tobari.ErrContextBootstrapDependency {
		t.Fatalf("replace AWS profile with EKS error = %v", err)
	}
	root := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProjectInContext(context.Background(), root, tobari.DefaultManifestName)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := os.ReadFile(filepath.Join(runtime.projectHomePath(instance.ID), ".kube", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projected), `"AWS_PROFILE"`) || strings.Contains(string(projected), `"token":`) {
		t.Fatalf("projected kubeconfig = %s", projected)
	}
	removed, err := runtime.ConfigureContextEKSBootstrap(context.Background(), tobari.DefaultManifestName, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Bootstrap.EKSContext != "" || len(removed.Bootstrap.Adapters) != 1 {
		t.Fatalf("removed EKS bootstrap = %+v", removed.Bootstrap)
	}
}
