#!/bin/sh
set -u

packages='./internal/domain/tobari ./internal/infra/workspaceauthoritystore ./internal/app/workspaceauthoritycmd ./internal/cli'
research_packages='./internal/infra/operatorconsole ./internal/cli'
tests='Test(PolicyMemoryReviewedDecisionSetRejectsEmptyAndPreservesReviewItemID|PolicyMemoryReviewedTemplateCompactsExactRuleAndPendingCandidate|PolicyMemoryReviewedPublicationIsOneOrderedMultiContextResult|PolicyMemoryReviewedTemplateRejectsHostileOrStaleAuthority|PolicyMemoryReviewedSetRejectsConcurrentCollectionDrift|PolicyMemoryReviewedPublicationJSONExposesOnlyTaskOwnedActiveReferences|PolicyMemoryReviewedPublicationCompactsOnlyBothCompleteCollections|ApplyReviewedPublishesOneSetAndReplaysAcrossUnrelatedPureMutation|ApplyReviewedSettlesTwoContextAllowAndDenyAsOneGlobalMutation|ApplyReviewedFixedTargetDoesNotReplayAnOlderDifferentSet|ApplyReviewedRejectsConfirmedCollectionDriftBeforeDecisionOrEffect|ApplyReviewedResumesOneGlobalDecisionAcrossPublicationBoundaries|ApplyReviewedReplaysAfterTerminalRenameResultUncertainty|ApplyReviewedConsumesOneExplicitCompleteSetAndReturnsExhaustiveResult|ApplyReviewedRejectsInvalidOrSubstitutedSetBeforeSemanticSuccess|InvokerDoesNotOverwriteConfirmedSuccessWithLateCancellation|EmitMutationResultPreservesConfirmedSuccessAfterCancellation|BatchCB3)'

status=0
go test $packages -run "$tests" -count=1 || status=$?
go test -race $packages -run "$tests" -count=1 || status=$?
go test -timeout=30s -tags='tobari_dev tobari_research' $research_packages -run 'Test(Handler|PolicyApply|RunOwns|BatchCB3)' -count=1 || status=$?
go test -timeout=60s -race -tags='tobari_dev tobari_research' $research_packages -run 'Test(Handler|PolicyApply|RunOwns|BatchCB3)' -count=1 || status=$?
exit "$status"
