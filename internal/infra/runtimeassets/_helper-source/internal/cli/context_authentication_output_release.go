//go:build !tobari_research

package cli

func contextAuthenticationOutputField() OutputField {
	return OutputField{
		Name: "authentication", Type: OutputFieldTypeObject,
		Description: "Native Context-Home-owned authentication mode shared by sibling Workspaces, or not applicable when this task does not observe authentication.",
		Fields: []OutputField{
			{Name: "mode", Type: OutputFieldTypeString, Description: "Authentication ownership mode available on the release surface.", Enum: []string{"native_workspace", "not_applicable"}},
		},
	}
}
