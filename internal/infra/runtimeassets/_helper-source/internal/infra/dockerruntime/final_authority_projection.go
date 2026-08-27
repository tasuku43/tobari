package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

// FinalWorkspacePrincipalRow is one fully observed frozen Gateway principal
// row plus the exact Docker container/spec evidence that authorized it. The
// Template ID and spec remain validation evidence and are never serialized as
// frozen Gateway selectors.
type FinalWorkspacePrincipalRow struct {
	ContextID    tobari.ContextID
	WorkspaceID  tobari.WorkspaceID
	TemplateID   tobari.WorkspaceTemplateID
	Presentation string
	ProjectRoot  string
	ContainerID  string
	ResolvedSpec tobari.SemanticDigest
	WorkspaceIP  string
	GatewayIP    string
	Network      string
}

// FinalGatewayComponentAuthority is the exact healthy component identity that
// supplied the Gateway endpoint and interpretation observed for one complete
// materialized projection. It remains private infrastructure evidence.
type FinalGatewayComponentAuthority struct {
	ContainerID string
	ImageID     string
	Role        string
	State       string
	Health      string
	Networks    []FinalGatewayNetworkAddress
}

type FinalGatewayNetworkAddress struct {
	Name    string
	Address string
}

const finalPolicyProjectionSchemaVersion = 2

func (a FinalGatewayComponentAuthority) Validate() error {
	if !containerIDPattern.MatchString(a.ContainerID) || !imageIDPattern.MatchString(a.ImageID) || a.Role != gatewayRole || a.State != "running" || a.Health != "healthy" || a.Networks == nil {
		return fmt.Errorf("final Gateway component authority is invalid")
	}
	previous := ""
	for _, network := range a.Networks {
		address, err := netip.ParseAddr(network.Address)
		if !projectPrincipalNetworkPattern.MatchString(network.Name) || err != nil || !address.Is4() || !address.IsGlobalUnicast() || previous != "" && network.Name <= previous {
			return fmt.Errorf("final Gateway network authority is invalid")
		}
		previous = network.Name
	}
	return nil
}

func finalGatewayComponentAuthority(observation appliedClusterComponentObservation) (FinalGatewayComponentAuthority, error) {
	result := FinalGatewayComponentAuthority{
		ContainerID: observation.ContainerID, ImageID: observation.ImageID, Role: observation.Role,
		State: observation.State, Health: observation.Health, Networks: make([]FinalGatewayNetworkAddress, 0, len(observation.NetworkAddresses)),
	}
	for name, address := range observation.NetworkAddresses {
		result.Networks = append(result.Networks, FinalGatewayNetworkAddress{Name: name, Address: address})
	}
	sort.Slice(result.Networks, func(i, j int) bool { return result.Networks[i].Name < result.Networks[j].Name })
	return result, result.Validate()
}

func (p FinalWorkspacePrincipalRow) gatewayBinding() projectPrincipalBinding {
	return projectPrincipalBinding{
		ProjectID: string(p.WorkspaceID), WorkspaceManifestID: string(p.ContextID), WorkspaceManifestName: p.Presentation,
		ProjectRoot: p.ProjectRoot, WorkspaceIP: p.WorkspaceIP, GatewayIP: p.GatewayIP, Network: p.Network,
	}
}

func (p FinalWorkspacePrincipalRow) validateFor(authority tobari.WorkspacePolicyPrincipalAuthority) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	if p.ContextID != authority.ContextID || p.WorkspaceID != authority.WorkspaceID || p.TemplateID != authority.TemplateID ||
		p.Presentation != authority.Presentation || p.ProjectRoot != authority.ProjectRoot || p.ResolvedSpec != authority.AppliedEntry.ResolvedSpec ||
		!runtimeLifecycleContainerID.MatchString(p.ContainerID) {
		return fmt.Errorf("materialized final principal crosses stable or AppliedEntry authority")
	}
	registry := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{p.gatewayBinding()}}
	return registry.Validate()
}

// FinalWorkspacePolicyProjection is the post-observation content authority.
// Unlike the pre-observation plan, its digest includes every frozen principal
// network and endpoint value. It still does not include operation mode or the
// input collection revision, which remain exact plan preconditions.
type FinalWorkspacePolicyProjection struct {
	Plan               tobari.WorkspacePolicyProjection
	Principals         []FinalWorkspacePrincipalRow
	Gateway            FinalGatewayComponentAuthority
	MaterializedDigest tobari.SemanticDigest
}

func (p FinalWorkspacePolicyProjection) Validate() error {
	if err := p.Plan.Validate(); err != nil {
		return err
	}
	if p.Principals == nil {
		return fmt.Errorf("materialized final principal collection is unknown")
	}
	if err := p.Gateway.Validate(); err != nil {
		return err
	}
	expected := make(map[tobari.WorkspaceID]tobari.WorkspacePolicyPrincipalAuthority)
	for _, item := range p.Plan.Contexts {
		if item.Principal != nil {
			expected[item.Principal.WorkspaceID] = *item.Principal
		}
	}
	previous := tobari.WorkspaceID("")
	bindings := make([]projectPrincipalBinding, 0, len(p.Principals))
	for _, principal := range p.Principals {
		if previous != "" && principal.WorkspaceID <= previous {
			return fmt.Errorf("materialized final principals must be unique and sorted")
		}
		authority, exists := expected[principal.WorkspaceID]
		if !exists {
			return fmt.Errorf("materialized final projection contains an extra principal")
		}
		if err := principal.validateFor(authority); err != nil {
			return err
		}
		delete(expected, principal.WorkspaceID)
		bindings = append(bindings, principal.gatewayBinding())
		previous = principal.WorkspaceID
	}
	if len(expected) != 0 {
		return fmt.Errorf("materialized final projection omits a principal")
	}
	if err := (projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: bindings}).Validate(); err != nil {
		return err
	}
	want, err := finalWorkspacePolicyProjectionDigest(p.Plan.Contexts, p.Principals, p.Gateway)
	if err != nil {
		return err
	}
	if p.MaterializedDigest != want {
		return fmt.Errorf("materialized final projection digest does not bind policy and observed principal content")
	}
	return nil
}

func finalWorkspacePolicyProjectionDigest(contexts []tobari.WorkspacePolicyProjectionContext, principals []FinalWorkspacePrincipalRow, gateway FinalGatewayComponentAuthority) (tobari.SemanticDigest, error) {
	encoded, err := json.Marshal(struct {
		Contexts   []tobari.WorkspacePolicyProjectionContext
		Principals []FinalWorkspacePrincipalRow
		Gateway    FinalGatewayComponentAuthority
	}{contexts, principals, gateway})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])), nil
}

// ObserveFinalWorkspacePolicyProjection performs two complete bounded Docker
// observations. Any missing, extra, changed, or ambiguous principal evidence
// fails before policy or registry mutation.
func (r *Runtime) ObserveFinalWorkspacePolicyProjection(ctx context.Context, plan tobari.WorkspacePolicyProjection) (FinalWorkspacePolicyProjection, error) {
	if r == nil {
		return FinalWorkspacePolicyProjection{}, fmt.Errorf("Docker runtime is unavailable")
	}
	if err := plan.Validate(); err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	firstGateway, first, err := r.observeFinalWorkspacePrincipalRows(ctx, plan)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	if r.finalProjectionAfterFirstObservation != nil {
		r.finalProjectionAfterFirstObservation()
	}
	secondGateway, second, err := r.observeFinalWorkspacePrincipalRows(ctx, plan)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstGateway, secondGateway) {
		return FinalWorkspacePolicyProjection{}, fmt.Errorf("final principal network authority changed between complete observations")
	}
	result := FinalWorkspacePolicyProjection{Plan: plan.Clone(), Principals: append([]FinalWorkspacePrincipalRow{}, second...), Gateway: secondGateway}
	result.MaterializedDigest, err = finalWorkspacePolicyProjectionDigest(result.Plan.Contexts, result.Principals, result.Gateway)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	return result, result.Validate()
}

func (r *Runtime) observeFinalWorkspacePrincipalRows(ctx context.Context, plan tobari.WorkspacePolicyProjection) (FinalGatewayComponentAuthority, []FinalWorkspacePrincipalRow, error) {
	gateway, missing, err := r.observeAppliedClusterComponent(ctx, "gateway", gatewayContainer)
	if err != nil {
		return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("observe exact final Gateway component: %w", err)
	}
	if missing {
		return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("final Gateway component is missing")
	}
	gatewayAuthority, err := finalGatewayComponentAuthority(gateway)
	if err != nil {
		return FinalGatewayComponentAuthority{}, nil, err
	}
	rows := make([]FinalWorkspacePrincipalRow, 0)
	for _, item := range plan.Contexts {
		if item.Principal == nil {
			continue
		}
		authority := *item.Principal
		container, network, err := tobari.ProjectResourceNames(string(authority.WorkspaceID))
		if err != nil {
			return FinalGatewayComponentAuthority{}, nil, err
		}
		if err := r.verifyOwnedProjectResource(ctx, "network", network, string(authority.WorkspaceID), projectNetRole); err != nil {
			return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("observe exact final Workspace network: %w", err)
		}
		format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
			`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
			`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
			`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
			`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}},` +
			`"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`
		output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, container}, os.Environ())
		if err != nil {
			return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("observe final Workspace container: %w: %s", err, boundedDiagnostic(output))
		}
		var observed finalWorkspaceContainerObservation
		if err := decodeStrictJSON(output, &observed); err != nil {
			return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("decode final Workspace container: %w", err)
		}
		if err := observed.validateFor(authority.WorkspaceID, authority.AppliedEntry.ResolvedSpec, ""); err != nil {
			return FinalGatewayComponentAuthority{}, nil, err
		}
		subnet, err := r.projectNetworkSubnet(ctx, network)
		if err != nil {
			return FinalGatewayComponentAuthority{}, nil, err
		}
		gatewayAddress, exists := gateway.NetworkAddresses[network]
		if !exists || gatewayAddress == "" {
			return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("healthy final Gateway is not attached to exact Workspace network")
		}
		workspaceAddress, err := r.workspaceNetworkAddress(ctx, container, network)
		if err != nil {
			return FinalGatewayComponentAuthority{}, nil, err
		}
		if err := validateProjectNetworkEndpoints(subnet, workspaceAddress, gatewayAddress); err != nil {
			return FinalGatewayComponentAuthority{}, nil, err
		}
		rows = append(rows, FinalWorkspacePrincipalRow{
			ContextID: authority.ContextID, WorkspaceID: authority.WorkspaceID, TemplateID: authority.TemplateID,
			Presentation: authority.Presentation, ProjectRoot: authority.ProjectRoot,
			ContainerID: observed.ID, ResolvedSpec: authority.AppliedEntry.ResolvedSpec,
			WorkspaceIP: workspaceAddress, GatewayIP: gatewayAddress, Network: network,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].WorkspaceID < rows[j].WorkspaceID })
	expectedNetworks := map[string]struct{}{"tobari-control": {}, "tobari-egress": {}}
	for _, row := range rows {
		expectedNetworks[row.Network] = struct{}{}
	}
	if len(gatewayAuthority.Networks) != len(expectedNetworks) {
		return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("final Gateway network topology contains a missing or extra attachment")
	}
	for _, network := range gatewayAuthority.Networks {
		if _, exists := expectedNetworks[network.Name]; !exists {
			return FinalGatewayComponentAuthority{}, nil, fmt.Errorf("final Gateway network topology contains a missing or extra attachment")
		}
	}
	return gatewayAuthority, rows, nil
}

// FinalAggregateProjection is the exact existing OPA/Gateway artifact result
// produced from final authority. It is dormant and does not activate policy,
// publish principal registry state, or change the current cluster selector.
type FinalAggregateProjection struct {
	AggregateRevision  string
	PolicyDirectory    string
	GatewayConfig      string
	MaterializedDigest tobari.SemanticDigest
	EvaluatorIdentity  tobari.PolicyEvaluatorIdentity
	PolicyDataIdentity tobari.PolicyDataIdentity
}

const finalAggregatePublicationReceiptSchema = 1

// FinalAggregatePublicationReceipt is the private exact publication evidence
// joining selected per-Context axes, observed principal rows, rendered policy
// and Gateway artifacts, and the concrete OPA aggregate revision. Per-axis
// receipts alone can never confirm a different global snapshot.
type FinalAggregatePublicationReceipt struct {
	SchemaVersion        int                                      `json:"schema_version"`
	MaterializedDigest   tobari.SemanticDigest                    `json:"materialized_digest"`
	AggregateRevision    string                                   `json:"aggregate_revision"`
	EvaluatorIdentity    tobari.PolicyEvaluatorIdentity           `json:"evaluator_identity"`
	PolicyDataIdentity   tobari.PolicyDataIdentity                `json:"policy_data_identity"`
	PolicyArtifact       tobari.SemanticDigest                    `json:"policy_artifact_digest"`
	GatewayArtifact      tobari.SemanticDigest                    `json:"gateway_artifact_digest"`
	TemplateReceipts     []tobari.TemplatePolicyActivationReceipt `json:"template_policy_receipts"`
	PolicyMemoryReceipts []tobari.PolicyMemoryActivationReceipt   `json:"policy_memory_receipts"`
	PrincipalDigest      tobari.SemanticDigest                    `json:"principal_digest"`
	ReceiptDigest        tobari.SemanticDigest                    `json:"receipt_digest"`
}

type finalAggregatePublicationReceiptContent struct {
	SchemaVersion        int
	MaterializedDigest   tobari.SemanticDigest
	AggregateRevision    string
	EvaluatorIdentity    tobari.PolicyEvaluatorIdentity
	PolicyDataIdentity   tobari.PolicyDataIdentity
	PolicyArtifact       tobari.SemanticDigest
	GatewayArtifact      tobari.SemanticDigest
	TemplateReceipts     []tobari.TemplatePolicyActivationReceipt
	PolicyMemoryReceipts []tobari.PolicyMemoryActivationReceipt
	PrincipalDigest      tobari.SemanticDigest
}

func (r FinalAggregatePublicationReceipt) content() finalAggregatePublicationReceiptContent {
	return finalAggregatePublicationReceiptContent{
		r.SchemaVersion, r.MaterializedDigest, r.AggregateRevision, r.EvaluatorIdentity, r.PolicyDataIdentity, r.PolicyArtifact, r.GatewayArtifact,
		r.TemplateReceipts, r.PolicyMemoryReceipts, r.PrincipalDigest,
	}
}

func (r FinalAggregatePublicationReceipt) ValidateFor(material FinalWorkspacePolicyProjection) error {
	if err := material.Validate(); err != nil {
		return err
	}
	if r.SchemaVersion != finalAggregatePublicationReceiptSchema || r.MaterializedDigest != material.MaterializedDigest || !aggregateRevisionPattern.MatchString(r.AggregateRevision) || r.EvaluatorIdentity.Validate() != nil || r.PolicyDataIdentity.Validate() != nil || r.PolicyArtifact.Validate() != nil || r.GatewayArtifact.Validate() != nil || r.PrincipalDigest.Validate() != nil {
		return fmt.Errorf("final aggregate publication receipt metadata is invalid")
	}
	templates := make([]tobari.TemplatePolicyActivationReceipt, len(material.Plan.Contexts))
	memories := make([]tobari.PolicyMemoryActivationReceipt, len(material.Plan.Contexts))
	for index, item := range material.Plan.Contexts {
		templates[index] = item.TemplateReceipt
		memories[index] = item.MemoryReceipt
	}
	if !reflect.DeepEqual(r.TemplateReceipts, templates) || !reflect.DeepEqual(r.PolicyMemoryReceipts, memories) {
		return fmt.Errorf("final aggregate publication receipt crosses per-Context activation authority")
	}
	principalDigest, err := digestFinalValue(material.Principals)
	if err != nil || principalDigest != r.PrincipalDigest {
		return fmt.Errorf("final aggregate publication principal receipt is inconsistent: %w", err)
	}
	want, err := digestFinalValue(r.content())
	if err != nil || want != r.ReceiptDigest {
		return fmt.Errorf("final aggregate publication receipt digest is inconsistent: %w", err)
	}
	return nil
}

func (r *Runtime) NewFinalAggregatePublicationReceipt(material FinalWorkspacePolicyProjection, aggregate FinalAggregateProjection) (FinalAggregatePublicationReceipt, error) {
	if err := material.Validate(); err != nil {
		return FinalAggregatePublicationReceipt{}, err
	}
	if aggregate.MaterializedDigest != material.MaterializedDigest || !aggregateRevisionPattern.MatchString(aggregate.AggregateRevision) || aggregate.EvaluatorIdentity.Validate() != nil || aggregate.PolicyDataIdentity.Validate() != nil {
		return FinalAggregatePublicationReceipt{}, fmt.Errorf("final aggregate artifact crosses materialized authority")
	}
	policyDigest, err := digestFinalArtifactTree(aggregate.PolicyDirectory, 64*1024*1024)
	if err != nil {
		return FinalAggregatePublicationReceipt{}, err
	}
	gatewayData, err := readOwnerPolicyFile(aggregate.GatewayConfig, 256*1024)
	if err != nil {
		return FinalAggregatePublicationReceipt{}, err
	}
	gatewayDigest := sha256.Sum256(gatewayData)
	principalDigest, err := digestFinalValue(material.Principals)
	if err != nil {
		return FinalAggregatePublicationReceipt{}, err
	}
	receipt := FinalAggregatePublicationReceipt{
		SchemaVersion: finalAggregatePublicationReceiptSchema, MaterializedDigest: material.MaterializedDigest,
		AggregateRevision: aggregate.AggregateRevision, EvaluatorIdentity: aggregate.EvaluatorIdentity,
		PolicyDataIdentity: aggregate.PolicyDataIdentity, PolicyArtifact: policyDigest,
		GatewayArtifact: tobari.SemanticDigest("sha256:" + hex.EncodeToString(gatewayDigest[:])), PrincipalDigest: principalDigest,
		TemplateReceipts:     make([]tobari.TemplatePolicyActivationReceipt, len(material.Plan.Contexts)),
		PolicyMemoryReceipts: make([]tobari.PolicyMemoryActivationReceipt, len(material.Plan.Contexts)),
	}
	for index, item := range material.Plan.Contexts {
		receipt.TemplateReceipts[index] = item.TemplateReceipt
		receipt.PolicyMemoryReceipts[index] = item.MemoryReceipt
	}
	receipt.ReceiptDigest, err = digestFinalValue(receipt.content())
	if err != nil {
		return FinalAggregatePublicationReceipt{}, err
	}
	return receipt, receipt.ValidateFor(material)
}

func (r *Runtime) ConfirmFinalAggregatePublicationReceipt(material FinalWorkspacePolicyProjection, aggregate FinalAggregateProjection, expected FinalAggregatePublicationReceipt) error {
	observed, err := r.NewFinalAggregatePublicationReceipt(material, aggregate)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(observed, expected) {
		return fmt.Errorf("final aggregate artifacts changed after exact publication review")
	}
	return nil
}

func digestFinalValue(value any) (tobari.SemanticDigest, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])), nil
}

func digestFinalArtifactTree(root string, maximum int64) (tobari.SemanticDigest, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", fmt.Errorf("final policy artifact root is invalid")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("final policy artifact root is unsafe: %w", err)
	}
	type artifact struct {
		Path string
		Data []byte
	}
	files := []artifact{}
	remaining := maximum
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil || entry.Type()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("final policy artifact path is unsafe: %w", err)
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > remaining {
			return fmt.Errorf("final policy artifacts exceed %d bytes", maximum)
		}
		data, err := readOwnerPolicyFile(path, int(remaining))
		if err != nil {
			return err
		}
		remaining -= int64(len(data))
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
			return fmt.Errorf("final policy artifact path is invalid")
		}
		files = append(files, artifact{Path: filepath.ToSlash(relative), Data: data})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return digestFinalValue(files)
}

func (r *Runtime) BuildFinalAggregateProjection(ctx context.Context, material FinalWorkspacePolicyProjection) (FinalAggregateProjection, error) {
	if err := material.Validate(); err != nil {
		return FinalAggregateProjection{}, err
	}
	items := make([]aggregateContext, 0, len(material.Plan.Contexts))
	for _, contextAuthority := range material.Plan.Contexts {
		item, err := finalAggregateContext(contextAuthority)
		if err != nil {
			return FinalAggregateProjection{}, err
		}
		if err := r.testFinalContextPolicy(ctx, contextAuthority, item); err != nil {
			return FinalAggregateProjection{}, err
		}
		items = append(items, item)
	}
	result, err := r.materializeAggregateProjection(ctx, items, nil)
	if err != nil {
		return FinalAggregateProjection{}, err
	}
	return FinalAggregateProjection{
		AggregateRevision: result.Revision, PolicyDirectory: result.PolicyDirectory,
		GatewayConfig: result.GatewayConfig, MaterializedDigest: material.MaterializedDigest,
		EvaluatorIdentity: result.EvaluatorIdentity, PolicyDataIdentity: result.PolicyDataIdentity,
	}, nil
}

func finalAggregateContext(authority tobari.WorkspacePolicyProjectionContext) (aggregateContext, error) {
	if err := authority.Validate(); err != nil {
		return aggregateContext{}, err
	}
	policy := tobari.ManifestPolicy{
		SchemaVersion: tobari.ManifestPolicySchemaVersion, Name: authority.Presentation,
		DestinationCeiling: authority.TemplatePolicy.Boundary.DestinationCeiling,
		MethodPolicy:       authority.TemplatePolicy.Boundary.MethodPolicy,
		BaselineGrants:     authority.TemplatePolicy.Policy.BaselineGrants,
		BaselineTemplates:  authority.TemplatePolicy.Policy.BaselineTemplates,
		MCPBaselineGrants:  authority.TemplatePolicy.Policy.MCPBaselineGrants,
		BaselineDenies:     authority.TemplatePolicy.Policy.BaselineDenies,
		GraphQLEndpoints:   authority.TemplatePolicy.Policy.GraphQLEndpoints,
		MCPEndpoints:       authority.TemplatePolicy.Policy.MCPEndpoints,
	}
	if err := policy.Validate(); err != nil {
		return aggregateContext{}, err
	}
	effective, err := tobari.ApplyNativeToolAuthReadiness(authority.TemplatePolicy.Policy.NativeReadiness == tobari.ManifestNativeReadinessEnabled, true, policy)
	if err != nil {
		return aggregateContext{}, err
	}
	allows, denies, learnedGraphQL, err := finalPolicyMemoryRows(authority)
	if err != nil {
		return aggregateContext{}, err
	}
	semantic, hasSemantic := authority.TemplatePolicy.Policy.FinalSemanticModules()
	staticGraphQL := []tobari.GraphQLEndpoint{}
	staticMCP := []tobari.MCPEndpoint{}
	if hasSemantic {
		staticGraphQL = semantic.GraphQLEndpoints()
		staticMCP = semantic.MCPEndpoints()
	}
	graphql, err := aggregateGraphQLEndpoints(append(learnedGraphQL, staticGraphQL...), effective.GraphQLEndpoints)
	if err != nil {
		return aggregateContext{}, err
	}
	mcp, err := aggregateFinalMCPEndpoints(staticMCP, effective.MCPEndpoints)
	if err != nil {
		return aggregateContext{}, err
	}
	kubernetes, err := finalKubernetesEndpoints(authority.Principal)
	if err != nil {
		return aggregateContext{}, err
	}
	data := map[string]any{
		"schema_version": finalPolicyProjectionSchemaVersion,
		"boundary":       map[string]any{"graphql_endpoints": graphql, "mcp_endpoints": mcp, "kubernetes_endpoints": kubernetes},
		"rules":          map[string]any{learnedPolicyDataName: allows, learnedDenyDataName: denies},
		"policy": map[string]any{
			"destination_mode": effective.DestinationCeiling.Mode, "authorities": effective.DestinationCeiling.Authorities,
			"method_default": effective.MethodPolicy.Default, "method_overrides": effective.MethodPolicy.Overrides,
			"baseline_grants": effective.BaselineGrants, "baseline_templates": effective.BaselineTemplates,
			"mcp_baseline_grants": effective.MCPBaselineGrants, "baseline_denies": effective.BaselineDenies,
			"semantic": semantic,
		},
	}
	finalAuthority := authority.Clone()
	return aggregateContext{
		contextID: string(authority.ContextID), presentation: authority.Presentation,
		finalAuthority: &finalAuthority, data: data,
		graphqlEndpoints: graphql, mcpEndpoints: mcp, kubernetesEndpoints: kubernetes, contextPolicy: effective,
	}, nil
}

func finalPolicyMemoryRows(authority tobari.WorkspacePolicyProjectionContext) ([]map[string]any, []map[string]any, []tobari.GraphQLEndpoint, error) {
	allows := []map[string]any{}
	denies := []map[string]any{}
	graphql := []tobari.GraphQLEndpoint{}
	if authority.Principal == nil {
		return allows, denies, graphql, nil
	}
	for _, rule := range authority.PolicyMemory.Rules {
		var encoded []byte
		var err error
		switch rule.Decision {
		case tobari.PolicyMemoryAllow:
			var projected tobari.LearnedPolicyRule
			projected, err = tobari.NewLearnedPolicyRuleFromPolicyMemory(
				authority.ContextID, authority.Presentation, authority.Principal.WorkspaceID, authority.ProjectRoot, rule,
			)
			if err == nil {
				encoded, err = json.Marshal(projected)
			}
		case tobari.PolicyMemoryDeny:
			var projected tobari.PolicyDenyRule
			projected, err = tobari.NewPolicyDenyRuleFromPolicyMemory(
				authority.ContextID, authority.Presentation, authority.Principal.WorkspaceID, authority.ProjectRoot, rule,
			)
			if err == nil {
				encoded, err = json.Marshal(projected)
			}
		default:
			err = fmt.Errorf("final Policy Memory decision is invalid")
		}
		if err != nil {
			return nil, nil, nil, err
		}
		row := map[string]any{}
		if err := json.Unmarshal(encoded, &row); err != nil {
			return nil, nil, nil, err
		}
		completeFinalPolicyProtocolCoordinates(row, rule.Body.PolicyProtocolIdentity)
		if rule.Decision == tobari.PolicyMemoryAllow {
			allows = append(allows, row)
		} else {
			denies = append(denies, row)
		}
		if rule.Body.EffectiveProtocol() == tobari.PolicyProtocolGraphQL {
			graphql = append(graphql, tobari.GraphQLEndpoint{Scheme: rule.Body.Scheme, Host: rule.Body.Host, Port: rule.Body.Port, Path: rule.Body.Path})
		}
	}
	return allows, denies, graphql, nil
}

// completeFinalPolicyProtocolCoordinates keeps meaningful empty coordinates on
// the OPA wire. The retained domain JSON omits zero-value siblings so unrelated
// protocols stay closed, but core Kubernetes and OCI catalog identities use an
// empty string as an exact coordinate rather than an absent value.
func completeFinalPolicyProtocolCoordinates(row map[string]any, identity tobari.PolicyProtocolIdentity) {
	switch identity.EffectiveProtocol() {
	case tobari.PolicyProtocolKubernetes:
		if identity.KubernetesKind == tobari.KubernetesRequestResource {
			row["kubernetes_group"] = identity.KubernetesGroup
		}
	case tobari.PolicyProtocolOCI:
		if identity.OCIAction == "list" && identity.OCIObject == "catalog" {
			row["oci_repository"] = identity.OCIRepository
		}
	}
}

func finalKubernetesEndpoints(principal *tobari.WorkspacePolicyPrincipalAuthority) ([]tobari.GraphQLEndpoint, error) {
	if principal == nil || principal.CreationDefaults.Bootstrap == nil || principal.CreationDefaults.Bootstrap.EKS == nil {
		return []tobari.GraphQLEndpoint{}, nil
	}
	eks := principal.CreationDefaults.Bootstrap.EKS
	if err := eks.Validate(); err != nil {
		return nil, err
	}
	endpoint := tobari.GraphQLEndpoint{Scheme: "https", Host: strings.TrimPrefix(eks.Server, "https://"), Port: 443, Path: "/"}
	if err := endpoint.Validate(); err != nil {
		return nil, err
	}
	return []tobari.GraphQLEndpoint{endpoint}, nil
}

func (r *Runtime) testFinalContextPolicy(ctx context.Context, authority tobari.WorkspacePolicyProjectionContext, item aggregateContext) error {
	data, err := json.Marshal(map[string]any{"tobari": item.data})
	if err != nil {
		return err
	}
	rego, err := canonicalEvaluatorModule()
	if err != nil {
		return err
	}
	tests, err := runtimeassets.Read("opa/policy/tobari_test.rego")
	if err != nil {
		return err
	}
	return r.testPolicyPreflight(ctx, policyPreflight{
		data: data, evaluator: rego, tests: tests,
	})
}
