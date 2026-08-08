package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const (
	authBrokerAPIKey  = "io.tobari.auth-broker-api" // #nosec G101 -- stable image-contract label key, not a credential.
	authBrokerRoleKey = "io.tobari.auth-broker-role"
	authBrokerAPI     = "1"
	authBrokerRole    = "credential-resolution"
)

type authBrokerImageMetadata struct {
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	OS           string   `json:"Os"`
	Config       struct {
		User       string            `json:"User"`
		Labels     map[string]string `json:"Labels"`
		Entrypoint []string          `json:"Entrypoint"`
	} `json:"Config"`
}

func (r *Runtime) prepareAuthBrokerImage(ctx context.Context) (string, error) {
	selection, err := r.selectAuthBrokerImage(ctx)
	if err != nil {
		return "", err
	}
	if err := r.verifyAuthBrokerImage(ctx, selection.Image, selection.RequireDigest); err != nil {
		return "", err
	}
	return selection.Image, nil
}

func (r *Runtime) selectAuthBrokerImage(ctx context.Context) (sharedImageSelection, error) {
	selection, err := r.imageResolver().AuthBrokerImage(ctx, r)
	if err != nil {
		return sharedImageSelection{}, err
	}
	if selection.RequireDigest {
		if err := validateAuthBrokerImageReference(selection.Image); err != nil {
			return sharedImageSelection{}, invalidAuthBrokerImageReference(err)
		}
	}
	return selection, nil
}

func (r *Runtime) verifyAuthBrokerImage(ctx context.Context, image string, requireDigest bool) error {
	if requireDigest {
		if err := validateAuthBrokerImageReference(image); err != nil {
			return invalidAuthBrokerImageReference(err)
		}
	}

	metadata, err := r.inspectAuthBrokerImage(ctx, image)
	if err != nil && requireDigest {
		if _, pullErr := r.runner.Output(ctx, []string{"pull", image}, os.Environ()); pullErr != nil {
			return fault.Wrap(
				fault.KindUnavailable, "auth_broker_image_unavailable",
				"The verified Auth Broker image could not be obtained; check Docker registry access and retry.", true, pullErr,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker and registry access."},
				fault.NextAction{Command: "cluster up", Reason: "Retry the shared cluster after the image is available."},
			)
		}
		metadata, err = r.inspectAuthBrokerImage(ctx, image)
	}
	if err != nil {
		if !requireDigest {
			return fault.Wrap(
				fault.KindUnavailable, "auth_broker_image_unavailable",
				"The selected Auth Broker image could not be inspected; build the development image and retry.", true, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker image availability."},
			)
		}
		return fault.Wrap(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not satisfy Tobari's verified image contract.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}

	if requireDigest && !hasDigest(metadata.RepoDigests, image) {
		return fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image did not resolve to the configured immutable digest.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	labels := metadata.Config.Labels
	if labels[authBrokerAPIKey] != authBrokerAPI || labels[authBrokerRoleKey] != authBrokerRole {
		return fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not declare Tobari's credential-resolution API contract.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	if isRootImageUser(metadata.Config.User) {
		return fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image must declare a non-root default user.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	if len(metadata.Config.Entrypoint) != 1 || metadata.Config.Entrypoint[0] != "/opt/tobari/entrypoint.sh" {
		return fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not declare Tobari's locked broker entrypoint.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Auth Broker image entrypoint contract."},
		)
	}

	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "auth_broker_image_unavailable",
			"Docker Engine platform information could not be read before Auth Broker startup.", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine readiness."},
		)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS == "" || imageArch == "" || serverOS == "" || serverArch == "" || imageOS != serverOS || imageArch != serverArch {
		return fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image architecture does not match the Docker Engine.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine and Auth Broker image platforms."},
		)
	}
	return nil
}

func invalidAuthBrokerImageReference(cause error) error {
	return fault.Wrap(
		fault.KindContract, "auth_broker_image_incompatible",
		"The configured Auth Broker image is not an immutable digest reference.", false, cause,
		fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
	)
}

func (r *Runtime) inspectAuthBrokerImage(ctx context.Context, image string) (authBrokerImageMetadata, error) {
	output, err := r.runner.Output(
		ctx, []string{"image", "inspect", "--format", "{{json .}}", image}, os.Environ(),
	)
	if err != nil {
		return authBrokerImageMetadata{}, err
	}
	var metadata authBrokerImageMetadata
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&metadata); err != nil {
		return authBrokerImageMetadata{}, fmt.Errorf("decode Auth Broker image metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return authBrokerImageMetadata{}, fmt.Errorf("Auth Broker image metadata contains trailing data")
	}
	return metadata, nil
}

func validateAuthBrokerImageReference(image string) error {
	name, digest, found := strings.Cut(image, "@sha256:")
	if !found || name == "" || len(digest) != 64 {
		return fmt.Errorf("Auth Broker image reference must contain a 64-character sha256 digest")
	}
	for _, character := range digest {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return fmt.Errorf("Auth Broker image digest contains a non-hex character")
		}
	}
	if strings.Trim(digest, "0") == "" {
		return fmt.Errorf("Auth Broker image digest cannot be all zeroes")
	}
	return nil
}
