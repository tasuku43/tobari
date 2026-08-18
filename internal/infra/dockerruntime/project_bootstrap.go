package dockerruntime

import (
	"encoding/json"
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
	if err := syncDirectory(awsDirectory); err != nil {
		return err
	}
	if snapshot.EKS == nil {
		return nil
	}
	kubeDirectory := filepath.Join(home, ".kube")
	if _, err := os.Lstat(kubeDirectory); err == nil {
		return fmt.Errorf("Workspace Kubernetes bootstrap target already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(kubeDirectory, 0o700); err != nil {
		return err
	}
	if err := requirePrivateDirectory(kubeDirectory); err != nil {
		return err
	}
	kubeconfig, err := encodeProjectEKSConfig(snapshot.AWS.Profile, *snapshot.EKS)
	if err != nil {
		return err
	}
	if err := initializeBytes(filepath.Join(kubeDirectory, "config"), kubeconfig, 0o600); err != nil {
		return err
	}
	return syncDirectory(kubeDirectory)
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

func encodeProjectEKSConfig(awsProfile string, eks tobari.ContextEKSBootstrap) ([]byte, error) {
	if err := eks.Validate(); err != nil {
		return nil, err
	}
	if awsProfile == "" {
		return nil, fmt.Errorf("AWS profile is required for EKS bootstrap")
	}
	type namedCluster struct {
		Name    string `json:"name"`
		Cluster struct {
			CertificateAuthorityData string `json:"certificate-authority-data"`
			Server                   string `json:"server"`
		} `json:"cluster"`
	}
	type namedContext struct {
		Name    string `json:"name"`
		Context struct {
			Cluster   string `json:"cluster"`
			User      string `json:"user"`
			Namespace string `json:"namespace,omitempty"`
		} `json:"context"`
	}
	type namedUser struct {
		Name string `json:"name"`
		User struct {
			Exec struct {
				APIVersion         string              `json:"apiVersion"`
				Args               []string            `json:"args"`
				Command            string              `json:"command"`
				Env                []map[string]string `json:"env"`
				InteractiveMode    string              `json:"interactiveMode"`
				ProvideClusterInfo bool                `json:"provideClusterInfo"`
			} `json:"exec"`
		} `json:"user"`
	}
	cluster := namedCluster{Name: eks.ContextName}
	cluster.Cluster.CertificateAuthorityData = eks.CertificateAuthorityData
	cluster.Cluster.Server = eks.Server
	contextEntry := namedContext{Name: eks.ContextName}
	contextEntry.Context.Cluster = eks.ContextName
	contextEntry.Context.User = eks.ContextName
	contextEntry.Context.Namespace = eks.Namespace
	user := namedUser{Name: eks.ContextName}
	user.User.Exec.APIVersion = "client.authentication.k8s.io/v1beta1"
	user.User.Exec.Args = []string{"--region", eks.Region, "eks", "get-token", "--cluster-name", eks.ClusterName, "--output", "json"}
	user.User.Exec.Command = "aws"
	user.User.Exec.Env = []map[string]string{{"name": "AWS_PROFILE", "value": awsProfile}}
	user.User.Exec.InteractiveMode = "IfAvailable"
	config := struct {
		APIVersion     string         `json:"apiVersion"`
		Kind           string         `json:"kind"`
		Preferences    map[string]any `json:"preferences"`
		Clusters       []namedCluster `json:"clusters"`
		Contexts       []namedContext `json:"contexts"`
		Users          []namedUser    `json:"users"`
		CurrentContext string         `json:"current-context"`
	}{APIVersion: "v1", Kind: "Config", Preferences: map[string]any{}, Clusters: []namedCluster{cluster}, Contexts: []namedContext{contextEntry}, Users: []namedUser{user}, CurrentContext: eks.ContextName}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
