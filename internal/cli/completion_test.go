package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/completioncmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type completionCLIRuntimeFake struct {
	managed tobari.RuntimeManifest
}

func newCompletionCLIRuntimeFake() *completionCLIRuntimeFake {
	revisions := []tobari.RuntimeRevision{
		{Ordinal: 1, Revision: "sha256:" + strings.Repeat("a", 64), Image: "tobari-runtime-sre:one", ImageDigest: "sha256:" + strings.Repeat("c", 64), CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/sre/revisions/one/source"},
		{Ordinal: 2, Revision: "sha256:" + strings.Repeat("b", 64), Image: "tobari-runtime-sre:two", ImageDigest: "sha256:" + strings.Repeat("d", 64), CreatedAt: time.Unix(2, 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/sre/revisions/two/source"},
	}
	return &completionCLIRuntimeFake{managed: tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077", Name: "sre",
		Kind: tobari.RuntimeKindManaged, SourcePath: "/tmp/tobari/runtimes/sre/source", Revisions: revisions,
	}}
}

func (f *completionCLIRuntimeFake) ListContexts(context.Context) (tobari.ManifestListResult, error) {
	return tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
		Items: []tobari.ManifestSummary{},
	}, nil
}

func (f *completionCLIRuntimeFake) ListRuntimes(context.Context) (tobari.RuntimeListResult, error) {
	standard := tobari.RuntimeSummary{
		ID: tobari.StandardRuntimeID, RuntimeRef: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Ready: true, Head: 1, Revision: "sha256:" + strings.Repeat("9", 64),
	}
	standard.RevisionRef = tobari.RuntimeRevisionRef(standard.ID, standard.Revision)
	return tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: []tobari.RuntimeSummary{standard, tobari.RuntimeSummaryFrom(f.managed)}}, nil
}

func (f *completionCLIRuntimeFake) RuntimeHistory(_ context.Context, _ string) (tobari.RuntimeReport, error) {
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeHistory, Runtime: f.managed}, nil
}

func completionRecordValues(records []completionRecord) []string {
	values := make([]string, len(records))
	for index, record := range records {
		values[index] = record.kind + ":" + record.value
	}
	return values
}

func TestCompletionPlansCatalogCommandWords(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	tests := []struct {
		name    string
		current int
		words   []string
		want    []string
	}{
		{name: "root prefix", current: 2, words: []string{"tobari", "mani"}, want: []string{"candidate:manifest"}},
		{name: "root choices", current: 2, words: []string{"tobari", "c"}, want: []string{"candidate:cluster", "candidate:completion", "candidate:config"}},
		{name: "nested prefix", current: 3, words: []string{"tobari", "manifest", "r"}, want: []string{"candidate:runtime"}},
		{name: "help selector", current: 4, words: []string{"tobari", "help", "runtime", "b"}, want: []string{"candidate:build"}},
		{name: "command flags", current: 4, words: []string{"tobari", "manifest", "show", "--"}, want: []string{"candidate:--name", "candidate:--details", "candidate:--format"}},
		{name: "allowed value", current: 5, words: []string{"tobari", "manifest", "show", "--format", "j"}, want: []string{"candidate:json"}},
		{name: "inline allowed value", current: 4, words: []string{"tobari", "manifest", "show", "--format=j"}, want: []string{"candidate:--format=json"}},
		{name: "global value", current: 3, words: []string{"tobari", "--error-format", "j"}, want: []string{"candidate:json"}},
		{name: "directory", current: 4, words: []string{"tobari", "doctor", "--root", ""}, want: []string{"directive:directories"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records, err := command.planCompletion(context.Background(), test.current, test.words)
			if err != nil {
				t.Fatal(err)
			}
			if got := completionRecordValues(records); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("records = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompletionPlansDynamicCandidatesFromTypedService(t *testing.T) {
	fake := newCompletionCLIRuntimeFake()
	command := &CLI{catalog: DefaultCatalog(), completion: completioncmd.New(fake)}
	tests := []struct {
		name    string
		current int
		words   []string
		want    []string
	}{
		{name: "root context", current: 3, words: []string{"tobari", "--manifest", "d"}, want: []string{}},
		{name: "command context", current: 5, words: []string{"tobari", "manifest", "show", "--name", "d"}, want: []string{}},
		{name: "runtime name", current: 5, words: []string{"tobari", "runtime", "show", "--name", "s"}, want: []string{"candidate:standard", "candidate:sre"}},
		{name: "opaque runtime", current: 5, words: []string{"tobari", "runtime", "build", "--id", "s"}, want: []string{}},
		{name: "ready runtime", current: 6, words: []string{"tobari", "manifest", "runtime", "set", "--runtime", "s"}, want: []string{"candidate:standard", "candidate:sre@2", "candidate:sre@1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records, err := command.planCompletion(context.Background(), test.current, test.words)
			if err != nil {
				t.Fatal(err)
			}
			if got := completionRecordValues(records); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("records = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompletionCandidatesCommandEmitsBoundedTSV(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), passingInspector("unused"))
	if code := runCLI(command, []string{"completion", "candidates", "--current=2", "--", "tobari", "mani"}); code != ExitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "candidate\tmanifest\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "\\t") {
		t.Fatalf("TSV structure was escaped: %q", stdout.String())
	}
}

func TestZshCompletionAdapterIsStaticAndLive(t *testing.T) {
	if !strings.HasPrefix(zshCompletionAdapter, "#compdef tobari\n") ||
		!strings.Contains(zshCompletionAdapter, "command tobari completion candidates") ||
		!strings.Contains(zshCompletionAdapter, "_directories") ||
		strings.Contains(zshCompletionAdapter, "manifest runtime set") {
		t.Fatalf("zsh adapter = %q", zshCompletionAdapter)
	}
}

func TestCompletionRecordProjectionRejectsStructuralInjection(t *testing.T) {
	for _, value := range []string{"", "bad\tvalue", "bad\nvalue", "bad\rvalue", "bad\u2028value", "bad\u2029value"} {
		if _, err := renderCompletionRecords([]completionRecord{{kind: "candidate", value: value}}); err == nil {
			t.Fatalf("value %q passed projection", value)
		}
	}
}

func TestCatalogDeclaresTypedCompletionSources(t *testing.T) {
	tests := map[string]map[string]InputCompletion{
		"doctor":               {"--root": InputCompletionDirectory},
		"help":                 {"command": InputCompletionCommand},
		"manifest show":        {"--name": InputCompletionContextName},
		"manifest create":      {"--copy-from": InputCompletionContextName, "--name": InputCompletionNone, "--runtime": InputCompletionReadyRuntimeReference},
		"manifest runtime set": {"--runtime": InputCompletionReadyRuntimeReference, "--manifest": InputCompletionContextName},
		"runtime show":         {"--name": InputCompletionRuntimeName},
		"runtime create":       {"--copy-source-from": InputCompletionRuntimeName},
		"runtime build":        {"--id": InputCompletionNone},
		"runtime restore":      {"--id": InputCompletionNone},
		"runtime delete":       {"--id": InputCompletionNone},
	}
	for path, expected := range tests {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("missing %q", path)
		}
		for name, source := range expected {
			matched := false
			for _, input := range spec.Agent.Inputs {
				if input.Name == name {
					matched = true
					if input.Completion != source {
						t.Errorf("%s %s completion = %q, want %q", path, name, input.Completion, source)
					}
				}
			}
			if !matched {
				t.Errorf("%s lacks %s", path, name)
			}
		}
	}
}
