package tobari

import (
	"strings"
	"testing"
)

func awsQueryRule(host string) SemanticAWSRule {
	return SemanticAWSRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		WireProtocol:          AWSWireProtocolQuery, Service: "sts", ProtocolVersion: "2011-06-15", Operation: "GetCallerIdentity",
	}
}

func awsQueryEffect() SemanticRequestEffect {
	return SemanticRequestEffect{
		Scheme: "https", Host: "sts.us-east-1.amazonaws.com", Port: 443, Method: "POST", Path: "/",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolAWS, AWSWireProtocol: AWSWireProtocolQuery, AWSService: "sts", AWSProtocolVersion: "2011-06-15", AWSOperation: "GetCallerIdentity"},
	}
}

func TestSemanticAWSQueryAndJSONProjectionAreExact(t *testing.T) {
	query := awsQueryRule("sts.us-east-1.amazonaws.com")
	if !query.Matches(awsQueryEffect()) {
		t.Fatal("AWS Query rule did not match its complete projection")
	}
	jsonRule := SemanticAWSRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "dynamodb.us-east-1.amazonaws.com", Port: 443},
		WireProtocol:          AWSWireProtocolJSON, Service: "dynamodb", TargetNamespace: "DynamoDB_20120810", Operation: "GetItem",
	}
	jsonEffect := SemanticRequestEffect{
		Scheme: "https", Host: "dynamodb.us-east-1.amazonaws.com", Port: 443, Method: "POST", Path: "/",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolAWS, AWSWireProtocol: AWSWireProtocolJSON, AWSService: "dynamodb", AWSTargetNamespace: "DynamoDB_20120810", AWSOperation: "GetItem"},
	}
	if !jsonRule.Matches(jsonEffect) {
		t.Fatal("AWS JSON rule did not match namespace and operation")
	}
	jsonEffect.Identity.AWSTargetNamespace = "Other_20200101"
	if jsonRule.Matches(jsonEffect) {
		t.Fatal("AWS JSON rule ignored target namespace")
	}
}

func TestSemanticAWSJSONNamespaceSharesGatewayTargetBudget(t *testing.T) {
	namespace := strings.Repeat("N", 129)
	rule := SemanticAWSRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "dynamodb.us-east-1.amazonaws.com", Port: 443},
		WireProtocol:          AWSWireProtocolJSON, Service: "dynamodb", TargetNamespace: namespace, Operation: "GetItem",
	}
	if err := rule.Validate(); err != nil {
		t.Fatalf("129-byte namespace inside the 256-byte target budget rejected: %v", err)
	}
	tooLong := rule
	tooLong.TargetNamespace = strings.Repeat("N", 256-len(tooLong.Operation))
	if err := tooLong.Validate(); err == nil {
		t.Fatal("AWS JSON target beyond 256 bytes was accepted")
	}
}

func TestSemanticAWSOperationAllowsOnlyExactOrTerminalPrefix(t *testing.T) {
	effect := awsQueryEffect()
	prefix := awsQueryRule(effect.Host)
	prefix.Operation = "Get*"
	if err := prefix.Validate(); err != nil || !prefix.Matches(effect) {
		t.Fatalf("valid terminal prefix matcher failed: %v", err)
	}
	for _, operation := range []string{"*", "Get*Identity", "Get**", "Get Caller"} {
		rule := prefix
		rule.Operation = operation
		if err := rule.Validate(); err == nil {
			t.Fatalf("invalid AWS operation matcher %q was accepted", operation)
		}
	}
}

func TestSemanticAWSServiceXORServicesAndSetIdentity(t *testing.T) {
	base := awsQueryRule("sts.us-east-1.amazonaws.com")
	base.Service, base.Services = "", []string{"sts", "iam"}
	if err := base.Validate(); err != nil {
		t.Fatalf("AWS services rule rejected: %v", err)
	}
	for _, rule := range []SemanticAWSRule{
		func() SemanticAWSRule { value := base; value.Service = "sts"; return value }(),
		func() SemanticAWSRule { value := base; value.Services = []string{"sts"}; return value }(),
		func() SemanticAWSRule { value := base; value.Services = []string{"sts", "sts"}; return value }(),
	} {
		if err := rule.Validate(); err == nil {
			t.Fatalf("invalid service/services rule accepted: %+v", rule)
		}
	}
	reordered := base
	reordered.Services = []string{"iam", "sts"}
	if err := (SemanticAWSRuleSet{Rules: []SemanticAWSRule{base, reordered}}).Validate(); err == nil {
		t.Fatal("reordered duplicate AWS rules were accepted")
	}
}

func TestSemanticAWSPolicyRejectsCombinedHostServiceAndPrefixShadow(t *testing.T) {
	allow := awsQueryRule("sts.us-east-1.amazonaws.com")
	allow.Host, allow.Hosts = "", []string{"sts.us-east-1.amazonaws.com", "sts.us-west-2.amazonaws.com"}
	allow.Service, allow.Services = "", []string{"sts", "iam"}
	denies := make([]SemanticAWSRule, 0, 4)
	for _, host := range allow.Hosts {
		for _, service := range allow.Services {
			deny := awsQueryRule(host)
			deny.Service = service
			deny.Operation = "Get*"
			denies = append(denies, deny)
		}
	}
	policy := SemanticAWSPolicy{Allow: SemanticAWSRuleSet{Rules: []SemanticAWSRule{allow}}, Deny: SemanticAWSRuleSet{Rules: denies}}
	if err := policy.Validate(); err == nil {
		t.Fatal("AWS Allow covered by combined Deny rules was accepted")
	}
	policy.Deny.Rules = policy.Deny.Rules[:3]
	if err := policy.Validate(); err != nil {
		t.Fatalf("partially covered AWS Allow rejected: %v", err)
	}
}

func TestSemanticAWSEffectRejectsNonRPCTransport(t *testing.T) {
	for _, mutate := range []func(*SemanticRequestEffect){
		func(effect *SemanticRequestEffect) { effect.Method = "GET" },
		func(effect *SemanticRequestEffect) { effect.Path = "/operation" },
	} {
		effect := awsQueryEffect()
		mutate(&effect)
		if err := effect.Validate(); err == nil {
			t.Fatalf("invalid AWS transport accepted: %+v", effect)
		}
	}
}

func TestSemanticAWSRuleRejectsAuthorityOutsideClassifier(t *testing.T) {
	for _, mutate := range []func(*SemanticAWSRule){
		func(rule *SemanticAWSRule) { rule.Host = "aws.vendor.dev" },
		func(rule *SemanticAWSRule) { rule.Port = 8443 },
	} {
		rule := awsQueryRule("sts.us-east-1.amazonaws.com")
		mutate(&rule)
		if err := rule.Validate(); err == nil {
			t.Fatalf("unreachable AWS static authority accepted: %+v", rule.SemanticRuleAuthority)
		}
	}
}
