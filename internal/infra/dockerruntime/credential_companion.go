package dockerruntime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/infra/companionruntime"
)

const (
	credentialCompanionStopTimeout = 5 * time.Second
	credentialCompanionPollDelay   = 250 * time.Millisecond
	credentialCompanionPollCount   = 60
)

type credentialCompanionContainer struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	User      string `json:"user"`
}

func (r *Runtime) startCredentialCompanion(ctx context.Context, rootKey []byte) error {
	if ctx == nil || r.companion == nil || r.companionEntropy == nil || len(rootKey) != 32 {
		return credentialCompanionFault(companionruntime.ErrUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	container, err := r.inspectCredentialCompanionContainer(ctx)
	if err != nil {
		return credentialCompanionFault(err)
	}
	uid, gid := currentIDs()
	bootstrap, err := companionruntime.NewBootstrap(
		r.companionEntropy, rootKey, container.ID, uid, gid, r.stateDirectory,
	)
	if err != nil {
		return credentialCompanionFault(err)
	}
	defer bootstrap.Clear()
	response, err := r.runBrokerControl(
		ctx, nil, "companion_prepare", "--epoch-id", bootstrap.EpochID(),
	)
	if err != nil || response.State != "prepared" || response.EpochID != bootstrap.EpochID() {
		if err == nil {
			err = companionruntime.ErrProtocol
		}
		return credentialCompanionFault(err)
	}
	stopContext, stopCancel := context.WithTimeout(ctx, credentialCompanionStopTimeout)
	err = r.companion.WaitForStopped(stopContext, r.stateDirectory)
	stopCancel()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return credentialCompanionFault(err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	process, err := r.companion.Start(bootstrap)
	if err != nil || process == nil {
		if err == nil {
			err = companionruntime.ErrUnavailable
		}
		return credentialCompanionFault(err)
	}
	detached := false
	defer func() {
		if !detached {
			_ = process.Abort()
		}
	}()
	if err := r.waitForCredentialCompanionReady(ctx, bootstrap.EpochID()); err != nil {
		return err
	}
	if err := process.Detach(); err != nil {
		return credentialCompanionFault(err)
	}
	detached = true
	return nil
}

func (r *Runtime) inspectCredentialCompanionContainer(
	ctx context.Context,
) (credentialCompanionContainer, error) {
	uid, gid := currentIDs()
	expectedUser := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	output, err := r.runner.Output(
		ctx,
		[]string{
			"inspect", "--format",
			`{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},"component":{{json (index .Config.Labels "` + componentLabel + `")}},"user":{{json .Config.User}}}`,
			authBrokerContainer,
		},
		os.Environ(),
	)
	if err != nil {
		return credentialCompanionContainer{}, fmt.Errorf("inspect Auth Broker identity: %w: %s", err, boundedDiagnostic(output))
	}
	var observed credentialCompanionContainer
	if err := decodeStrictJSON(bytes.TrimSpace(output), &observed); err != nil ||
		!validDockerContainerID(observed.ID) || observed.Owner != ownerValue ||
		observed.Component != "auth-broker" || observed.User != expectedUser {
		return credentialCompanionContainer{}, fmt.Errorf("Auth Broker identity is invalid")
	}
	return observed, nil
}

func validDockerContainerID(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, current := range []byte(value) {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

func (r *Runtime) waitForCredentialCompanionReady(ctx context.Context, expectedEpoch string) error {
	if !companionruntime.ValidEpochID(expectedEpoch) {
		return credentialCompanionFault(companionruntime.ErrProtocol)
	}
	for attempt := 0; attempt < credentialCompanionPollCount; attempt++ {
		state, epoch, err := r.credentialCompanionStatus(ctx)
		if err == nil {
			switch {
			case state == "ready" && epoch == expectedEpoch:
				return nil
			case state == "prepared" && epoch == expectedEpoch:
			case state == "absent" || epoch != expectedEpoch:
				return credentialCompanionFault(companionruntime.ErrProtocol)
			default:
				return credentialCompanionFault(companionruntime.ErrProtocol)
			}
		}
		timer := time.NewTimer(credentialCompanionPollDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return credentialCompanionFault(fmt.Errorf("credential companion readiness timed out"))
}

func (r *Runtime) credentialCompanionStatus(ctx context.Context) (string, string, error) {
	response, err := r.runBrokerControl(ctx, nil, "companion_status")
	if err != nil {
		return "", "", err
	}
	return response.State, response.EpochID, nil
}

func (r *Runtime) waitForCredentialCompanionStopped(ctx context.Context) error {
	if r.companion == nil {
		return credentialCompanionFault(companionruntime.ErrUnavailable)
	}
	stopContext, cancel := context.WithTimeout(ctx, credentialCompanionStopTimeout)
	defer cancel()
	if err := r.companion.WaitForStopped(stopContext, r.stateDirectory); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return credentialCompanionFault(err)
	}
	return nil
}

func credentialCompanionFault(err error) error {
	if err == nil {
		err = companionruntime.ErrUnavailable
	}
	return fault.Wrap(
		fault.KindUnavailable,
		"credential_companion_unavailable",
		"The trusted-host credential companion is unavailable.",
		true,
		err,
		fault.NextAction{
			Command: "cluster up",
			Reason:  "Reconcile the shared cluster and its Auth Broker companion session.",
		},
	)
}
