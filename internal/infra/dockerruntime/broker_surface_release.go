//go:build !tobari_research

package dockerruntime

import "path/filepath"

const brokerRuntimeEnabled = false

var clusterContainers = map[string]string{
	"gateway": gatewayContainer,
	"opa":     opaContainer,
}

var clusterComponentOrder = []string{"gateway", "opa"}

func composeFileArgs(runtimeDirectory string) []string {
	return []string{"-f", filepath.Join(runtimeDirectory, "compose.yaml")}
}
