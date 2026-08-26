package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	resumablePermissionReviewFixtureSHA256 = "6ee16b544eae589b511ff3a23285ae0f6db20335f6b97ded381b05d9ce7c94d3"
	resumablePermissionReviewAnswerSHA256  = "d61aaea1ba6e57712b0abd139eaa0ab6bc0047c9ed2e71d8dcd0f7f415d43003"
)

type resumablePermissionReviewFixture struct {
	SchemaVersion     int                            `json:"schema_version"`
	Inbox             tobari.PolicyCandidateReport   `json:"inbox"`
	EmptyInbox        tobari.PolicyCandidateReport   `json:"empty_inbox"`
	RefreshedInbox    tobari.PolicyCandidateReport   `json:"refreshed_inbox"`
	DecisionSet       tobari.PolicyReviewDecisionSet `json:"decision_set"`
	ZeroStaged        tobari.PolicyReviewDecisionSet `json:"zero_staged"`
	Receipt           tobari.PolicyReviewChange      `json:"receipt"`
	ActivationFailure fault.Error                    `json:"activation_failure"`
	Navigation        struct {
		LearnableCommand         string `json:"learnable_command"`
		DiagnosticCommand        string `json:"diagnostic_command"`
		WorkspaceContainerBefore string `json:"workspace_container_before"`
		WorkspaceContainerAfter  string `json:"workspace_container_after"`
		OPAContainerBefore       string `json:"opa_container_before"`
		OPAContainerAfter        string `json:"opa_container_after"`
	} `json:"navigation"`
}

type resumablePermissionReviewAnswer struct {
	SchemaVersion  int `json:"schema_version"`
	RoutineSuccess struct {
		TaskInvocations         int `json:"task_invocations"`
		ExternalProcessingSteps int `json:"external_processing_steps"`
	} `json:"routine_success"`
	Cases []struct {
		Name                  string   `json:"name"`
		RequiredFacts         []string `json:"required_facts"`
		ExactNextArgv         []string `json:"exact_next_argv"`
		UnsupportedInferences []string `json:"unsupported_inferences"`
	} `json:"cases"`
}

func readPinnedResumablePermissionReviewCorpus(t *testing.T) (resumablePermissionReviewFixture, resumablePermissionReviewAnswer) {
	t.Helper()
	read := func(path, wantSHA string, target any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != wantSHA {
			t.Fatalf("%s SHA-256 = %s, want %s", path, got, wantSHA)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	var fixture resumablePermissionReviewFixture
	var answer resumablePermissionReviewAnswer
	read("testdata/resumable-permission-review-fixture.json", resumablePermissionReviewFixtureSHA256, &fixture)
	read("testdata/resumable-permission-review-answer-key.json", resumablePermissionReviewAnswerSHA256, &answer)
	return fixture, answer
}

func TestResumablePermissionReviewTypedCorpusClosesInterpretationBoundaries(t *testing.T) {
	t.Parallel()
	fixture, answer := readPinnedResumablePermissionReviewCorpus(t)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 || len(answer.Cases) != 10 {
		t.Fatalf("corpus shape fixture=%d answer=%d cases=%d", fixture.SchemaVersion, answer.SchemaVersion, len(answer.Cases))
	}
	if answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalProcessingSteps != 0 {
		t.Fatalf("routine-success processing = %+v", answer.RoutineSuccess)
	}
	wantCases := map[string]bool{
		"learnable_denial_navigation": false, "nonlearnable_denial_diagnostic": false,
		"mixed_final_review": false, "stale_refresh": false, "confirmed_receipt": false,
		"cancel": false, "same_session_retry": false, "empty_inbox": false,
		"zero_staged": false, "activation_failure": false,
	}
	for _, item := range answer.Cases {
		if _, known := wantCases[item.Name]; !known || wantCases[item.Name] {
			t.Fatalf("unknown or duplicate answer case %q", item.Name)
		}
		if len(item.RequiredFacts) == 0 || len(item.UnsupportedInferences) == 0 {
			t.Fatalf("answer case lacks evidence boundaries: %+v", item)
		}
		wantCases[item.Name] = true
	}
	for name, found := range wantCases {
		if !found {
			t.Fatalf("answer key lacks fixture boundary %q", name)
		}
	}
	for name, validate := range map[string]func() error{
		"inbox": fixture.Inbox.Validate, "empty inbox": fixture.EmptyInbox.Validate,
		"refreshed inbox": fixture.RefreshedInbox.Validate,
		"decision set":    fixture.DecisionSet.Validate, "receipt": fixture.Receipt.Validate,
		"activation failure": fixture.ActivationFailure.Validate,
	} {
		if err := validate(); err != nil {
			t.Fatalf("%s fixture: %v", name, err)
		}
	}
	if err := fixture.ZeroStaged.Validate(); err == nil {
		t.Fatal("zero staged fixture crossed the mutation boundary")
	}
	if len(fixture.EmptyInbox.Items) != 0 {
		t.Fatalf("empty inbox = %+v", fixture.EmptyInbox.Items)
	}
	if len(fixture.Inbox.Items) != 2 || fixture.Inbox.Items[0].WorkspaceManifestName != fixture.Inbox.Items[1].WorkspaceManifestName ||
		fixture.Inbox.Items[0].ProjectRoot != fixture.Inbox.Items[1].ProjectRoot || fixture.Inbox.Items[0].ID == fixture.Inbox.Items[1].ID {
		t.Fatalf("fixture does not carry matching labels with distinct IDs: %+v", fixture.Inbox.Items)
	}

	selector := &policyReviewSelector{}
	selector.Stage(fixture.DecisionSet.Decisions[0].ReviewItemID, policyReviewActionAllow)
	selector.Stage(fixture.DecisionSet.Decisions[1].ReviewItemID, policyReviewActionDeny)
	var review strings.Builder
	if err := writePolicyReviewFinalLines(&review, fixture.Inbox, selector.staged, selector.stagedOrder); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Allow exact", "Deny exact", fixture.Inbox.Items[0].ID, fixture.Inbox.Items[1].ID,
		"GraphQL mutation.updateIssue", "Staging grants no authority",
	} {
		if !strings.Contains(review.String(), want) {
			t.Fatalf("final review %q lacks %q", review.String(), want)
		}
	}
	if removed := selector.Reconcile(fixture.RefreshedInbox); removed != 1 {
		t.Fatalf("stale decisions removed = %d, want 1", removed)
	}
	remaining := selector.OrderedDecisions()
	if len(remaining) != 1 || remaining[0].CandidateID != fixture.Inbox.Items[1].ID || remaining[0].Action != policyReviewActionDeny {
		t.Fatalf("reconciled staged decisions = %+v", remaining)
	}

	receipt := string(renderPolicyReviewChange(fixture.Receipt, false))
	for _, want := range []string{
		fixture.Receipt.ActiveRevision, fixture.Receipt.Decisions[0].ReviewItemID,
		fixture.Receipt.Decisions[1].ReviewItemID, "1 Allow, 1 Deny", "current Workspace and agent session running",
	} {
		if !strings.Contains(receipt, want) {
			t.Fatalf("receipt %q lacks %q", receipt, want)
		}
	}
	if strings.Contains(receipt, "Re-enter") {
		t.Fatalf("receipt requires session replacement: %q", receipt)
	}
	if fixture.Navigation.LearnableCommand != "tobari review permissions" || fixture.Navigation.DiagnosticCommand != "tobari cluster denials" ||
		fixture.Navigation.WorkspaceContainerBefore != fixture.Navigation.WorkspaceContainerAfter ||
		fixture.Navigation.OPAContainerBefore != fixture.Navigation.OPAContainerAfter {
		t.Fatalf("navigation/session answer = %+v", fixture.Navigation)
	}
	raw, err := os.ReadFile("testdata/resumable-permission-review-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"authorization", "cookie", "access_token", "refresh_token", "client_secret",
		"header", "query", "body", "credential", "handle",
	} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("typed fixture contains secret-bearing field %q", forbidden)
		}
	}
}
