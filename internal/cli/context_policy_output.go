package cli

func contextPolicyMethodPolicyOutput(description string) *OutputField {
	return &OutputField{Type: OutputFieldTypeObject, Description: description, Fields: []OutputField{
		{Name: "default", Type: OutputFieldTypeString, Description: "Decision for every HTTP method without an exact override.", Enum: []string{"allow", "exact_review", "deny"}},
		{Name: "overrides", Type: OutputFieldTypeArray, Description: "Exact HTTP method decision overrides.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact method override.", Fields: []OutputField{
			{Name: "method", Type: OutputFieldTypeString, Description: "Exact uppercase HTTP method."},
			{Name: "decision", Type: OutputFieldTypeString, Description: "Decision for this method.", Enum: []string{"allow", "exact_review", "deny"}},
		}}},
	}}
}
