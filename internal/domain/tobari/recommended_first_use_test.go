package tobari

import (
	"reflect"
	"testing"
)

func TestRecommendedFirstUseDraftOwnsDisplayedAndCreatedSettings(t *testing.T) {
	session, err := NewWorkspaceDirectSession([]string{"claude", "--flag", ""})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := NewRecommendedFirstUseDraft("/workspace/example", session)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ContextName != DefaultContextName || draft.Access.SourceAccess != ContextSourceAccessReadWrite ||
		draft.Access.RoutineTraffic != ContextRoutineTrafficReady ||
		draft.Access.MethodPolicy.Default != ContextMethodExactReview ||
		draft.Access.PrivateTargets != ContextMethodDeny || draft.RuntimeSelection != "standard@1" ||
		draft.HostConfiguration != RecommendedHostConfigurationNotImported ||
		draft.Session != (RecommendedFirstUseSession{Kind: RecommendedFirstUseSessionDirect, Executable: "claude"}) {
		t.Fatalf("recommended draft = %+v", draft)
	}
	composition := draft.Composition()
	if composition.NativeReadiness != ContextNativeReadinessEnabled || composition.RuntimeSelection != "standard@1" ||
		composition.Bootstrap != nil || composition.MethodPolicy == nil ||
		!reflect.DeepEqual(*composition.MethodPolicy, draft.Access.MethodPolicy) {
		t.Fatalf("recommended composition = %+v", composition)
	}
}

func TestRecommendedFirstUseDraftRejectsPresentationInference(t *testing.T) {
	draft, err := NewRecommendedFirstUseDraft("/workspace/example", NewWorkspaceShellSession())
	if err != nil {
		t.Fatal(err)
	}
	draft.Access.RoutineTraffic = ContextRoutineTrafficLimited
	if err := draft.Validate(); err == nil {
		t.Fatal("mutated effective Access unexpectedly validated")
	}
}
