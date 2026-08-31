package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type serviceOwnerAnchor struct {
	value   string
	records []serviceRendezvousRecord
}

func serviceOwnerRowsMatch(record serviceRendezvousRecord, response serviceRendezvousResponse) bool {
	for _, request := range response.Requests {
		if request.Validate() != nil || request.State != tobari.ServiceStatePending ||
			request.AttachmentID != record.AttachmentID || request.ContextID != record.ContextID || request.WorkspaceID != record.WorkspaceID ||
			request.Context != record.Context || request.ProjectRoot != record.ProjectRoot {
			return false
		}
	}
	for _, exposure := range response.Exposures {
		if exposure.Validate() != nil || exposure.AttachmentID != record.AttachmentID ||
			exposure.ContextID != record.ContextID || exposure.WorkspaceID != record.WorkspaceID ||
			exposure.Context != record.Context || exposure.ProjectRoot != record.ProjectRoot {
			return false
		}
	}
	if response.Exposure != nil {
		exposure := *response.Exposure
		if exposure.Validate() != nil || exposure.AttachmentID != record.AttachmentID ||
			exposure.ContextID != record.ContextID || exposure.WorkspaceID != record.WorkspaceID ||
			exposure.Context != record.Context || exposure.ProjectRoot != record.ProjectRoot {
			return false
		}
	}
	return true
}

func validServiceRendezvousRecord(entry string, record serviceRendezvousRecord) bool {
	contextID, contextErr := tobari.ParseContextID(record.ContextID)
	workspaceID, workspaceErr := tobari.ParseWorkspaceID(record.WorkspaceID)
	return record.SchemaVersion == 1 && record.OwnerPID > 0 && record.OwnerUID == os.Getuid() && len(record.Nonce) == 64 &&
		entry == record.AttachmentID+".json" && record.SocketName == "owner-"+record.Nonce[:32]+".sock" &&
		!strings.ContainsAny(record.SocketName, `/\`) && tobari.ValidateAttachmentEpochID(record.AttachmentID) == nil &&
		contextErr == nil && workspaceErr == nil && contextID.Validate() == nil && workspaceID.Validate() == nil &&
		tobari.ValidateName(record.Context) == nil && tobari.ValidateCanonicalRoot(record.ProjectRoot) == nil
}

func readExactServiceOwnerRecord(path string, entry os.DirEntry) (serviceRendezvousRecord, error) {
	before, err := os.Lstat(path)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 || before.Size() > workspaceServiceMessageLimit {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner record is unsafe")
	}
	if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner registry entry is unsafe")
	}
	opened, err := os.Open(path) // #nosec G304 -- path is one bounded anchored registry entry and identity is verified before decoding.
	if err != nil {
		return serviceRendezvousRecord{}, err
	}
	defer opened.Close()
	openedInfo, err := opened.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm() != 0o600 || !os.SameFile(before, openedInfo) {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner record changed before read")
	}
	data, err := io.ReadAll(io.LimitReader(opened, workspaceServiceMessageLimit+1))
	if err != nil || len(data) > workspaceServiceMessageLimit {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner record exceeds its bound")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || after.Mode() != before.Mode() {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner record changed during read")
	}
	var record serviceRendezvousRecord
	if decodeStrictJSON(data, &record) != nil || !validServiceRendezvousRecord(entry.Name(), record) {
		return serviceRendezvousRecord{}, fmt.Errorf("service owner record is invalid")
	}
	return record, nil
}

// anchorServiceOwners takes one immutable directory-entry anchor. Reads never
// remove or repair records; an unsafe or contradictory owner fails the task.
func (r *Runtime) anchorServiceOwners(ctx context.Context) (serviceOwnerAnchor, error) {
	if err := ctx.Err(); err != nil {
		return serviceOwnerAnchor{}, err
	}
	anchor, err := serviceEntropy("", 32)
	if err != nil {
		return serviceOwnerAnchor{}, err
	}
	directory := r.serviceExposureLiveDirectory()
	if err := requirePrivateDirectory(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return serviceOwnerAnchor{value: anchor, records: []serviceRendezvousRecord{}}, nil
		}
		return serviceOwnerAnchor{}, fmt.Errorf("inspect service owner registry: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return serviceOwnerAnchor{}, fmt.Errorf("read service owner registry: %w", err)
	}
	result := serviceOwnerAnchor{value: anchor, records: make([]serviceRendezvousRecord, 0, len(entries))}
	attachments, sockets, nonces := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || filepath.Base(entry.Name()) != entry.Name() {
			return serviceOwnerAnchor{}, fault.New(fault.KindContract, "unsafe_service_owner", "service owner registry contains an unsafe entry", false)
		}
		record, err := readExactServiceOwnerRecord(filepath.Join(directory, entry.Name()), entry)
		if err != nil {
			return serviceOwnerAnchor{}, fault.Wrap(fault.KindContract, "unsafe_service_owner", "service owner registry is unsafe", false, err)
		}
		if _, duplicate := attachments[record.AttachmentID]; duplicate {
			return serviceOwnerAnchor{}, fault.New(fault.KindContract, "duplicate_service_owner", "service attachment authority is duplicated", false)
		}
		if _, duplicate := sockets[record.SocketName]; duplicate {
			return serviceOwnerAnchor{}, fault.New(fault.KindContract, "duplicate_service_owner", "service owner endpoint is duplicated", false)
		}
		if _, duplicate := nonces[record.Nonce]; duplicate {
			return serviceOwnerAnchor{}, fault.New(fault.KindContract, "duplicate_service_owner", "service owner nonce is duplicated", false)
		}
		attachments[record.AttachmentID], sockets[record.SocketName], nonces[record.Nonce] = struct{}{}, struct{}{}, struct{}{}
		result.records = append(result.records, record)
	}
	sort.Slice(result.records, func(i, j int) bool { return result.records[i].AttachmentID < result.records[j].AttachmentID })
	return result, nil
}

func (r *Runtime) callServiceOwner(ctx context.Context, record serviceRendezvousRecord, request serviceRendezvousRequest) (serviceRendezvousResponse, error) {
	if !validServiceRendezvousRecord(record.AttachmentID+".json", record) {
		return serviceRendezvousResponse{}, errors.New("invalid service owner record")
	}
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", filepath.Join(r.serviceExposureSocketDirectory(), record.SocketName))
	if err != nil {
		return serviceRendezvousResponse{}, err
	}
	defer connection.Close()
	peerPID, peerUID, err := servicePeerIdentity(connection.(*net.UnixConn))
	if err != nil || peerPID != record.OwnerPID || (peerUID >= 0 && peerUID != record.OwnerUID) {
		return serviceRendezvousResponse{}, errors.New("service owner peer identity mismatch")
	}
	_ = connection.SetDeadline(time.Now().Add(workspaceServiceSetupTimeout))
	request.SchemaVersion = 1
	request.AttachmentID = record.AttachmentID
	request.Nonce = record.Nonce
	request.ReviewerPID = os.Getpid()
	encoded, _ := json.Marshal(request)
	if _, err = connection.Write(append(encoded, '\n')); err != nil {
		return serviceRendezvousResponse{}, err
	}
	line, err := bufio.NewReader(io.LimitReader(connection, workspaceServiceMessageLimit+1)).ReadBytes('\n')
	if err != nil || len(line) > workspaceServiceMessageLimit {
		return serviceRendezvousResponse{}, errors.New("invalid service owner response")
	}
	var response serviceRendezvousResponse
	if decodeStrictJSON(bytes.TrimSuffix(line, []byte{'\n'}), &response) != nil || response.SchemaVersion != 1 || !response.OK ||
		response.AttachmentID != record.AttachmentID || response.ContextID != record.ContextID || response.WorkspaceID != record.WorkspaceID ||
		response.Requests == nil || response.Exposures == nil || !serviceOwnerRowsMatch(record, response) {
		return response, errors.New("service owner rejected action or contradicted its identity")
	}
	return response, nil
}

func observationFor(anchor string, observed, unavailable int) tobari.ServiceOwnerObservation {
	state := tobari.ServiceObservationComplete
	if unavailable > 0 && observed > 0 {
		state = tobari.ServiceObservationPartial
	} else if unavailable > 0 {
		state = tobari.ServiceObservationUnavailable
	}
	return tobari.ServiceOwnerObservation{Scope: tobari.ServiceHostScope, Anchor: anchor, Coverage: tobari.ServiceBoundedWindow, Observation: state, ObservedOwnerCount: observed, UnavailableOwnerCount: unavailable}
}

func (r *Runtime) collectServiceStatus(ctx context.Context) (tobari.ServiceStatusSnapshot, error) {
	anchor, err := r.anchorServiceOwners(ctx)
	if err != nil {
		return tobari.ServiceStatusSnapshot{}, err
	}
	requests, exposures := []tobari.ServiceRequest{}, []tobari.ServiceExposure{}
	observed, unavailable := 0, 0
	for _, record := range anchor.records {
		callContext, cancel := context.WithTimeout(ctx, workspaceServiceSetupTimeout)
		response, callErr := r.callServiceOwner(callContext, record, serviceRendezvousRequest{Operation: "snapshot"})
		cancel()
		if callErr != nil {
			unavailable++
			continue
		}
		observed++
		requests = append(requests, response.Requests...)
		exposures = append(exposures, response.Exposures...)
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
	sort.Slice(exposures, func(i, j int) bool { return exposures[i].ID < exposures[j].ID })
	result := tobari.ServiceStatusSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observationFor(anchor.value, observed, unavailable), Requests: requests, Exposures: exposures}
	if err := result.Validate(); err != nil {
		return tobari.ServiceStatusSnapshot{}, fault.Wrap(fault.KindContract, "invalid_service_status", "service owner snapshot is contradictory", false, err)
	}
	return result, nil
}

func (r *Runtime) ReviewServiceRequests(ctx context.Context) (tobari.ServiceReviewSnapshot, error) {
	status, err := r.collectServiceStatus(ctx)
	if err != nil {
		return tobari.ServiceReviewSnapshot{}, err
	}
	result := tobari.ServiceReviewSnapshot{SchemaVersion: 1, ServiceOwnerObservation: status.ServiceOwnerObservation, Requests: status.Requests}
	return result, result.Validate()
}

func (r *Runtime) ServiceStatus(ctx context.Context) (tobari.ServiceStatusSnapshot, error) {
	return r.collectServiceStatus(ctx)
}

// ObserveStatusServices returns only counts for one exact selected Workspace.
// It never returns a request/exposure reference, URL, or port and contacts at
// most the one matching live owner from the bounded registry anchor.
func (r *Runtime) ObserveStatusServices(ctx context.Context, contextID tobari.ContextID, workspaceID tobari.WorkspaceID) (tobari.ServiceSummary, error) {
	if contextID.Validate() != nil || workspaceID.Validate() != nil {
		return tobari.ServiceSummary{}, fmt.Errorf("status Service scope is invalid")
	}
	anchor, err := r.anchorServiceOwners(ctx)
	if err != nil {
		return tobari.ServiceSummary{}, err
	}
	status := tobari.ServiceStatusSnapshot{SchemaVersion: 1, ServiceOwnerObservation: observationFor(anchor.value, 0, 0), Requests: []tobari.ServiceRequest{}, Exposures: []tobari.ServiceExposure{}}
	var selected *serviceRendezvousRecord
	for _, record := range anchor.records {
		if record.ContextID != string(contextID) || record.WorkspaceID != string(workspaceID) {
			continue
		}
		if selected != nil {
			return tobari.ServiceSummary{}, fmt.Errorf("status Service owner scope is ambiguous")
		}
		copy := record
		selected = &copy
	}
	if selected != nil {
		callContext, cancel := context.WithTimeout(ctx, workspaceServiceSetupTimeout)
		response, callErr := r.callServiceOwner(callContext, *selected, serviceRendezvousRequest{Operation: "snapshot"})
		cancel()
		if callErr != nil {
			status.ServiceOwnerObservation = observationFor(anchor.value, 0, 1)
		} else {
			status.ServiceOwnerObservation = observationFor(anchor.value, 1, 0)
			status.Requests, status.Exposures = response.Requests, response.Exposures
		}
	}
	if err := status.Validate(); err != nil {
		return tobari.ServiceSummary{}, err
	}
	return status.SummaryFor(contextID, workspaceID)
}

func (r *Runtime) resolveServiceRequestOwner(ctx context.Context, id string) (serviceRendezvousRecord, error) {
	if tobari.ValidateServiceRequestID(id) != nil {
		return serviceRendezvousRecord{}, fault.New(fault.KindInvalidInput, "invalid_service_request", "service request reference is invalid", false)
	}
	anchor, err := r.anchorServiceOwners(ctx)
	if err != nil {
		return serviceRendezvousRecord{}, err
	}
	var matched *serviceRendezvousRecord
	for _, record := range anchor.records {
		response, callErr := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "snapshot"})
		if callErr != nil {
			return serviceRendezvousRecord{}, fault.Wrap(fault.KindUnavailable, "service_observation_incomplete", "service request ownership cannot be resolved exactly", false, callErr)
		}
		for _, request := range response.Requests {
			if request.ID == id {
				if matched != nil {
					return serviceRendezvousRecord{}, fault.New(fault.KindContract, "ambiguous_service_request", "service request reference is not unique", false)
				}
				copy := record
				matched = &copy
			}
		}
	}
	if matched == nil {
		return serviceRendezvousRecord{}, fault.New(fault.KindNotFound, "service_request_not_found", "service request is no longer pending", false)
	}
	return *matched, nil
}

func (r *Runtime) resolveServiceExposureOwner(ctx context.Context, id string) (serviceRendezvousRecord, tobari.ServiceExposure, error) {
	if tobari.ValidateServiceExposureID(id) != nil {
		return serviceRendezvousRecord{}, tobari.ServiceExposure{}, fault.New(fault.KindInvalidInput, "invalid_service_exposure", "service exposure reference is invalid", false)
	}
	anchor, err := r.anchorServiceOwners(ctx)
	if err != nil {
		return serviceRendezvousRecord{}, tobari.ServiceExposure{}, err
	}
	var matched *serviceRendezvousRecord
	var exposure tobari.ServiceExposure
	for _, record := range anchor.records {
		response, callErr := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "snapshot"})
		if callErr != nil {
			return serviceRendezvousRecord{}, tobari.ServiceExposure{}, fault.Wrap(fault.KindUnavailable, "service_observation_incomplete", "service exposure ownership cannot be resolved exactly", false, callErr)
		}
		for _, candidate := range response.Exposures {
			if candidate.ID == id {
				if matched != nil {
					return serviceRendezvousRecord{}, tobari.ServiceExposure{}, fault.New(fault.KindContract, "ambiguous_service_exposure", "service exposure reference is not unique", false)
				}
				copy := record
				matched, exposure = &copy, candidate
			}
		}
	}
	if matched == nil {
		return serviceRendezvousRecord{}, tobari.ServiceExposure{}, fault.New(fault.KindNotFound, "service_exposure_not_found", "service exposure is no longer active", false)
	}
	return *matched, exposure, nil
}

func (r *Runtime) AllowServiceRequest(ctx context.Context, id string) (tobari.ServiceExposure, error) {
	record, err := r.resolveServiceRequestOwner(ctx, id)
	if err != nil {
		return tobari.ServiceExposure{}, err
	}
	response, err := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "allow", RequestID: id})
	if err != nil {
		return tobari.ServiceExposure{}, fault.Wrap(fault.KindRejected, "service_request_stale", "service request changed before approval", false, err)
	}
	if response.Exposure == nil || response.Exposure.RequestID != id || response.Exposure.AttachmentID != record.AttachmentID {
		return tobari.ServiceExposure{}, fault.New(fault.KindContract, "invalid_service_exposure_result", "service owner returned contradictory exposure authority", false)
	}
	return *response.Exposure, nil
}

func (r *Runtime) DenyServiceRequest(ctx context.Context, id string) error {
	record, err := r.resolveServiceRequestOwner(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "deny", RequestID: id})
	if err != nil {
		return fault.Wrap(fault.KindRejected, "service_request_stale", "service request changed before denial", false, err)
	}
	return nil
}

func (r *Runtime) StopServiceExposure(ctx context.Context, id string) error {
	record, _, err := r.resolveServiceExposureOwner(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "stop", ExposureID: id})
	if err != nil {
		return fault.Wrap(fault.KindRejected, "service_exposure_stale", "service exposure changed before stop completed", false, err)
	}
	return nil
}

func (r *Runtime) OpenServiceExposure(ctx context.Context, id string) (tobari.ServiceOpenResult, error) {
	_, exposure, err := r.resolveServiceExposureOwner(ctx, id)
	if err != nil {
		return tobari.ServiceOpenResult{}, err
	}
	dispatchContext, cancel := context.WithTimeout(ctx, workspaceServiceSetupTimeout)
	defer cancel()
	outcome := r.dispatchServiceExposureBrowser(dispatchContext, exposure.URL)
	result := tobari.ServiceOpenResult{SchemaVersion: 1, ID: exposure.ID, URL: exposure.URL, Outcome: outcome}
	return result, result.Validate()
}

// Host composition cannot originate an attachment-local helper request or
// inspect the helper's current-attachment-only status.
func (*Runtime) RequestService(context.Context, int) (tobari.ServiceExposure, error) {
	return tobari.ServiceExposure{}, errors.New("service requests originate inside an attached Workspace")
}
func (*Runtime) AttachmentServiceStatus(context.Context) (tobari.ServiceAttachmentStatus, error) {
	return tobari.ServiceAttachmentStatus{}, errors.New("service attachment status is available only inside its Workspace")
}
