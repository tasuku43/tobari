package tobari

import "fmt"

// FirstEntryStage is the fixed invocation-only checkpoint vocabulary for the
// root journey. It is presentation metadata, not persisted authority.
type FirstEntryStage string

const (
	FirstEntryCheckRequirements FirstEntryStage = "check_requirements"
	FirstEntryResolveContext    FirstEntryStage = "resolve_context"
	FirstEntryPrepareProtection FirstEntryStage = "prepare_protection"
	FirstEntryPrepareWorkspace  FirstEntryStage = "prepare_workspace"
	FirstEntryEnterWorkspace    FirstEntryStage = "enter_workspace"
)

var firstEntryStages = [...]FirstEntryStage{
	FirstEntryCheckRequirements,
	FirstEntryResolveContext,
	FirstEntryPrepareProtection,
	FirstEntryPrepareWorkspace,
	FirstEntryEnterWorkspace,
}

func FirstEntryStages() []FirstEntryStage {
	stages := make([]FirstEntryStage, len(firstEntryStages))
	copy(stages, firstEntryStages[:])
	return stages
}

// FirstEntryStageState is deliberately independent from desired, active,
// applied, and observed domain state. A succeeded stage proves only its exact
// checkpoint.
type FirstEntryStageState string

const (
	FirstEntryStagePending   FirstEntryStageState = "pending"
	FirstEntryStageRunning   FirstEntryStageState = "running"
	FirstEntryStageSucceeded FirstEntryStageState = "succeeded"
	FirstEntryStageSkipped   FirstEntryStageState = "skipped"
	FirstEntryStageBlocked   FirstEntryStageState = "blocked"
	FirstEntryStageFailed    FirstEntryStageState = "failed"
	FirstEntryStageUnknown   FirstEntryStageState = "unknown"
)

type FirstEntryProgress struct {
	Stage FirstEntryStage
	State FirstEntryStageState
}

// FirstEntryProgressSink receives invocation-only checkpoint changes. A sink
// cannot influence authority, mutation, retry, or child-session decisions.
type FirstEntryProgressSink func(FirstEntryProgress)

func (p FirstEntryProgress) Validate() error {
	if FirstEntryStageIndex(p.Stage) < 0 {
		return fmt.Errorf("first-entry stage %q is invalid", p.Stage)
	}
	switch p.State {
	case FirstEntryStagePending, FirstEntryStageRunning, FirstEntryStageSucceeded,
		FirstEntryStageSkipped, FirstEntryStageBlocked, FirstEntryStageFailed,
		FirstEntryStageUnknown:
		return nil
	default:
		return fmt.Errorf("first-entry stage state %q is invalid", p.State)
	}
}

func FirstEntryStageIndex(stage FirstEntryStage) int {
	for index, candidate := range firstEntryStages {
		if candidate == stage {
			return index
		}
	}
	return -1
}
