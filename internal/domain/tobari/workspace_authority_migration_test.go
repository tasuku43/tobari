package tobari

import (
	"reflect"
	"testing"
	"time"
)

func predecessorRevisionFixture(generation uint64, legacy, policy string) PredecessorTemplateRevision {
	return PredecessorTemplateRevision{
		Generation: generation, Revision: authorityDigest(legacy), Body: predecessorTemplateBody(templateBodyFixture(policy)),
	}
}

func migrationCandidateEffectFixture() PolicyCandidateEffect {
	return PolicyCandidateEffect{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/candidate",
		Segments: []string{}, Examples: []string{"/candidate"},
	}
}

func workspaceAuthorityMigrationFixture() WorkspaceAuthorityMigrationInput {
	manifestID := string(testTemplateAuthorityID)
	workspaceID := string(testWorkspaceAuthorityID)
	defaultID := manifestID
	revision := predecessorRevisionFixture(1, "1", "b")
	templateRevision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, revision.Generation, revision.TemplateBody())
	if err != nil {
		panic(err)
	}
	candidateEffect := migrationCandidateEffectFixture()
	candidatePayload, err := policyCandidateEffectDigest(candidateEffect)
	if err != nil {
		panic(err)
	}
	return WorkspaceAuthorityMigrationInput{
		Source: WorkspaceAuthorityMigrationSource, SourceDigest: authorityDigest("0"), PredecessorComplete: true,
		ClusterStopped: true, LiveAttachments: 0,
		Templates: []PredecessorTemplate{{ID: manifestID, Name: "restricted", CurrentGeneration: 1, CurrentRevision: revision.Revision, Revisions: []PredecessorTemplateRevision{revision}}},
		Workspaces: []PredecessorWorkspace{{
			ID: workspaceID, ProjectRoot: "/workspace/example", ManifestID: manifestID, Home: "/workspace/home",
			HomeDigest: authorityDigest("8"), CreationDefaults: templateRevision.Slices.CreationDefaultsDigest,
			LastSuccessfulEntry: &PredecessorWorkspaceAppliedEntry{
				ManifestGeneration: 1, ManifestRevision: revision.Revision, RuntimeID: templateRevision.Slices.RuntimeID,
				RuntimeRevision: templateRevision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(1, 0).UTC(),
			},
			DockerObservation: PredecessorWorkspaceDockerObservation{
				State: PredecessorDockerObservationExactOwned, WorkspaceID: workspaceID,
				ManifestGeneration: 1, ManifestRevision: revision.Revision, RuntimeID: templateRevision.Slices.RuntimeID,
				RuntimeRevision: templateRevision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"),
			},
		}},
		ContextAssignments: []ContextIDAssignment{{ProjectRoot: "/workspace/example", PredecessorManifestID: manifestID, ContextID: testContextAuthorityID}},
		PolicySets: []PredecessorPolicySet{{
			ManifestID: manifestID, WorkspaceID: workspaceID, ProjectRoot: "/workspace/example",
			Rules: []PredecessorPolicyRule{{ID: "plr_0123456789abcdef0123456789abcdef", Decision: PolicyMemoryAllow, Body: policyMemoryBodyFixture("/items/1")}},
		}},
		PendingCandidates: []PredecessorPendingCandidate{{
			ID: "pcy_abcdef0123456789abcdef0123456789", ManifestID: manifestID, WorkspaceID: workspaceID,
			ProjectRoot: "/workspace/example", PayloadDigest: candidatePayload, Effect: candidateEffect,
		}},
		DefaultManifestID: &defaultID,
		ResearchAuthority: PredecessorResearchAuthority{Present: true, Complete: true, Platform: ResearchAuthorityMacOS, SourceDigest: authorityDigest("9")},
	}
}

func cloneWorkspaceAuthorityMigrationInput(input WorkspaceAuthorityMigrationInput) WorkspaceAuthorityMigrationInput {
	result := input
	result.Templates = append([]PredecessorTemplate{}, input.Templates...)
	for index := range result.Templates {
		result.Templates[index].Revisions = make([]PredecessorTemplateRevision, len(input.Templates[index].Revisions))
		for revisionIndex, revision := range input.Templates[index].Revisions {
			result.Templates[index].Revisions[revisionIndex] = revision
			result.Templates[index].Revisions[revisionIndex].Body = predecessorTemplateBody(revision.TemplateBody())
		}
	}
	result.Workspaces = append([]PredecessorWorkspace{}, input.Workspaces...)
	for index := range result.Workspaces {
		if input.Workspaces[index].LastSuccessfulEntry != nil {
			entry := *input.Workspaces[index].LastSuccessfulEntry
			result.Workspaces[index].LastSuccessfulEntry = &entry
		}
	}
	result.ContextAssignments = append([]ContextIDAssignment{}, input.ContextAssignments...)
	result.PolicySets = append([]PredecessorPolicySet{}, input.PolicySets...)
	for index := range result.PolicySets {
		result.PolicySets[index].Rules = append([]PredecessorPolicyRule{}, input.PolicySets[index].Rules...)
		for ruleIndex := range result.PolicySets[index].Rules {
			result.PolicySets[index].Rules[ruleIndex].Body = input.PolicySets[index].Rules[ruleIndex].Body.Clone()
		}
	}
	result.PendingCandidates = append([]PredecessorPendingCandidate{}, input.PendingCandidates...)
	for index := range result.PendingCandidates {
		result.PendingCandidates[index].Effect = input.PendingCandidates[index].Effect.Clone()
	}
	if input.DefaultManifestID != nil {
		value := *input.DefaultManifestID
		result.DefaultManifestID = &value
	}
	return result
}

func TestWorkspaceAuthorityMigrationPlanPreservesIdentityAndSeparatesAuthority(t *testing.T) {
	input := workspaceAuthorityMigrationFixture()
	before := cloneWorkspaceAuthorityMigrationInput(input)
	plan, err := BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("pure migration planning mutated its predecessor input")
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(plan.Templates) != 1 || string(plan.Templates[0].ID) != input.Templates[0].ID || plan.Templates[0].Current.Generation != 1 {
		t.Fatalf("Template identity was not byte-preserved: %#v", plan.Templates)
	}
	if plan.Templates[0].Current.Revision == input.Templates[0].CurrentRevision {
		t.Fatal("Template semantic revision was not recomputed for the new static body")
	}
	if len(plan.Contexts) != 1 || plan.Contexts[0].ID != testContextAuthorityID || plan.Contexts[0].TemplateID != testTemplateAuthorityID {
		t.Fatalf("fresh Context mapping is invalid: %#v", plan.Contexts)
	}
	if len(plan.PolicyMemories) != 1 || plan.PolicyMemories[0].ContextID != testContextAuthorityID || len(plan.PolicyMemories[0].Rules) != 1 || plan.PolicyMemories[0].Rules[0].ID == input.PolicySets[0].Rules[0].ID {
		t.Fatalf("Policy Memory mapping is invalid: %#v", plan.PolicyMemories)
	}
	if len(plan.Workspaces) != 1 || string(plan.Workspaces[0].Binding.ID) != input.Workspaces[0].ID || plan.Workspaces[0].Binding.ContextID != testContextAuthorityID || plan.Workspaces[0].Adoption != WorkspaceMigrationCurrent || plan.Workspaces[0].PreservedHomeDigest != input.Workspaces[0].HomeDigest {
		t.Fatalf("Workspace migration is invalid: %#v", plan.Workspaces)
	}
	if len(plan.PendingCandidates) != 1 || plan.PendingCandidates[0].PredecessorID != input.PendingCandidates[0].ID || plan.PendingCandidates[0].ID == input.PendingCandidates[0].ID || plan.PendingCandidates[0].ContextID != testContextAuthorityID || plan.PendingCandidates[0].ObservingWorkspaceID != testWorkspaceAuthorityID {
		t.Fatalf("pending candidate migration is invalid: %#v", plan.PendingCandidates)
	}
	if plan.DefaultTemplateID == nil || *plan.DefaultTemplateID != testTemplateAuthorityID {
		t.Fatalf("default Template mapping = %#v", plan.DefaultTemplateID)
	}
	if plan.ResearchAuthDisposition != ResearchAuthReauthenticationRequired || !plan.ResearchQuarantine.Required || !plan.ResearchQuarantine.LeaveKeychainUntouched || plan.ResearchQuarantine.MoveFilesystemRootKey {
		t.Fatalf("macOS research quarantine = %#v", plan.ResearchQuarantine)
	}

	again, err := BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil || !reflect.DeepEqual(plan, again) {
		t.Fatalf("journaled Context mapping did not produce an idempotent plan: %v", err)
	}
	clone := plan.Clone()
	clone.Templates[0].Retained[0].Generation = 99
	clone.PolicyMemories[0].Rules[0].Body.Examples[0] = "/changed"
	clone.Workspaces[0].Binding.LastSuccessfulEntry.ResolvedSpec = authorityDigest("5")
	clone.PendingCandidates[0].Effect.Examples[0] = "/changed"
	if plan.Templates[0].Retained[0].Generation != 1 || plan.PolicyMemories[0].Rules[0].Body.Examples[0] == "/changed" || plan.Workspaces[0].Binding.LastSuccessfulEntry.ResolvedSpec == authorityDigest("5") || plan.PendingCandidates[0].Effect.Examples[0] == "/changed" {
		t.Fatal("migration plan clone shares authority storage")
	}

	tampered := plan.Clone()
	tampered.Workspaces[0].PreservedHomeDigest = authorityDigest("4")
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered complete migration plan passed its digest")
	}
}

func TestMigratedPendingCandidateIsCompleteForExactDecision(t *testing.T) {
	plan, err := BuildWorkspaceAuthorityMigrationPlan(workspaceAuthorityMigrationFixture())
	if err != nil {
		t.Fatal(err)
	}
	migrated := plan.PendingCandidates[0]
	candidate, err := migrated.Authority()
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range []PolicyMemoryDecision{PolicyMemoryAllow, PolicyMemoryDeny} {
		t.Run(string(decision), func(t *testing.T) {
			previous := plan.PolicyMemories[0].Clone()
			rule, err := NewPolicyMemoryRule(candidate.ContextID, decision, candidate.Effect.RuleBody(candidate.ID))
			if err != nil {
				t.Fatal(err)
			}
			rules := append([]PolicyMemoryRule{}, previous.Rules...)
			rules = append(rules, rule)
			current, changed, err := PublishPolicyMemory(candidate.ContextID, rules, &previous)
			if err != nil || !changed {
				t.Fatalf("publish migrated decision: changed=%t err=%v", changed, err)
			}
			template := plan.Templates[0].Clone()
			context := plan.Contexts[0]
			workspace := plan.Workspaces[0].Binding
			templateReceipt := TemplatePolicyActivationReceipt{ContextID: context.ID, TemplateID: template.ID, PolicySliceDigest: template.Current.Slices.PolicySliceDigest}
			memoryReceipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: current.Revision}
			snapshot := ContextAuthoritySnapshot{
				Context: context, Template: template, PolicyMemory: current, ActiveTemplatePolicy: &templateReceipt,
				ActivePolicyMemory: ptrPolicyMemory(current), ActivePolicyMemoryRef: &memoryReceipt, Workspace: &workspace,
			}
			publication := PolicyCandidatePublication{
				Candidate: candidate, RuleID: rule.ID, Previous: previous,
				Memory: PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true},
			}
			if err := publication.ValidateFor(candidate.ID, decision); err != nil {
				t.Fatalf("migrated candidate could not complete exact %s: %v", decision, err)
			}
		})
	}
}

func ptrPolicyMemory(memory PolicyMemoryRevision) *PolicyMemoryRevision {
	clone := memory.Clone()
	return &clone
}

func TestWorkspaceAuthorityMigrationMapsRetainedCurrentAndPendingEntry(t *testing.T) {
	input := workspaceAuthorityMigrationFixture()
	first := input.Templates[0].Revisions[0]
	second := predecessorRevisionFixture(2, "2", "2")
	input.Templates[0].Revisions = append(input.Templates[0].Revisions, second)
	input.Templates[0].CurrentGeneration = second.Generation
	input.Templates[0].CurrentRevision = second.Revision
	plan, err := BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Templates[0].Retained) != 2 || plan.Templates[0].Current.Generation != 2 || plan.Workspaces[0].Adoption != WorkspaceMigrationPending || plan.Workspaces[0].Binding.LastSuccessfulEntry.TemplateRevision != plan.Templates[0].Retained[0].Revision {
		t.Fatalf("retained/current mapping = %#v / %#v; first=%#v", plan.Templates[0], plan.Workspaces[0], first)
	}

	input.Workspaces[0].LastSuccessfulEntry = nil
	input.Workspaces[0].DockerObservation = PredecessorWorkspaceDockerObservation{State: PredecessorDockerObservationUnknown, WorkspaceID: input.Workspaces[0].ID}
	plan, err = BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil || plan.Workspaces[0].Adoption != WorkspaceMigrationUnverified || plan.Workspaces[0].Binding.LastSuccessfulEntry != nil {
		t.Fatalf("unverified Workspace mapping = %#v, %v", plan.Workspaces[0], err)
	}
}

func TestWorkspaceAuthorityMigrationRequiresExactOwnedDockerEvidenceForAppliedEntry(t *testing.T) {
	for _, state := range []PredecessorDockerObservationState{
		PredecessorDockerObservationMissing,
		PredecessorDockerObservationMismatched,
		PredecessorDockerObservationUnknown,
	} {
		t.Run(string(state), func(t *testing.T) {
			input := workspaceAuthorityMigrationFixture()
			input.Workspaces[0].DockerObservation = PredecessorWorkspaceDockerObservation{State: state, WorkspaceID: input.Workspaces[0].ID}
			plan, err := BuildWorkspaceAuthorityMigrationPlan(input)
			if err != nil {
				t.Fatal(err)
			}
			workspace := plan.Workspaces[0]
			if workspace.Adoption != WorkspaceMigrationUnverified || workspace.Binding.LastSuccessfulEntry != nil {
				t.Fatalf("%s evidence retained AppliedEntry: %#v", state, workspace)
			}
		})
	}

	for name, mutate := range map[string]func(*PredecessorWorkspaceDockerObservation){
		"wrong Workspace": func(value *PredecessorWorkspaceDockerObservation) {
			value.WorkspaceID = "01912345-6789-7abc-8def-0123456789ff"
		},
		"wrong Runtime revision": func(value *PredecessorWorkspaceDockerObservation) { value.RuntimeRevision = authorityDigest("4") },
		"wrong resolved spec":    func(value *PredecessorWorkspaceDockerObservation) { value.ResolvedSpec = authorityDigest("4") },
	} {
		t.Run(name, func(t *testing.T) {
			input := workspaceAuthorityMigrationFixture()
			mutate(&input.Workspaces[0].DockerObservation)
			if _, err := BuildWorkspaceAuthorityMigrationPlan(input); err == nil {
				t.Fatal("mismatched exact-owned evidence retained authority")
			}
		})
	}
}

func TestWorkspaceAuthorityMigrationLinuxQuarantinesFilesystemRootKey(t *testing.T) {
	input := workspaceAuthorityMigrationFixture()
	input.ResearchAuthority.Platform = ResearchAuthorityLinux
	plan, err := BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ResearchQuarantine.MoveFilesystemRootKey || plan.ResearchQuarantine.LeaveKeychainUntouched {
		t.Fatalf("Linux research quarantine = %#v", plan.ResearchQuarantine)
	}

	input.ResearchAuthority = PredecessorResearchAuthority{}
	plan, err = BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil || plan.ResearchAuthDisposition != ResearchAuthNotPresent || plan.ResearchQuarantine.Required {
		t.Fatalf("absent research authority = %#v, %v", plan.ResearchQuarantine, err)
	}
}

func TestWorkspaceAuthorityMigrationTransformsOnlyExactAdvancedSourcePair(t *testing.T) {
	input := workspaceAuthorityMigrationFixture()
	revision := &input.Templates[0].Revisions[0]
	revision.Body.Policy.Mode = ManifestPolicyModeAdvanced
	revision.Body.Policy.AdvancedPolicy = &WorkspaceTemplateAdvancedPolicySources{
		Tobari: "package tobari_template\nallow := false\n", TobariTest: "package tobari_template\ntest_deny := true\n",
	}
	plan, err := BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Templates[0].Current.Body.Policy.AdvancedPolicy
	if got == nil || got.Tobari != revision.Body.Policy.AdvancedPolicy.Tobari || got.TobariTest != revision.Body.Policy.AdvancedPolicy.TobariTest {
		t.Fatalf("Advanced predecessor pair was not transformed exactly: %#v", got)
	}

	missing := workspaceAuthorityMigrationFixture()
	missingRevision := &missing.Templates[0].Revisions[0]
	missingRevision.Body.Policy.Mode = ManifestPolicyModeAdvanced
	missingRevision.Body.Policy.AdvancedPolicy = &WorkspaceTemplateAdvancedPolicySources{Tobari: "package tobari_template"}
	if _, err := BuildWorkspaceAuthorityMigrationPlan(missing); err == nil {
		t.Fatal("incomplete predecessor Advanced source pair migrated")
	}
}

func TestWorkspaceAuthorityMigrationFailsClosedOnOrdinaryInvalidSources(t *testing.T) {
	tests := map[string]func(*WorkspaceAuthorityMigrationInput){
		"wrong source":                    func(input *WorkspaceAuthorityMigrationInput) { input.Source = "other" },
		"incomplete predecessor":          func(input *WorkspaceAuthorityMigrationInput) { input.PredecessorComplete = false },
		"final authority already present": func(input *WorkspaceAuthorityMigrationInput) { input.FinalAuthorityPresent = true },
		"running cluster":                 func(input *WorkspaceAuthorityMigrationInput) { input.ClusterStopped = false },
		"live attachment":                 func(input *WorkspaceAuthorityMigrationInput) { input.LiveAttachments = 1 },
		"nil Template collection":         func(input *WorkspaceAuthorityMigrationInput) { input.Templates = nil },
		"missing Context assignment":      func(input *WorkspaceAuthorityMigrationInput) { input.ContextAssignments = []ContextIDAssignment{} },
		"Context collides with Template": func(input *WorkspaceAuthorityMigrationInput) {
			input.ContextAssignments[0].ContextID = ContextID(input.Templates[0].ID)
		},
		"Context collides with Workspace": func(input *WorkspaceAuthorityMigrationInput) {
			input.ContextAssignments[0].ContextID = ContextID(input.Workspaces[0].ID)
		},
		"default is unknown": func(input *WorkspaceAuthorityMigrationInput) {
			value := "01912345-6789-7abc-8def-0123456789ff"
			input.DefaultManifestID = &value
		},
		"policy crosses Workspace": func(input *WorkspaceAuthorityMigrationInput) { input.PolicySets[0].ProjectRoot = "/workspace/other" },
		"candidate crosses Workspace": func(input *WorkspaceAuthorityMigrationInput) {
			input.PendingCandidates[0].ManifestID = "01912345-6789-7abc-8def-0123456789ff"
		},
		"candidate payload and effect mismatch": func(input *WorkspaceAuthorityMigrationInput) {
			input.PendingCandidates[0].Effect.Path = "/other"
			input.PendingCandidates[0].Effect.Examples[0] = "/other"
		},
		"incomplete research authority": func(input *WorkspaceAuthorityMigrationInput) { input.ResearchAuthority.Complete = false },
		"AppliedEntry Runtime drift": func(input *WorkspaceAuthorityMigrationInput) {
			input.Workspaces[0].LastSuccessfulEntry.RuntimeRevision = authorityDigest("4")
		},
		"Workspace creation receipt drift": func(input *WorkspaceAuthorityMigrationInput) {
			input.Workspaces[0].CreationDefaults = authorityDigest("4")
		},
		"retained Boundary change": func(input *WorkspaceAuthorityMigrationInput) {
			second := predecessorRevisionFixture(2, "2", "2")
			second.Body.Boundary.SourceAccess = ManifestSourceAccessReadWrite
			input.Templates[0].Revisions = append(input.Templates[0].Revisions, second)
			input.Templates[0].CurrentGeneration, input.Templates[0].CurrentRevision = second.Generation, second.Revision
		},
		"current revision missing": func(input *WorkspaceAuthorityMigrationInput) {
			input.Templates[0].CurrentRevision = authorityDigest("4")
		},
		"adjacent semantic no-op": func(input *WorkspaceAuthorityMigrationInput) {
			second := predecessorRevisionFixture(2, "1", "2")
			input.Templates[0].Revisions = append(input.Templates[0].Revisions, second)
			input.Templates[0].CurrentGeneration, input.Templates[0].CurrentRevision = second.Generation, second.Revision
		},
		"same digest with another immutable body": func(input *WorkspaceAuthorityMigrationInput) {
			second := predecessorRevisionFixture(2, "2", "2")
			third := predecessorRevisionFixture(3, "1", "3")
			input.Templates[0].Revisions = append(input.Templates[0].Revisions, second, third)
			input.Templates[0].CurrentGeneration, input.Templates[0].CurrentRevision = third.Generation, third.Revision
		},
		"rule decision and ID disagree": func(input *WorkspaceAuthorityMigrationInput) {
			input.PolicySets[0].Rules[0].Decision = PolicyMemoryDeny
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := workspaceAuthorityMigrationFixture()
			mutate(&input)
			if _, err := BuildWorkspaceAuthorityMigrationPlan(input); err == nil {
				t.Fatal("invalid predecessor source produced a migration plan")
			}
		})
	}
}

func TestWorkspaceAuthorityRollbackEligibilityFailsClosed(t *testing.T) {
	valid := WorkspaceAuthorityRollbackObservation{
		JournalComplete: true, BackupComplete: true, JournalSourceDigest: authorityDigest("1"), BackupSourceDigest: authorityDigest("1"), FinalStateMatchesJournaledPlan: true,
	}
	if got := EvaluateWorkspaceAuthorityRollback(valid); !got.Eligible || got.Reason != WorkspaceAuthorityRollbackEligible {
		t.Fatalf("eligible rollback = %#v", got)
	}
	tests := map[string]struct {
		mutate func(*WorkspaceAuthorityRollbackObservation)
		want   WorkspaceAuthorityRollbackReason
	}{
		"incomplete journal":    {func(value *WorkspaceAuthorityRollbackObservation) { value.JournalComplete = false }, WorkspaceAuthorityRollbackIncomplete},
		"incomplete backup":     {func(value *WorkspaceAuthorityRollbackObservation) { value.BackupComplete = false }, WorkspaceAuthorityRollbackIncomplete},
		"digest mismatch":       {func(value *WorkspaceAuthorityRollbackObservation) { value.BackupSourceDigest = authorityDigest("2") }, WorkspaceAuthorityRollbackDigestMismatch},
		"final drift":           {func(value *WorkspaceAuthorityRollbackObservation) { value.FinalStateMatchesJournaledPlan = false }, WorkspaceAuthorityRollbackFinalDrift},
		"predecessor collision": {func(value *WorkspaceAuthorityRollbackObservation) { value.PredecessorCanonicalStatePresent = true }, WorkspaceAuthorityRollbackSourceCollision},
		"fresh auth collision":  {func(value *WorkspaceAuthorityRollbackObservation) { value.FreshCanonicalAuthStatePresent = true }, WorkspaceAuthorityRollbackAuthCollision},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observation := valid
			test.mutate(&observation)
			got := EvaluateWorkspaceAuthorityRollback(observation)
			if got.Eligible || got.Reason != test.want {
				t.Fatalf("rollback eligibility = %#v, want %s", got, test.want)
			}
		})
	}
}
