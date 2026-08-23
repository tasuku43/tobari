//go:build !tobari_research

package doctor

func expectedCheckInventory() []CheckSpec {
	return append([]CheckSpec(nil), commonCheckInventory...)
}
