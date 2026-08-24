#!/bin/sh
set -u

root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root" || exit 1

pattern='^(TestBatchD.*|TestADR0084WholeCatalogReferenceGraphIsExact|TestRuntimePruneDryRunAndApplyRoundTripExactPlanReference|TestFinalClusterStatusReportsEveryValidLifecycleJournalAndCollectionDrift|TestFinalOnlyStoreAcceptsFreshOrCompleteFinalOnlyWhenLegacyIsAbsent|TestFinalStorePolicyReadsPropagateLegacyGuardWithoutFallback|TestFinalAuthorityLegacyGuard.*|TestConfigOnlyLegacyResearchAuthorityBlocksFreshFinalStoreWithoutMutation|TestEveryDeclaredLegacyRootBlocksFreshFinalStoreWithoutMutation|TestConfigOnlyLegacyAuthorityRejectsTemplateCreateBeforeLifecycleLock)$'
failed=0

run_cell() {
  label=$1
  shift
  echo "BATCH_D_CELL $label"
  if "$@"; then
    echo "BATCH_D_PASS $label"
  else
    echo "BATCH_D_FAIL $label"
    failed=$((failed + 1))
  fi
}

packages='./internal/cli ./internal/infra/workspaceauthoritystore ./internal/infra/dockerruntime'
run_cell standard sh -c "go test $packages -run '$pattern' -count=1"
run_cell standard-race sh -c "go test -race $packages -run '$pattern' -count=1"
run_cell research sh -c "go test -tags='tobari_dev tobari_research' $packages -run '$pattern' -count=1"
run_cell research-race sh -c "go test -race -tags='tobari_dev tobari_research' $packages -run '$pattern' -count=1"
run_cell diff-check git diff --check

if [ "$failed" -ne 0 ]; then
  echo "BATCH_D_FAILED_CELLS=$failed"
  exit 1
fi
echo "BATCH_D_GREEN"
