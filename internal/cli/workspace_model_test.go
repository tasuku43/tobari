package cli

import (
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func testDesiredWorkspaceEntry() tobari.DesiredEntry {
	digest := "sha256:" + strings.Repeat("a", 64)
	return tobari.DesiredEntry{
		ManifestGeneration: 1,
		ManifestRevision:   digest,
		EntryRevision:      digest,
		RuntimeID:          tobari.StandardRuntimeID,
		RuntimeRevision:    digest,
	}
}

func testAppliedWorkspaceEntry() tobari.AppliedEntry {
	desired := testDesiredWorkspaceEntry()
	return tobari.AppliedEntry{
		ManifestGeneration: desired.ManifestGeneration,
		ManifestRevision:   desired.ManifestRevision,
		EntryRevision:      desired.EntryRevision,
		RuntimeID:          desired.RuntimeID,
		RuntimeRevision:    desired.RuntimeRevision,
		ResolvedSpec:       "sha256:" + strings.Repeat("b", 64),
		ReconciledAt:       time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
	}
}

func ptrDesiredEntry(value tobari.DesiredEntry) *tobari.DesiredEntry { return &value }

func ptrAppliedEntry(value tobari.AppliedEntry) *tobari.AppliedEntry { return &value }
