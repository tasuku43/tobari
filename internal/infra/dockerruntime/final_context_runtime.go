package dockerruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// resolveFinalContextLoginRuntimeImage is called only while the installation
// lifecycle lock is held. It observes the stable Runtime catalog and immutable
// Docker material twice; the persisted Template selector is correlation input,
// never execution authority.
func (r *Runtime) resolveFinalContextLoginRuntimeImage(ctx context.Context, binding tobari.RuntimeBinding) (string, error) {
	before, err := r.observeFinalContextLoginRuntimeImage(ctx, binding)
	if err != nil {
		return "", err
	}
	after, err := r.observeFinalContextLoginRuntimeImage(ctx, binding)
	if err != nil {
		return "", err
	}
	if before != after {
		return "", fmt.Errorf("final Context Runtime execution material changed during observation")
	}
	return after.imageID, nil
}

type finalContextLoginRuntimeObservation struct {
	binding tobari.RuntimeBinding
	imageID string
}

func (r *Runtime) observeFinalContextLoginRuntimeImage(ctx context.Context, binding tobari.RuntimeBinding) (finalContextLoginRuntimeObservation, error) {
	if err := binding.Validate(); err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	if err := ctx.Err(); err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	build, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	prune, err := r.readRuntimePruneJournalObserved()
	if err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	deletion, err := r.readRuntimeDeleteJournalObserved()
	if err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	if build != nil || prune != nil || deletion != nil {
		return finalContextLoginRuntimeObservation{}, fmt.Errorf("Runtime lifecycle recovery must complete before credential acquisition")
	}

	manifest, err := r.standardRuntimeManifest()
	if err != nil {
		return finalContextLoginRuntimeObservation{}, err
	}
	if binding.RuntimeID != tobari.StandardRuntimeID {
		manifest, err = r.resolveManagedRuntimeReferenceUnlocked(tobari.RuntimeRef(binding.RuntimeID))
		if err != nil {
			return finalContextLoginRuntimeObservation{}, err
		}
	}
	expected, err := manifest.Binding(binding.Ordinal)
	if err != nil || !reflect.DeepEqual(expected, binding) || expected.Revision != binding.Revision {
		return finalContextLoginRuntimeObservation{}, fmt.Errorf("final Context Runtime binding does not match immutable Runtime authority")
	}

	var imageID string
	if manifest.Kind == tobari.RuntimeKindManaged {
		selector := managedLibraryRuntimeImage(manifest.Name, manifest.ID, expected.Revision)
		if expected.Image != selector {
			return finalContextLoginRuntimeObservation{}, fmt.Errorf("managed Runtime selector is not canonical")
		}
		imageID, err = r.inspectManagedRuntimeBuildEvidence(ctx, selector, manifest.ID, expected.Revision)
		if err != nil {
			return finalContextLoginRuntimeObservation{}, err
		}
		revision := manifest.Revisions[expected.Ordinal-1]
		if imageID != revision.ImageDigest {
			return finalContextLoginRuntimeObservation{}, fmt.Errorf("managed Runtime immutable image digest changed")
		}
		if err := r.validateManagedRuntimeBuildCompatibility(ctx, imageID); err != nil {
			return finalContextLoginRuntimeObservation{}, err
		}
	} else {
		imageID, err = r.inspectFinalStandardRuntimeImage(ctx, expected.Image)
		if err != nil {
			return finalContextLoginRuntimeObservation{}, err
		}
	}
	return finalContextLoginRuntimeObservation{binding: expected, imageID: imageID}, nil
}

func (r *Runtime) inspectFinalStandardRuntimeImage(ctx context.Context, selector string) (string, error) {
	if tobari.ValidateImageSelector(selector) != nil {
		return "", fmt.Errorf("standard Runtime selector is invalid")
	}
	format := `{"id":{{json .Id}},` +
		`"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},` +
		`"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},` +
		`"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`
	stdout := &boundedBuffer{limit: 4096}
	stderr := &boundedBuffer{limit: 4096}
	if err := r.runner.Run(ctx, []string{"image", "inspect", "--format", format, selector}, os.Environ(), nil, stdout, stderr); err != nil || stdout.overflow || stderr.overflow {
		return "", fmt.Errorf("inspect standard Runtime execution material: %w", err)
	}
	var evidence struct {
		ID         string   `json:"id"`
		API        string   `json:"api"`
		Lifetime   string   `json:"lifetime"`
		User       string   `json:"user"`
		Entrypoint []string `json:"entrypoint"`
	}
	if err := json.Unmarshal(stdout.buffer.Bytes(), &evidence); err != nil || tobari.ValidateDigest(evidence.ID) != nil ||
		evidence.API != tobari.RuntimeImageAPI || evidence.Lifetime != tobari.RuntimeImageLifetimeCommand ||
		evidence.User != "tobari" || !equalStrings(evidence.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		return "", fmt.Errorf("standard Runtime execution material is incompatible")
	}
	return evidence.ID, nil
}
