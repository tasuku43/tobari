package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeWorkspaceContainerAuthority struct {
	ContainerID  string
	WorkspaceID  string
	ResolvedSpec string
	RuntimeID    string
	Revision     string
}

type runtimeProtectionObservation struct {
	AuthorityDigest tobari.SemanticDigest
	Inventory       tobari.RuntimeProtectionInventory
	Containers      map[string]runtimeWorkspaceContainerAuthority
}

// FinalRuntimeProtectionSource is the narrow final-authority observation seam
// owned by WP03. A host-side Store adapter satisfies it structurally without
// making dockerruntime import or rediscover the final authority store.
type FinalRuntimeProtectionSource interface {
	ReadFinalRuntimeProtectionAuthority(context.Context) (tobari.FinalRuntimeProtectionAuthority, error)
}

// BindFinalRuntimeProtectionSource installs the final-only protection source
// during composition. Rebinding would let a later caller replace authority
// underneath lifecycle observation, so it is rejected.
func (r *Runtime) BindFinalRuntimeProtectionSource(source FinalRuntimeProtectionSource) error {
	if r == nil || source == nil {
		return fmt.Errorf("final Runtime protection source is required")
	}
	if r.finalRuntimeProtectionSource != nil {
		return fmt.Errorf("final Runtime protection source is already bound")
	}
	r.finalRuntimeProtectionSource = source
	return nil
}

// ReadRuntimeProtectionInventory returns the complete, lock-consistent graph
// needed by future Runtime retirement. It performs no retirement decision and
// intentionally does not derive last-used from reconciliation timestamps.
func (r *Runtime) ReadRuntimeProtectionInventory(ctx context.Context) (tobari.RuntimeProtectionInventory, error) {
	var result runtimeProtectionObservation
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		observationContext, cancel := context.WithTimeout(lockContext, runtimeLifecycleWallBudget)
		defer cancel()
		budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
		var readErr error
		result, readErr = r.readRuntimeProtectionInventoryObserved(observationContext, &budget)
		return readErr
	})
	if err != nil {
		return tobari.RuntimeProtectionInventory{}, err
	}
	return result.Inventory, nil
}

// readRuntimeProtectionInventoryObserved requires the caller to hold the
// lifecycle observation/effect lock. Keeping the join lock outside this helper
// lets lifecycle planning observe catalog, protection, and Docker evidence as
// one coherent zero-write snapshot.
func (r *Runtime) readRuntimeProtectionInventoryObserved(ctx context.Context, budget *runtimeLifecycleBudget) (runtimeProtectionObservation, error) {
	result := runtimeProtectionObservation{Inventory: tobari.RuntimeProtectionInventory{Complete: true, Items: []tobari.RuntimeProtection{}}, Containers: make(map[string]runtimeWorkspaceContainerAuthority)}
	if r == nil || r.finalRuntimeProtectionSource == nil {
		return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	authority, err := r.finalRuntimeProtectionSource.ReadFinalRuntimeProtectionAuthority(ctx)
	if err != nil {
		return runtimeProtectionObservation{}, err
	}
	if err := authority.Validate(); err != nil {
		return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	result.AuthorityDigest = authority.AuthorityDigest
	if !authority.Present {
		return result, result.Inventory.Validate()
	}
	templates := make(map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplate, len(authority.Templates))
	for _, template := range authority.Templates {
		templates[template.ID] = template
		if template.Current.Slices.RuntimeID != tobari.StandardRuntimeID {
			result.Inventory.Items = append(result.Inventory.Items, finalTemplateRuntimeProtection(template.ID, template.Current, tobari.RuntimeProtectedByTemplateCurrent))
		}
		for _, revision := range template.Retained {
			if revision.Revision == template.Current.Revision || revision.Slices.RuntimeID == tobari.StandardRuntimeID {
				continue
			}
			result.Inventory.Items = append(result.Inventory.Items, finalTemplateRuntimeProtection(template.ID, revision, tobari.RuntimeProtectedByTemplateRetained))
		}
	}
	contexts := make(map[tobari.ContextID]tobari.ContextBinding, len(authority.Contexts))
	for _, binding := range authority.Contexts {
		contexts[binding.ID] = binding
		template := templates[binding.TemplateID]
		if template.Current.Slices.RuntimeID != tobari.StandardRuntimeID {
			result.Inventory.Items = append(result.Inventory.Items, tobari.RuntimeProtection{
				RuntimeID: template.Current.Slices.RuntimeID, RuntimeRevision: string(template.Current.Slices.RuntimeRevision),
				Reason: tobari.RuntimeProtectedByContextDesired, WorkspaceTemplateID: template.ID,
				TemplateRevision: template.Current.Revision, ContextID: binding.ID,
			})
		}
	}
	for _, workspace := range authority.Workspaces {
		binding := contexts[workspace.ContextID]
		template := templates[binding.TemplateID]
		if workspace.LastSuccessfulEntry != nil {
			observed, containerID, observeErr := r.observeFinalWorkspaceRuntimeProtection(ctx, workspace, budget)
			if observeErr != nil {
				return runtimeProtectionObservation{}, observeErr
			}
			if protection, ok := finalWorkspaceRuntimeProtection(workspace, *workspace.LastSuccessfulEntry, observed); ok {
				result.Inventory.Items = append(result.Inventory.Items, protection)
				if observed {
					if _, exists := result.Containers[containerID]; exists {
						return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
					}
					entry := *workspace.LastSuccessfulEntry
					result.Containers[containerID] = runtimeWorkspaceContainerAuthority{ContainerID: containerID, WorkspaceID: string(workspace.ID), ResolvedSpec: string(entry.ResolvedSpec), RuntimeID: entry.RuntimeID, Revision: string(entry.RuntimeRevision)}
				}
			}
		}
		if template.Current.Slices.RuntimeID != tobari.StandardRuntimeID &&
			(workspace.LastSuccessfulEntry == nil || workspace.LastSuccessfulEntry.EntrySliceDigest != template.Current.Slices.EntrySliceDigest) {
			result.Inventory.Items = append(result.Inventory.Items, tobari.RuntimeProtection{
				RuntimeID: template.Current.Slices.RuntimeID, RuntimeRevision: string(template.Current.Slices.RuntimeRevision),
				Reason: tobari.RuntimeProtectedByWorkspacePending, WorkspaceTemplateID: template.ID,
				TemplateRevision: template.Current.Revision, ContextID: workspace.ContextID, WorkspaceID: workspace.ID,
			})
		}
	}
	sort.Slice(result.Inventory.Items, func(i, j int) bool {
		return runtimeProtectionSortKey(result.Inventory.Items[i]) < runtimeProtectionSortKey(result.Inventory.Items[j])
	})
	result.Inventory.Items, err = canonicalRuntimeProtectionItems(result.Inventory.Items)
	if err != nil {
		return runtimeProtectionObservation{}, err
	}
	return result, result.Inventory.Validate()
}

func finalTemplateRuntimeProtection(templateID tobari.WorkspaceTemplateID, revision tobari.WorkspaceTemplateRevision, reason tobari.RuntimeProtectionReason) tobari.RuntimeProtection {
	return tobari.RuntimeProtection{
		RuntimeID: revision.Slices.RuntimeID, RuntimeRevision: string(revision.Slices.RuntimeRevision), Reason: reason,
		WorkspaceTemplateID: templateID, TemplateRevision: revision.Revision,
	}
}

func finalWorkspaceRuntimeProtection(workspace tobari.WorkspaceBinding, entry tobari.WorkspaceAppliedEntry, observed bool) (tobari.RuntimeProtection, bool) {
	if entry.RuntimeID == tobari.StandardRuntimeID {
		return tobari.RuntimeProtection{}, false
	}
	reason := tobari.RuntimeProtectedByWorkspaceApplied
	if observed {
		reason = tobari.RuntimeProtectedByWorkspaceObserved
	}
	return tobari.RuntimeProtection{
		RuntimeID: entry.RuntimeID, RuntimeRevision: string(entry.RuntimeRevision), Reason: reason,
		WorkspaceTemplateID: entry.TemplateID, TemplateRevision: entry.TemplateRevision, ContextID: workspace.ContextID, WorkspaceID: workspace.ID,
	}, true
}

func (r *Runtime) observeFinalWorkspaceRuntimeProtection(ctx context.Context, workspace tobari.WorkspaceBinding, budget *runtimeLifecycleBudget) (bool, string, error) {
	if workspace.LastSuccessfulEntry == nil {
		return false, "", nil
	}
	container, _, err := tobari.ProjectResourceNames(string(workspace.ID))
	if err != nil {
		return false, "", err
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	output, diagnostic, err := budget.run(ctx, r.runner, []string{"container", "inspect", "--format", format, container}, os.Environ(), 4096)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, "", err
		}
		if isMissingRuntimeContainerInspect(err, diagnostic, container) {
			return false, "", nil
		}
		return false, "", tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	var observed workspaceRuntimeObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return false, "", tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	entry := workspace.LastSuccessfulEntry
	if observed.ID == "" || !runtimeLifecycleContainerID.MatchString(observed.ID) || observed.Owner != ownerValue || observed.Component != "tobari" ||
		observed.Workspace != string(workspace.ID) || observed.Role != projectWorkRole || observed.Spec != string(entry.ResolvedSpec) {
		return false, "", tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	return true, observed.ID, nil
}

func runtimeProtectionSortKey(item tobari.RuntimeProtection) string {
	return item.RuntimeID + "\x00" + item.RuntimeRevision + "\x00" + string(item.Reason) + "\x00" +
		string(item.WorkspaceTemplateID) + "\x00" + string(item.TemplateRevision) + "\x00" + string(item.ContextID) + "\x00" + string(item.WorkspaceID)
}

func canonicalRuntimeProtectionItems(items []tobari.RuntimeProtection) ([]tobari.RuntimeProtection, error) {
	result := make([]tobari.RuntimeProtection, 0, len(items))
	owners := make(map[string]string, len(items))
	for _, item := range items {
		authority := runtimeProtectionSortKey(item)
		owner := string(item.Reason) + "\x00" + string(item.WorkspaceTemplateID) + "\x00" + string(item.TemplateRevision) + "\x00" + string(item.ContextID) + "\x00" + string(item.WorkspaceID)
		target := item.RuntimeID + "\x00" + item.RuntimeRevision
		if previous, exists := owners[owner]; exists && previous != target {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		owners[owner] = target
		if len(result) != 0 && runtimeProtectionSortKey(result[len(result)-1]) == authority {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

type workspaceRuntimeObservation struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	Workspace string `json:"workspace"`
	Role      string `json:"role"`
	Spec      string `json:"spec"`
}

func (r *Runtime) observeWorkspaceRuntimeProtection(ctx context.Context, workspace tobari.Workspace, budget *runtimeLifecycleBudget) (bool, error) {
	if workspace.Runtime.ContainerID == "" {
		return false, nil
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	output, diagnostic, err := budget.run(ctx, r.runner, []string{"container", "inspect", "--format", format, workspace.Runtime.ContainerID}, os.Environ(), 4096)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		if isMissingRuntimeContainerInspect(err, diagnostic, workspace.Runtime.ContainerID) {
			return false, nil
		}
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	var observed workspaceRuntimeObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	if observed.ID != workspace.Runtime.ContainerID || observed.Owner != ownerValue || observed.Component != "tobari" ||
		observed.Workspace != workspace.ID || observed.Role != projectWorkRole || workspace.LastSuccessfulEntry == nil ||
		observed.Spec != workspace.LastSuccessfulEntry.ResolvedSpec {
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	return true, nil
}

func isMissingRuntimeContainerInspect(err error, diagnostic []byte, containerID string) bool {
	if err == nil || !validRuntimeProtectionContainerSelector(containerID) {
		return false
	}
	message := strings.TrimSuffix(string(diagnostic), "\n")
	message = strings.TrimSuffix(message, "\r")
	accepted := []string{
		"Error: No such container: " + containerID,
		"Error response from daemon: No such container: " + containerID,
		"Error: No such object: " + containerID,
		"Error response from daemon: No such object: " + containerID,
	}
	return slices.Contains(accepted, message)
}

func validRuntimeProtectionContainerSelector(value string) bool {
	if runtimeLifecycleContainerID.MatchString(value) {
		return true
	}
	const prefix, suffix = "tobari-", "-work"
	if len(value) != len(prefix)+12+len(suffix) || !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return false
	}
	for _, character := range value[len(prefix) : len(prefix)+12] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func readPrivateDirectoryIfPresent(path string) ([]os.DirEntry, bool, error) {
	if err := requirePrivateDirectory(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []os.DirEntry{}, false, nil
		}
		return nil, false, fmt.Errorf("protection authority directory is unsafe: %w", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}
