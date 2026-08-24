package tobari

import (
	"fmt"
)

// FinalRuntimeProtectionAuthority is the complete final-owner input to WP03
// Runtime protection. It deliberately omits Policy Memory and other unrelated
// authority while binding the projection to the exact complete collection
// generation and revision observed by the final store.
type FinalRuntimeProtectionAuthority struct {
	Present              bool                `json:"present"`
	CollectionGeneration uint64              `json:"collection_generation,omitempty"`
	CollectionRevision   SemanticDigest      `json:"collection_revision,omitempty"`
	Templates            []WorkspaceTemplate `json:"workspace_templates"`
	Contexts             []ContextBinding    `json:"contexts"`
	Workspaces           []WorkspaceBinding  `json:"workspaces"`
	AuthorityDigest      SemanticDigest      `json:"authority_digest"`
}

type finalRuntimeProtectionAuthorityContent struct {
	Present              bool
	CollectionGeneration uint64
	CollectionRevision   SemanticDigest
	Templates            []WorkspaceTemplate
	Contexts             []ContextBinding
	Workspaces           []WorkspaceBinding
}

// NewFinalRuntimeProtectionAuthority projects one already-validated complete
// final collection. An absent final collection is represented explicitly and
// never inferred from missing predecessor paths.
func NewFinalRuntimeProtectionAuthority(collection WorkspaceAuthorityCollection, present bool) (FinalRuntimeProtectionAuthority, error) {
	result := FinalRuntimeProtectionAuthority{
		Present: present, Templates: []WorkspaceTemplate{}, Contexts: []ContextBinding{}, Workspaces: []WorkspaceBinding{},
	}
	if present {
		if err := collection.Validate(); err != nil {
			return FinalRuntimeProtectionAuthority{}, err
		}
		result.CollectionGeneration = collection.Generation
		result.CollectionRevision = collection.Revision
		result.Templates = cloneWorkspaceTemplates(collection.Templates)
		result.Contexts = make([]ContextBinding, len(collection.Contexts))
		for index, record := range collection.Contexts {
			result.Contexts[index] = record.Context
		}
		result.Workspaces = cloneWorkspaceBindings(collection.Workspaces)
	}
	digest, err := finalRuntimeProtectionAuthorityDigest(result)
	if err != nil {
		return FinalRuntimeProtectionAuthority{}, err
	}
	result.AuthorityDigest = digest
	return result, result.Validate()
}

func (a FinalRuntimeProtectionAuthority) Validate() error {
	if a.Templates == nil || a.Contexts == nil || a.Workspaces == nil {
		return fmt.Errorf("final Runtime protection authority is incomplete")
	}
	if !a.Present {
		if a.CollectionGeneration != 0 || a.CollectionRevision != "" || len(a.Templates) != 0 || len(a.Contexts) != 0 || len(a.Workspaces) != 0 {
			return fmt.Errorf("absent final Runtime protection authority carries collection content")
		}
	} else {
		if a.CollectionGeneration == 0 || a.CollectionRevision.Validate() != nil {
			return fmt.Errorf("final Runtime protection collection receipt is invalid")
		}
		if err := ValidateWorkspaceTemplateAuthorities(a.Templates); err != nil {
			return err
		}
		if err := ValidateContextBindings(a.Contexts); err != nil {
			return err
		}
		templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(a.Templates))
		previousTemplate := WorkspaceTemplateID("")
		for _, template := range a.Templates {
			if previousTemplate != "" && template.ID <= previousTemplate {
				return fmt.Errorf("final Runtime protection Templates must be unique and sorted")
			}
			templates[template.ID] = template
			previousTemplate = template.ID
		}
		contexts := make(map[ContextID]ContextBinding, len(a.Contexts))
		previousContext := ContextID("")
		for _, context := range a.Contexts {
			if previousContext != "" && context.ID <= previousContext {
				return fmt.Errorf("final Runtime protection Contexts must be unique and sorted")
			}
			if _, exists := templates[context.TemplateID]; !exists {
				return fmt.Errorf("final Runtime protection Context has no Template authority")
			}
			contexts[context.ID] = context
			previousContext = context.ID
		}
		workspaceContexts := make(map[ContextID]struct{}, len(a.Workspaces))
		previousWorkspace := WorkspaceID("")
		for _, workspace := range a.Workspaces {
			if previousWorkspace != "" && workspace.ID <= previousWorkspace {
				return fmt.Errorf("final Runtime protection Workspaces must be unique and sorted")
			}
			context, exists := contexts[workspace.ContextID]
			if !exists {
				return fmt.Errorf("final Runtime protection Workspace has no Context authority")
			}
			if _, exists := workspaceContexts[workspace.ContextID]; exists {
				return fmt.Errorf("final Runtime protection Context has multiple Workspaces")
			}
			if err := workspace.ValidateFor(context); err != nil {
				return err
			}
			if workspace.LastSuccessfulEntry != nil {
				template := templates[context.TemplateID]
				found := false
				for _, revision := range template.Retained {
					if revision.Revision == workspace.LastSuccessfulEntry.TemplateRevision {
						if err := workspace.LastSuccessfulEntry.ValidateForRevision(context, revision); err != nil {
							return err
						}
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("final Runtime protection AppliedEntry has no retained Template revision")
				}
			}
			workspaceContexts[workspace.ContextID] = struct{}{}
			previousWorkspace = workspace.ID
		}
	}
	want, err := finalRuntimeProtectionAuthorityDigest(a)
	if err != nil {
		return err
	}
	if a.AuthorityDigest != want {
		return fmt.Errorf("final Runtime protection digest does not bind its collection projection")
	}
	return nil
}

func (a FinalRuntimeProtectionAuthority) Clone() FinalRuntimeProtectionAuthority {
	result := a
	result.Templates = cloneWorkspaceTemplates(a.Templates)
	result.Contexts = append([]ContextBinding{}, a.Contexts...)
	result.Workspaces = cloneWorkspaceBindings(a.Workspaces)
	return result
}

func finalRuntimeProtectionAuthorityDigest(a FinalRuntimeProtectionAuthority) (SemanticDigest, error) {
	content := finalRuntimeProtectionAuthorityContent{
		Present: a.Present, CollectionGeneration: a.CollectionGeneration, CollectionRevision: a.CollectionRevision,
		Templates: a.Templates, Contexts: a.Contexts, Workspaces: a.Workspaces,
	}
	return semanticIdentity(content)
}
