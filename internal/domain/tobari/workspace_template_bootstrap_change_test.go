package tobari

import "testing"

func TestWorkspaceTemplateBootstrapRequestFreezesClosedActionMatrix(t *testing.T) {
	valid := []WorkspaceTemplateBootstrapRequest{
		{Kind: WorkspaceTemplateChangeBootstrapAWS, Action: WorkspaceTemplateBootstrapConfigure, Selector: "engineering"},
		{Kind: WorkspaceTemplateChangeBootstrapAWS, Action: WorkspaceTemplateBootstrapRefresh},
		{Kind: WorkspaceTemplateChangeBootstrapAWS, Action: WorkspaceTemplateBootstrapRemove},
		{Kind: WorkspaceTemplateChangeBootstrapEKS, Action: WorkspaceTemplateBootstrapConfigure, Selector: "platform"},
		{Kind: WorkspaceTemplateChangeBootstrapEKS, Action: WorkspaceTemplateBootstrapRefresh},
		{Kind: WorkspaceTemplateChangeBootstrapEKS, Action: WorkspaceTemplateBootstrapRemove},
	}
	for _, request := range valid {
		if err := request.Validate(); err != nil {
			t.Errorf("valid request %+v: %v", request, err)
		}
	}
	invalid := []WorkspaceTemplateBootstrapRequest{
		{},
		{Kind: WorkspaceTemplateChangeShell, Action: WorkspaceTemplateBootstrapRemove},
		{Kind: WorkspaceTemplateChangeBootstrapAWS, Action: WorkspaceTemplateBootstrapConfigure},
		{Kind: WorkspaceTemplateChangeBootstrapEKS, Action: WorkspaceTemplateBootstrapConfigure, Selector: "../platform"},
		{Kind: WorkspaceTemplateChangeBootstrapAWS, Action: WorkspaceTemplateBootstrapRefresh, Selector: "engineering"},
		{Kind: WorkspaceTemplateChangeBootstrapEKS, Action: WorkspaceTemplateBootstrapRemove, Selector: "platform"},
	}
	for _, request := range invalid {
		if err := request.Validate(); err == nil {
			t.Errorf("invalid request accepted: %+v", request)
		}
	}
}
