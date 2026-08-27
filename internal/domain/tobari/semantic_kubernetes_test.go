package tobari

import "testing"

func kubernetesResourceIdentity() PolicyProtocolIdentity {
	return PolicyProtocolIdentity{
		Scheme: "https", Protocol: PolicyProtocolKubernetes, KubernetesKind: KubernetesRequestResource,
		KubernetesVerb: "get", KubernetesGroup: "", KubernetesVersion: "v1", KubernetesResource: "pods",
		KubernetesNamespace: "team", KubernetesName: "demo", KubernetesSubresource: "log", KubernetesDryRun: "none",
	}
}

func kubernetesResourceRule(host string) SemanticKubernetesRule {
	i := kubernetesResourceIdentity()
	return SemanticKubernetesRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		Resource: &SemanticKubernetesResourceRule{
			Group: i.KubernetesGroup, Version: i.KubernetesVersion, Resource: i.KubernetesResource,
			Namespace: i.KubernetesNamespace, Name: i.KubernetesName, Subresource: i.KubernetesSubresource,
			Verb: i.KubernetesVerb, DryRun: i.KubernetesDryRun,
		},
	}
}

func TestSemanticKubernetesResourceProjectionMatchesExactTransport(t *testing.T) {
	rule := kubernetesResourceRule("cluster.us-east-1.eks.amazonaws.com")
	effect := SemanticRequestEffect{
		Scheme: "https", Host: rule.Host, Port: 443, Method: "GET", Path: "/api/v1/namespaces/team/pods/demo/log",
		Identity: kubernetesResourceIdentity(),
	}
	if !rule.Matches(effect) {
		t.Fatal("structured Kubernetes resource rule did not match its complete effect")
	}
	for name, mutate := range map[string]func(*SemanticRequestEffect){
		"path":        func(value *SemanticRequestEffect) { value.Path = "/api/v1/namespaces/other/pods/demo/log" },
		"method":      func(value *SemanticRequestEffect) { value.Method = "POST" },
		"group":       func(value *SemanticRequestEffect) { value.Identity.KubernetesGroup = "apps" },
		"version":     func(value *SemanticRequestEffect) { value.Identity.KubernetesVersion = "v2" },
		"resource":    func(value *SemanticRequestEffect) { value.Identity.KubernetesResource = "services" },
		"namespace":   func(value *SemanticRequestEffect) { value.Identity.KubernetesNamespace = "other" },
		"name":        func(value *SemanticRequestEffect) { value.Identity.KubernetesName = "other" },
		"subresource": func(value *SemanticRequestEffect) { value.Identity.KubernetesSubresource = "status" },
		"verb":        func(value *SemanticRequestEffect) { value.Identity.KubernetesVerb = "patch" },
		"dry_run":     func(value *SemanticRequestEffect) { value.Identity.KubernetesDryRun = "all" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := effect
			mutate(&changed)
			if rule.Matches(changed) {
				t.Fatalf("Kubernetes rule matched changed effect %+v", changed)
			}
		})
	}
}

func TestSemanticKubernetesNonResourceIsSeparateVariant(t *testing.T) {
	rule := SemanticKubernetesRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "cluster.us-east-1.eks.amazonaws.com", Port: 443},
		NonResource:           &SemanticKubernetesNonResourceRule{Path: "/openapi/v3", Verb: "get"},
	}
	effect := SemanticRequestEffect{
		Scheme: "https", Host: rule.Host, Port: 443, Method: "GET", Path: "/openapi/v3",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolKubernetes, KubernetesKind: KubernetesRequestNonResource, KubernetesVerb: "get", KubernetesNonResourcePath: "/openapi/v3"},
	}
	if !rule.Matches(effect) {
		t.Fatal("Kubernetes non-resource rule did not match")
	}
	resource := kubernetesResourceIdentity()
	effect.Identity = resource
	effect.Path = "/api/v1/namespaces/team/pods/demo/log"
	if rule.Matches(effect) {
		t.Fatal("Kubernetes non-resource rule matched a resource request")
	}
}

func TestKubernetesNonResourcePathSetIsCanonicalAndClosed(t *testing.T) {
	valid := []string{"/api", "/apis", "/api/v1", "/apis/apps/v1", "/healthz", "/openapi/v3"}
	for _, path := range valid {
		if !validKubernetesNonResourcePath(path) {
			t.Fatalf("valid Kubernetes non-resource path rejected: %q", path)
		}
	}
	invalid := []string{"/apis/apps", "//healthz", "/api//", "/healthz/", "/version\u2028", "/api/%76%31"}
	for _, path := range invalid {
		if validKubernetesNonResourcePath(path) {
			t.Fatalf("invalid Kubernetes non-resource path accepted: %q", path)
		}
	}
}

func TestKubernetesProjectionRejectsInconsistentVerbShapeAndTransport(t *testing.T) {
	for _, mutate := range []func(*PolicyProtocolIdentity){
		func(value *PolicyProtocolIdentity) { value.KubernetesKind = "future" },
		func(value *PolicyProtocolIdentity) { value.KubernetesVerb = "list" },
		func(value *PolicyProtocolIdentity) { value.KubernetesSubresource = "exec" },
		func(value *PolicyProtocolIdentity) { value.KubernetesDryRun = "all" },
	} {
		identity := kubernetesResourceIdentity()
		mutate(&identity)
		if err := identity.Validate(); err == nil {
			t.Fatalf("inconsistent Kubernetes projection accepted: %+v", identity)
		}
	}
}

func TestSemanticKubernetesPolicyRejectsCombinedShadowAndUnreachableAuthority(t *testing.T) {
	allow := kubernetesResourceRule("cluster.us-east-1.eks.amazonaws.com")
	allow.Host, allow.Hosts = "", []string{"cluster.us-east-1.eks.amazonaws.com", "cluster.us-west-2.eks.amazonaws.com"}
	first := kubernetesResourceRule("cluster.us-east-1.eks.amazonaws.com")
	second := kubernetesResourceRule("cluster.us-west-2.eks.amazonaws.com")
	policy := SemanticKubernetesPolicy{Allow: SemanticKubernetesRuleSet{Rules: []SemanticKubernetesRule{allow}}, Deny: SemanticKubernetesRuleSet{Rules: []SemanticKubernetesRule{first, second}}}
	if err := policy.Validate(); err == nil {
		t.Fatal("Kubernetes Allow covered by combined Deny rules was accepted")
	}
	first.Host = "kubernetes.vendor.dev"
	if err := first.Validate(); err == nil {
		t.Fatal("Kubernetes static authority outside EKS classifier was accepted")
	}
}

func TestSemanticKubernetesRuleRequiresExactlyOneVariant(t *testing.T) {
	base := kubernetesResourceRule("cluster.us-east-1.eks.amazonaws.com")
	both := base
	both.NonResource = &SemanticKubernetesNonResourceRule{Path: "/version", Verb: "get"}
	if err := both.Validate(); err == nil {
		t.Fatal("Kubernetes rule with both variants was accepted")
	}
	base.Resource = nil
	if err := base.Validate(); err == nil {
		t.Fatal("Kubernetes rule without a variant was accepted")
	}
}
