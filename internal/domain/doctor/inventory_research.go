//go:build tobari_dev && tobari_research

package doctor

var checkInventory = append(append([]CheckSpec{}, commonCheckInventory[:len(commonCheckInventory)-1]...),
	CheckSpec{ID: CheckIDAuthProviderManifests, Prerequisites: []CheckID{CheckIDContext}},
	CheckSpec{ID: CheckIDAuthVaultPaths, Prerequisites: []CheckID{CheckIDContext}},
	CheckSpec{ID: CheckIDAuthRootKey, Prerequisites: []CheckID{CheckIDAuthVaultPaths}},
	CheckSpec{ID: CheckIDAuthBroker, Prerequisites: []CheckID{CheckIDState, CheckIDDockerEngine}},
	CheckSpec{ID: CheckIDCredentialCompanion, Prerequisites: []CheckID{CheckIDAuthBroker}},
	CheckSpec{ID: CheckIDAuthVaultIntegrity, Prerequisites: []CheckID{CheckIDAuthBroker, CheckIDAuthProviderManifests, CheckIDContext}},
	CheckSpec{ID: CheckIDAuthProjectHandles, Prerequisites: []CheckID{CheckIDAuthVaultIntegrity, CheckIDAuthProviderManifests, CheckIDState}},
	commonCheckInventory[len(commonCheckInventory)-1],
)
