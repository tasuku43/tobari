package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

const (
	truthfulAuthStateFixtureSHA256 = "418b3a7c136cce2d2ae308e2867ae2772ccfd6d02e7056046a6badc6270a0bfa"
	truthfulAuthStateAnswerSHA256  = "6434c5947a3c5ca9b2aa3d1de86c0c548c215e080f4d63ffcc1b6d2ab9fd7cc3"
)

type truthfulAuthStateFixture struct {
	SchemaVersion int `json:"schema_version"`
	StatusCases   []struct {
		Name string               `json:"name"`
		Auth authStatusProjection `json:"auth"`
	} `json:"status_cases"`
	MutationCases []struct {
		Name string               `json:"name"`
		Auth authResultProjection `json:"auth"`
	} `json:"mutation_cases"`
}

type truthfulAuthStateAnswer struct {
	SchemaVersion int `json:"schema_version"`
	Cases         []struct {
		Name                    string                                 `json:"name"`
		AggregateState          authbroker.WorkspaceActivationState    `json:"aggregate_state"`
		Coverage                authbroker.WorkspaceActivationCoverage `json:"coverage"`
		ConfiguredProviders     int                                    `json:"configured_providers"`
		ExactActions            int                                    `json:"exact_actions"`
		ExternalProcessingCount int                                    `json:"external_processing_count"`
	} `json:"cases"`
	NegativeInferenceCanaries []string `json:"negative_inference_canaries"`
}

type truthfulAuthActualCase struct {
	activation          authbroker.WorkspaceActivation
	configuredProviders int
}

func readPinnedTruthfulAuthStateCorpus(t *testing.T) (truthfulAuthStateFixture, truthfulAuthStateAnswer) {
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
	var fixture truthfulAuthStateFixture
	var answer truthfulAuthStateAnswer
	read("testdata/truthful-auth-state-fixture.json", truthfulAuthStateFixtureSHA256, &fixture)
	read("testdata/truthful-auth-state-answer-key.json", truthfulAuthStateAnswerSHA256, &answer)
	return fixture, answer
}

func TestTruthfulAuthStateTypedCorpusClosesInterpretationBoundaries(t *testing.T) {
	t.Parallel()
	fixture, answer := readPinnedTruthfulAuthStateCorpus(t)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 || len(answer.Cases) != 10 {
		t.Fatalf("corpus shape fixture=%d answer=%d cases=%d", fixture.SchemaVersion, answer.SchemaVersion, len(answer.Cases))
	}
	if len(fixture.StatusCases)+len(fixture.MutationCases) != len(answer.Cases) {
		t.Fatalf("fixture cases=%d answer cases=%d", len(fixture.StatusCases)+len(fixture.MutationCases), len(answer.Cases))
	}

	actual := make(map[string]truthfulAuthActualCase, len(answer.Cases))
	for _, item := range fixture.StatusCases {
		providers := make([]authbroker.ProviderStatus, 0, len(item.Auth.Providers))
		configured := 0
		for _, provider := range item.Auth.Providers {
			revision := ""
			if provider.CredentialRevision != nil {
				revision = *provider.CredentialRevision
			}
			providers = append(providers, authbroker.ProviderStatus{
				Provider: provider.Provider, State: provider.State,
				AccountLabel: provider.AccountLabel, CredentialRevision: revision,
			})
			if provider.State == authbroker.ProviderCredentialConfigured {
				configured++
			}
		}
		contextID := ""
		if item.Auth.ContextID != nil {
			contextID = *item.Auth.ContextID
		}
		result := authbroker.StatusResult{
			Task: authbroker.TaskStatus, ContextState: item.Auth.ContextState,
			Context: item.Auth.Context, ContextID: contextID,
			StorageBackend: item.Auth.StorageBackend, BrokerState: item.Auth.BrokerState,
			Providers: providers, WorkspaceActivation: item.Auth.WorkspaceActivation,
		}
		if err := result.Validate(); err != nil {
			t.Fatalf("status case %q: %v", item.Name, err)
		}
		addTruthfulAuthActualCase(t, actual, item.Name, truthfulAuthActualCase{activation: item.Auth.WorkspaceActivation, configuredProviders: configured})
	}
	for _, item := range fixture.MutationCases {
		contextID, revision := "", ""
		if item.Auth.ContextID != nil {
			contextID = *item.Auth.ContextID
		}
		if item.Auth.CredentialRevision != nil {
			revision = *item.Auth.CredentialRevision
		}
		task := authbroker.TaskLogout
		if item.Name == "login_changed" {
			task = authbroker.TaskLogin
		}
		result := authbroker.Result{
			Task: task, ContextState: item.Auth.ContextState, Provider: item.Auth.Provider,
			Context: item.Auth.Context, ContextID: contextID, Configured: item.Auth.Configured,
			AccountLabel: item.Auth.AccountLabel, StorageBackend: item.Auth.StorageBackend,
			BrokerState: item.Auth.BrokerState, CredentialRevision: revision, Change: item.Auth.Change,
			WorkspaceActivation: item.Auth.WorkspaceActivation,
		}
		if err := result.Validate(); err != nil {
			t.Fatalf("mutation case %q: %v", item.Name, err)
		}
		configured := 0
		if item.Auth.Configured {
			configured = 1
		}
		addTruthfulAuthActualCase(t, actual, item.Name, truthfulAuthActualCase{activation: item.Auth.WorkspaceActivation, configuredProviders: configured})
	}

	seenAnswers := make(map[string]struct{}, len(answer.Cases))
	for _, expected := range answer.Cases {
		if _, duplicate := seenAnswers[expected.Name]; duplicate {
			t.Fatalf("duplicate answer case %q", expected.Name)
		}
		seenAnswers[expected.Name] = struct{}{}
		observed, found := actual[expected.Name]
		if !found {
			t.Fatalf("answer case %q lacks a fixture", expected.Name)
		}
		if observed.activation.State != expected.AggregateState || observed.activation.Coverage != expected.Coverage ||
			observed.configuredProviders != expected.ConfiguredProviders {
			t.Fatalf("case %q facts = state:%s coverage:%s configured:%d", expected.Name,
				observed.activation.State, observed.activation.Coverage, observed.configuredProviders)
		}
		actions := 0
		for _, workspace := range observed.activation.Workspaces {
			if workspace.NextAction == nil {
				continue
			}
			actions++
			if workspace.NextAction.WorkingDirectory != workspace.Root || len(workspace.NextAction.Argv) != 3 ||
				workspace.NextAction.Argv[0] != "tobari" || workspace.NextAction.Argv[1] != "--context" ||
				workspace.NextAction.Argv[2] != workspace.Context {
				t.Fatalf("case %q action is not bound to exact root and Context: %+v", expected.Name, workspace.NextAction)
			}
		}
		if actions != expected.ExactActions || expected.ExternalProcessingCount != 0 {
			t.Fatalf("case %q actions=%d processing=%d", expected.Name, actions, expected.ExternalProcessingCount)
		}
	}
	if len(answer.NegativeInferenceCanaries) < 7 {
		t.Fatalf("negative inference canaries = %d", len(answer.NegativeInferenceCanaries))
	}

	raw, err := os.ReadFile("testdata/truthful-auth-state-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"authorization", "cookie", "access_token", "refresh_token", "client_secret", "primary_secret", "brokered_handle"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("typed fixture contains secret-bearing field %q", forbidden)
		}
	}
}

func addTruthfulAuthActualCase(t *testing.T, cases map[string]truthfulAuthActualCase, name string, value truthfulAuthActualCase) {
	t.Helper()
	if _, duplicate := cases[name]; duplicate {
		t.Fatalf("duplicate fixture case %q", name)
	}
	cases[name] = value
}
