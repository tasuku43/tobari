package projectconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	SchemaVersion          = 2
	maximumBinaryNameBytes = 96 // Leaves room for the mandatory Windows .exe suffix under the 100-byte archive-entry limit.
)

type Config struct {
	SchemaVersion int         `json:"schema_version"`
	Profile       string      `json:"profile"`
	Project       Project     `json:"project"`
	PublicGuard   PublicGuard `json:"public_guard"`
}

type Project struct {
	Name             string `json:"name"`
	BinaryName       string `json:"binary_name"`
	GoModule         string `json:"go_module"`
	GitHubOwner      string `json:"github_owner"`
	GitHubRepository string `json:"github_repository"`
	Description      string `json:"description"`
	FormulaClass     string `json:"formula_class"`
	LicenseSPDX      string `json:"license_spdx"`
	SecurityContact  string `json:"security_contact"`
}

type PublicGuard struct {
	DocumentationLocale string   `json:"documentation_locale"`
	DenylistFile        string   `json:"denylist_file"`
	Required            []string `json:"required_paths"`
}

var (
	binaryPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._+()-]{0,79}$`)
	modulePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*(/[A-Za-z0-9][A-Za-z0-9._~-]*)+$`)
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	formulaPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	licensePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*$`)
	localePattern  = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
)

func Load(root string) (Config, error) {
	path := filepath.Join(root, ".harness", "project.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path is a fixed file below the caller-selected repository root.
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Config{}, fmt.Errorf("parse %s: trailing JSON data", path)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func Write(root string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(root, ".harness", "project.json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (c Config) Validate() error {
	if c.SchemaVersion == 1 {
		return errors.New("project config schema_version 1 requires explicit migration: choose the trusted documentation locale in the project thesis or product contract, add public_guard.documentation_locale, then set schema_version to 2; no locale default is applied")
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("project config schema_version = %d, want %d", c.SchemaVersion, SchemaVersion)
	}
	if c.Profile != "template" && c.Profile != "ready" {
		return errors.New(`project config profile must be "template" or "ready"`)
	}
	p := c.Project
	if !namePattern.MatchString(p.Name) || strings.TrimSpace(p.Name) != p.Name {
		return fmt.Errorf("project name %q contains characters unsafe for exact replacement", p.Name)
	}
	if isWindowsReservedBaseName(p.BinaryName) {
		return fmt.Errorf("binary_name %q is a reserved Windows device basename", p.BinaryName)
	}
	if !binaryPattern.MatchString(p.BinaryName) || len(p.BinaryName) > maximumBinaryNameBytes {
		return fmt.Errorf("binary_name %q must match %s and contain at most %d bytes", p.BinaryName, binaryPattern, maximumBinaryNameBytes)
	}
	if strings.EqualFold(p.BinaryName, "LICENSE") {
		return fmt.Errorf("binary_name %q collides with the required release archive LICENSE entry", p.BinaryName)
	}
	if err := validateModule(p.GoModule); err != nil {
		return err
	}
	if !repoPattern.MatchString(p.GitHubOwner) {
		return fmt.Errorf("github_owner %q is invalid", p.GitHubOwner)
	}
	if !repoPattern.MatchString(p.GitHubRepository) {
		return fmt.Errorf("github_repository %q is invalid", p.GitHubRepository)
	}
	if strings.TrimSpace(p.Description) != p.Description || p.Description == "" || len(p.Description) > 80 || strings.ContainsAny(p.Description, "\r\n\"\\") {
		return errors.New("description must be a non-empty, quote-free single line of at most 80 bytes")
	}
	if !formulaPattern.MatchString(p.FormulaClass) {
		return fmt.Errorf("formula_class %q must match %s", p.FormulaClass, formulaPattern)
	}
	if !licensePattern.MatchString(p.LicenseSPDX) {
		return fmt.Errorf("license_spdx %q is invalid", p.LicenseSPDX)
	}
	address, err := mail.ParseAddress(p.SecurityContact)
	if err != nil {
		return fmt.Errorf("security_contact must be an email address: %w", err)
	}
	if address.Address != p.SecurityContact {
		return errors.New("security_contact must contain only an email address")
	}
	if !localePattern.MatchString(c.PublicGuard.DocumentationLocale) {
		return fmt.Errorf("public_guard.documentation_locale %q must be one explicit language tag", c.PublicGuard.DocumentationLocale)
	}
	if c.PublicGuard.DenylistFile == "" || filepath.IsAbs(c.PublicGuard.DenylistFile) || !filepath.IsLocal(c.PublicGuard.DenylistFile) {
		return errors.New("public_guard.denylist_file must be a local relative path")
	}
	for _, path := range c.PublicGuard.Required {
		if path == "" || filepath.IsAbs(path) || !filepath.IsLocal(path) {
			return fmt.Errorf("required path %q must be local and relative", path)
		}
	}
	return nil
}

func isWindowsReservedBaseName(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "CON", "AUX", "PRN", "NUL":
		return true
	}
	if len(upper) != 4 || upper[3] < '1' || upper[3] > '9' {
		return false
	}
	return strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")
}

func validateModule(module string) error {
	if !modulePattern.MatchString(module) || strings.Contains(module, "..") {
		return fmt.Errorf("go_module %q is invalid", module)
	}
	first, _, _ := strings.Cut(module, "/")
	if !strings.Contains(first, ".") {
		return fmt.Errorf("go_module %q must begin with a domain", module)
	}
	return nil
}

func ReadyProblems(project Project) []string {
	var problems []string
	// A derived repository may legitimately remain under the template owner and
	// may deliberately retain its license. The fields below are the runnable and
	// user-facing identity that bootstrap must actually replace.
	checks := []struct {
		name     string
		value    string
		template string
	}{
		{"name", project.Name, Defaults.Name},
		{"binary_name", project.BinaryName, Defaults.BinaryName},
		{"go_module", project.GoModule, Defaults.GoModule},
		{"github_repository", project.GitHubRepository, Defaults.GitHubRepository},
		{"description", project.Description, Defaults.Description},
		{"formula_class", project.FormulaClass, Defaults.FormulaClass},
		{"security_contact", project.SecurityContact, Defaults.SecurityContact},
	}
	for _, check := range checks {
		if check.value == check.template {
			problems = append(problems, check.name+" still uses the runnable template default")
		}
	}
	return problems
}
