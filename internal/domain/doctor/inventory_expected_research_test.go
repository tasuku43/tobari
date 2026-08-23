//go:build tobari_dev && tobari_research

package doctor

func expectedCheckInventory() []CheckSpec {
	return append([]CheckSpec{
		{ID: CheckIDDockerCLI},
		{ID: CheckIDDockerEngine, Prerequisites: []CheckID{CheckIDDockerCLI}},
		{ID: CheckIDDockerContext, Prerequisites: []CheckID{CheckIDDockerCLI}},
		{ID: CheckIDDockerCompose, Prerequisites: []CheckID{CheckIDDockerCLI}},
		{ID: CheckIDProxyPort},
		{ID: CheckIDRoot},
		{ID: CheckIDRootSharing, Prerequisites: []CheckID{CheckIDRoot}},
		{ID: CheckIDContext},
		{ID: CheckIDState, Prerequisites: []CheckID{CheckIDContext}},
		{ID: CheckIDPolicy, Prerequisites: []CheckID{CheckIDContext, CheckIDDockerEngine}},
		{ID: CheckIDPolicyData, Prerequisites: []CheckID{CheckIDContext}},
		{ID: CheckIDImageConfig, Prerequisites: []CheckID{CheckIDContext}},
	},
		CheckSpec{ID: CheckIDAuthProviderManifests, Prerequisites: []CheckID{CheckIDContext}},
		CheckSpec{ID: CheckIDAuthVaultPaths, Prerequisites: []CheckID{CheckIDContext}},
		CheckSpec{ID: CheckIDAuthRootKey, Prerequisites: []CheckID{CheckIDAuthVaultPaths}},
		CheckSpec{ID: CheckIDAuthBroker, Prerequisites: []CheckID{CheckIDState, CheckIDDockerEngine}},
		CheckSpec{ID: CheckIDCredentialCompanion, Prerequisites: []CheckID{CheckIDAuthBroker}},
		CheckSpec{ID: CheckIDAuthVaultIntegrity, Prerequisites: []CheckID{CheckIDAuthBroker, CheckIDAuthProviderManifests, CheckIDContext}},
		CheckSpec{ID: CheckIDAuthProjectHandles, Prerequisites: []CheckID{CheckIDAuthVaultIntegrity, CheckIDAuthProviderManifests, CheckIDState}},
		CheckSpec{ID: CheckIDOwnedResources, Prerequisites: []CheckID{CheckIDDockerEngine}},
	)
}
