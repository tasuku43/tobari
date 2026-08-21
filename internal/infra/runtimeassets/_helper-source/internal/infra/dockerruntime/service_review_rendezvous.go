package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func validServiceRendezvousRecord(entry string, record serviceRendezvousRecord) bool {
	return record.SchemaVersion == 1 && record.OwnerPID > 0 && len(record.Nonce) == 64 &&
		entry == record.AttachmentID+".json" && record.SocketName == "owner-"+record.Nonce[:32]+".sock" &&
		!strings.ContainsAny(record.SocketName, `/\`) && tobari.ValidateAttachmentEpochID(record.AttachmentID) == nil
}

func (r *Runtime) liveServiceRecords(ctx context.Context) []serviceRendezvousRecord {
	directory := r.serviceExposureLiveDirectory()
	entries, err := os.ReadDir(directory)
	if err != nil {
		return []serviceRendezvousRecord{}
	}
	records := []serviceRendezvousRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			_ = os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path) // #nosec G304 -- one lstat-verified regular 0600 entry below the owner-only live-record directory.
		if err != nil || len(data) > workspaceServiceMessageLimit {
			_ = os.Remove(path)
			continue
		}
		var record serviceRendezvousRecord
		if decodeStrictJSON(data, &record) != nil || !validServiceRendezvousRecord(entry.Name(), record) {
			_ = os.Remove(path)
			continue
		}
		probeContext, cancel := context.WithTimeout(ctx, workspaceServiceSetupTimeout)
		_, probeErr := r.callServiceOwner(probeContext, record, serviceRendezvousRequest{Operation: "snapshot"})
		cancel()
		if probeErr != nil {
			_ = os.Remove(path)
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].AttachmentID < records[j].AttachmentID })
	return records
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
	if err != nil || peerPID != record.OwnerPID || (peerUID >= 0 && peerUID != os.Getuid()) {
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
	if decodeStrictJSON(bytes.TrimSuffix(line, []byte{'\n'}), &response) != nil || response.SchemaVersion != 1 || !response.OK {
		return response, errors.New("service owner rejected action")
	}
	return response, nil
}

func (r *Runtime) ListServiceRequests(ctx context.Context) (tobari.ServiceRequestList, error) {
	result := tobari.ServiceRequestList{Scope: "live_attachments", Requests: []tobari.ServiceRequest{}}
	for _, record := range r.liveServiceRecords(ctx) {
		response, err := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "snapshot"})
		if err != nil {
			continue
		}
		result.Requests = append(result.Requests, response.Requests...)
	}
	sort.Slice(result.Requests, func(i, j int) bool { return result.Requests[i].ID < result.Requests[j].ID })
	if err := result.Validate(); err != nil {
		return tobari.ServiceRequestList{}, err
	}
	return result, nil
}

func (r *Runtime) findServiceRequestOwner(ctx context.Context, id string) (serviceRendezvousRecord, error) {
	if tobari.ValidateServiceRequestID(id) != nil {
		return serviceRendezvousRecord{}, fault.New(fault.KindInvalidInput, "invalid_service_request", "service request reference is invalid", false)
	}
	var matched *serviceRendezvousRecord
	for _, record := range r.liveServiceRecords(ctx) {
		response, err := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "snapshot"})
		if err != nil {
			continue
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

func (r *Runtime) AllowServiceRequest(ctx context.Context, id string) (tobari.ServiceExposure, error) {
	record, err := r.findServiceRequestOwner(ctx, id)
	if err != nil {
		return tobari.ServiceExposure{}, err
	}
	response, err := r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "allow", RequestID: id})
	if err != nil {
		return tobari.ServiceExposure{}, fault.Wrap(fault.KindRejected, "service_request_stale", "service request changed before approval", false, err)
	}
	if response.Exposure == nil {
		return tobari.ServiceExposure{}, errors.New("service owner omitted exposure")
	}
	return *response.Exposure, nil
}
func (r *Runtime) DenyServiceRequest(ctx context.Context, id string) error {
	record, err := r.findServiceRequestOwner(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.callServiceOwner(ctx, record, serviceRendezvousRequest{Operation: "deny", RequestID: id})
	if err != nil {
		return fault.Wrap(fault.KindRejected, "service_request_stale", "service request changed before denial", false, err)
	}
	return nil
}

// Host review has no attachment-local helper scope for these operations.
func (*Runtime) RequestService(context.Context, int) (tobari.ServiceExposure, error) {
	return tobari.ServiceExposure{}, errors.New("service requests originate inside an attached Workspace")
}
func (*Runtime) ListServiceExposures(context.Context) (tobari.ServiceExposureList, error) {
	return tobari.ServiceExposureList{}, errors.New("service exposure inventory is attachment-local")
}
func (*Runtime) StopServiceExposure(context.Context, string) error {
	return errors.New("service exposure stop is attachment-local")
}
