//go:build linux || darwin

package credentialhost

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeExecRunnerUsesPrivatePTYAndStillSuppressesSyntheticToken(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "claude")
	script := `#!/bin/sh
case "$1" in
  --version)
    printf '2.1.220 (Claude Code)\n'
    ;;
  setup-token)
    test -t 0 && test -t 1 && test -t 2 || exit 3
	IFS= read -r approval
	test "$approval" = continue || exit 5
    printf '\033[?25lThis will guide you through long-lived (1-year) auth token setup for your Claude account. Claude subscription required.\r\n'
    printf '\033[32m✓ Long-lived authentication token created successfully!\033[0m\r\n'
    printf 'Your OAuth token (valid for 1 year):\r\n'
	printf 'sk-ant-oat01-synthetic_token_canary_1234567890\r\n' > /dev/tty
    printf "Store this token securely. You won't be able to see it again.\r\n"
    printf 'Use this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>\r\n\033[?25h'
    ;;
  *)
    exit 4
    ;;
esac
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	inputMaster, inputSlave, err := openClaudePTY()
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer inputMaster.Close()
	defer inputSlave.Close()
	if _, err := inputMaster.Write([]byte("continue\n")); err != nil {
		t.Fatal(err)
	}

	var visible bytes.Buffer
	driver := NewClaudeDriver(nil)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), executable, ClaudeLoginStreams{
		Stdin: inputSlave, Output: &visible,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Clear()
	if string(credential.Token()) != claudeTokenCanary {
		t.Fatalf("token did not survive private PTY capture: %v", credential)
	}
	if strings.Contains(visible.String(), claudeTokenCanary) || strings.ContainsRune(visible.String(), '\x1b') {
		t.Fatalf("private PTY leaked secret/control output: %q", visible.String())
	}
}
