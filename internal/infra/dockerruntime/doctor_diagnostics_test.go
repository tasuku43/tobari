package dockerruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
)

func TestCheckGatewayConfigAcceptsEveryProjectedEndpointKind(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gateway.json")
	document := []byte(`{"version":"v2","contexts":{"01912345-6789-7abc-8def-0123456789ad":{"name":"default","graphql_endpoints":[{"scheme":"https","host":"api.github.com","port":443,"path":"/graphql"}],"mcp_endpoints":[{"scheme":"https","host":"chatgpt.com","port":443,"path":"/backend-api/ps/mcp"}],"kubernetes_endpoints":[]}}}`)
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if detail, status := (&Runtime{}).checkGatewayConfigAt(path); status != doctor.CheckStatusPass {
		t.Fatalf("checkGatewayConfigAt() = %q, %q, want pass", detail, status)
	}
}

func TestCheckGatewayConfigRejectsDuplicateMCPEndpoints(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gateway.json")
	document := []byte(`{"version":"v2","contexts":{"01912345-6789-7abc-8def-0123456789ad":{"name":"default","graphql_endpoints":[],"mcp_endpoints":[{"scheme":"https","host":"chatgpt.com","port":443,"path":"/backend-api/ps/mcp"},{"scheme":"https","host":"chatgpt.com","port":443,"path":"/backend-api/ps/mcp"}],"kubernetes_endpoints":[]}}}`)
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	if detail, status := (&Runtime{}).checkGatewayConfigAt(path); status != doctor.CheckStatusFail || detail != "gateway.json contains duplicate MCP endpoint projections" {
		t.Fatalf("checkGatewayConfigAt() = %q, %q", detail, status)
	}
}
