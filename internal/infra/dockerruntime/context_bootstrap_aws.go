package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxHostAWSConfigBytes = 256 * 1024

func (r *Runtime) hostAWSConfigPath() (string, error) {
	home := r.hostHomeDirectory
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve host home: %w", err)
		}
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return "", fmt.Errorf("host home is not canonical")
	}
	return filepath.Join(home, ".aws", "config"), nil
}

// PrepareContextAWSBootstrap reads only the fixed host shared-config file and
// returns a normalized, secret-free initial snapshot. It never reads the AWS
// credentials file or IAM Identity Center token cache.
func (r *Runtime) PrepareContextAWSBootstrap(ctx context.Context, profile string) (tobari.ContextBootstrapSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextBootstrapSnapshot{}, err
	}
	aws, err := r.readHostAWSBootstrap(profile)
	if err != nil {
		return tobari.ContextBootstrapSnapshot{}, err
	}
	return tobari.NewContextBootstrapSnapshot(1, aws)
}

// ConfigureContextAWSBootstrap atomically replaces or removes the recipe used
// only by future Workspace creation. An empty profile refreshes the currently
// selected profile; remove performs no host read.
func (r *Runtime) PreviewContextAWSBootstrap(ctx context.Context, name, profile string) (tobari.ContextBootstrapPreview, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextBootstrapPreview{}, err
	}
	manifest, _, err := r.resolveContext(name)
	if err != nil {
		return tobari.ContextBootstrapPreview{}, err
	}
	if profile == "" {
		if manifest.Bootstrap == nil {
			return tobari.ContextBootstrapPreview{}, tobari.ErrContextBootstrapNotConfigured
		}
		profile = manifest.Bootstrap.AWS.Profile
	}
	if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil && profile != manifest.Bootstrap.AWS.Profile {
		return tobari.ContextBootstrapPreview{}, tobari.ErrContextBootstrapDependency
	}
	aws, err := r.readHostAWSBootstrap(profile)
	if err != nil {
		return tobari.ContextBootstrapPreview{}, err
	}
	generation := uint64(1)
	if manifest.Bootstrap != nil {
		generation = manifest.Bootstrap.Generation + 1
	}
	var candidate tobari.ContextBootstrapSnapshot
	if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil {
		candidate, err = tobari.NewContextBootstrapSnapshotWithEKS(generation, aws, *manifest.Bootstrap.EKS)
	} else {
		candidate, err = tobari.NewContextBootstrapSnapshot(generation, aws)
	}
	if err != nil {
		return tobari.ContextBootstrapPreview{}, err
	}
	if manifest.Bootstrap != nil && candidate.Revision == manifest.Bootstrap.Revision {
		candidate.Generation = manifest.Bootstrap.Generation
	}
	return tobari.NewContextBootstrapPreview(manifest.Name, manifest.Bootstrap, candidate)
}

func (r *Runtime) ConfigureContextAWSBootstrap(ctx context.Context, name, profile, expectedRevision string, remove bool) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		if name == "" {
			name = active
		}
		manifest, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		if remove {
			if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil {
				return tobari.ErrContextBootstrapDependency
			}
			manifest.Bootstrap = nil
		} else {
			if profile == "" {
				if manifest.Bootstrap == nil {
					return tobari.ErrContextBootstrapNotConfigured
				}
				profile = manifest.Bootstrap.AWS.Profile
			}
			if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil && profile != manifest.Bootstrap.AWS.Profile {
				return tobari.ErrContextBootstrapDependency
			}
			aws, readErr := r.readHostAWSBootstrap(profile)
			if readErr != nil {
				return readErr
			}
			generation := uint64(1)
			if manifest.Bootstrap != nil {
				generation = manifest.Bootstrap.Generation + 1
			}
			var candidate tobari.ContextBootstrapSnapshot
			var createErr error
			if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil {
				candidate, createErr = tobari.NewContextBootstrapSnapshotWithEKS(generation, aws, *manifest.Bootstrap.EKS)
			} else {
				candidate, createErr = tobari.NewContextBootstrapSnapshot(generation, aws)
			}
			if createErr != nil {
				return createErr
			}
			if manifest.Bootstrap != nil && manifest.Bootstrap.Revision == candidate.Revision {
				candidate.Generation = manifest.Bootstrap.Generation
			}
			if expectedRevision != "" && candidate.Revision != expectedRevision {
				return tobari.ErrContextBootstrapSourceChanged
			}
			manifest.Bootstrap = &candidate
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context bootstrap snapshot: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigBootstrapAWS, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (r *Runtime) readHostAWSBootstrap(profile string) (tobari.ContextAWSBootstrap, error) {
	if profile == "" {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS profile is required")
	}
	path, err := r.hostAWSConfigPath()
	if err != nil {
		return tobari.ContextAWSBootstrap{}, err
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("inspect host AWS configuration directory: %w", err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 || parentInfo.Mode().Perm()&0o022 != 0 {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("host AWS configuration directory is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("inspect host AWS shared config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || info.Size() > maxHostAWSConfigBytes {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("host AWS shared config is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- exact fixed child of the resolved host home.
	if err != nil {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("read host AWS shared config: %w", err)
	}
	return parseHostAWSBootstrap(data, profile)
}

func parseHostAWSBootstrap(data []byte, profile string) (tobari.ContextAWSBootstrap, error) {
	if len(data) == 0 || len(data) > maxHostAWSConfigBytes || bytes.IndexByte(data, 0) >= 0 {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("host AWS shared config is empty or oversized")
	}
	sections := make(map[string]map[string]string)
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxHostAWSConfigBytes)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config line %d has an empty section", lineNumber)
			}
			if _, duplicate := sections[section]; duplicate {
				return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config section %q is duplicated", section)
			}
			sections[section] = make(map[string]string)
			continue
		}
		if section == "" {
			return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config line %d is outside a section", lineNumber)
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config line %d is not a key/value", lineNumber)
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if key == "" || value == "" {
			return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config line %d has an empty key or value", lineNumber)
		}
		if _, duplicate := sections[section][key]; duplicate {
			return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS config key %q is duplicated in section %q", key, section)
		}
		sections[section][key] = value
	}
	if err := scanner.Err(); err != nil {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("scan host AWS shared config: %w", err)
	}
	profileSection := "profile " + profile
	if profile == "default" {
		profileSection = "default"
	}
	profileValues, found := sections[profileSection]
	if !found {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS profile %q does not exist", profile)
	}
	allowedProfile := map[string]bool{"sso_session": true, "sso_account_id": true, "sso_role_name": true, "region": true, "output": true}
	if err := rejectUnknownAWSBootstrapKeys(profileSection, profileValues, allowedProfile); err != nil {
		return tobari.ContextAWSBootstrap{}, err
	}
	sessionName := profileValues["sso_session"]
	sessionSection := "sso-session " + sessionName
	sessionValues, found := sections[sessionSection]
	if !found {
		return tobari.ContextAWSBootstrap{}, fmt.Errorf("AWS SSO session %q does not exist", sessionName)
	}
	allowedSession := map[string]bool{"sso_start_url": true, "sso_region": true, "sso_registration_scopes": true}
	if err := rejectUnknownAWSBootstrapKeys(sessionSection, sessionValues, allowedSession); err != nil {
		return tobari.ContextAWSBootstrap{}, err
	}
	scopes := []string{}
	if raw := sessionValues["sso_registration_scopes"]; raw != "" {
		for _, scope := range strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' }) {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	result := tobari.ContextAWSBootstrap{
		Profile: profile, SSOSession: sessionName, SSOStartURL: sessionValues["sso_start_url"], SSORegion: sessionValues["sso_region"],
		SSORegistrationScopes: scopes, AccountID: profileValues["sso_account_id"], RoleName: profileValues["sso_role_name"],
		Region: profileValues["region"], Output: profileValues["output"],
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextAWSBootstrap{}, err
	}
	return result, nil
}

func rejectUnknownAWSBootstrapKeys(section string, values map[string]string, allowed map[string]bool) error {
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("AWS bootstrap section %q contains unsupported key %q", section, key)
		}
	}
	return nil
}
