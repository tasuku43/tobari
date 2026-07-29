---
name: bootstrap-derived-cli
description: Initialize a new CLI repository derived from this foundry template. Use immediately after creating a repository from the template while .harness/project.json has profile "template", or when asked to bootstrap, initialize, rename, or start a derived CLI. Guides identity resolution, transactional bootstrap, verification, and the required thesis and security handoff before capability work.
---

# Bootstrap a Derived CLI

Turn the runnable foundry template into a named project without duplicating the
bootstrap mechanism. Use the repository's validated bootstrap tool for all
identity replacement and renames.

## 1. Confirm the starting state

Work from the repository root. Require these files:

- `.harness/project.json`
- `tools/bootstrap`
- `tools/projectmeta`
- `docs/00_theses.md`
- `AGENTS.md`

Read `AGENTS.md`, then inspect the profile:

```sh
go run ./tools/projectmeta --field profile
```

- If it is `template`, continue.
- If it is `ready`, do not rerun bootstrap or change the profile by hand. Treat
  that stored value as identity-ready: report that identity bootstrap has
  completed, not that product work is complete, and inspect existing metadata
  only if the user asked for verification.
- For any other value, stop and report the validation error.

Preserve unrelated changes. Inspect `git status --short` before editing and
distinguish existing work from bootstrap changes.

## 2. Resolve the project identity

Read `.harness/project.json` and resolve every `project` field:

- `name`: human-facing project name;
- `binary_name`: portable lowercase executable basename of at most 96 bytes so
  its Windows `.exe` archive entry remains within the 100-byte format limit;
- `go_module`: canonical module import path, normally the eventual repository
  URL without a scheme;
- `github_owner` and `github_repository`: intended public repository identity;
- `description`: one public-safe sentence;
- `formula_class`: valid Ruby class name for the Homebrew Formula;
- `license_spdx`: deliberate public license choice;
- `security_contact`: public vulnerability-reporting address.

Also resolve `public_guard.documentation_locale` as one explicit language tag
for trusted repository and CLI-authored prose. Do not infer or default a locale
for an existing schema-1 derived repository: first record the language choice
in its thesis or product contract, then add the field and move the project
configuration to schema 2. Machine identifiers and external provider data do
not change locale.

The GitHub owner and license may deliberately match the template. The
repository, Go module, binary, display name, description, Formula class, and
security contact are project-specific and must not retain their runnable
template defaults.

Use an existing Git remote only as evidence. Do not invent an owner, license,
security address, or publication destination. Ask one concise, grouped question
when a material value cannot be inferred safely.

The module path does not need a live remote for local builds. It must still be
the canonical path that source imports will use after publication. Go
`internal` visibility is based on the importing package's path prefix, so the
bootstrap updates `go.mod` and all repository imports together.

Reject credentials, private URLs, internal organization names, personal data,
and copied private history. Keep synthetic examples public-safe.

## 3. Configure and preview

Edit only `.harness/project.json` to set the resolved identity and locale
policy. Leave
`profile` as `template`; the bootstrap tool owns the transition to the stored
`ready` value, which means identity-ready only.

Preview the complete transaction:

```sh
go run ./tools/bootstrap --dry-run
```

Review every reported update and rename. Continue only when all targets are
inside the repository and correspond to the requested identity. Do not perform
ad hoc search-and-replace, manual command-directory renames, or direct edits to
`tools/internal/projectconfig/defaults.go`; that file records template
provenance in a derived repository.

## 4. Apply once

When the initiating request authorizes initialization and the preview is
correct, run:

```sh
go run ./tools/bootstrap
```

The operation plans all changes before writing and rolls back a failed commit.
If it fails, report the exact error and inspect the tree. Do not claim identity
readiness or force the profile.

## 5. Verify the mechanical result

Verify the metadata and source tree:

```sh
go run ./tools/projectmeta --field profile
go run ./tools/projectmeta --field go_module
go run ./tools/projectmeta --field binary_name
gofmt -l .
go build ./...
./scripts/check.sh fast
./scripts/check.sh public
git diff --check
```

Require:

- stored profile is `ready` (identity-ready only);
- the explicitly selected `documentation_locale` is unchanged;
- `go.mod`, repository imports, `cmd/<binary_name>`, and the Formula template
  agree with `.harness/project.json`;
- `gofmt -l .` prints nothing;
- both gates pass;
- no old runnable identity remains outside
  `tools/internal/projectconfig/defaults.go`;
- a second `go run ./tools/bootstrap --dry-run` is rejected as already
  bootstrapped.

Run the gates before staging or committing the bootstrap diff. Repository guard
must accept the tracked-deletion/untracked-destination state produced by a
transactional path rename while continuing to inspect the destination.

Treat `public` success as identity and repository-boundary evidence, not product
or publication approval.

## 6. Concretize the product before capabilities

Bootstrap changes identity, not intent. Before using `$add-capability`, revise
at least these documents for the actual tool:

1. `docs/00_theses.md`: north star, primary users, outcomes, non-goals, and the
   first testable slice;
2. `docs/01_product_contract.md`: supported user tasks, public vocabulary,
   compatibility, and deliberately unsupported capabilities;
3. `docs/03_security_model.md`: credentials, data, trust boundaries, network
   destinations, and side-effect policy;
4. `docs/07_authentication.md`: OAuth, PAT, or neither, plus storage, refresh,
   account-selection, and dependency decisions when an external API is used;
5. `docs/08_external_api_contracts.md`: pagination, rate limits, timeout,
   idempotency, schema fixtures, and fault mapping when applicable;
6. `docs/06_release.md`: ownership and publication promises.

Use the smallest concrete thesis that can decide the first vertical slice.
Record unknowns instead of inventing certainty, then improve the thesis as
implementation evidence appears.

Run the full gate after this project-specific documentation pass:

```sh
./scripts/check.sh full
```

Do not create a remote, push, publish, or release merely because bootstrap was
requested. Commit only when the user's request includes that local repository
operation.

## 7. Report the handoff

Report separately:

- the final project identity and module path;
- bootstrap and gate evidence;
- project-specific thesis, security, authentication, and release decisions
  completed now;
- decisions still owned by the derived project;
- any pre-existing changes left untouched.

The repository is ready for capability work only when identity bootstrap is
complete and the first project-specific thesis can guide `$add-capability`.
