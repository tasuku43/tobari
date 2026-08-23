package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
func (r *Runtime) PrepareContextAWSBootstrap(ctx context.Context, profile string) (tobari.ManifestBootstrapSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	aws, err := r.readHostAWSBootstrap(profile)
	if err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	return tobari.NewContextBootstrapSnapshot(1, aws)
}

// DiscoverContextAWSBootstraps returns every profile as a typed selectable or
// unavailable candidate. It reads the fixed shared-config file once and never
// reads credentials or SSO cache state.
func (r *Runtime) DiscoverContextAWSBootstraps(ctx context.Context) (tobari.ManifestAWSBootstrapDiscovery, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestAWSBootstrapDiscovery{}, err
	}
	data, err := r.readHostAWSConfigBytes()
	if err != nil {
		state := tobari.ManifestBootstrapDiscoveryRejected
		reason := bootstrapDiscoveryReason(err)
		if errors.Is(err, os.ErrNotExist) {
			state = tobari.ManifestBootstrapDiscoveryMissing
			reason = "Host AWS shared config was not found."
		}
		result := tobari.ManifestAWSBootstrapDiscovery{State: state, Reason: reason, Candidates: []tobari.ManifestAWSBootstrapCandidate{}}
		return result, result.Validate()
	}
	sections, err := parseHostAWSConfig(data)
	if err != nil {
		result := tobari.ManifestAWSBootstrapDiscovery{State: tobari.ManifestBootstrapDiscoveryRejected, Reason: bootstrapDiscoveryReason(err), Candidates: []tobari.ManifestAWSBootstrapCandidate{}}
		return result, result.Validate()
	}
	profiles := make([]string, 0)
	for section := range sections {
		profile := ""
		if section == "default" {
			profile = "default"
		} else if strings.HasPrefix(section, "profile ") {
			profile = strings.TrimSpace(strings.TrimPrefix(section, "profile "))
		}
		if profile == "" {
			continue
		}
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	candidates := make([]tobari.ManifestAWSBootstrapCandidate, 0, len(profiles))
	for _, profile := range profiles {
		aws, resolveErr := resolveHostAWSBootstrap(sections, profile)
		if resolveErr != nil {
			candidates = append(candidates, tobari.ManifestAWSBootstrapCandidate{Profile: profile, State: tobari.ManifestBootstrapCandidateUnavailable, Reason: bootstrapDiscoveryReason(resolveErr)})
			continue
		}
		snapshot, snapshotErr := tobari.NewContextBootstrapSnapshot(1, aws)
		if snapshotErr != nil {
			candidates = append(candidates, tobari.ManifestAWSBootstrapCandidate{Profile: profile, State: tobari.ManifestBootstrapCandidateUnavailable, Reason: bootstrapDiscoveryReason(snapshotErr)})
			continue
		}
		candidates = append(candidates, tobari.ManifestAWSBootstrapCandidate{Profile: profile, State: tobari.ManifestBootstrapCandidateAvailable, Snapshot: &snapshot})
	}
	result := tobari.ManifestAWSBootstrapDiscovery{State: tobari.ManifestBootstrapDiscoveryAvailable, Candidates: candidates}
	return result, result.Validate()
}

// ConfigureContextAWSBootstrap atomically replaces or removes the recipe used
// only by future Workspace creation. An empty profile refreshes the currently
// selected profile; remove performs no host read.
func (r *Runtime) PreviewContextAWSBootstrap(ctx context.Context, name, profile string) (tobari.ManifestBootstrapPreview, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	manifest, _, err := r.resolveContext(name)
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	if profile == "" {
		if manifest.Bootstrap == nil {
			return tobari.ManifestBootstrapPreview{}, tobari.ErrContextBootstrapNotConfigured
		}
		profile = manifest.Bootstrap.AWS.Profile
	}
	if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil && profile != manifest.Bootstrap.AWS.Profile {
		return tobari.ManifestBootstrapPreview{}, tobari.ErrContextBootstrapDependency
	}
	aws, err := r.readHostAWSBootstrap(profile)
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	generation := uint64(1)
	if manifest.Bootstrap != nil {
		generation = manifest.Bootstrap.Generation + 1
	}
	var candidate tobari.ManifestBootstrapSnapshot
	if manifest.Bootstrap != nil && manifest.Bootstrap.EKS != nil {
		candidate, err = tobari.NewContextBootstrapSnapshotWithEKS(generation, aws, *manifest.Bootstrap.EKS)
	} else {
		candidate, err = tobari.NewContextBootstrapSnapshot(generation, aws)
	}
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	if manifest.Bootstrap != nil && candidate.Revision == manifest.Bootstrap.Revision {
		candidate.Generation = manifest.Bootstrap.Generation
	}
	return tobari.NewContextBootstrapPreview(manifest.Name, manifest.Bootstrap, candidate)
}

func (r *Runtime) ConfigureContextAWSBootstrap(ctx context.Context, name, profile, expectedRevision string, remove bool) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	var result tobari.ManifestReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readDefaultManifestName()
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
		previous := manifest
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
			var candidate tobari.ManifestBootstrapSnapshot
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
		manifest, err = r.publishWorkspaceManifestUpdate(previous, manifest)
		if err != nil {
			return fmt.Errorf("write Context bootstrap snapshot: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigBootstrapAWS, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func (r *Runtime) readHostAWSBootstrap(profile string) (tobari.ManifestAWSBootstrap, error) {
	if profile == "" {
		return tobari.ManifestAWSBootstrap{}, fmt.Errorf("AWS profile is required")
	}
	data, err := r.readHostAWSConfigBytes()
	if err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	return parseHostAWSBootstrap(data, profile)
}

func (r *Runtime) readHostAWSConfigBytes() ([]byte, error) {
	path, err := r.hostAWSConfigPath()
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return nil, fmt.Errorf("inspect host AWS configuration directory: %w", err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 || parentInfo.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("host AWS configuration directory is unsafe")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect host AWS shared config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || info.Size() > maxHostAWSConfigBytes {
		return nil, fmt.Errorf("host AWS shared config is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- exact fixed child of the resolved host home.
	if err != nil {
		return nil, fmt.Errorf("read host AWS shared config: %w", err)
	}
	return data, nil
}

func parseHostAWSBootstrap(data []byte, profile string) (tobari.ManifestAWSBootstrap, error) {
	sections, err := parseHostAWSConfig(data)
	if err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	return resolveHostAWSBootstrap(sections, profile)
}

func parseHostAWSConfig(data []byte) (map[string]map[string]string, error) {
	if len(data) == 0 || len(data) > maxHostAWSConfigBytes || bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("host AWS shared config is empty or oversized")
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
				return nil, fmt.Errorf("AWS config line %d has an empty section", lineNumber)
			}
			if _, duplicate := sections[section]; duplicate {
				return nil, fmt.Errorf("AWS config section %q is duplicated", section)
			}
			sections[section] = make(map[string]string)
			continue
		}
		if section == "" {
			return nil, fmt.Errorf("AWS config line %d is outside a section", lineNumber)
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("AWS config line %d is not a key/value", lineNumber)
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("AWS config line %d has an empty key or value", lineNumber)
		}
		if _, duplicate := sections[section][key]; duplicate {
			return nil, fmt.Errorf("AWS config key %q is duplicated in section %q", key, section)
		}
		sections[section][key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan host AWS shared config: %w", err)
	}
	return sections, nil
}

func resolveHostAWSBootstrap(sections map[string]map[string]string, profile string) (tobari.ManifestAWSBootstrap, error) {
	if err := tobari.ValidateContextAWSBootstrapProfileName(profile); err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	profileSection := "profile " + profile
	if profile == "default" {
		profileSection = "default"
	}
	profileValues, found := sections[profileSection]
	if !found {
		return tobari.ManifestAWSBootstrap{}, fmt.Errorf("AWS profile %q does not exist", profile)
	}
	allowedProfile := map[string]bool{"sso_session": true, "sso_account_id": true, "sso_role_name": true, "region": true, "output": true}
	if err := rejectUnknownAWSBootstrapKeys(profileSection, profileValues, allowedProfile); err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	sessionName := profileValues["sso_session"]
	sessionSection := "sso-session " + sessionName
	sessionValues, found := sections[sessionSection]
	if !found {
		return tobari.ManifestAWSBootstrap{}, fmt.Errorf("AWS SSO session %q does not exist", sessionName)
	}
	allowedSession := map[string]bool{"sso_start_url": true, "sso_region": true, "sso_registration_scopes": true}
	if err := rejectUnknownAWSBootstrapKeys(sessionSection, sessionValues, allowedSession); err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	scopes := []string{}
	if raw := sessionValues["sso_registration_scopes"]; raw != "" {
		for _, scope := range strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' }) {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	result := tobari.ManifestAWSBootstrap{
		Profile: profile, SSOSession: sessionName, SSOStartURL: sessionValues["sso_start_url"], SSORegion: sessionValues["sso_region"],
		SSORegistrationScopes: scopes, AccountID: profileValues["sso_account_id"], RoleName: profileValues["sso_role_name"],
		Region: profileValues["region"], Output: profileValues["output"],
	}
	if err := result.Validate(); err != nil {
		return tobari.ManifestAWSBootstrap{}, err
	}
	return result, nil
}

func bootstrapDiscoveryReason(err error) string {
	if err == nil {
		return "Host configuration is unavailable."
	}
	value := strings.Map(func(r rune) rune {
		if r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, err.Error())
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		var bounded strings.Builder
		for _, r := range value {
			if bounded.Len()+len(string(r)) > 512 {
				break
			}
			bounded.WriteRune(r)
		}
		value = bounded.String()
	}
	if value == "" {
		return "Host configuration is unavailable."
	}
	return value
}

func rejectUnknownAWSBootstrapKeys(section string, values map[string]string, allowed map[string]bool) error {
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("AWS bootstrap section %q contains unsupported key %q", section, key)
		}
	}
	return nil
}
