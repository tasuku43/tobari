package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxRuntimeLifecycleRuntimes   = 256
	maxRuntimeLifecycleMaterials  = 1024
	maxRuntimeLifecycleContainers = 256
	maxRuntimeLifecycleListBytes  = 64 * 1024
	maxRuntimeLifecycleTags       = 256
	maxRuntimeLifecycleInspect    = 64 * 1024
	runtimeLifecycleCallBudget    = 6000
	runtimeLifecycleWallBudget    = 30 * time.Second
)

var runtimeLifecycleContainerID = regexp.MustCompile(`^[0-9a-f]{64}$`)

type runtimeLifecycleLocalObservation struct {
	Runtimes   []tobari.RuntimeManifest
	Protection runtimeProtectionObservation
	Build      *runtimeBuildJournal
}

type runtimeImageObservation struct {
	ID        string   `json:"id"`
	Size      int64    `json:"size"`
	RepoTags  []string `json:"repo_tags"`
	Owner     string   `json:"owner"`
	Component string   `json:"component"`
	RuntimeID string   `json:"runtime_id"`
	Revision  string   `json:"revision"`
}

type runtimeContainerObservation struct {
	ID        string `json:"id"`
	Image     string `json:"image"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	Workspace string `json:"workspace"`
	Role      string `json:"role"`
	Spec      string `json:"spec"`
}

type runtimeContentUse struct{ workspace, external bool }

type runtimeLifecycleBudget struct {
	remaining int
}

func (b *runtimeLifecycleBudget) run(ctx context.Context, runner commandRunner, args, environment []string, limit int) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if b == nil || b.remaining <= 0 {
		return nil, nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	b.remaining--
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: limit}
	err := runner.Run(ctx, args, environment, nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return nil, nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	return append([]byte(nil), stdout.buffer.Bytes()...), append([]byte(nil), stderr.buffer.Bytes()...), err
}

type runtimeMaterialTarget struct {
	RuntimeID      string
	Revision       string
	TagRole        tobari.RuntimeMaterialTagRole
	Selector       string
	RecordedDigest string
	Name           string
}

// ReadRuntimeLifecycleSnapshot proves a complete local catalog/protection/
// journal join under the non-creating lifecycle read lock. Docker cannot be
// locked, so its bounded evidence and the local authority are both observed
// before and after and any drift rejects the snapshot.
func (r *Runtime) ReadRuntimeLifecycleSnapshot(ctx context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	var result tobari.RuntimeLifecycleSnapshot
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		observationContext, cancel := context.WithTimeout(lockContext, runtimeLifecycleWallBudget)
		defer cancel()
		budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
		before, err := r.readRuntimeLifecycleLocalObserved(observationContext, &budget)
		if err != nil {
			return err
		}
		beforeSnapshot, err := r.observeRuntimeLifecycleDocker(observationContext, before, &budget)
		if err != nil {
			return err
		}
		after, err := r.readRuntimeLifecycleLocalObserved(observationContext, &budget)
		if err != nil {
			return err
		}
		afterSnapshot, err := r.observeRuntimeLifecycleDocker(observationContext, after, &budget)
		if err != nil {
			return err
		}
		beforeToken, err := runtimeLifecycleToken(before, beforeSnapshot)
		if err != nil {
			return err
		}
		afterToken, err := runtimeLifecycleToken(after, afterSnapshot)
		if err != nil {
			return err
		}
		if beforeToken != afterToken {
			return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		result = beforeSnapshot
		return nil
	})
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	return result, time.Now().UTC(), nil
}

// ReadRuntimeBuildRecovery observes one active build journal without creating
// state or consulting Docker. The returned stable Runtime reference is the
// only authority accepted by the separate review-confirmed mutation boundary.
func (r *Runtime) ReadRuntimeBuildRecovery(ctx context.Context) (tobari.RuntimeBuildRecovery, bool, error) {
	var result tobari.RuntimeBuildRecovery
	found := false
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		journal, err := r.readStrictRuntimeBuildJournalInventory()
		if err != nil || journal == nil {
			return err
		}
		runtimes, err := r.readStrictRuntimeCatalogObserved(journal)
		if err != nil {
			return err
		}
		managed := false
		for _, manifest := range runtimes {
			if manifest.ID == journal.RuntimeID && manifest.Name == journal.RuntimeName && manifest.Kind == tobari.RuntimeKindManaged {
				managed = true
				break
			}
		}
		if !managed {
			return fmt.Errorf("Runtime build recovery lacks current managed Runtime authority")
		}
		kind := tobari.RuntimeBuildRecoveryFailed
		switch journal.Phase {
		case runtimeBuildPhaseSnapshotting, runtimeBuildPhasePrepared:
			kind = tobari.RuntimeBuildRecoveryPreDocker
		case runtimeBuildPhaseBuilding:
			kind = tobari.RuntimeBuildRecoveryBuilding
		case runtimeBuildPhaseBuilt, runtimeBuildPhaseFinalTagged, runtimeBuildPhaseStagingReleased, runtimeBuildPhaseSnapshotPublished, runtimeBuildPhaseManifestCommitted:
			kind = tobari.RuntimeBuildRecoveryPublication
		case runtimeBuildPhaseCompleting:
			kind = tobari.RuntimeBuildRecoveryCleanup
		case runtimeBuildPhaseOrphanStaging:
			kind = tobari.RuntimeBuildRecoveryOrphan
		case runtimeBuildPhaseFailed:
			kind = tobari.RuntimeBuildRecoveryFailed
		default:
			return fmt.Errorf("Runtime build recovery phase is invalid")
		}
		result = tobari.RuntimeBuildRecovery{RuntimeID: journal.RuntimeID, RuntimeRef: tobari.RuntimeRef(journal.RuntimeID), Name: journal.RuntimeName, Kind: kind}
		if err := result.Validate(); err != nil {
			return err
		}
		found = true
		return lockContext.Err()
	})
	if err != nil {
		return tobari.RuntimeBuildRecovery{}, false, err
	}
	return result, found, nil
}

func runtimeLifecycleToken(local runtimeLifecycleLocalObservation, snapshot tobari.RuntimeLifecycleSnapshot) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(struct {
		Local    runtimeLifecycleLocalObservation `json:"local"`
		Snapshot tobari.RuntimeLifecycleSnapshot  `json:"snapshot"`
	}{Local: local, Snapshot: snapshot})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func (r *Runtime) readRuntimeLifecycleLocalObserved(ctx context.Context, budget *runtimeLifecycleBudget) (runtimeLifecycleLocalObservation, error) {
	if err := ctx.Err(); err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	journal, err := r.readStrictRuntimeBuildJournalInventory()
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	runtimes, err := r.readStrictRuntimeCatalogObserved(journal)
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	protection, err := r.readRuntimeProtectionInventoryObserved(ctx, budget)
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	return runtimeLifecycleLocalObservation{Runtimes: runtimes, Protection: protection, Build: journal}, nil
}

func (r *Runtime) readStrictRuntimeBuildJournalInventory() (*runtimeBuildJournal, error) {
	directory := r.runtimeLifecycleDirectory()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || (entry.Name() != runtimeBuildJournalFile && entry.Name() != runtimeBuildSnapshotDir) {
			return nil, fmt.Errorf("Runtime lifecycle journal inventory contains an unknown entry")
		}
	}
	journal, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return nil, err
	}
	if journal == nil {
		if len(entries) != 0 {
			return nil, fmt.Errorf("Runtime lifecycle staging state lacks journal authority")
		}
		return nil, nil
	}
	snapshotRoot := filepath.Dir(journal.SnapshotPath)
	if info, err := os.Lstat(snapshotRoot); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("Runtime build staging snapshot is unsafe")
		}
		children, err := os.ReadDir(snapshotRoot)
		if err != nil || len(children) != 1 || children[0].Name() != "source" || !children[0].IsDir() || children[0].Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Runtime build staging snapshot inventory is incomplete")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if journal.Phase != runtimeBuildPhaseSnapshotting && journal.Phase != runtimeBuildPhaseSnapshotPublished && journal.Phase != runtimeBuildPhaseManifestCommitted && journal.Phase != runtimeBuildPhaseCompleting {
		return nil, fmt.Errorf("Runtime build journal lacks its staging snapshot")
	}
	return journal, nil
}

func (r *Runtime) readStrictRuntimeCatalogObserved(journal *runtimeBuildJournal) ([]tobari.RuntimeManifest, error) {
	result := []tobari.RuntimeManifest{r.standardRuntimeManifest()}
	entries, err := os.ReadDir(r.runtimesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > maxRuntimeLifecycleRuntimes {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	materialCount := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, fmt.Errorf("Runtime catalog contains an unknown child")
		}
		manifest, err := r.readRuntimeManifest(entry.Name())
		if err != nil {
			return nil, err
		}
		if err := r.validateStrictRuntimeDirectory(manifest, journal); err != nil {
			return nil, err
		}
		materialCount += len(manifest.Revisions)
		if materialCount > maxRuntimeLifecycleMaterials {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		result = append(result, manifest)
	}
	sort.Slice(result[1:], func(i, j int) bool { return result[i+1].Name < result[j+1].Name })
	return result, nil
}

func (r *Runtime) validateStrictRuntimeDirectory(manifest tobari.RuntimeManifest, journal *runtimeBuildJournal) error {
	entries, err := os.ReadDir(r.runtimeDirectory(manifest.Name))
	if err != nil {
		return err
	}
	want := map[string]bool{"runtime.json": true, "source": true, "revisions": true}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !want[entry.Name()] {
			return fmt.Errorf("Runtime directory contains an unknown child")
		}
		delete(want, entry.Name())
	}
	if len(want) != 0 {
		return fmt.Errorf("Runtime directory inventory is incomplete")
	}
	revisionEntries, err := os.ReadDir(r.runtimeRevisionsDirectory(manifest.Name))
	if err != nil {
		return err
	}
	wantRevisions := make(map[string]bool, len(manifest.Revisions)+1)
	for _, revision := range manifest.Revisions {
		wantRevisions[strings.TrimPrefix(revision.Revision, "sha256:")] = true
	}
	if journal != nil && journal.Phase == runtimeBuildPhaseSnapshotPublished && journal.RuntimeID == manifest.ID && journal.RuntimeName == manifest.Name && journal.Revision != "" {
		wantRevisions[strings.TrimPrefix(journal.Revision, "sha256:")] = true
	}
	for _, entry := range revisionEntries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !wantRevisions[entry.Name()] {
			return fmt.Errorf("Runtime revision store contains an unknown child")
		}
		children, err := os.ReadDir(filepath.Join(r.runtimeRevisionsDirectory(manifest.Name), entry.Name()))
		if err != nil || len(children) != 1 || children[0].Name() != "source" || !children[0].IsDir() || children[0].Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime revision snapshot inventory is incomplete")
		}
		delete(wantRevisions, entry.Name())
	}
	if len(wantRevisions) != 0 {
		return fmt.Errorf("Runtime revision snapshot inventory is incomplete")
	}
	return nil
}

func (r *Runtime) observeRuntimeLifecycleDocker(ctx context.Context, local runtimeLifecycleLocalObservation, budget *runtimeLifecycleBudget) (tobari.RuntimeLifecycleSnapshot, error) {
	targets := make([]runtimeMaterialTarget, 0)
	for _, runtime := range local.Runtimes {
		if runtime.Kind != tobari.RuntimeKindManaged {
			continue
		}
		for _, revision := range runtime.Revisions {
			targets = append(targets, runtimeMaterialTarget{RuntimeID: runtime.ID, Revision: revision.Revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Selector: revision.Image, RecordedDigest: revision.ImageDigest, Name: runtime.Name})
		}
	}
	journalInventory := tobari.RuntimeLifecycleJournals{Complete: true, Active: []tobari.RuntimeLifecycleActivity{}, FailedBuilds: []tobari.RuntimeFailedBuildArtifact{}}
	if local.Build != nil {
		if local.Build.Phase == runtimeBuildPhaseFailed && local.Build.StagingArtifact == runtimeBuildStagingOwned && local.Build.AttemptSettlement == runtimeBuildAttemptSettled {
			targets = append(targets, runtimeMaterialTarget{RuntimeID: local.Build.RuntimeID, Revision: local.Build.Revision, TagRole: tobari.RuntimeMaterialTagJournaledStaging, Selector: local.Build.StagingImage, RecordedDigest: local.Build.ImageDigest, Name: local.Build.RuntimeName})
		} else {
			journalInventory.Active = append(journalInventory.Active, runtimeLifecycleActivityFromBuild(*local.Build))
		}
	}
	if len(targets) > maxRuntimeLifecycleMaterials {
		return tobari.RuntimeLifecycleSnapshot{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	materials, err := r.observeRuntimeMaterials(ctx, targets, local.Protection.Containers, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, err
	}
	successful := make([]tobari.RuntimeMaterialObservation, 0, len(materials))
	for index, material := range materials {
		if targets[index].TagRole == tobari.RuntimeMaterialTagJournaledStaging {
			journalInventory.FailedBuilds = append(journalInventory.FailedBuilds, tobari.RuntimeFailedBuildArtifact{RuntimeID: targets[index].RuntimeID, Revision: targets[index].Revision, RuntimeRef: tobari.RuntimeRef(targets[index].RuntimeID), Name: targets[index].Name, Material: material})
		} else {
			successful = append(successful, material)
		}
	}
	sortRuntimeLifecycleJournals(&journalInventory)
	return tobari.RuntimeLifecycleSnapshot{CatalogComplete: true, Runtimes: local.Runtimes, Protection: local.Protection.Inventory, Materials: successful, Journals: journalInventory}, nil
}

func (r *Runtime) observeRuntimeMaterials(ctx context.Context, targets []runtimeMaterialTarget, workspaces map[string]runtimeWorkspaceContainerAuthority, budget *runtimeLifecycleBudget) ([]tobari.RuntimeMaterialObservation, error) {
	digests := make(map[string]string, len(targets))
	for _, target := range targets {
		if target.RecordedDigest != "" {
			digests[target.RuntimeID+"\x00"+target.Revision] = target.RecordedDigest
		}
	}
	containerUse, err := r.observeRuntimeContainerUse(ctx, workspaces, digests, budget)
	if err != nil {
		return nil, err
	}
	result := make([]tobari.RuntimeMaterialObservation, 0, len(targets))
	for _, target := range targets {
		material, err := r.observeRuntimeMaterial(ctx, target, containerUse, budget)
		if err != nil {
			return nil, err
		}
		result = append(result, material)
	}
	return result, nil
}

func (r *Runtime) observeRuntimeMaterial(ctx context.Context, target runtimeMaterialTarget, containerUse map[string]runtimeContentUse, budget *runtimeLifecycleBudget) (tobari.RuntimeMaterialObservation, error) {
	material := tobari.RuntimeMaterialObservation{RuntimeID: target.RuntimeID, Revision: target.Revision, TagRole: target.TagRole, ObservationComplete: true}
	tagImage, tagMissing, err := r.inspectRuntimeLifecycleImage(ctx, target.Selector, budget)
	if err != nil {
		return tobari.RuntimeMaterialObservation{}, err
	}
	material.TagPresent = !tagMissing
	if !tagMissing {
		member := false
		for _, tag := range tagImage.RepoTags {
			if tag == target.Selector {
				member = true
				break
			}
		}
		if !member {
			return tobari.RuntimeMaterialObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
	}
	contentDigest := target.RecordedDigest
	if contentDigest == "" && !tagMissing {
		contentDigest = tagImage.ID
	}
	var content runtimeImageObservation
	contentMissing := contentDigest == ""
	if !contentMissing && !tagMissing && tagImage.ID == contentDigest {
		content = tagImage
	} else if !contentMissing {
		content, contentMissing, err = r.inspectRuntimeLifecycleImage(ctx, contentDigest, budget)
		if err != nil {
			return tobari.RuntimeMaterialObservation{}, err
		}
	}
	if !contentMissing {
		material.ContentPresent = true
		material.ImageVirtualBytes = &content.Size
		material.OwnershipVerified = content.Owner == ownerValue && content.Component == managedRuntimeComponentLabel && content.RuntimeID == target.RuntimeID && content.Revision == target.Revision
		for _, repoTag := range content.RepoTags {
			if repoTag != target.Selector {
				material.SharedContent = true
				break
			}
		}
		if use, ok := containerUse[contentDigest]; ok {
			material.WorkspaceInUse, material.ExternalInUse = use.workspace, use.external
		}
	}
	switch {
	case !tagMissing && target.RecordedDigest != "" && tagImage.ID != target.RecordedDigest:
		material.Availability = tobari.RuntimeAvailabilityMismatched
	case !tagMissing && (!material.ContentPresent || !material.OwnershipVerified):
		material.Availability = tobari.RuntimeAvailabilityUnknown
		material.MigrationUnverified = true
	case !tagMissing:
		material.Availability = tobari.RuntimeAvailabilityAvailable
	case material.ContentPresent && !material.OwnershipVerified:
		material.Availability = tobari.RuntimeAvailabilityUnknown
		material.MigrationUnverified = true
	default:
		material.Availability = tobari.RuntimeAvailabilityMissing
	}
	if err := material.Validate(); err != nil {
		return tobari.RuntimeMaterialObservation{}, err
	}
	return material, nil
}

func (r *Runtime) inspectRuntimeLifecycleImage(ctx context.Context, selector string, budget *runtimeLifecycleBudget) (runtimeImageObservation, bool, error) {
	format := `{"id":{{json .Id}},"size":{{json .Size}},"repo_tags":{{json .RepoTags}},` +
		`"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"runtime_id":{{json (index .Config.Labels "` + managedRuntimeIDLabel + `")}},` +
		`"revision":{{json (index .Config.Labels "` + managedRuntimeRevisionLabel + `")}}}`
	output, diagnostic, err := budget.run(ctx, r.runner, []string{"image", "inspect", "--format", format, selector}, os.Environ(), maxRuntimeLifecycleInspect)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return runtimeImageObservation{}, false, err
		}
		if isMissingRuntimeImageInspect(err, diagnostic, selector) {
			return runtimeImageObservation{}, true, nil
		}
		return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	var image runtimeImageObservation
	if decodeStrictJSON(output, &image) != nil || tobari.ValidateDigest(image.ID) != nil || image.Size < 0 || len(image.RepoTags) > maxRuntimeLifecycleTags {
		return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	seen := make(map[string]bool, len(image.RepoTags))
	for _, tag := range image.RepoTags {
		if tag == "" || len(tag) > 512 || strings.ContainsAny(tag, "\x00\r\n") || seen[tag] {
			return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		seen[tag] = true
	}
	return image, false, nil
}

func (r *Runtime) observeRuntimeContainerUse(ctx context.Context, workspaces map[string]runtimeWorkspaceContainerAuthority, revisionDigests map[string]string, budget *runtimeLifecycleBudget) (map[string]runtimeContentUse, error) {
	output, _, err := budget.run(ctx, r.runner, []string{"container", "ls", "--all", "--no-trunc", "--format", "{{.ID}}"}, os.Environ(), maxRuntimeLifecycleListBytes)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	trimmed := strings.TrimSuffix(string(output), "\n")
	if trimmed == "" {
		return map[string]runtimeContentUse{}, nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > maxRuntimeLifecycleContainers {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	use := make(map[string]runtimeContentUse)
	seen := make(map[string]bool, len(lines))
	format := `{"id":{{json .Id}},"image":{{json .Image}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	for _, id := range lines {
		if !runtimeLifecycleContainerID.MatchString(id) || seen[id] {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		seen[id] = true
		observedBytes, _, inspectErr := budget.run(ctx, r.runner, []string{"container", "inspect", "--format", format, id}, os.Environ(), 4096)
		if errors.Is(inspectErr, context.Canceled) || errors.Is(inspectErr, context.DeadlineExceeded) {
			return nil, inspectErr
		}
		if inspectErr != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		var observed runtimeContainerObservation
		if decodeStrictJSON(observedBytes, &observed) != nil || observed.ID != id || tobari.ValidateDigest(observed.Image) != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		current := use[observed.Image]
		if authority, trusted := workspaces[id]; trusted {
			expectedDigest, knownRevision := revisionDigests[authority.RuntimeID+"\x00"+authority.Revision]
			if !knownRevision || observed.Owner != ownerValue || observed.Component != "tobari" || observed.Workspace != authority.WorkspaceID || observed.Role != projectWorkRole || observed.Spec != authority.ResolvedSpec || observed.Image != expectedDigest {
				return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
			}
			current.workspace = true
		} else {
			current.external = true
		}
		use[observed.Image] = current
	}
	return use, nil
}
