package dockerruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"gopkg.in/yaml.v3"
)

const maxHostKubeconfigBytes = 512 * 1024

type kubeconfigNamedNode struct {
	Name    string    `yaml:"name"`
	Cluster yaml.Node `yaml:"cluster,omitempty"`
	Context yaml.Node `yaml:"context,omitempty"`
	User    yaml.Node `yaml:"user,omitempty"`
}

type hostKubeconfig struct {
	APIVersion     string                `yaml:"apiVersion"`
	Kind           string                `yaml:"kind"`
	Preferences    yaml.Node             `yaml:"preferences"`
	Clusters       []kubeconfigNamedNode `yaml:"clusters"`
	Contexts       []kubeconfigNamedNode `yaml:"contexts"`
	Users          []kubeconfigNamedNode `yaml:"users"`
	CurrentContext string                `yaml:"current-context"`
	Extensions     yaml.Node             `yaml:"extensions,omitempty"`
}

func (r *Runtime) hostKubeconfigPath() (string, error) {
	home := r.hostHomeDirectory
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve host home: %w", err)
		}
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return "", fmt.Errorf("host home is not canonical")
	}
	return filepath.Join(home, ".kube", "config"), nil
}

func (r *Runtime) readHostEKSBootstrap(contextName, awsProfile string) (tobari.ManifestEKSBootstrap, error) {
	if contextName == "" || awsProfile == "" {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("Kubernetes context and AWS profile are required")
	}
	data, err := r.readHostKubeconfigBytes()
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	return parseHostEKSBootstrap(data, contextName, awsProfile)
}

// DiscoverContextEKSBootstraps resolves every kubeconfig context against the
// exact reviewed AWS semantic bundle. It performs no command or network call.
func (r *Runtime) DiscoverContextEKSBootstraps(ctx context.Context, aws tobari.ManifestBootstrapSnapshot) (tobari.ManifestEKSBootstrapDiscovery, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestEKSBootstrapDiscovery{}, err
	}
	if err := aws.Validate(); err != nil || aws.EKS != nil {
		return tobari.ManifestEKSBootstrapDiscovery{}, fmt.Errorf("AWS bootstrap discovery scope is invalid")
	}
	data, err := r.readHostKubeconfigBytes()
	if err != nil {
		state := tobari.ManifestBootstrapDiscoveryRejected
		reason := bootstrapDiscoveryReason(err)
		if os.IsNotExist(err) {
			state = tobari.ManifestBootstrapDiscoveryMissing
			reason = "Host kubeconfig was not found."
		}
		result := tobari.ManifestEKSBootstrapDiscovery{State: state, Reason: reason, AWSRevision: aws.Revision, Candidates: []tobari.ManifestEKSBootstrapCandidate{}}
		return result, result.Validate()
	}
	config, err := parseHostKubeconfig(data)
	if err != nil {
		result := tobari.ManifestEKSBootstrapDiscovery{State: tobari.ManifestBootstrapDiscoveryRejected, Reason: bootstrapDiscoveryReason(err), AWSRevision: aws.Revision, Candidates: []tobari.ManifestEKSBootstrapCandidate{}}
		return result, result.Validate()
	}
	names := make([]string, 0, len(config.Contexts))
	for _, entry := range config.Contexts {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	candidates := make([]tobari.ManifestEKSBootstrapCandidate, 0, len(names))
	for _, name := range names {
		eks, resolveErr := resolveHostEKSBootstrap(config, name, aws.AWS.Profile)
		if resolveErr != nil {
			candidates = append(candidates, tobari.ManifestEKSBootstrapCandidate{WorkspaceManifestName: name, State: tobari.ManifestBootstrapCandidateUnavailable, Reason: bootstrapDiscoveryReason(resolveErr)})
			continue
		}
		composed, composeErr := tobari.NewContextBootstrapSnapshotWithEKS(aws.Generation, aws.AWS, eks)
		if composeErr != nil {
			candidates = append(candidates, tobari.ManifestEKSBootstrapCandidate{WorkspaceManifestName: name, State: tobari.ManifestBootstrapCandidateUnavailable, Reason: bootstrapDiscoveryReason(composeErr)})
			continue
		}
		candidates = append(candidates, tobari.ManifestEKSBootstrapCandidate{WorkspaceManifestName: name, State: tobari.ManifestBootstrapCandidateAvailable, Snapshot: &composed})
	}
	result := tobari.ManifestEKSBootstrapDiscovery{State: tobari.ManifestBootstrapDiscoveryAvailable, AWSRevision: aws.Revision, Candidates: candidates}
	return result, result.Validate()
}

func (r *Runtime) readHostKubeconfigBytes() ([]byte, error) {
	path, err := r.hostKubeconfigPath()
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return nil, fmt.Errorf("inspect host Kubernetes configuration directory: %w", err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 || parentInfo.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("host Kubernetes configuration directory is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect host kubeconfig: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > maxHostKubeconfigBytes {
		return nil, fmt.Errorf("host kubeconfig is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- exact fixed child of the resolved host home.
	if err != nil {
		return nil, fmt.Errorf("read host kubeconfig: %w", err)
	}
	return data, nil
}

func parseHostEKSBootstrap(data []byte, contextName, awsProfile string) (tobari.ManifestEKSBootstrap, error) {
	config, err := parseHostKubeconfig(data)
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	return resolveHostEKSBootstrap(config, contextName, awsProfile)
}

func parseHostKubeconfig(data []byte) (hostKubeconfig, error) {
	if len(data) == 0 || len(data) > maxHostKubeconfigBytes || bytes.IndexByte(data, 0) >= 0 {
		return hostKubeconfig{}, fmt.Errorf("host kubeconfig is empty or oversized")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config hostKubeconfig
	if err := decoder.Decode(&config); err != nil {
		return hostKubeconfig{}, fmt.Errorf("decode host kubeconfig: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return hostKubeconfig{}, fmt.Errorf("host kubeconfig must contain one document")
	}
	if config.APIVersion != "v1" || config.Kind != "Config" {
		return hostKubeconfig{}, fmt.Errorf("host kubeconfig identity is invalid")
	}
	if err := rejectYAMLAliases(config); err != nil {
		return hostKubeconfig{}, err
	}
	if !emptyYAMLNode(config.Extensions) || !emptyMappingNode(config.Preferences) {
		return hostKubeconfig{}, fmt.Errorf("host kubeconfig preferences or extensions are unsupported")
	}
	for kind, entries := range map[string][]kubeconfigNamedNode{"cluster": config.Clusters, "context": config.Contexts, "user": config.Users} {
		seen := map[string]struct{}{}
		for _, entry := range entries {
			if entry.Name == "" {
				return hostKubeconfig{}, fmt.Errorf("host kubeconfig has an unnamed %s", kind)
			}
			if _, duplicate := seen[entry.Name]; duplicate {
				return hostKubeconfig{}, fmt.Errorf("host kubeconfig %s %q is duplicated", kind, entry.Name)
			}
			seen[entry.Name] = struct{}{}
			validShape := (kind == "cluster" && entry.Cluster.Kind != 0 && entry.Context.Kind == 0 && entry.User.Kind == 0) ||
				(kind == "context" && entry.Cluster.Kind == 0 && entry.Context.Kind != 0 && entry.User.Kind == 0) ||
				(kind == "user" && entry.Cluster.Kind == 0 && entry.Context.Kind == 0 && entry.User.Kind != 0)
			if !validShape {
				return hostKubeconfig{}, fmt.Errorf("host kubeconfig %s %q has unsupported outer fields", kind, entry.Name)
			}
		}
	}
	return config, nil
}

func resolveHostEKSBootstrap(config hostKubeconfig, contextName, awsProfile string) (tobari.ManifestEKSBootstrap, error) {
	contextEntry, err := uniqueKubeconfigEntry(config.Contexts, contextName, "context")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	contextValues, err := exactYAMLMapping(contextEntry.Context, map[string]bool{"cluster": true, "user": true, "namespace": true})
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes context: %w", err)
	}
	clusterRef, err := requiredStringNode(contextValues["cluster"], "cluster")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	userRef, err := requiredStringNode(contextValues["user"], "user")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	namespace, err := optionalStringNode(contextValues["namespace"])
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes namespace is invalid")
	}
	clusterEntry, err := uniqueKubeconfigEntry(config.Clusters, clusterRef, "cluster")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	clusterValues, err := exactYAMLMapping(clusterEntry.Cluster, map[string]bool{"server": true, "certificate-authority-data": true})
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes cluster: %w", err)
	}
	server, err := requiredStringNode(clusterValues["server"], "server")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	caData, err := requiredStringNode(clusterValues["certificate-authority-data"], "certificate-authority-data")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	userEntry, err := uniqueKubeconfigEntry(config.Users, userRef, "user")
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	userValues, err := exactYAMLMapping(userEntry.User, map[string]bool{"exec": true})
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes user: %w", err)
	}
	execValues, err := exactYAMLMapping(userValues["exec"], map[string]bool{"apiVersion": true, "command": true, "args": true, "env": true, "interactiveMode": true, "provideClusterInfo": true})
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec: %w", err)
	}
	apiVersion, err := requiredStringNode(execValues["apiVersion"], "apiVersion")
	if err != nil || apiVersion != "client.authentication.k8s.io/v1beta1" {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec API version is unsupported")
	}
	command, err := requiredStringNode(execValues["command"], "command")
	if err != nil || command != "aws" {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec command is unsupported")
	}
	args, err := stringSequence(execValues["args"])
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec args are invalid")
	}
	region, clusterName, err := parseExactEKSGetTokenArgs(args)
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	if err := validateEKSExecEnvironment(execValues["env"], awsProfile); err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	if value, valueErr := optionalStringNode(execValues["interactiveMode"]); valueErr != nil || (value != "" && value != "IfAvailable") {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec interactive mode is unsupported")
	}
	if node := execValues["provideClusterInfo"]; !emptyYAMLNode(node) && (node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || node.Value != "false") {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes exec cluster-info mode is unsupported")
	}
	parsedServer, err := url.Parse(server)
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes server is invalid")
	}
	parsedServer.Host = strings.ToLower(parsedServer.Host)
	parsedServer.Path = strings.TrimSuffix(parsedServer.Path, "/")
	decodedCA, err := base64.StdEncoding.DecodeString(strings.TrimSpace(caData))
	if err != nil {
		return tobari.ManifestEKSBootstrap{}, fmt.Errorf("selected Kubernetes certificate authority data is invalid")
	}
	result := tobari.ManifestEKSBootstrap{
		WorkspaceManifestName: contextName, ClusterName: clusterName, Region: region,
		Server: parsedServer.String(), CertificateAuthorityData: base64.StdEncoding.EncodeToString(decodedCA), Namespace: namespace,
	}
	if err := result.Validate(); err != nil {
		return tobari.ManifestEKSBootstrap{}, err
	}
	return result, nil
}

func parseExactEKSGetTokenArgs(args []string) (string, string, error) {
	if len(args) != 8 || args[0] != "--region" || args[2] != "eks" || args[3] != "get-token" || args[4] != "--cluster-name" || args[6] != "--output" || args[7] != "json" {
		return "", "", fmt.Errorf("selected Kubernetes exec is not the reviewed AWS EKS get-token contract")
	}
	if args[1] == "" || args[5] == "" {
		return "", "", fmt.Errorf("selected Kubernetes exec region or cluster is empty")
	}
	return args[1], args[5], nil
}

func validateEKSExecEnvironment(node yaml.Node, awsProfile string) error {
	if emptyYAMLNode(node) {
		return fmt.Errorf("selected Kubernetes exec must bind AWS_PROFILE")
	}
	if node.Kind != yaml.SequenceNode || len(node.Content) != 1 {
		return fmt.Errorf("selected Kubernetes exec environment is unsupported")
	}
	values, err := exactYAMLMapping(*node.Content[0], map[string]bool{"name": true, "value": true})
	if err != nil {
		return fmt.Errorf("selected Kubernetes exec environment is invalid")
	}
	name, nameErr := requiredStringNode(values["name"], "name")
	value, valueErr := requiredStringNode(values["value"], "value")
	if nameErr != nil || valueErr != nil || name != "AWS_PROFILE" || value != awsProfile {
		return fmt.Errorf("selected Kubernetes exec AWS_PROFILE does not match the Context AWS profile")
	}
	return nil
}

func uniqueKubeconfigEntry(entries []kubeconfigNamedNode, name, kind string) (kubeconfigNamedNode, error) {
	var found *kubeconfigNamedNode
	seen := make(map[string]struct{}, len(entries))
	for index := range entries {
		entry := entries[index]
		if entry.Name == "" {
			return kubeconfigNamedNode{}, fmt.Errorf("host kubeconfig has an unnamed %s", kind)
		}
		if _, duplicate := seen[entry.Name]; duplicate {
			return kubeconfigNamedNode{}, fmt.Errorf("host kubeconfig %s %q is duplicated", kind, entry.Name)
		}
		seen[entry.Name] = struct{}{}
		validShape := false
		switch kind {
		case "cluster":
			validShape = entry.Cluster.Kind != 0 && entry.Context.Kind == 0 && entry.User.Kind == 0
		case "context":
			validShape = entry.Cluster.Kind == 0 && entry.Context.Kind != 0 && entry.User.Kind == 0
		case "user":
			validShape = entry.Cluster.Kind == 0 && entry.Context.Kind == 0 && entry.User.Kind != 0
		}
		if !validShape {
			return kubeconfigNamedNode{}, fmt.Errorf("host kubeconfig %s %q has unsupported outer fields", kind, entry.Name)
		}
		if entry.Name == name {
			copy := entry
			found = &copy
		}
	}
	if found == nil {
		return kubeconfigNamedNode{}, fmt.Errorf("Kubernetes %s %q does not exist", kind, name)
	}
	return *found, nil
}

func exactYAMLMapping(node yaml.Node, allowed map[string]bool) (map[string]yaml.Node, error) {
	if node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return nil, fmt.Errorf("value must be a mapping")
	}
	result := make(map[string]yaml.Node, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !allowed[key.Value] {
			return nil, fmt.Errorf("field %q is unsupported", key.Value)
		}
		if _, duplicate := result[key.Value]; duplicate {
			return nil, fmt.Errorf("field %q is duplicated", key.Value)
		}
		result[key.Value] = *value
	}
	return result, nil
}

func requiredStringNode(node yaml.Node, name string) (string, error) {
	value, err := optionalStringNode(node)
	if err != nil || value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", name)
	}
	return value, nil
}

func optionalStringNode(node yaml.Node) (string, error) {
	if emptyYAMLNode(node) {
		return "", nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || len(node.Value) > tobari.MaxContextBootstrapValueBytes || strings.ContainsRune(node.Value, 0) {
		return "", fmt.Errorf("value must be a bounded string")
	}
	return node.Value, nil
}

func stringSequence(node yaml.Node) ([]string, error) {
	if node.Kind != yaml.SequenceNode || len(node.Content) > 32 {
		return nil, fmt.Errorf("value must be a bounded sequence")
	}
	result := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		value, err := requiredStringNode(*item, "sequence item")
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func emptyYAMLNode(node yaml.Node) bool {
	return node.Kind == 0 || (node.Kind == yaml.ScalarNode && node.Tag == "!!null") || (node.Kind == yaml.SequenceNode && len(node.Content) == 0) || (node.Kind == yaml.MappingNode && len(node.Content) == 0)
}

func emptyMappingNode(node yaml.Node) bool {
	return node.Kind == 0 || (node.Kind == yaml.MappingNode && len(node.Content) == 0)
}

func rejectYAMLAliases(config hostKubeconfig) error {
	var inspect func(yaml.Node) error
	inspect = func(node yaml.Node) error {
		if node.Kind == yaml.AliasNode || node.Anchor != "" {
			return fmt.Errorf("host kubeconfig aliases and anchors are unsupported")
		}
		for _, child := range node.Content {
			if err := inspect(*child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, node := range []yaml.Node{config.Preferences, config.Extensions} {
		if err := inspect(node); err != nil {
			return err
		}
	}
	for _, entries := range [][]kubeconfigNamedNode{config.Clusters, config.Contexts, config.Users} {
		for _, entry := range entries {
			for _, node := range []yaml.Node{entry.Cluster, entry.Context, entry.User} {
				if err := inspect(node); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// PrepareContextEKSBootstrap composes one strict host kube context with an
// already prepared AWS bootstrap for direct Context creation.
func (r *Runtime) PrepareContextEKSBootstrap(ctx context.Context, base tobari.ManifestBootstrapSnapshot, contextName string) (tobari.ManifestBootstrapSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	if err := base.Validate(); err != nil || base.EKS != nil {
		return tobari.ManifestBootstrapSnapshot{}, fmt.Errorf("base AWS bootstrap is invalid")
	}
	eks, err := r.readHostEKSBootstrap(contextName, base.AWS.Profile)
	if err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	return tobari.NewContextBootstrapSnapshotWithEKS(base.Generation, base.AWS, eks)
}

func (r *Runtime) PreviewContextEKSBootstrap(ctx context.Context, name, contextName string) (tobari.ManifestBootstrapPreview, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	manifest, _, err := r.resolveContext(name)
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	if manifest.Bootstrap == nil {
		return tobari.ManifestBootstrapPreview{}, tobari.ErrContextBootstrapNotConfigured
	}
	if contextName == "" {
		if manifest.Bootstrap.EKS == nil {
			return tobari.ManifestBootstrapPreview{}, tobari.ErrContextBootstrapNotConfigured
		}
		contextName = manifest.Bootstrap.EKS.WorkspaceManifestName
	}
	eks, err := r.readHostEKSBootstrap(contextName, manifest.Bootstrap.AWS.Profile)
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	candidate, err := tobari.NewContextBootstrapSnapshotWithEKS(manifest.Bootstrap.Generation+1, manifest.Bootstrap.AWS, eks)
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	if candidate.Revision == manifest.Bootstrap.Revision {
		candidate.Generation = manifest.Bootstrap.Generation
	}
	return tobari.NewContextBootstrapPreview(manifest.Name, manifest.Bootstrap, candidate)
}

func (r *Runtime) ConfigureContextEKSBootstrap(ctx context.Context, name, contextName, expectedRevision string, remove bool) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	var result tobari.ManifestReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readDefaultManifestName()
		if err != nil {
			return err
		}
		if name == "" {
			name = active
		}
		manifest, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		previous := manifest
		if manifest.Bootstrap == nil {
			return tobari.ErrContextBootstrapNotConfigured
		}
		if remove {
			if manifest.Bootstrap.EKS == nil {
				return tobari.ErrContextBootstrapNotConfigured
			}
			candidate, createErr := tobari.NewContextBootstrapSnapshot(manifest.Bootstrap.Generation+1, manifest.Bootstrap.AWS)
			if createErr != nil {
				return createErr
			}
			manifest.Bootstrap = &candidate
		} else {
			if contextName == "" {
				if manifest.Bootstrap.EKS == nil {
					return tobari.ErrContextBootstrapNotConfigured
				}
				contextName = manifest.Bootstrap.EKS.WorkspaceManifestName
			}
			eks, readErr := r.readHostEKSBootstrap(contextName, manifest.Bootstrap.AWS.Profile)
			if readErr != nil {
				return readErr
			}
			candidate, createErr := tobari.NewContextBootstrapSnapshotWithEKS(manifest.Bootstrap.Generation+1, manifest.Bootstrap.AWS, eks)
			if createErr != nil {
				return createErr
			}
			if candidate.Revision == manifest.Bootstrap.Revision {
				candidate.Generation = manifest.Bootstrap.Generation
			}
			if expectedRevision != "" && candidate.Revision != expectedRevision {
				return tobari.ErrContextBootstrapSourceChanged
			}
			manifest.Bootstrap = &candidate
		}
		manifest, err = r.publishWorkspaceManifestUpdate(previous, manifest)
		if err != nil {
			return fmt.Errorf("write Context EKS bootstrap snapshot: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigBootstrapEKS, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}
