# Work Context: Keep the auth-broker experiment detached from `main`

This file records verified facts and boundary evidence. It does not treat the
experiment branch as an accepted product design.

## Current behavior

- This packet adds only deferred-work evidence to the supported `main` line. It
  does not alter `codex/auth-broker`, auth production code, or the experiment's
  branch-only ADRs. The first-wave merge was performed independently; the
  packet records the resulting main boundary below.
- Verified refs:

  | Ref | Object |
  |---|---|
  | `main` | `966dd08841a7ccd88212dd9c8683562c99e17aa9` |
  | `codex/auth-broker` | `4ccc756c2e0c6e07fcaaa788ecfd836bfb8bd730` |
  | `git merge-base main codex/auth-broker` | `98a9b7daf36201f191d056e8163573b2c52986d8` |

- `git rev-list --left-right --count main...codex/auth-broker` returned `23 5`.
- `git diff --stat main...codex/auth-broker` reported 77 changed paths, 7,153 insertions, and 382 deletions. The branch-only scope includes auth CLI/application/domain/infrastructure code, auth-profile fixtures, an auth runtime, two auth ADRs, and related contract changes.
- `main` has no tree paths matching `authcmd`, `authprofile`, `authbroker`, `authprofiles`, `runtimes/auth`, or `auth-broker`. The experiment ref contains `internal/cli/auth_catalog.go` and `internal/infra/authbroker/store.go`.
- `main`'s capability ledger has no `auth.broker` or `auth.profile.authoring` entry and its Catalog has no auth command path.

## Relevant structure and constraints

- Supported entry point and Catalog: `cmd/tobari`, `internal/cli/catalog.go`, runtime Catalog specifications, and `.harness/capabilities.json`.
- Supported authentication: tool-native state below one Tobari home plus the retained generic Gateway passthrough/managed credential-profile boundary. These are not the auth-broker experiment.
- Experiment-only paths: `codex/auth-broker:internal/cli/auth.go`, `internal/cli/auth_catalog.go`, `internal/app/authcmd`, `internal/domain/authprofile`, `internal/infra/authbroker`, and `runtimes/auth`.
- The thesis keeps provider-specific semantic adapters, Gateway-managed OAuth refresh, SigV4, and Keychain integration outside the MVP. Architecture requires four-layer ownership and one Catalog source of truth. Security keeps host credentials out of Tobari and binds managed material to the exact project principal and host inside trusted infrastructure.
- No credentials, auth state, external API, Docker resource, branch ref, coordinator packet, or production code is changed by this packet.

## Thesis evidence and future condition

- An auth experiment can look like a normal CLI capability if its Catalog, capability ledger, runtime, and docs are copied without a boundary check.
- The current product value is the predeclared isolation boundary and denial-to-review-to-retry loop; broker work is lower priority until that value is coherent.
- A future restart requires a new reviewed packet proving product value, explicit thesis acceptance, four-layer/Catalog contracts, secret custody and project/host binding, failure-closed behavior, public-safe output, agent-readiness coverage, and full E2E gates.

## Reproduction and E2E evidence

The following checks were rerun on 2026-08-03 with Go 1.26.5. They use a
temporary archive of `main`, not a checkout and not the dirty worktree.

### Ref and branch-only reachability

```sh
set -euo pipefail
repo=.
main_ref=$(git -C "$repo" rev-parse main)
auth_ref=$(git -C "$repo" rev-parse codex/auth-broker)
base_ref=$(git -C "$repo" merge-base main codex/auth-broker)
test "$main_ref" = 966dd08841a7ccd88212dd9c8683562c99e17aa9
test "$auth_ref" = 4ccc756c2e0c6e07fcaaa788ecfd836bfb8bd730
test "$base_ref" = 98a9b7daf36201f191d056e8163573b2c52986d8
if git -C "$repo" merge-base --is-ancestor codex/auth-broker main; then exit 1; fi
if git -C "$repo" cat-file -e main:internal/cli/auth_catalog.go 2>/dev/null; then exit 1; fi
git -C "$repo" cat-file -e codex/auth-broker:internal/cli/auth_catalog.go
git -C "$repo" cat-file -e codex/auth-broker:internal/infra/authbroker/store.go
test -z "$(git -C "$repo" ls-tree -r --name-only main | rg -i '(^|/)(authcmd|authprofile|authbroker|authprofiles)(/|$)|(^|/)runtimes/auth(/|$)|auth-broker' || true)"
```

Result: passed. Divergence was `23 5`; experiment paths resolved only from
`codex/auth-broker`.

### Main build/test and public-surface E2E

```sh
set -euo pipefail
repo=.
snapshot=$(mktemp -d /private/tmp/tobari-auth-broker-main.XXXXXX)
go_cache=$(mktemp -d /private/tmp/tobari-auth-broker-cache.XXXXXX)
git -C "$repo" archive --format=tar main | tar -xf - -C "$snapshot"
(
  cd "$snapshot"
  git init -q
  git add -A
  git config user.email e2e@example.invalid
  git config user.name e2e
  git commit -qm main-snapshot
  GOCACHE="$go_cache" task check
  GOCACHE="$go_cache" task public:check
  GOCACHE="$go_cache" task build
  help_output=$(./bin/tobari help --format agent)
  printf '%s\n' "$help_output" | jq -e '([.commands[].path | select(test("(^| )auth($| )|auth-broker|auth profile"; "i"))] | length) == 0' >/dev/null
  if ./bin/tobari help auth --format agent >/dev/null 2>/dev/null; then exit 1; fi
  ! git ls-files | rg -i '(^|/)(authcmd|authprofile|authbroker|authprofiles)(/|$)|(^|/)runtimes/auth(/|$)|auth-broker'
  ! git grep -n -E 'Path: "auth|authCapabilityID|authBrokerCapabilityID|"id": "auth\.(broker|profile\.authoring)"' -- internal/cli .harness
  ! rg -n -i 'auth.?broker|auth\.broker|auth\.profile\.authoring' internal/cli .harness README.md docs/00_theses.md docs/01_product_contract.md docs/02_architecture.md docs/03_security_model.md docs/04_harness.md
)
```

Result: `task check`, `task public:check`, and `task build` passed.
`help --format agent` had no auth-broker path; `help auth` was rejected;
Catalog, capability, and implementation-path negative checks passed.

### Security baseline

`task security` was rerun in the clean main archive at `966dd08` and passed:
`all modules verified`, `repoguard (security): OK`, and `No vulnerabilities
found.` The first-wave merge's stable Gateway label annotation removes the
previous G101 baseline finding without changing the auth boundary.

### Commit handoff

The packet is intentionally limited to its four Markdown evidence files. The
auth implementation remains reachable only from `codex/auth-broker`; no auth
code, command, Catalog entry, capability, or branch ref is staged by this
packet. The final docs-only commit SHA is recorded in `tasks.md` after the
commit is created.

## Security and public-boundary notes

- No secret or credential value was read, copied, or emitted.
- Temporary archives, caches, and logs are outside the repository and are not product assets.
- This packet adds no command, reference, effect, mutation, dependency, schema, runtime, or publication surface.

## Glossary

- `auth-broker`: provider-facing experiment reachable from `codex/auth-broker`, not a supported `main` command.
- `tool-native authentication`: state created by a tool below one Tobari home.
- `managed credential profile`: retained non-secret Gateway configuration; it does not imply an auth-broker CLI.
- `governance boundary E2E`: ref reachability, main build/test, public Catalog/help negative path, and branch-only experiment proof.
