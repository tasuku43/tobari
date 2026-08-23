//go:build !tobari_research

package cli

func clusterDownOutcome() string {
	return "Remove shared containers and networks after every logical Workspace is deleted. With --purge, also remove shared CA volumes and the active policy-bundle volume."
}

func clusterDownPreservedText() string {
	return "project roots and Workspace homes"
}
