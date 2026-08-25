//go:build tobari_dev && tobari_research

package cli

func clusterDownOutcome() string {
	return "Remove shared containers and networks after every logical Workspace is deleted. With --purge, also remove shared CA volumes and the active policy-bundle volume. Preserve encrypted Context vaults and the installation root key in both modes."
}

func clusterDownPreservedText() string {
	return "encrypted Context vaults and installation root key"
}
