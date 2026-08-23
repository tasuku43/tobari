//go:build tobari_dev && tobari_research

package cli

func contextAuthenticationOutputField() OutputField {
	return OutputField{
		Name: "authentication", Type: OutputFieldTypeObject,
		Description: "Workspace-native authentication mode or research Broker status without credential values.",
		Fields: []OutputField{
			{Name: "mode", Type: OutputFieldTypeString, Description: "Authentication ownership mode compiled into this executable.", Enum: []string{"native_workspace", "broker", "not_applicable"}},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Research Auth Broker observation.", Enum: []string{"not_applicable", "ready", "locked", "unavailable"}, Optional: true},
			{Name: "declared_bindings", Type: OutputFieldTypeString, Description: "Research authentication route for installed declared provider bindings.", Enum: []string{"broker_required"}, Optional: true},
			{Name: "undeclared_bindings", Type: OutputFieldTypeString, Description: "Research route for bindings absent from the provider projection.", Enum: []string{"workspace_owned_compatibility"}, Optional: true},
			{Name: "providers", Type: OutputFieldTypeArray, Description: "Installed provider states, or null when this mutation did not observe authentication.", Nullable: true, SemanticScope: "Every installed provider for the selected Workspace Manifest when authentication was observed.", Items: &OutputField{
				Type: OutputFieldTypeObject, Description: "One installed provider observation.", Fields: []OutputField{
					{Name: "provider", Type: OutputFieldTypeString, Description: "Installed provider ID."},
					{Name: "state", Type: OutputFieldTypeString, Description: "Provider credential state.", Enum: []string{"configured", "not_configured", "unavailable"}},
					{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free account label, or null.", Nullable: true},
					{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Secret-free credential revision, or null.", Nullable: true},
				},
			}},
		},
	}
}
