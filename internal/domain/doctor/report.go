// Package doctor defines the diagnostic result returned by the doctor use case.
package doctor

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CheckID is one stable member of doctor's complete finite inventory.
type CheckID string

const (
	CheckIDDockerCLI             CheckID = "docker_cli"
	CheckIDDockerEngine          CheckID = "docker_engine"
	CheckIDDockerContext         CheckID = "docker_context"
	CheckIDDockerCompose         CheckID = "docker_compose"
	CheckIDProxyPort             CheckID = "proxy_port"
	CheckIDRoot                  CheckID = "root"
	CheckIDRootSharing           CheckID = "root_sharing"
	CheckIDContext               CheckID = "context"
	CheckIDState                 CheckID = "state"
	CheckIDPolicy                CheckID = "policy"
	CheckIDPolicyData            CheckID = "policy_data"
	CheckIDCredentialConfig      CheckID = "credential_config" // #nosec G101 -- public diagnostic ID, not a credential.
	CheckIDImageConfig           CheckID = "image_config"
	CheckIDAuthProviderManifests CheckID = "auth_provider_manifests"
	CheckIDAuthVaultPaths        CheckID = "auth_vault_paths"
	CheckIDAuthRootKey           CheckID = "auth_root_key"
	CheckIDAuthBroker            CheckID = "auth_broker"
	CheckIDAuthVaultIntegrity    CheckID = "auth_vault_integrity"
	CheckIDAuthProjectHandles    CheckID = "auth_project_handles"
	CheckIDOwnedResources        CheckID = "owned_resources"
)

// CheckSpec declares one check and only the checks that must pass before it can
// be observed. Inventory order is the public report order and topological order.
type CheckSpec struct {
	ID            CheckID
	Prerequisites []CheckID
}

var checkInventory = []CheckSpec{
	{ID: CheckIDDockerCLI},
	{ID: CheckIDDockerEngine, Prerequisites: []CheckID{CheckIDDockerCLI}},
	{ID: CheckIDDockerContext, Prerequisites: []CheckID{CheckIDDockerCLI}},
	{ID: CheckIDDockerCompose, Prerequisites: []CheckID{CheckIDDockerCLI}},
	{ID: CheckIDProxyPort},
	{ID: CheckIDRoot},
	{ID: CheckIDRootSharing, Prerequisites: []CheckID{CheckIDRoot}},
	{ID: CheckIDContext},
	{ID: CheckIDState, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDPolicy, Prerequisites: []CheckID{CheckIDContext, CheckIDDockerEngine}},
	{ID: CheckIDPolicyData, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDCredentialConfig, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDImageConfig, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDAuthProviderManifests, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDAuthVaultPaths, Prerequisites: []CheckID{CheckIDContext}},
	{ID: CheckIDAuthRootKey, Prerequisites: []CheckID{CheckIDAuthVaultPaths}},
	{ID: CheckIDAuthBroker, Prerequisites: []CheckID{CheckIDState, CheckIDDockerEngine}},
	{ID: CheckIDAuthVaultIntegrity, Prerequisites: []CheckID{CheckIDAuthBroker, CheckIDAuthProviderManifests, CheckIDContext}},
	{ID: CheckIDAuthProjectHandles, Prerequisites: []CheckID{CheckIDAuthVaultIntegrity, CheckIDAuthProviderManifests, CheckIDState}},
	{ID: CheckIDOwnedResources, Prerequisites: []CheckID{CheckIDDockerEngine}},
}

// CheckInventory returns a defensive copy of the canonical finite graph.
func CheckInventory() []CheckSpec {
	result := make([]CheckSpec, len(checkInventory))
	for index, spec := range checkInventory {
		var prerequisites []CheckID
		if spec.Prerequisites != nil {
			prerequisites = append([]CheckID{}, spec.Prerequisites...)
		}
		result[index] = CheckSpec{ID: spec.ID, Prerequisites: prerequisites}
	}
	return result
}

// ValidateInventory rejects unknown, duplicate, cyclic, or non-topological
// check declarations.
func ValidateInventory(inventory []CheckSpec) error {
	if len(inventory) == 0 {
		return fmt.Errorf("doctor check inventory is empty")
	}
	seen := make(map[CheckID]struct{}, len(inventory))
	for index, spec := range inventory {
		if err := spec.ID.Validate(); err != nil {
			return fmt.Errorf("doctor check inventory item %d: %w", index, err)
		}
		if _, duplicate := seen[spec.ID]; duplicate {
			return fmt.Errorf("doctor check inventory contains duplicate %q", spec.ID)
		}
		prerequisites := make(map[CheckID]struct{}, len(spec.Prerequisites))
		for _, prerequisite := range spec.Prerequisites {
			if _, duplicate := prerequisites[prerequisite]; duplicate {
				return fmt.Errorf("doctor check %q repeats prerequisite %q", spec.ID, prerequisite)
			}
			prerequisites[prerequisite] = struct{}{}
			if _, precedes := seen[prerequisite]; !precedes {
				return fmt.Errorf("doctor check %q prerequisite %q is unknown or does not precede it", spec.ID, prerequisite)
			}
		}
		seen[spec.ID] = struct{}{}
	}
	return nil
}

// Validate rejects values outside the closed public inventory.
func (id CheckID) Validate() error {
	for _, spec := range checkInventory {
		if id == spec.ID {
			return nil
		}
	}
	return fmt.Errorf("doctor check ID is missing or invalid: %q", id)
}

// CheckStatus is the machine-readable outcome of one diagnostic check.
type CheckStatus string

const (
	CheckStatusPass    CheckStatus = "pass"
	CheckStatusWarn    CheckStatus = "warn"
	CheckStatusFail    CheckStatus = "fail"
	CheckStatusBlocked CheckStatus = "blocked"
)

// Validate rejects empty and unknown statuses.
func (s CheckStatus) Validate() error {
	switch s {
	case CheckStatusPass, CheckStatusWarn, CheckStatusFail, CheckStatusBlocked:
		return nil
	default:
		return fmt.Errorf("check status is missing or invalid: %q", s)
	}
}

// Recovery gives one concrete correction and the exact next Tobari command.
type Recovery struct {
	Action      string
	NextCommand string
}

// Validate rejects vague or structurally unsafe recovery facts.
func (r Recovery) Validate() error {
	if invalidText(r.Action) || strings.TrimSpace(r.Action) != r.Action || r.Action == "" {
		return fmt.Errorf("doctor recovery action is missing or invalid")
	}
	if invalidText(r.NextCommand) || strings.TrimSpace(r.NextCommand) != r.NextCommand || r.NextCommand == "" {
		return fmt.Errorf("doctor recovery next command is missing or invalid")
	}
	return nil
}

// Observation is one check result returned by infrastructure. Infrastructure
// cannot manufacture blocked results; only the application scheduler can.
type Observation struct {
	Status CheckStatus
	Detail string
}

// Validate enforces the infrastructure observation boundary.
func (o Observation) Validate() error {
	if err := o.Status.Validate(); err != nil {
		return err
	}
	if o.Status == CheckStatusBlocked {
		return fmt.Errorf("infrastructure observation cannot be blocked")
	}
	if !utf8.ValidString(o.Detail) {
		return fmt.Errorf("doctor observation detail is invalid UTF-8")
	}
	return nil
}

// Check is one row in a diagnostic report.
type Check struct {
	Name      CheckID
	Status    CheckStatus
	Detail    string
	BlockedBy *CheckID
	Recovery  *Recovery
}

// Report is the complete result of a doctor inspection.
type Report struct {
	Checks []Check
}

// Validate ensures that adapters return the complete deterministic inventory.
func (r Report) Validate() error {
	inventory := CheckInventory()
	if err := ValidateInventory(inventory); err != nil {
		return err
	}
	if len(r.Checks) != len(inventory) {
		return fmt.Errorf("doctor report contains %d checks, want complete inventory of %d", len(r.Checks), len(inventory))
	}
	statuses := make(map[CheckID]CheckStatus, len(r.Checks))
	for index, check := range r.Checks {
		spec := inventory[index]
		if check.Name != spec.ID {
			return fmt.Errorf("doctor check %d is %q, want %q", index, check.Name, spec.ID)
		}
		if err := check.Status.Validate(); err != nil {
			return fmt.Errorf("doctor check %q: %w", check.Name, err)
		}
		if !utf8.ValidString(check.Detail) {
			return fmt.Errorf("doctor check %q has invalid UTF-8 detail", check.Name)
		}
		if check.Status == CheckStatusBlocked {
			if check.BlockedBy == nil {
				return fmt.Errorf("blocked doctor check %q lacks its prerequisite", check.Name)
			}
			if check.Recovery != nil {
				return fmt.Errorf("blocked doctor check %q duplicates prerequisite recovery", check.Name)
			}
			if !containsCheckID(spec.Prerequisites, *check.BlockedBy) {
				return fmt.Errorf("doctor check %q is blocked by undeclared prerequisite %q", check.Name, *check.BlockedBy)
			}
			if statuses[*check.BlockedBy] == CheckStatusPass {
				return fmt.Errorf("doctor check %q is blocked by passing prerequisite %q", check.Name, *check.BlockedBy)
			}
		} else if check.BlockedBy != nil {
			return fmt.Errorf("non-blocked doctor check %q declares blocked_by", check.Name)
		}
		if check.Status == CheckStatusFail && check.Recovery == nil {
			return fmt.Errorf("failed doctor check %q lacks recovery", check.Name)
		}
		if check.Status == CheckStatusPass && check.Recovery != nil {
			return fmt.Errorf("passed doctor check %q declares recovery", check.Name)
		}
		if check.Recovery != nil {
			if err := check.Recovery.Validate(); err != nil {
				return fmt.Errorf("doctor check %q: %w", check.Name, err)
			}
		}
		statuses[check.Name] = check.Status
	}
	return nil
}

// Healthy reports whether no check observed an actual failure. Blocked checks
// do not duplicate the unhealthy root cause that prevented their observation.
func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Status == CheckStatusFail {
			return false
		}
	}
	return true
}

// FirstFailureRecovery returns the first concrete correction in report order.
func (r Report) FirstFailureRecovery() (Recovery, bool) {
	for _, check := range r.Checks {
		if check.Status == CheckStatusFail && check.Recovery != nil {
			return *check.Recovery, true
		}
	}
	return Recovery{}, false
}

// PrimaryRecovery selects the first failed check's recovery, or the first
// actionable warning when the report has no observed failure.
func (r Report) PrimaryRecovery() (Recovery, bool) {
	if recovery, exists := r.FirstFailureRecovery(); exists {
		return recovery, true
	}
	for _, check := range r.Checks {
		if check.Status == CheckStatusWarn && check.Recovery != nil {
			return *check.Recovery, true
		}
	}
	return Recovery{}, false
}

func containsCheckID(values []CheckID, target CheckID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func invalidText(value string) bool {
	return !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0
}
