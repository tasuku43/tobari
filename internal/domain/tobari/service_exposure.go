package tobari

import (
	"fmt"
	"net/url"
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

	ServiceObservationComplete    ServiceObservationState = "complete"
	ServiceObservationPartial     ServiceObservationState = "partial"
	ServiceObservationUnavailable ServiceObservationState = "unavailable"
	ServiceHostScope              string                  = "live_service_attachments"
	ServiceAttachmentScope        string                  = "current_attachment"
	ServiceBoundedWindow          string                  = "bounded_window"

	ServiceOpenNotDispatched  ServiceOpenOutcome = "open_not_dispatched"
	ServiceOpenRequested      ServiceOpenOutcome = "open_requested"
	ServiceOpenOutcomeUnknown ServiceOpenOutcome = "open_outcome_unknown"
)

var (
	serviceRequestIDPattern    = regexp.MustCompile(`^srq_[0-9a-f]{32}$`)
	serviceExposureIDPattern   = regexp.MustCompile(`^exp_[0-9a-f]{32}$`)
	serviceOriginLabelPattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	serviceSnapshotAnchorRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type ServiceObservationState string
type ServiceOpenOutcome string

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

func validateServiceIdentity(contextID, workspaceID, attachmentID, contextName, projectRoot string) error {
	if _, err := ParseContextID(contextID); err != nil {
		return fmt.Errorf("service Context identity is invalid")
	}
	if _, err := ParseWorkspaceID(workspaceID); err != nil {
		return fmt.Errorf("service Workspace identity is invalid")
	}
	if ValidateAttachmentEpochID(attachmentID) != nil || ValidateName(contextName) != nil || ValidateCanonicalRoot(projectRoot) != nil {
		return fmt.Errorf("service attachment presentation is invalid")
	}
	return nil
}

// ServiceRequest is one attachment-owned request visible to the trusted host.
// Context and Project-root presentation are non-authoritative; exact actions
// bind the opaque request reference back to the live owner.
type ServiceRequest struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	AttachmentID  string `json:"attachment_id"`
	ContextID     string `json:"context_id"`
	WorkspaceID   string `json:"workspace_id"`
	Context       string `json:"context"`
	ProjectRoot   string `json:"project_root"`
	TargetPort    int    `json:"target_port"`
	State         string `json:"state"`
}

func (r ServiceRequest) Validate() error {
	if r.SchemaVersion != ServiceExposureSchema || ValidateServiceRequestID(r.ID) != nil ||
		validateServiceIdentity(r.ContextID, r.WorkspaceID, r.AttachmentID, r.Context, r.ProjectRoot) != nil ||
		ValidateServicePort(r.TargetPort) != nil {
		return fmt.Errorf("service request is invalid")
	}
	switch r.State {
	case ServiceStatePending, ServiceStateDenied, ServiceStateWithdrawn:
		return nil
	default:
		return fmt.Errorf("service request state is invalid")
	}
}

// ServiceExposure is one live listener. URL is access authority while ID is
// lifecycle mutation authority; neither is derived from the other.
type ServiceExposure struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	RequestID     string `json:"request_id"`
	AttachmentID  string `json:"attachment_id"`
	ContextID     string `json:"context_id"`
	WorkspaceID   string `json:"workspace_id"`
	Context       string `json:"context"`
	ProjectRoot   string `json:"project_root"`
	TargetPort    int    `json:"target_port"`
	HostPort      int    `json:"host_port"`
	URL           string `json:"url"`
	State         string `json:"state"`
	Connections   int    `json:"connections"`
}

func serviceExposureAuthority(port int, label string) string {
	return "svc-" + label + ".localhost:" + strconv.Itoa(port)
}

func ServiceExposureURL(port int, label string) (string, error) {
	if port < 1 || port > 65535 || !serviceOriginLabelPattern.MatchString(label) {
		return "", fmt.Errorf("service exposure origin is invalid")
	}
	return "http://" + serviceExposureAuthority(port, label) + "/", nil
}

func ParseServiceExposureURL(value string) (string, int, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "/" || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", 0, fmt.Errorf("service exposure URL is invalid")
	}
	port, err := strconv.Atoi(parsed.Port())
	hostname := parsed.Hostname()
	const prefix, suffix = "svc-", ".localhost"
	if err != nil || port < 1 || port > 65535 || len(hostname) <= len(prefix)+len(suffix) ||
		hostname[:len(prefix)] != prefix || hostname[len(hostname)-len(suffix):] != suffix {
		return "", 0, fmt.Errorf("service exposure URL is invalid")
	}
	label := hostname[len(prefix) : len(hostname)-len(suffix)]
	if !serviceOriginLabelPattern.MatchString(label) || parsed.Host != serviceExposureAuthority(port, label) {
		return "", 0, fmt.Errorf("service exposure URL is invalid")
	}
	return label, port, nil
}

func (e ServiceExposure) Validate() error {
	_, urlPort, urlErr := ParseServiceExposureURL(e.URL)
	if e.SchemaVersion != ServiceExposureSchema || ValidateServiceExposureID(e.ID) != nil ||
		ValidateServiceRequestID(e.RequestID) != nil ||
		validateServiceIdentity(e.ContextID, e.WorkspaceID, e.AttachmentID, e.Context, e.ProjectRoot) != nil ||
		ValidateServicePort(e.TargetPort) != nil || e.HostPort < 1 || e.HostPort > 65535 ||
		urlErr != nil || urlPort != e.HostPort || e.Connections < 0 {
		return fmt.Errorf("service exposure is invalid")
	}
	switch e.State {
	case ServiceStateListening, ServiceStateRelaying, ServiceStateUnavailable:
		return nil
	default:
		return fmt.Errorf("service exposure state is invalid")
	}
}

type ServiceOwnerObservation struct {
	Scope                 string                  `json:"scope"`
	Anchor                string                  `json:"anchor"`
	Coverage              string                  `json:"coverage"`
	Observation           ServiceObservationState `json:"observation"`
	ObservedOwnerCount    int                     `json:"observed_owner_count"`
	UnavailableOwnerCount int                     `json:"unavailable_owner_count"`
}

func (o ServiceOwnerObservation) Validate() error {
	if o.Scope != ServiceHostScope || o.Coverage != ServiceBoundedWindow ||
		!serviceSnapshotAnchorRegex.MatchString(o.Anchor) || o.ObservedOwnerCount < 0 || o.UnavailableOwnerCount < 0 {
		return fmt.Errorf("service owner observation is invalid")
	}
	switch o.Observation {
	case ServiceObservationComplete:
		if o.UnavailableOwnerCount != 0 {
			return fmt.Errorf("complete service observation has unavailable owners")
		}
	case ServiceObservationPartial:
		if o.ObservedOwnerCount == 0 || o.UnavailableOwnerCount == 0 {
			return fmt.Errorf("partial service observation counts are invalid")
		}
	case ServiceObservationUnavailable:
		if o.ObservedOwnerCount != 0 || o.UnavailableOwnerCount == 0 {
			return fmt.Errorf("unavailable service observation counts are invalid")
		}
	default:
		return fmt.Errorf("service observation state is invalid")
	}
	return nil
}

type ServiceReviewSnapshot struct {
	SchemaVersion int `json:"schema_version"`
	ServiceOwnerObservation
	Requests []ServiceRequest `json:"requests"`
}

func (s ServiceReviewSnapshot) Validate() error {
	if s.SchemaVersion != ServiceExposureSchema || s.ServiceOwnerObservation.Validate() != nil || s.Requests == nil {
		return fmt.Errorf("service review snapshot is invalid")
	}
	return validateServiceRows(s.Requests, nil)
}

type ServiceStatusSnapshot struct {
	SchemaVersion int `json:"schema_version"`
	ServiceOwnerObservation
	Requests  []ServiceRequest  `json:"requests"`
	Exposures []ServiceExposure `json:"exposures"`
}

func (s ServiceStatusSnapshot) Validate() error {
	if s.SchemaVersion != ServiceExposureSchema || s.ServiceOwnerObservation.Validate() != nil || s.Requests == nil || s.Exposures == nil {
		return fmt.Errorf("service status snapshot is invalid")
	}
	return validateServiceRows(s.Requests, s.Exposures)
}

func validateServiceRows(requests []ServiceRequest, exposures []ServiceExposure) error {
	requestIDs, exposureIDs := map[string]struct{}{}, map[string]struct{}{}
	for _, request := range requests {
		if request.Validate() != nil || request.State != ServiceStatePending {
			return fmt.Errorf("service snapshot contains an invalid request")
		}
		if _, exists := requestIDs[request.ID]; exists {
			return fmt.Errorf("service snapshot contains duplicate request references")
		}
		requestIDs[request.ID] = struct{}{}
	}
	for _, exposure := range exposures {
		if exposure.Validate() != nil {
			return fmt.Errorf("service snapshot contains an invalid exposure")
		}
		if _, exists := exposureIDs[exposure.ID]; exists {
			return fmt.Errorf("service snapshot contains duplicate exposure references")
		}
		exposureIDs[exposure.ID] = struct{}{}
	}
	return nil
}

type ServicePendingStatus struct {
	TargetPort int    `json:"target_port"`
	State      string `json:"state"`
}

func (s ServicePendingStatus) Validate() error {
	if ValidateServicePort(s.TargetPort) != nil || s.State != ServiceStatePending {
		return fmt.Errorf("service pending status is invalid")
	}
	return nil
}

type ServiceAttachmentStatus struct {
	SchemaVersion int                    `json:"schema_version"`
	Scope         string                 `json:"scope"`
	AttachmentID  string                 `json:"attachment_id"`
	Pending       []ServicePendingStatus `json:"pending"`
	Exposures     []ServiceExposure      `json:"exposures"`
}

func (s ServiceAttachmentStatus) Validate() error {
	if s.SchemaVersion != ServiceExposureSchema || s.Scope != ServiceAttachmentScope ||
		ValidateAttachmentEpochID(s.AttachmentID) != nil || s.Pending == nil || s.Exposures == nil {
		return fmt.Errorf("service attachment status is invalid")
	}
	for _, pending := range s.Pending {
		if pending.Validate() != nil {
			return fmt.Errorf("service attachment status contains invalid pending state")
		}
	}
	for _, exposure := range s.Exposures {
		if exposure.Validate() != nil || exposure.AttachmentID != s.AttachmentID {
			return fmt.Errorf("service attachment status contains invalid exposure")
		}
	}
	return nil
}

type ServiceSummary struct {
	SchemaVersion         int                     `json:"schema_version"`
	Observation           ServiceObservationState `json:"observation"`
	PendingCount          int                     `json:"pending_count"`
	ActiveCount           int                     `json:"active_count"`
	UnavailableOwnerCount int                     `json:"unavailable_owner_count"`
	Attention             bool                    `json:"attention"`
}

func (s ServiceSummary) Validate() error {
	if s.SchemaVersion != ServiceExposureSchema || s.PendingCount < 0 || s.ActiveCount < 0 || s.UnavailableOwnerCount < 0 ||
		(s.Attention != (s.PendingCount > 0 || s.UnavailableOwnerCount > 0)) {
		return fmt.Errorf("service summary is invalid")
	}
	switch s.Observation {
	case ServiceObservationComplete:
		if s.UnavailableOwnerCount != 0 {
			return fmt.Errorf("complete service summary has unavailable owners")
		}
	case ServiceObservationPartial, ServiceObservationUnavailable:
		if s.UnavailableOwnerCount == 0 {
			return fmt.Errorf("uncertain service summary has no unavailable owner")
		}
	default:
		return fmt.Errorf("service summary observation is invalid")
	}
	return nil
}

func (s ServiceStatusSnapshot) SummaryFor(contextID ContextID, workspaceID WorkspaceID) (ServiceSummary, error) {
	if err := s.Validate(); err != nil || contextID.Validate() != nil || workspaceID.Validate() != nil {
		return ServiceSummary{}, fmt.Errorf("service summary scope is invalid")
	}
	result := ServiceSummary{SchemaVersion: ServiceExposureSchema, Observation: s.Observation, UnavailableOwnerCount: s.UnavailableOwnerCount}
	for _, request := range s.Requests {
		if request.ContextID == string(contextID) && request.WorkspaceID == string(workspaceID) {
			result.PendingCount++
		}
	}
	for _, exposure := range s.Exposures {
		if exposure.ContextID == string(contextID) && exposure.WorkspaceID == string(workspaceID) {
			result.ActiveCount++
		}
	}
	result.Attention = result.PendingCount > 0 || result.UnavailableOwnerCount > 0
	return result, result.Validate()
}

type ServiceOpenResult struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	URL           string             `json:"url"`
	Outcome       ServiceOpenOutcome `json:"outcome"`
}

func (r ServiceOpenResult) Validate() error {
	if r.SchemaVersion != ServiceExposureSchema || ValidateServiceExposureID(r.ID) != nil {
		return fmt.Errorf("service open result is invalid")
	}
	if _, _, err := ParseServiceExposureURL(r.URL); err != nil {
		return fmt.Errorf("service open result is invalid")
	}
	switch r.Outcome {
	case ServiceOpenNotDispatched, ServiceOpenRequested, ServiceOpenOutcomeUnknown:
		return nil
	default:
		return fmt.Errorf("service open outcome is invalid")
	}
}

type ServiceCleanupReceipt struct {
	SchemaVersion         int `json:"schema_version"`
	PendingWithdrawnCount int `json:"pending_withdrawn_count"`
	ExposureClosedCount   int `json:"exposure_closed_count"`
	StreamClosedCount     int `json:"stream_closed_count"`
}

func (r ServiceCleanupReceipt) Validate() error {
	if r.SchemaVersion != ServiceExposureSchema || r.PendingWithdrawnCount < 0 || r.ExposureClosedCount < 0 || r.StreamClosedCount < 0 {
		return fmt.Errorf("service cleanup receipt is invalid")
	}
	return nil
}
