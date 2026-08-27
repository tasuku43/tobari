package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/operation"
)

func readEffectTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var snapshot []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := relative + "|" + info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += "|" + string(data)
		}
		snapshot = append(snapshot, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(snapshot)
	return snapshot
}

func dockerMutationCall(call string) bool {
	fields := strings.Fields(call)
	if len(fields) == 0 {
		return false
	}
	for _, command := range []string{"run", "create", "start", "stop", "restart", "rm", "exec", "cp", "pull", "build", "tag"} {
		if fields[0] == command {
			return true
		}
	}
	if fields[0] == "compose" {
		for _, field := range fields[1:] {
			switch field {
			case "up", "down", "start", "stop", "restart", "rm", "build", "pull":
				return true
			}
		}
	}
	if len(fields) > 1 && (fields[0] == "network" || fields[0] == "volume" || fields[0] == "image") {
		switch fields[1] {
		case "create", "rm", "prune", "connect", "disconnect", "pull", "build", "tag":
			return true
		}
	}
	return false
}

func TestEveryCatalogReadExecutesOnFreshXDGWithoutDurableOrExternalMutation(t *testing.T) {
	extraArgs := map[string][]string{
		"doctor":                      {"--format=json"},
		"help":                        {"--format=agent"},
		"version":                     {"--format=json"},
		"completion zsh":              {},
		"completion candidates":       {"--current=2", "--", "tobari", "cont"},
		"template list":               {"--format=json"},
		"template migration plan":     {"--id=wtpl1_01912345-6789-7abc-8def-0123456789a1", "--format=json"},
		"template plan":               {"--id=wtpl1_01912345-6789-7abc-8def-0123456789a1", "--format=json"},
		"template show":               {"--name=standard", "--format=json"},
		"context list":                {"--format=json"},
		"context plan":                {"--id=ctx1_01912345-6789-7abc-8def-0123456789a2", "--format=json"},
		"context show":                {"--id=ctx1_01912345-6789-7abc-8def-0123456789a2", "--format=json"},
		"installation migration plan": {"--format=json"},
		"workspace list":              {"--format=json"},
		"workspace status":            {"--id=wsp1_01912345-6789-7abc-8def-0123456789a3", "--format=json"},
		"status":                      {"--format=json"},
		"cluster status":              {"--format=json"},
		"cluster denials":             {"--format=json"},
		"cluster logs":                {},
		"policy candidates":           {"--format=json"},
		"review permissions":          {"--format=json"},
		"review runtimes":             {"--format=json"},
		"policy rules":                {"--format=json"},
		"review services":             {},
		"service status":              {"--format=json"},
		"runtime history":             {"--name=standard", "--format=json"},
		"runtime list":                {"--format=json"},
		"runtime prune dry-run":       {"--format=json"},
		"runtime show":                {"--name=standard", "--format=json"},
	}
	if _, found := DefaultCatalog().Lookup("serve"); found {
		extraArgs["serve"] = []string{"--no-open"}
	}
	if len(authCommandSpecs()) != 0 {
		extraArgs["auth status"] = []string{"--context=ctx1_01912345-6789-7abc-8def-0123456789a2", "--format=json"}
	}

	var readPaths []string
	for _, spec := range DefaultCatalog().Commands() {
		if spec.Effect == operation.EffectRead {
			readPaths = append(readPaths, spec.Path)
		}
	}
	sort.Strings(readPaths)
	covered := make([]string, 0, len(extraArgs))
	for path := range extraArgs {
		covered = append(covered, path)
	}
	sort.Strings(covered)
	if !reflect.DeepEqual(covered, readPaths) {
		t.Fatalf("first-use read coverage differs from catalog\ncovered=%v\ncatalog=%v", covered, readPaths)
	}

	for _, path := range readPaths {
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			root := t.TempDir()
			configHome, stateHome, dataHome := filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data")
			for _, directory := range []string{configHome, stateHome, dataHome, filepath.Join(root, "home"), filepath.Join(root, "bin")} {
				if err := os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			record := filepath.Join(root, "docker.calls")
			docker := filepath.Join(root, "bin", "docker")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TOBARI_DOCKER_RECORD\"\n" +
				"if [ \"$1\" = version ]; then printf '%s\\n' '{\"Os\":\"linux\",\"Arch\":\"arm64\"}'; fi\nexit 0\n"
			if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", filepath.Join(root, "home"))
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("XDG_STATE_HOME", stateHome)
			t.Setenv("XDG_DATA_HOME", dataHome)
			t.Setenv("PATH", filepath.Join(root, "bin"))
			t.Setenv("TOBARI_DOCKER_RECORD", record)
			t.Setenv("NO_COLOR", "1")
			before := readEffectTreeSnapshot(t, configHome)
			before = append(before, readEffectTreeSnapshot(t, stateHome)...)
			before = append(before, readEffectTreeSnapshot(t, dataHome)...)

			var stdout, stderr bytes.Buffer
			runContext := context.Background()
			cancel := func() {}
			if path == "serve" {
				runContext, cancel = context.WithTimeout(runContext, 100*time.Millisecond)
			}
			defer cancel()
			command := New(runContext, strings.NewReader(""), &stdout, &stderr)
			args := append(strings.Fields(path), extraArgs[path]...)
			if code := command.RunContext(runContext, args); code == ExitUsage {
				t.Fatalf("%s did not reach its handler: stderr=%q", path, stderr.String())
			}

			after := readEffectTreeSnapshot(t, configHome)
			after = append(after, readEffectTreeSnapshot(t, stateHome)...)
			after = append(after, readEffectTreeSnapshot(t, dataHome)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("%s changed fresh XDG state\nbefore=%v\nafter=%v\nstderr=%q", path, before, after, stderr.String())
			}
			calls, err := os.ReadFile(record)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, call := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
				if dockerMutationCall(call) {
					t.Fatalf("%s made external mutation call %q", path, call)
				}
			}
		})
	}
}
