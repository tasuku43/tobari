package tobari

import (
	"fmt"
	"path/filepath"
)

// RuntimeBuildStage identifies one stable stage of the Context runtime build.
// Docker's own progress remains an upstream diagnostic stream; these stages
// exist only so trusted CLI presentation can summarize state without parsing
// Docker prose.
type RuntimeBuildStage string

const (
	RuntimeBuildStagePrepare  RuntimeBuildStage = "prepare"
	RuntimeBuildStageBuild    RuntimeBuildStage = "build"
	RuntimeBuildStageValidate RuntimeBuildStage = "validate"
	RuntimeBuildStageInspect  RuntimeBuildStage = "inspect"
	RuntimeBuildStagePromote  RuntimeBuildStage = "promote"
	RuntimeBuildStageReport   RuntimeBuildStage = "report"
)

// RuntimeBuildProgressStatus describes the lifecycle of one build stage.
type RuntimeBuildProgressStatus string

const (
	RuntimeBuildProgressStarted   RuntimeBuildProgressStatus = "started"
	RuntimeBuildProgressCompleted RuntimeBuildProgressStatus = "completed"
	RuntimeBuildProgressFailed    RuntimeBuildProgressStatus = "failed"
)

// RuntimeBuildSelectionState states what is known about the authoritative
// Context image when a progress event is emitted.
type RuntimeBuildSelectionState string

const (
	RuntimeBuildSelectionUnchanged RuntimeBuildSelectionState = "unchanged"
	RuntimeBuildSelectionUncertain RuntimeBuildSelectionState = "uncertain"
	RuntimeBuildSelectionPromoted  RuntimeBuildSelectionState = "promoted"
)

// RuntimeBuildProgress is secret-free task metadata for build presentation.
// It deliberately excludes Docker output, which travels through the separate
// purpose-bound diagnostic stream.
type RuntimeBuildProgress struct {
	Stage          RuntimeBuildStage
	Status         RuntimeBuildProgressStatus
	ContextName    string
	Dockerfile     string
	PreviousImage  string
	CandidateImage string
	Selection      RuntimeBuildSelectionState
}

// RuntimeBuildProgressSink receives best-effort semantic build events.
type RuntimeBuildProgressSink func(RuntimeBuildProgress)

// Validate rejects progress values that could make the CLI infer task or
// state identity from unvalidated presentation data.
func (p RuntimeBuildProgress) Validate() error {
	switch p.Stage {
	case RuntimeBuildStagePrepare, RuntimeBuildStageBuild, RuntimeBuildStageValidate,
		RuntimeBuildStageInspect, RuntimeBuildStagePromote, RuntimeBuildStageReport:
	default:
		return fmt.Errorf("runtime build stage %q is invalid", p.Stage)
	}
	switch p.Status {
	case RuntimeBuildProgressStarted, RuntimeBuildProgressCompleted, RuntimeBuildProgressFailed:
	default:
		return fmt.Errorf("runtime build progress status %q is invalid", p.Status)
	}
	switch p.Selection {
	case RuntimeBuildSelectionUnchanged, RuntimeBuildSelectionUncertain, RuntimeBuildSelectionPromoted:
	default:
		return fmt.Errorf("runtime build selection state %q is invalid", p.Selection)
	}
	if err := ValidateName(p.ContextName); err != nil {
		return fmt.Errorf("runtime build Context: %w", err)
	}
	if p.Dockerfile == "" || !filepath.IsAbs(p.Dockerfile) || filepath.Clean(p.Dockerfile) != p.Dockerfile {
		return fmt.Errorf("runtime build Dockerfile must be canonical and absolute")
	}
	if err := ValidateImageSelector(p.PreviousImage); err != nil {
		return fmt.Errorf("runtime build previous image: %w", err)
	}
	if err := ValidateImageSelector(p.CandidateImage); err != nil || p.CandidateImage == BuiltinImageSelector {
		return fmt.Errorf("runtime build candidate image is invalid")
	}
	if p.Selection == RuntimeBuildSelectionPromoted &&
		p.Stage != RuntimeBuildStageReport &&
		!(p.Stage == RuntimeBuildStagePromote && p.Status == RuntimeBuildProgressCompleted) {
		return fmt.Errorf("promoted runtime selection is only valid after promotion")
	}
	return nil
}
