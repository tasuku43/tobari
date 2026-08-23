package systemdoctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/doctorcmd"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

func TestFallbackReturnsCompleteReportWithoutInventingXDGAuthority(t *testing.T) {
	binDirectory := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(binDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDirectory, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\ncase \"$1\" in version) echo 27.0.0;; context) echo synthetic;; compose) echo v2.29.0;; ps) exit 0;; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDirectory)
	report, err := doctorcmd.New(New(errors.New("synthetic XDG resolution failure"))).Run(
		context.Background(), operation.Intent{Command: "doctor", Effect: operation.EffectRead}, t.TempDir(),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("report.Validate() error = %v", err)
	}
	contextCheck := fallbackCheck(t, report, doctor.CheckIDContext)
	if contextCheck.Status != doctor.CheckStatusFail {
		t.Fatalf("context = %+v, want fail", contextCheck)
	}
	rootCheck := fallbackCheck(t, report, doctor.CheckIDRoot)
	if rootCheck.Status != doctor.CheckStatusWarn {
		t.Fatalf("root = %+v, want bounded warning", rootCheck)
	}
	rootSharing := fallbackCheck(t, report, doctor.CheckIDRootSharing)
	if rootSharing.Status != doctor.CheckStatusBlocked || rootSharing.BlockedBy == nil || *rootSharing.BlockedBy != doctor.CheckIDRoot {
		t.Fatalf("root sharing = %+v, want blocked by root warning", rootSharing)
	}
	first, found := report.FirstFailureRecovery()
	if !found || first.NextCommand != "doctor" || !strings.Contains(first.Action, "XDG") {
		t.Fatalf("first failure recovery = %+v, found=%t, want Context XDG recovery", first, found)
	}
	blocked := []doctor.CheckID{
		doctor.CheckIDState, doctor.CheckIDPolicy, doctor.CheckIDPolicyData,
	}
	if capabilitysurface.Compiled().IncludesResearch() {
		blocked = append(blocked, doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths)
	}
	for _, id := range blocked {
		check := fallbackCheck(t, report, id)
		if check.Status != doctor.CheckStatusBlocked || check.BlockedBy == nil || *check.BlockedBy != doctor.CheckIDContext {
			t.Fatalf("%s = %+v, want blocked by context", id, check)
		}
	}
}

func TestObserveHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(errors.New("unavailable")).ObserveDoctorCheck(ctx, ".", doctor.CheckIDContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("ObserveDoctorCheck() error = %v, want context.Canceled", err)
	}
}

func TestClassifyResolvedRootDoesNotBlameRootWhenHomeIsUnavailable(t *testing.T) {
	got := classifyResolvedRoot("/work/project", "", errors.New("synthetic home lookup failure"), true)
	if got.Status != doctor.CheckStatusWarn {
		t.Fatalf("home-unavailable root = %+v, want limited warning", got)
	}
	for _, unsafe := range []string{"/", "/synthetic-parent", "/synthetic-parent/example"} {
		got = classifyResolvedRoot(unsafe, "/synthetic-parent/example", nil, false)
		if got.Status != doctor.CheckStatusFail {
			t.Errorf("unsafe root %q = %+v, want fail", unsafe, got)
		}
	}
}

func fallbackCheck(t *testing.T, report doctor.Report, id doctor.CheckID) doctor.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == id {
			return check
		}
	}
	t.Fatalf("report lacks %s", id)
	return doctor.Check{}
}
