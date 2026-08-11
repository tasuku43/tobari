package dockerruntime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestBuildIdentityRequiredAPIsMatchCanonicalImageSources(t *testing.T) {
	t.Parallel()
	for _, contract := range []struct {
		name     string
		asset    string
		label    string
		required int
	}{
		{name: "Gateway", asset: "gateway/Dockerfile", label: "io.tobari.gateway-api", required: buildidentity.RequiredGatewayAPI},
		{name: "Auth Broker", asset: "authbroker/Dockerfile", label: "io.tobari.auth-broker-api", required: buildidentity.RequiredAuthBrokerAPI},
	} {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			t.Parallel()
			data, err := runtimeassets.Read(contract.asset)
			if err != nil {
				t.Fatal(err)
			}
			declaration := fmt.Sprintf(`%s="%d"`, contract.label, contract.required)
			if !strings.Contains(string(data), declaration) {
				t.Fatalf("%s does not declare build identity contract %q", contract.asset, declaration)
			}
		})
	}
}
