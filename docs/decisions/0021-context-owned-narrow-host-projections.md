# ADR 0021: Add Context-owned narrow host projections

- Status: Accepted
- Date: 2026-08-09
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, security, external host I/O, state compatibility, harness, and public boundary
- Supersedes: None
- Superseded by: None
- Revised by: ADR 0027 resets Context persistence and output to exact V1 and
  removes migration

## Context

Tobari deliberately keeps the host home, shell startup files, CLI
configuration, and credentials outside Workspaces. The first useful exception
was a fixed allowlist of shell-presentation values. Git commit identity creates
the same user need: a developer wants a familiar non-secret value in multiple
Workspaces without recreating it manually.

Treating each case as a file mount or tool-specific escape hatch would weaken
the host boundary. Treating each scalar as a separate low-level command would
also make routine configuration option-heavy and expose implementation
vocabulary instead of closing the user's setup task.

## Decision drivers

- Keep source files, directives, executables, credentials, and authentication
  state outside Workspaces.
- Let a Context own a small, reviewable set of non-secret presentation or
  identity fallbacks.
- Preserve Workspace and repository authority to override a Context fallback.
- Give humans a terminal workflow without making agents or scripts depend on
  prompts.
- Keep every mutation in the catalog, application policy gate, and atomic
  Context store.
- Make host reads purpose-limited, finite, bounded, and hostile-repository safe.

## Considered options

### Mount or copy host configuration files

This preserves upstream behavior, but also transfers unrelated configuration,
arbitrary includes, executable settings, authentication helpers, and future
keys that Tobari has not reviewed. It is rejected.

### Add one command per scalar below `context`

This mirrors the implementation but makes users remember target-first paths
and requires multiple invocations for one semantic Git identity. It is
rejected.

### Add a configuration-first namespace with narrow projections

Direct `config shell` configures one allowlisted shell-presentation value and
direct `config git` configures the atomic `user.name`/`user.email` identity
pair. Their terminal mode presents complete current and pending state on one
screen; Shell stages several distinct rows for one atomic Apply. Both target one
explicit or current Context. This option is selected.

## Decision

A narrow host projection is an explicit Context policy that selects only
thesis-declared, non-secret scalar values. Infrastructure may read the selected
host value, validates it through domain-owned bounds, and re-encodes it into a
Tobari-owned representation. The source file, source path, include directive,
executable setting, secret, credential, and other keys never cross the
boundary. New projections require thesis, product, architecture, security,
catalog, and harness updates; this ADR is not a generic projection API.

The public configuration surface is:

```text
config shell
config git
```

The existing pre-v1.0 `context shell configure` path is replaced without a
compatibility alias. Context lifecycle remains under `context`; runtime and
credential workflows remain under `runtime` and `auth` because they cross
different trust and side-effect boundaries.

Each configuration command supports two input modes for the same fixed-target
mutation. A complete setting group executes directly and never prompts. A
wholly omitted setting group with text success and error formats opens a
terminal staged editor. Shell passes every distinct staged row to the same
application use case once; Git stages its atomic source on the same screen.
Partial setting groups fail instead of prompting. Redirected or JSON
invocations require complete direct input. Cancellation before apply performs
no mutation. The catalog declares the input dependencies, terminal failures,
result, and mutation contract.

Shell retains only `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`. Git adds one
atomic policy with `default`, `inherit`, or `literal` source. New and migrated
Contexts use `default`, so personal identity is never imported implicitly.
`literal` owns a complete non-empty name/email pair. `inherit` asks a
purpose-built host adapter for only `user.name` and `user.email` from Git's
global scope for the stable canonical Workspace root.

The Git adapter invokes no shell and accepts no caller-selected key or route.
It makes at most two one-attempt, finite-time, bounded-output calls equivalent
to:

```text
git -C ROOT config --global --includes --null --get user.name
git -C ROOT config --global --includes --null --get user.email
```

Global scope deliberately excludes repository and worktree configuration from
the trusted host read while allowing the host's global include and applicable
`gitdir` conditional selection. The executable resolves to an absolute path
outside the project root. Its process receives only validated `HOME` and
optional `XDG_CONFIG_HOME` paths outside the root, a fixed locale, and fixed
Git controls; ambient `PATH`, loader controls, shell-startup controls, and all
other variables are removed. Raw diagnostics are not published, and malformed
or unsafe values fail closed. An absent or incomplete pair produces no Context
fallback; it does not prevent Workspace-owned or repository-owned identity
from operating.

Infrastructure writes a private per-Workspace system-scope Git configuration
that includes the image's `/etc/gitconfig` first and then, when complete, the
quoted Context identity. Its directory is mounted read-only and selected by
`GIT_CONFIG_SYSTEM`. Git's normal precedence therefore remains:

```text
image system config < Context fallback < Workspace global config < repository/worktree config
```

No credential helper, token, HTTP header, SSH command, signing key/program,
hook, alias, URL rewrite, filter, proxy, arbitrary key, host path, or include
directive is serialized. Git authentication remains tool-native or separately
brokered API authentication; this decision does not support private Git
transport.

## Consequences

### Positive

- Shell and Git setup share one discoverable human and machine experience.
- Contexts can provide familiar non-secret defaults without exposing host
  configuration or credential authority.
- Workspace global and repository-local configuration remain authoritative.
- Direct invocations are deterministic, while terminal users need not assemble
  every flag from help.
- The fixed host Git read can be exhaustively tested against malicious local
  includes, PATH substitution, unsafe output, timeouts, and absent values.

### Negative

- The catalog and CLI now own a terminal input-completion state machine in
  addition to ordinary typed argv parsing.
- Context manifest and report schemas advance, and older binaries cannot read
  the new manifest without the matching state backup.
- Git inheritance is a root-scoped fallback snapshot refreshed during
  Workspace reconciliation, not a byte-for-byte mirror of every Git scope or
  nested-repository conditional.
- Email is personal data even though it is not a credential; tests and docs
  must use synthetic values and runtime output must not log it.

## Risks and mitigations

- A malicious repository could add a host-file include. The host adapter reads
  only global scope, so local/worktree includes are not evaluated as host
  configuration.
- A repository could place a fake `git` on `PATH` or use relative loader and
  shell-startup controls. Infrastructure resolves an absolute executable,
  rejects one inside the project root, and does not pass `PATH`, loader, or
  shell-startup entries into the child process.
- A value could inject another config entry. Domain validation rejects line and
  control boundaries, and the encoder quotes backslashes and quotes before an
  atomic regular-file write.
- An interactive script could hang after omitting one flag. Any partial setting
  group is an error; wizard entry requires the whole group to be absent and
  terminal text streams to be present.
- A Context fallback could override deliberate project identity. System scope
  keeps Workspace-global and repository/worktree configuration at higher
  precedence.

## Mechanical enforcement

- Domain tests cover fixed inventories, source/value combinations, atomic Git
  pairs, hostile values, complete reports, and schema-4 preservation.
- Catalog and argv tests cover exact command paths, all-or-none setting groups,
  terminal/JSON behavior, fixed targets, help, result schema, and removal of
  the old path.
- Wizard tests cover raw-terminal and line fallback, current state, review,
  cancellation, stdout/stderr separation, and zero mutation on every failure.
- Host adapter tests cover absolute executable selection, exact keys/argv, the
  exact environment allowlist, project-owned and symlink-resolved config paths,
  loader/`PATH`/shell-startup canaries, call/output/time bounds, missing values,
  hostile repository config, and secret-free failures.
- Runtime tests cover private atomic projection, symlink and oversized-existing-
  file rejection before read or replacement, read-only directory mount, fixed
  environment, image-system inclusion, Context fallback precedence, and
  absence of every excluded Git key.
- Migration tests prove schema-4 ID, runtime, and shell settings, including an
  explicit empty literal, survive schema 5 unchanged while Git stays default.
- Agent-readiness validation proves one scoped help request plus one direct
  invocation closes each machine task with zero external reconstruction.

## Compatibility and migration

Context manifest schema 5 adds optional Git identity policy. Schema 4 is a
readable migration input; its stable ID, runtime recipe, and exact shell
overrides are preserved, and Git identity defaults to absent. Schemas 1 through
3 retain their existing migration behavior and seed only the established shell
default. Context report JSON advances from schema 5 to 6 and always includes a
complete Git identity object.

Before v1.0, `context shell configure` is removed rather than retained as a
second catalog path. Users and agents discover `config shell` through root or
namespace help. Rolling back requires restoring Context state written by the
matching older binary; Tobari does not silently downgrade schema 5.
