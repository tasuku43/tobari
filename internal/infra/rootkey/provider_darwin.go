//go:build darwin

package rootkey

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

type securityRunner interface {
	Run(context.Context, []string, io.Reader, io.Writer, io.Writer) error
	Output(context.Context, []string) ([]byte, error)
}

type osSecurityRunner struct{}

func securityCommandEnvironment() ([]string, error) {
	account, err := user.Current()
	if err != nil || account.HomeDir == "" || !filepath.IsAbs(account.HomeDir) || filepath.Clean(account.HomeDir) != account.HomeDir {
		return nil, fmt.Errorf("%w: current macOS account home is unavailable", ErrUnavailable)
	}
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "HOME=") {
			environment = append(environment, entry)
		}
	}
	// CLI tests and callers may select an isolated HOME/XDG tree. Keychain is
	// an installation-level OS facility and must still resolve the current
	// account's real login Keychain rather than trying to create one in that
	// temporary HOME.
	return append(environment, "HOME="+account.HomeDir), nil
}

func (osSecurityRunner) Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	environment, err := securityCommandEnvironment()
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", args...) // #nosec G204 -- executable and argv are fixed and contain no secret.
	// `security ... -w` otherwise opens the caller's controlling terminal and
	// ignores the purpose-bound stdin pipe. A new session makes the reviewed
	// prompt path consume stdin without placing the value in argv or env.
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = in, out, errOut
	return command.Run()
}

func (osSecurityRunner) Output(ctx context.Context, args []string) ([]byte, error) {
	environment, err := securityCommandEnvironment()
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", args...) // #nosec G204 -- executable and argv are fixed and contain no secret.
	command.Env = environment
	return command.Output()
}

type macOSProvider struct {
	runner  securityRunner
	random  io.Reader
	service string
}

func newMacOSProvider(runner securityRunner, random io.Reader) *macOSProvider {
	return newMacOSProviderForService(runner, random, keychainService)
}

func newMacOSProviderForService(runner securityRunner, random io.Reader, service string) *macOSProvider {
	return &macOSProvider{runner: runner, random: random, service: service}
}

func (p *macOSProvider) LoadOrCreate(ctx context.Context, encryptedStateExists bool) (Material, error) {
	if err := ctx.Err(); err != nil {
		return Material{}, err
	}
	raw, err := p.runner.Output(ctx, []string{"find-generic-password", "-a", keychainAccount, "-s", p.service, "-w"})
	if err == nil {
		return decodeKeychainMaterial(raw)
	}
	var exitError interface{ ExitCode() int }
	if !errors.As(err, &exitError) || exitError.ExitCode() != 44 {
		return Material{}, fmt.Errorf("%w: Keychain lookup failed", ErrDenied)
	}
	if encryptedStateExists {
		return Material{}, ErrMissingWithVault
	}
	value, err := readRandom(p.random)
	if err != nil {
		return Material{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(value)
	// With -w as the last option, security reads the password from its prompt
	// channel. The value is carried on stdin and never appears in argv or env.
	if err := p.runner.Run(
		ctx,
		[]string{"add-generic-password", "-a", keychainAccount, "-s", p.service, "-U", "-w"},
		// The Keychain prompt requires an entry and confirmation. Both copies
		// travel only through the detached subprocess stdin channel.
		bytes.NewBufferString(encoded+"\n"+encoded+"\n"), io.Discard, io.Discard,
	); err != nil {
		return Material{}, fmt.Errorf("%w: Keychain update failed", ErrDenied)
	}
	return newMaterial(value, BackendMacOSKeychain)
}

func (p *macOSProvider) Inspect(ctx context.Context, encryptedStateExists bool) (Backend, bool, error) {
	if err := ctx.Err(); err != nil {
		return BackendMacOSKeychain, false, err
	}
	raw, err := p.runner.Output(ctx, []string{"find-generic-password", "-a", keychainAccount, "-s", p.service, "-w"})
	if err != nil {
		var exitError interface{ ExitCode() int }
		if errors.As(err, &exitError) && exitError.ExitCode() == 44 {
			if encryptedStateExists {
				return BackendMacOSKeychain, false, ErrMissingWithVault
			}
			return BackendMacOSKeychain, false, nil
		}
		return BackendMacOSKeychain, false, fmt.Errorf("%w: Keychain lookup failed", ErrDenied)
	}
	material, err := decodeKeychainMaterial(raw)
	clear(raw)
	if err != nil {
		return BackendMacOSKeychain, false, err
	}
	clear(material.value[:])
	return BackendMacOSKeychain, true, nil
}

func decodeKeychainMaterial(raw []byte) (Material, error) {
	if len(raw) > 256 {
		return Material{}, fmt.Errorf("%w: Keychain root key is oversized", ErrUnsafe)
	}
	encoded := string(bytes.TrimSpace(raw))
	value, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Material{}, fmt.Errorf("%w: Keychain root key is invalid", ErrUnsafe)
	}
	return newMaterial(value, BackendMacOSKeychain)
}
