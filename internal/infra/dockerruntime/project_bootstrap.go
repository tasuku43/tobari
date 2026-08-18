package dockerruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func applyProjectBootstrap(home string, snapshot *tobari.ContextBootstrapSnapshot) error {
	if snapshot == nil {
		return nil
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := requirePrivateDirectory(home); err != nil {
		return fmt.Errorf("Workspace home is unsafe: %w", err)
	}
	awsDirectory := filepath.Join(home, ".aws")
	if _, err := os.Lstat(awsDirectory); err == nil {
		return fmt.Errorf("Workspace AWS bootstrap target already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(awsDirectory, 0o700); err != nil {
		return err
	}
	if err := requirePrivateDirectory(awsDirectory); err != nil {
		return err
	}
	config, err := encodeProjectAWSConfig(snapshot.AWS)
	if err != nil {
		return err
	}
	if err := initializeBytes(filepath.Join(awsDirectory, "config"), config, 0o600); err != nil {
		return err
	}
	return syncDirectory(awsDirectory)
}

func encodeProjectAWSConfig(aws tobari.ContextAWSBootstrap) ([]byte, error) {
	if err := aws.Validate(); err != nil {
		return nil, err
	}
	profileHeader := "profile " + aws.Profile
	if aws.Profile == "default" {
		profileHeader = "default"
	}
	var output strings.Builder
	fmt.Fprintf(&output, "[%s]\n", profileHeader)
	fmt.Fprintf(&output, "sso_session = %s\n", aws.SSOSession)
	fmt.Fprintf(&output, "sso_account_id = %s\n", aws.AccountID)
	fmt.Fprintf(&output, "sso_role_name = %s\n", aws.RoleName)
	if aws.Region != "" {
		fmt.Fprintf(&output, "region = %s\n", aws.Region)
	}
	if aws.Output != "" {
		fmt.Fprintf(&output, "output = %s\n", aws.Output)
	}
	fmt.Fprintf(&output, "\n[sso-session %s]\n", aws.SSOSession)
	fmt.Fprintf(&output, "sso_start_url = %s\n", aws.SSOStartURL)
	fmt.Fprintf(&output, "sso_region = %s\n", aws.SSORegion)
	if len(aws.SSORegistrationScopes) > 0 {
		fmt.Fprintf(&output, "sso_registration_scopes = %s\n", strings.Join(aws.SSORegistrationScopes, " "))
	}
	return []byte(output.String()), nil
}
