package tobari

import (
	"fmt"
	"regexp"
	"strconv"
)

const (
	ServiceExposureSchema             = 1
	ServiceAttachmentServicesTargetID = "current-attachment-services"
	ServicePortMinimum                = 1024
	ServicePortMaximum                = 65535
	ServiceStatePending               = "pending"
	ServiceStateDenied                = "denied"
	ServiceStateWithdrawn             = "withdrawn"
	ServiceStateListening             = "listening"
	ServiceStateRelaying              = "relaying"
	ServiceStateUnavailable           = "workspace_unavailable"
)

var (
	serviceRequestIDPattern  = regexp.MustCompile(`^srq_[0-9a-f]{32}$`)
	serviceExposureIDPattern = regexp.MustCompile(`^exp_[0-9a-f]{32}$`)
)

func ValidateServicePort(port int) error {
	if port < ServicePortMinimum || port > ServicePortMaximum {
		return fmt.Errorf("Workspace service port must be between %d and %d", ServicePortMinimum, ServicePortMaximum)
	}
	return nil
}

func ValidateServiceRequestID(id string) error {
	if !serviceRequestIDPattern.MatchString(id) {
		return fmt.Errorf("service request reference is invalid")
	}
	return nil
}

func ValidateServiceExposureID(id string) error {
	if !serviceExposureIDPattern.MatchString(id) {
		return fmt.Errorf("service exposure reference is invalid")
	}
	return nil
}

type ServiceRequest struct {
	SchemaVersion       int    `json:"schema_version"`
	ID                  string `json:"id"`
	AttachmentID        string `json:"attachment_id"`
	ProjectID           string `json:"workspace_id"`
	WorkspaceManifestID string `json:"workspace_manifest_id"`
	Workspace           string `json:"workspace"`
	TargetPort          int    `json:"target_port"`
	State               string `json:"state"`
}

func (r ServiceRequest) Validate() error {
	if r.SchemaVersion != ServiceExposureSchema || ValidateServiceRequestID(r.ID) != nil ||
		!attachmentEpochPattern.MatchString(r.AttachmentID) || !projectIDPattern.MatchString(r.ProjectID) ||
		!contextIDPattern.MatchString(r.WorkspaceManifestID) || r.Workspace == "" || ValidateServicePort(r.TargetPort) != nil {
		return fmt.Errorf("service request is invalid")
	}
	switch r.State {
	case ServiceStatePending, ServiceStateDenied, ServiceStateWithdrawn:
		return nil
	default:
		return fmt.Errorf("service request state is invalid")
	}
}

type ServiceExposure struct {
	SchemaVersion       int    `json:"schema_version"`
	ID                  string `json:"id"`
	RequestID           string `json:"request_id"`
	AttachmentID        string `json:"attachment_id"`
	ProjectID           string `json:"workspace_id"`
	WorkspaceManifestID string `json:"workspace_manifest_id"`
	Workspace           string `json:"workspace"`
	TargetPort          int    `json:"target_port"`
	HostPort            int    `json:"host_port"`
	URL                 string `json:"url"`
	State               string `json:"state"`
	Connections         int    `json:"connections"`
}

func (e ServiceExposure) Validate() error {
	if e.SchemaVersion != ServiceExposureSchema || ValidateServiceExposureID(e.ID) != nil ||
		ValidateServiceRequestID(e.RequestID) != nil || !attachmentEpochPattern.MatchString(e.AttachmentID) ||
		!projectIDPattern.MatchString(e.ProjectID) || !contextIDPattern.MatchString(e.WorkspaceManifestID) ||
		e.Workspace == "" || ValidateServicePort(e.TargetPort) != nil || e.HostPort < 1 || e.HostPort > 65535 ||
		e.URL != "http://127.0.0.1:"+strconv.Itoa(e.HostPort) || e.Connections < 0 {
		return fmt.Errorf("service exposure is invalid")
	}
	switch e.State {
	case ServiceStateListening, ServiceStateRelaying, ServiceStateUnavailable:
		return nil
	default:
		return fmt.Errorf("service exposure state is invalid")
	}
}

type ServiceRequestList struct {
	Scope    string           `json:"scope"`
	Requests []ServiceRequest `json:"requests"`
}

func (l ServiceRequestList) Validate() error {
	if l.Scope != "live_attachments" || l.Requests == nil {
		return fmt.Errorf("service request list is invalid")
	}
	seen := map[string]struct{}{}
	for _, request := range l.Requests {
		if request.Validate() != nil || request.State != ServiceStatePending {
			return fmt.Errorf("service request list contains an invalid request")
		}
		if _, exists := seen[request.ID]; exists {
			return fmt.Errorf("service request list contains duplicate references")
		}
		seen[request.ID] = struct{}{}
	}
	return nil
}

type ServiceExposureList struct {
	AttachmentID string            `json:"attachment_id"`
	Exposures    []ServiceExposure `json:"exposures"`
}

func (l ServiceExposureList) Validate() error {
	if !attachmentEpochPattern.MatchString(l.AttachmentID) || l.Exposures == nil {
		return fmt.Errorf("service exposure list is invalid")
	}
	seen := map[string]struct{}{}
	for _, exposure := range l.Exposures {
		if exposure.Validate() != nil || exposure.AttachmentID != l.AttachmentID {
			return fmt.Errorf("service exposure list contains an invalid exposure")
		}
		if _, exists := seen[exposure.ID]; exists {
			return fmt.Errorf("service exposure list contains duplicate references")
		}
		seen[exposure.ID] = struct{}{}
	}
	return nil
}
