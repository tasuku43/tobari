package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

const (
	authBrokerAPIKey  = "io.tobari.auth-broker-api" // #nosec G101 -- stable image-contract label key, not a credential.
	authBrokerRoleKey = "io.tobari.auth-broker-role"
	authBrokerRole    = "credential-resolution"
)

type authBrokerImageMetadata struct {
	ID           string   `json:"Id"`
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	OS           string   `json:"Os"`
	Config       struct {
		User       string                     `json:"User"`
		Labels     map[string]string          `json:"Labels"`
		Entrypoint []string                   `json:"Entrypoint"`
		Volumes    map[string]json.RawMessage `json:"Volumes"`
	} `json:"Config"`
}

func (r *Runtime) prepareAuthBrokerImage(ctx context.Context) (string, error) {
	selection, err := r.selectAuthBrokerImage(ctx)
	if err != nil {
		return "", err
	}
	identity, err := r.verifyAuthBrokerImage(ctx, selection.Image, selection.RequireDigest)
	if err != nil {
		return "", err
	}
	return identity, nil
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

func (r *Runtime) verifyAuthBrokerImage(ctx context.Context, image string, requireDigest bool) (string, error) {
	if requireDigest {
		if err := validateAuthBrokerImageReference(image); err != nil {
			return "", invalidAuthBrokerImageReference(err)
		}
	}

	metadata, err := r.inspectAuthBrokerImage(ctx, image)
	if errors.Is(err, errComponentImageMissing) && requireDigest {
		if pullErr := r.pullComponentImage(ctx, image); pullErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			return "", fault.Wrap(
				fault.KindUnavailable, "auth_broker_image_unavailable",
				"The verified Auth Broker image could not be obtained; check Docker registry access and retry.", true, pullErr,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker and registry access."},
				fault.NextAction{Command: "cluster up", Reason: "Retry the shared cluster after the image is available."},
			)
		}
		metadata, err = r.inspectAuthBrokerImage(ctx, image)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		var observationError componentImageObservationError
		if !requireDigest || errors.As(err, &observationError) {
			return "", fault.Wrap(
				fault.KindUnavailable, "auth_broker_image_unavailable",
				"The selected Auth Broker image could not be inspected; build the development image and retry.", true, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker image availability."},
			)
		}
		return "", fault.Wrap(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not satisfy Tobari's verified image contract.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}

	if requireDigest && !hasDigest(metadata.RepoDigests, image) {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image did not resolve to the configured immutable digest.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	labels := metadata.Config.Labels
	selectedAPI := parseAPIOrZero(labels[authBrokerAPIKey])
	if selectedAPI != buildidentity.RequiredAuthBrokerAPI {
		return "", r.incompatibleComponentAPI("Auth Broker", selectedAPI, buildidentity.RequiredAuthBrokerAPI, "auth_broker_image_incompatible")
	}
	if labels[authBrokerRoleKey] != authBrokerRole {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not declare Tobari's credential-resolution API contract.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	if isRootImageUser(metadata.Config.User) {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image must declare a non-root default user.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
		)
	}
	if len(metadata.Config.Entrypoint) != 1 || metadata.Config.Entrypoint[0] != "/opt/tobari/entrypoint.sh" {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not declare Tobari's locked broker entrypoint.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Auth Broker image entrypoint contract."},
		)
	}
	if err := validateComponentImageVolumes(metadata.Config.Volumes); err != nil {
		return "", fault.Wrap(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image declares a writable volume outside Tobari's reviewed mount closure.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image volume contract."},
		)
	}
	if !imageIDPattern.MatchString(metadata.ID) {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image does not expose a stable content identity.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image contract."},
		)
	}

	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fault.Wrap(
			fault.KindUnavailable, "auth_broker_image_unavailable",
			"Docker Engine platform information could not be read before Auth Broker startup.", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine readiness."},
		)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS == "" || imageArch == "" || serverOS == "" || serverArch == "" || imageOS != serverOS || imageArch != serverArch {
		return "", fault.New(
			fault.KindContract, "auth_broker_image_incompatible",
			"The Auth Broker image architecture does not match the Docker Engine.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine and Auth Broker image platforms."},
		)
	}
	return metadata.ID, nil
}

func invalidAuthBrokerImageReference(cause error) error {
	return fault.Wrap(
		fault.KindContract, "auth_broker_image_incompatible",
		"The configured Auth Broker image is not an immutable digest reference.", false, cause,
		fault.NextAction{Command: "doctor", Reason: "Inspect the installed Auth Broker image configuration."},
	)
}

func (r *Runtime) inspectAuthBrokerImage(ctx context.Context, image string) (authBrokerImageMetadata, error) {
	observed, err := r.inspectBoundedComponentImage(ctx, image)
	if err != nil {
		return authBrokerImageMetadata{}, err
	}
	var metadata authBrokerImageMetadata
	metadata.ID = observed.ID
	metadata.RepoDigests = observed.RepoDigests
	metadata.Architecture = observed.Architecture
	metadata.OS = observed.OS
	metadata.Config.User = observed.Config.User
	metadata.Config.Labels = observed.Config.Labels
	metadata.Config.Entrypoint = observed.Config.Entrypoint
	metadata.Config.Volumes = observed.Config.Volumes
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
