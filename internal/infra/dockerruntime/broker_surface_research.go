//go:build tobari_dev && tobari_research

package dockerruntime

import "path/filepath"

const brokerRuntimeEnabled = true

var clusterContainers = map[string]string{
	"auth-broker": authBrokerContainer,
	"gateway":     gatewayContainer,
	"opa":         opaContainer,
}

var clusterComponentOrder = []string{"auth-broker", "gateway", "opa"}

func composeFileArgs(runtimeDirectory string) []string {
	return []string{
		"-f", filepath.Join(runtimeDirectory, "compose.yaml"),
		"-f", filepath.Join(runtimeDirectory, "compose.experimental.yaml"),
	}
}
