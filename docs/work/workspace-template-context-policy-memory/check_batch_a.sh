#!/bin/sh
set -eu

packages='./internal/domain/tobari ./internal/infra/workspaceauthoritystore ./internal/app/workspaceauthoritycmd ./internal/cli'
tests='Test(ContextAuthorityAxes|PublishWorkspaceEntryAuthority|FinalDefaultPair|InitializeFinalDefaultPair|FreshDefaultPairEntry|ContextEntry|DefaultPair|FinalContextProjection|FinalContextHuman|BareStatus)'

go test $packages -run "$tests" -count=1
go test -race $packages -run "$tests" -count=1
