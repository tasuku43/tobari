package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const expectedConfiguratorInitialPrompt = "Begin the Tobari Configurator workflow now. Read and follow AGENTS.md in this directory, inspect observed.json and the current editable configuration sources, then introduce what you can help configure and ask the user what they want to achieve. Do not edit until you understand their intent."

type configuratorIsolationRunner struct {
	recordingRunner
	browserControlArgs        []string
	mutateContainerInspection func(map[string]any)
	cleanupErr                error
}

type exitingConfiguratorControlRunner struct {
	configuratorIsolationRunner
	agentStarted chan struct{}
	agentStopped chan struct{}
	release      chan struct{}
}

type relayLossConfiguratorControlRunner struct {
	configuratorIsolationRunner
	agentStarted chan struct{}
	agentStopped chan struct{}
	controlDone  chan struct{}
	releaseRelay chan struct{}
}

func (r *exitingConfiguratorControlRunner) Run(ctx context.Context, args, environment []string, input io.Reader, output, errOut io.Writer) error {
	if len(args) >= 3 && slices.Equal(args[:3], []string{"exec", "-i", "-t"}) {
		r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
		close(r.agentStarted)
		close(r.release)
		<-ctx.Done()
		close(r.agentStopped)
		return ctx.Err()
	}
	return r.configuratorIsolationRunner.Run(ctx, args, environment, input, output, errOut)
}

func (r *exitingConfiguratorControlRunner) RunWorkspaceBrowserControl(_ context.Context, args, _ []string, _ io.Reader, output, _ io.Writer) error {
	r.browserControlArgs = append([]string{}, args...)
	if _, err := io.WriteString(output, workspaceBrowserReadyFrame+"\n"); err != nil {
		return err
	}
	<-r.release
	return errors.New("synthetic post-ready browser control exit")
}

func (r *relayLossConfiguratorControlRunner) Run(ctx context.Context, args, environment []string, input io.Reader, output, errOut io.Writer) error {
	if len(args) >= 3 && slices.Equal(args[:3], []string{"exec", "-i", "-t"}) {
		r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
		close(r.agentStarted)
		close(r.releaseRelay)
		<-ctx.Done()
		close(r.agentStopped)
		return ctx.Err()
	}
	return r.configuratorIsolationRunner.Run(ctx, args, environment, input, output, errOut)
}

func (r *relayLossConfiguratorControlRunner) RunWorkspaceBrowserControl(ctx context.Context, args, _ []string, _ io.Reader, output, _ io.Writer) error {
	r.browserControlArgs = append([]string{}, args...)
	if _, err := io.WriteString(output, workspaceBrowserReadyFrame+"\n"); err != nil {
		return err
	}
	<-r.releaseRelay
	if closer, ok := output.(io.Closer); ok {
		_ = closer.Close()
	}
	<-ctx.Done()
	close(r.controlDone)
	return ctx.Err()
}

func recordedConfiguratorCall(calls []runnerCall, prefix ...string) []string {
	for _, call := range calls {
		if len(call.args) >= len(prefix) && slices.Equal(call.args[:len(prefix)], prefix) {
			return call.args
		}
	}
	return nil
}

func (r *configuratorIsolationRunner) RunWorkspaceBrowserControl(_ context.Context, args, _ []string, input io.Reader, output, _ io.Writer) error {
	r.browserControlArgs = append([]string{}, args...)
	if _, err := io.WriteString(output, workspaceBrowserReadyFrame+"\n"); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, input)
	return err
}

func (r *configuratorIsolationRunner) Run(ctx context.Context, args, environment []string, input io.Reader, output, errOut io.Writer) error {
	if len(args) >= 4 && args[0] == "image" && args[1] == "inspect" && strings.Contains(args[3], `"id"`) {
		encoded, err := json.Marshal(map[string]any{
			"id": "sha256:" + strings.Repeat("a", 64), "api": tobari.RuntimeImageAPI,
			"lifetime": tobari.RuntimeImageLifetimeCommand, "user": "tobari",
			"entrypoint": []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"},
		})
		if err != nil {
			return err
		}
		_, err = output.Write(encoded)
		return err
	}
	if r.cleanupErr != nil && ((len(args) >= 2 && args[0] == "network" && args[1] == "rm") || (len(args) >= 3 && args[0] == "container" && args[1] == "rm")) {
		r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
		return r.cleanupErr
	}
	return r.recordingRunner.Run(ctx, args, environment, input, output, errOut)
}

func (r *configuratorIsolationRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	networkID := strings.Repeat("b", 64)
	containerID := strings.Repeat("c", 64)
	if len(args) > 1 && args[0] == "network" && args[1] == "create" {
		return []byte(networkID + "\n"), nil
	}
	if len(args) > 1 && args[0] == "network" && args[1] == "inspect" {
		return json.Marshal(map[string]any{"id": networkID, "driver": "bridge", "internal": false, "owner": ownerValue, "component": "configurator"})
	}
	if len(args) > 1 && args[0] == "container" && args[1] == "create" {
		return []byte(containerID + "\n"), nil
	}
	if len(args) > 1 && args[0] == "container" && args[1] == "inspect" {
		format := strings.Join(args, " ")
		if !strings.Contains(format, `"mounts"`) {
			return json.Marshal(map[string]any{
				"id": containerID, "owner": ownerValue, "component": "configurator",
				"networks": map[string]any{"configurator-egress": map[string]any{"NetworkID": networkID}},
			})
		}
		var home, opener string
		for _, call := range r.outputs {
			for index, value := range call.args {
				if value == "--mount" && index+1 < len(call.args) {
					mount := call.args[index+1]
					if strings.HasSuffix(mount, ",dst=/var/lib/tobari") {
						home = strings.TrimSuffix(strings.TrimPrefix(mount, "type=bind,src="), ",dst=/var/lib/tobari")
					}
					if strings.HasSuffix(mount, ",dst="+workspaceBrowserOpenerPath+",readonly") {
						opener = strings.TrimSuffix(strings.TrimPrefix(mount, "type=bind,src="), ",dst="+workspaceBrowserOpenerPath+",readonly")
					}
				}
			}
		}
		uid, gid := currentIDs()
		var image, workdir string
		for _, call := range r.outputs {
			if len(call.args) < 2 || call.args[0] != "container" || call.args[1] != "create" {
				continue
			}
			for index, value := range call.args {
				if value == "--workdir" && index+1 < len(call.args) {
					workdir = call.args[index+1]
				}
			}
			if len(call.args) >= 4 {
				image = call.args[len(call.args)-4]
			}
		}
		observed := map[string]any{
			"id": containerID, "image_id": image, "owner": ownerValue, "component": "configurator",
			"mounts": []map[string]any{
				{"Type": "bind", "Source": home, "Destination": "/var/lib/tobari", "RW": true},
				{"Type": "bind", "Source": opener, "Destination": workspaceBrowserOpenerPath, "RW": false},
				{"Type": "bind", "Source": opener, "Destination": "/usr/local/bin/xdg-open", "RW": false},
			},
			"tmpfs":        map[string]string{"/tmp": "size=512m,mode=1777", "/run": "size=16m,mode=1777"},
			"network_mode": "none", "read_only": true, "privileged": false,
			"cap_add": []string{}, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"},
			"pids_limit": int64(512), "memory": int64(2 << 30), "memory_swap": int64(2 << 30), "nano_cpus": int64(2_000_000_000),
			"log_driver": "none", "user": strconv.Itoa(uid) + ":" + strconv.Itoa(gid), "hostname": "tobari-configurator", "image": image, "workdir": workdir, "tty": true, "open_stdin": true,
			"entrypoint": []string{"/usr/bin/tini"}, "cmd": []string{"--", "/usr/bin/sleep", "infinity"},
		}
		if r.mutateContainerInspection != nil {
			r.mutateContainerInspection(observed)
		}
		return json.Marshal(observed)
	}
	return compatibleImageInspection(), nil
}

func TestRunConfiguratorRejectsRuntimeImageIdentityDriftBeforeDirectEgress(t *testing.T) {
	root := t.TempDir()
	runner := &configuratorIsolationRunner{
		mutateContainerInspection: func(observed map[string]any) { observed["image_id"] = "sha256:" + strings.Repeat("d", 64) },
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	body.EntryDefaults.Runtime = binding
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	materializeConfiguratorDraftTestFiles(t, runtime, draft)
	err = runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "container isolation differs") {
		t.Fatalf("role-drift error = %v", err)
	}
	for _, call := range runner.runs {
		if len(call.args) == 0 {
			continue
		}
		joined := strings.Join(call.args, " ")
		if strings.Contains(joined, "network disconnect") || strings.Contains(joined, "network connect") || strings.Contains(joined, "container start") || strings.HasPrefix(joined, "exec ") {
			t.Fatalf("role-drift container reached egress/start/exec: %v", runner.runs)
		}
	}
}

func TestRunConfiguratorRequiresNativeLoginControlBeforeDockerMutation(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard)
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("unsupported browser control error=%v", err)
	}
	if len(runner.runs) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("unsupported browser control reached Docker: runs=%v outputs=%v", runner.runs, runner.outputs)
	}
}

func TestPrepareConfiguratorRuntimeRequiresNativeLoginControlBeforeDockerMutation(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := manifest.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.PrepareConfiguratorRuntime(context.Background(), binding)
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("unsupported browser control preparation error=%v", err)
	}
	if len(runner.runs) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("unsupported browser control prepared Docker material: runs=%v outputs=%v", runner.runs, runner.outputs)
	}
}

func TestRunConfiguratorUsesDirectEgressOneMutableHomeAndReadOnlyNativeLoginOpener(t *testing.T) {
	root := t.TempDir()
	runner := &configuratorIsolationRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	standardBinding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: standardBinding},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/host/secret-project", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "state", "configurator", draft.ID, "home")
	relative, err := tobari.ConfiguratorWorkingDirectory(draft)
	if err != nil {
		t.Fatal(err)
	}
	workdir := filepath.Join(home, filepath.FromSlash(relative))
	sourceRelative, err := tobari.ConfiguratorSourceDirectory(draft)
	if err != nil {
		t.Fatal(err)
	}
	templateDir := filepath.Join(home, filepath.FromSlash(sourceRelative), "templates", string(draft.TemplateID))
	for _, dir := range []string{home, workdir, templateDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"observed.json", "AGENTS.md", "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(workdir, name), []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"template.yaml", "policy.yaml"} {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 6 {
		t.Fatalf("Docker calls = %v", runner.runs)
	}
	create := recordedConfiguratorCall(runner.outputs, "container", "create")
	agent := runner.runs[3].args
	joined := strings.Join(append(append([]string{}, create...), agent...), "\n")
	immutableImage := "sha256:" + strings.Repeat("a", 64)
	if !strings.Contains(strings.Join(create, "\n"), immutableImage) || strings.Contains(strings.Join(create, "\n"), standardBinding.Image) {
		t.Fatalf("standard Configurator did not execute the resolved immutable image ID: %v", create)
	}
	for _, required := range []string{"--network\nnone", "--log-driver\nnone", "--cap-drop\nALL", "CODEX_HOME=/var/lib/tobari/.codex", "src=" + home + ",dst=/var/lib/tobari", "dst=" + workspaceBrowserOpenerPath + ",readonly", "dst=/usr/local/bin/xdg-open,readonly", "BROWSER=" + workspaceBrowserOpenerPath, "/usr/local/bin/codex"} {
		if !strings.Contains(joined, required) {
			t.Errorf("create argv lacks %q: %v", required, create)
		}
	}
	if strings.Count(strings.Join(create, "\n"), "type=bind") != 3 {
		t.Errorf("create argv does not have one mutable Home and two exact read-only opener projections: %v", create)
	}
	for _, forbidden := range []string{draft.ProjectRoot, "/var/run/docker.sock", "--privileged", "dst=/workspace/configuration"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("create argv exposes %q: %v", forbidden, create)
		}
	}
	if !slices.Equal(runner.runs[0].args[:3], []string{"network", "disconnect", "none"}) || !slices.Equal(runner.runs[1].args[:2], []string{"network", "connect"}) || !slices.Equal(runner.runs[2].args[:2], []string{"container", "start"}) || !slices.Equal(runner.runs[3].args[:3], []string{"exec", "-i", "-t"}) || !slices.Equal(runner.runs[4].args[:3], []string{"container", "rm", "--force"}) || !slices.Equal(runner.runs[5].args[:2], []string{"network", "rm"}) {
		t.Fatalf("lifecycle calls = %v", runner.runs)
	}
	if runner.runs[0].args[3] != strings.Repeat("c", 64) || runner.runs[1].args[2] != strings.Repeat("b", 64) || runner.runs[1].args[3] != strings.Repeat("c", 64) || runner.runs[4].args[3] != strings.Repeat("c", 64) || runner.runs[5].args[2] != strings.Repeat("b", 64) {
		t.Fatalf("lifecycle did not retain immutable Docker IDs: %v", runner.runs)
	}
	if !containsArgSequence(runner.browserControlArgs, "exec", "-i", "--user") || !containsArgSequence(runner.browserControlArgs, "python3", "-c", workspaceBrowserAgentProgram) || slicesContains(runner.browserControlArgs, "-t") {
		t.Fatalf("Configurator browser control argv = %q", runner.browserControlArgs)
	}
	if len(agent) < 2 || !slices.Equal(agent[len(agent)-2:], []string{"/usr/local/bin/codex", expectedConfiguratorInitialPrompt}) {
		t.Fatalf("Codex did not receive the fixed conversation opener: %q", agent)
	}
}

func TestRunConfiguratorReportsBoundedTransientCleanupUncertainty(t *testing.T) {
	root := t.TempDir()
	runner := &configuratorIsolationRunner{cleanupErr: errors.New("synthetic cleanup failure")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	body.EntryDefaults.Runtime = binding
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	materializeConfiguratorDraftTestFiles(t, runtime, draft)
	err = runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard)
	if !errors.Is(err, tobari.ErrConfiguratorTransientCleanupUnknown) {
		t.Fatalf("cleanup outcome=%v", err)
	}
	containerRemovals, networkRemovals := 0, 0
	for _, call := range runner.runs {
		if len(call.args) >= 2 && call.args[0] == "container" && call.args[1] == "rm" {
			containerRemovals++
		}
		if len(call.args) >= 2 && call.args[0] == "network" && call.args[1] == "rm" {
			networkRemovals++
		}
	}
	if containerRemovals != configuratorCleanupAttempts || networkRemovals != configuratorCleanupAttempts {
		t.Fatalf("bounded cleanup attempts container=%d network=%d", containerRemovals, networkRemovals)
	}
}

func TestRunConfiguratorUsesExactExistingContextRuntimeAndContextHome(t *testing.T) {
	root := t.TempDir()
	runner := &configuratorIsolationRunner{}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	binding := tobari.RuntimeBinding{RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "project-tools", Revision: "sha256:" + strings.Repeat("d", 64), Ordinal: 2, Image: "tobari-runtime:project-tools"}
	body := tobari.WorkspaceTemplateBody{
		Boundary:      tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:        tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: binding}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}
	seed, err := tobari.NewEvolveConfiguratorSeed("/host/secret-project", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, templateID)
	if err != nil {
		t.Fatal(err)
	}
	home, err := runtime.finalContextHome(contextID)
	if err != nil {
		t.Fatal(err)
	}
	working, _ := tobari.ConfiguratorWorkingDirectory(draft)
	source, _ := tobari.ConfiguratorSourceDirectory(draft)
	workdir := filepath.Join(home, filepath.FromSlash(working))
	templateDir := filepath.Join(home, filepath.FromSlash(source), "templates", string(templateID))
	for _, dir := range []string{home, workdir, templateDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(workdir, "observed.json"), filepath.Join(workdir, "AGENTS.md"), filepath.Join(workdir, "CLAUDE.md"), filepath.Join(templateDir, "template.yaml"), filepath.Join(templateDir, "policy.yaml")} {
		if err := os.WriteFile(path, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime.finalWorkspaceRuntimeMaterial = func(_ context.Context, expected tobari.RuntimeBinding) (tobari.RuntimeBinding, string, string, error) {
		return expected, binding.Image, "sha256:" + strings.Repeat("e", 64), nil
	}
	if err := runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	create := recordedConfiguratorCall(runner.outputs, "container", "create")
	joined := strings.Join(create, "\n")
	immutableImage := "sha256:" + strings.Repeat("e", 64)
	if !strings.Contains(joined, immutableImage) || strings.Contains(joined, binding.Image) || !strings.Contains(joined, "src="+home+",dst=/var/lib/tobari") || strings.Count(joined, "type=bind") != 3 {
		t.Fatalf("Context Runtime/Home not exact: %v", create)
	}
	agent := recordedConfiguratorCall(runner.runs, "exec", "-i", "-t")
	if len(agent) < 2 || !slices.Equal(agent[len(agent)-2:], []string{"/usr/local/bin/claude", expectedConfiguratorInitialPrompt}) {
		t.Fatalf("Claude Code did not receive the fixed conversation opener: %q", agent)
	}
}

func TestRunConfiguratorStopsAgentWhenNativeLoginControlExitsAfterReadiness(t *testing.T) {
	root := t.TempDir()
	runner := &exitingConfiguratorControlRunner{
		agentStarted: make(chan struct{}), agentStopped: make(chan struct{}), release: make(chan struct{}),
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	body.EntryDefaults.Runtime = binding
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	materializeConfiguratorDraftTestFiles(t, runtime, draft)
	err = runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard)
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("post-ready control exit error=%v", err)
	}
	select {
	case <-runner.agentStarted:
	default:
		t.Fatal("agent did not start after browser readiness")
	}
	select {
	case <-runner.agentStopped:
	default:
		t.Fatal("agent was not canceled after browser control exit")
	}
}

func TestRunConfiguratorStopsAgentWhenHostNativeLoginRelayClosesAfterReadiness(t *testing.T) {
	root := t.TempDir()
	runner := &relayLossConfiguratorControlRunner{
		agentStarted: make(chan struct{}), agentStopped: make(chan struct{}), controlDone: make(chan struct{}), releaseRelay: make(chan struct{}),
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	standard, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := standard.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	body.EntryDefaults.Runtime = binding
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	materializeConfiguratorDraftTestFiles(t, runtime, draft)
	err = runtime.RunConfigurator(context.Background(), draft, tobari.DirectEgressConfiguratorIsolation(), strings.NewReader(""), io.Discard)
	if !errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
		t.Fatalf("host relay loss error=%v", err)
	}
	for name, stopped := range map[string]<-chan struct{}{"agent": runner.agentStopped, "control": runner.controlDone} {
		select {
		case <-stopped:
		default:
			t.Fatalf("%s was not canceled after host relay loss", name)
		}
	}
}

func materializeConfiguratorDraftTestFiles(t *testing.T, runtime *Runtime, draft tobari.ConfiguratorDraft) {
	t.Helper()
	home, err := runtime.configuratorHome(draft)
	if err != nil {
		t.Fatal(err)
	}
	working, err := tobari.ConfiguratorWorkingDirectory(draft)
	if err != nil {
		t.Fatal(err)
	}
	source, err := tobari.ConfiguratorSourceDirectory(draft)
	if err != nil {
		t.Fatal(err)
	}
	workdir := filepath.Join(home, filepath.FromSlash(working))
	templateDir := filepath.Join(home, filepath.FromSlash(source), "templates", string(draft.TemplateID))
	for _, directory := range []string{home, workdir, templateDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(workdir, "observed.json"), filepath.Join(workdir, "AGENTS.md"), filepath.Join(workdir, "CLAUDE.md"),
		filepath.Join(templateDir, "template.yaml"), filepath.Join(templateDir, "policy.yaml"),
	} {
		if err := os.WriteFile(path, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestContextHomeRetirementFailsClosedForActiveConfiguratorLease(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	release, err := runtime.acquireConfiguratorAttachmentKeys(context.Background(), "context-"+string(contextID))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := runtime.AcquireTombstonedContextHomeRetirement(context.Background(), contextID); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("active Configurator retirement error=%v", err)
	}
}

func TestConfiguratorLeaseIsExclusiveAcrossAgentsAndContextRetirement(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, _ := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, _ := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}
	seed, _ := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	codex, _ := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)
	claude, _ := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, templateID)
	release, err := runtime.AcquireConfiguratorAttachment(context.Background(), codex)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := runtime.AcquireConfiguratorAttachment(context.Background(), claude); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("cross-agent Context attachment error=%v", err)
	}
	if _, err := runtime.AcquireContextHomeRetirement(context.Background(), contextID); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Context retirement raced active Configurator: %v", err)
	}
}

func TestWorkspaceSessionLeaseAndConfiguratorAreMutuallyExclusive(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, _ := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, _ := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}
	seed, _ := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	draft, _ := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)

	releaseWorkspace, err := runtime.AcquireWorkspaceEntryAttachment(context.Background(), contextID, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	releaseSecondWorkspace, err := runtime.AcquireWorkspaceEntryAttachment(context.Background(), contextID, "/workspace/example")
	if err != nil {
		t.Fatalf("second Workspace did not share the exact Context attachment: %v", err)
	}
	releaseSession, err := runtime.acquireWorkspaceSessionAttachment(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	releaseSecondSession, err := runtime.acquireWorkspaceSessionAttachment(context.Background())
	if err != nil {
		t.Fatalf("second Workspace did not share the installation session fence: %v", err)
	}
	if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("global cluster fence ignored live Workspace attachments: %v", err)
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion:    tobari.WorkspaceBindingSchemaVersion,
		ID:               "01912345-6789-7abc-8def-0123456789ad",
		ContextID:        contextID,
		ProjectRoot:      "/workspace/example",
		Home:             "/workspace/home",
		CreationDefaults: revision.Slices.CreationDefaultsDigest,
	}
	if err := runtime.ConfirmWorkspaceRetirementAllowed(context.Background(), workspace, false); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Workspace retirement ignored live shared attachment: %v", err)
	}
	if _, err := runtime.AcquireConfiguratorAttachment(context.Background(), draft); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Configurator raced live Workspace attachment: %v", err)
	}
	if _, err := runtime.AcquireContextHomeRetirement(context.Background(), contextID); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Context retirement raced shared Workspace attachments: %v", err)
	}
	if err := releaseWorkspace(); err != nil {
		t.Fatal(err)
	}
	if err := releaseSession(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("global cluster fence ignored the remaining Workspace borrower: %v", err)
	}
	if _, err := runtime.AcquireConfiguratorAttachment(context.Background(), draft); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Configurator ignored the remaining Workspace borrower: %v", err)
	}
	if err := releaseSecondWorkspace(); err != nil {
		t.Fatal(err)
	}
	if err := releaseSecondSession(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); err != nil {
		t.Fatalf("global cluster fence remained after all Workspace attachments closed: %v", err)
	}

	releaseConfigurator, err := runtime.AcquireConfiguratorAttachment(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConfigurator()
	if _, err := runtime.AcquireWorkspaceEntryAttachment(context.Background(), contextID, "/workspace/example"); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Workspace raced live Configurator attachment: %v", err)
	}
}

func TestConfiguratorLeaseIsExclusiveAcrossBootstrapAgentsForOneProject(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", configuratorRuntimeBodyFixture())
	codex, _ := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	claude, _ := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, "01912345-6789-7abc-8def-0123456789ad", "01912345-6789-7abc-8def-0123456789ae")
	release, err := runtime.AcquireConfiguratorAttachment(context.Background(), codex)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := runtime.AcquireConfiguratorAttachment(context.Background(), claude); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("cross-agent bootstrap attachment error=%v", err)
	}
}

func TestCompatibleRuntimeRejectsImageDeclaredWritableVolume(t *testing.T) {
	runner := &recordingRunner{outputData: []byte(`{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"],"volumes":{"/data":{}}}`)}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.validateCompatibleImage(context.Background(), "example.invalid/runtime:test"); err == nil {
		t.Fatal("image-declared writable volume was accepted")
	}
}

func TestPrepareConfiguratorRuntimeRequiresExactStandardManifestBinding(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &configuratorIsolationRunner{})
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:test"}
	manifest, err := runtime.standardRuntimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := manifest.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareConfiguratorRuntime(context.Background(), binding); err != nil {
		t.Fatalf("exact standard binding rejected: %v", err)
	}
	historical := historicalStandardRuntimeBinding(strings.Repeat("5", 64))
	if err := runtime.PrepareConfiguratorRuntime(context.Background(), historical); err != nil {
		t.Fatalf("exact historical standard binding rejected: %v", err)
	}
	for name, corrupt := range map[string]func(*tobari.RuntimeBinding){
		"revision": func(value *tobari.RuntimeBinding) { value.Revision = "sha256:" + strings.Repeat("f", 64) },
		"image":    func(value *tobari.RuntimeBinding) { value.Image = "example.invalid/runtime:other" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := binding
			corrupt(&candidate)
			if err := runtime.PrepareConfiguratorRuntime(context.Background(), candidate); err == nil {
				t.Fatal("non-canonical standard binding accepted")
			}
		})
	}
}

func configuratorRuntimeBodyFixture() tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}
