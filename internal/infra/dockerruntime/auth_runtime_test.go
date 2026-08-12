package dockerruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

func TestSupportsBuiltinAuthHelperIsGitHubOnly(t *testing.T) {
	github := authbroker.Provider{ID: "github", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}
	if !supportsBuiltinAuthHelper(github) {
		t.Fatal("GitHub helper was rejected")
	}
	for _, provider := range []authbroker.Provider{
		{ID: "aws", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "aws-sso"}},
		{ID: "datadog", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "pup-oauth"}},
		{ID: "openai", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "codex-chatgpt-oauth"}},
		{ID: "anthropic", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "claude-setup-token"}},
	} {
		if supportsBuiltinAuthHelper(provider) {
			t.Fatalf("retired helper accepted: %+v", provider)
		}
	}
}

func TestClassifyGitHubLoginFailuresIsSecretFree(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
		kind fault.Kind
	}{
		{context.Canceled, "github_login_cancelled", fault.KindRejected},
		{credentialhost.ErrGitHubExecutable, "github_cli_unavailable", fault.KindUnavailable},
		{credentialhost.ErrGitHubLoginFailed, "github_login_failed", fault.KindRejected},
	} {
		public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "github"))
		if !ok || public.Code != test.code || public.Kind != test.kind || public.Retryable || errors.Unwrap(public) != nil {
			t.Fatalf("fault = %+v, ok=%t", public, ok)
		}
	}
}
