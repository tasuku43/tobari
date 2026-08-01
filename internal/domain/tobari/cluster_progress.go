package tobari

import "fmt"

// ClusterUpProgressStep identifies one fixed, user-meaningful startup stage.
// The set is deliberately small so progress remains stable even when the
// infrastructure implementation changes its Docker calls.
type ClusterUpProgressStep string

const (
	ClusterUpProgressPrepare           ClusterUpProgressStep = "prepare"
	ClusterUpProgressPolicy            ClusterUpProgressStep = "policy"
	ClusterUpProgressBuildImage        ClusterUpProgressStep = "build_image"
	ClusterUpProgressStartServices     ClusterUpProgressStep = "start_services"
	ClusterUpProgressConnectNetworks   ClusterUpProgressStep = "connect_networks"
	ClusterUpProgressWaitForHealth     ClusterUpProgressStep = "wait_for_health"
	ClusterUpProgressReconcileProjects ClusterUpProgressStep = "reconcile_projects"
	ClusterUpProgressFinalize          ClusterUpProgressStep = "finalize"
	ClusterUpProgressVerifyStatus      ClusterUpProgressStep = "verify_status"
)

// ClusterUpProgressStatus describes the lifecycle of one startup stage.
type ClusterUpProgressStatus string

const (
	ClusterUpProgressStarted   ClusterUpProgressStatus = "started"
	ClusterUpProgressUpdated   ClusterUpProgressStatus = "updated"
	ClusterUpProgressCompleted ClusterUpProgressStatus = "completed"
	ClusterUpProgressFailed    ClusterUpProgressStatus = "failed"
)

// ClusterUpProgress is a fixed, secret-free signal for human startup
// presentation. It is not a command result and carries no runtime diagnostic.
type ClusterUpProgress struct {
	Step   ClusterUpProgressStep
	Status ClusterUpProgressStatus
}

// ClusterUpProgressSink receives best-effort startup presentation signals.
type ClusterUpProgressSink func(ClusterUpProgress)

// Validate rejects progress events outside the bounded vocabulary.
func (p ClusterUpProgress) Validate() error {
	switch p.Step {
	case ClusterUpProgressPrepare,
		ClusterUpProgressPolicy,
		ClusterUpProgressBuildImage,
		ClusterUpProgressStartServices,
		ClusterUpProgressConnectNetworks,
		ClusterUpProgressWaitForHealth,
		ClusterUpProgressReconcileProjects,
		ClusterUpProgressFinalize,
		ClusterUpProgressVerifyStatus:
	default:
		return fmt.Errorf("cluster-up progress step %q is invalid", p.Step)
	}
	switch p.Status {
	case ClusterUpProgressStarted,
		ClusterUpProgressUpdated,
		ClusterUpProgressCompleted,
		ClusterUpProgressFailed:
	default:
		return fmt.Errorf("cluster-up progress status %q is invalid", p.Status)
	}
	if p.Status == ClusterUpProgressUpdated && p.Step != ClusterUpProgressWaitForHealth {
		return fmt.Errorf("cluster-up progress updates are only valid while waiting for health")
	}
	return nil
}
