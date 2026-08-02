# ADR 0014: Preserve host-home-relative project paths in CWD-owned runtimes

- Status: Accepted
- Date: 2026-08-02
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, and harness
- Supersedes: None
- Superseded by: None

## Context

The CWD-owned runtime already gives each Workspace `HOME=/var/lib/tobari` and
one persistent home mount, but previously exposed every selected host root
under a mirrored `/workspace` path. A project below the host home should feel
like the corresponding directory below the container home without weakening
the selected-root boundary.

A project bind nested below `/var/lib/tobari` masks any image-layer contents at
that destination while the container exists. It does not delete those image
files, but an image cannot rely on them being visible after the home mount and
project mount are applied.

## Decision drivers

- Preserve the natural home-relative path for projects below the host home.
- Never mount the host home wholesale.
- Keep the selected root as the only host project bind mount.
- Preserve the existing path contract for roots outside the host home.
- Keep runtime image tools available after nested mounts.
- Avoid implicit copying, migration, or deletion of image or home contents.

## Considered options

### Option A: Keep every project under `/workspace`

This avoids nested mounts but makes the container's stable home unrelated to
the host-relative project path. Rejected for the CWD-first experience.

### Option B: Mount the host home wholesale at `/var/lib/tobari`

This gives the desired path but exposes unrelated host files, credentials, and
directories. Rejected by the host-home isolation boundary.

### Option C: Mount only the selected root below the container home

Canonical roots below the host home map their relative suffix below
`/var/lib/tobari`; the per-Workspace home is mounted first and the selected
project root is mounted second. Other roots keep `/workspace`. Chosen.

## Decision

The infrastructure adapter derives one container project root from the
canonical host root and canonical host home:

```text
host `$HOME/path/to`  ->  container `/var/lib/tobari/path/to`
host `/work`           ->  container `/workspace/work`
```

The same mapping is used for `TOBARI_ROOT`, Docker create's workdir and root
mount, the desired runtime spec hash, and the interactive nested CWD. The
domain's generic `/workspace` mapper and retired named-Tobari compatibility
path remain unchanged.

`/var/lib/tobari` is a mutable per-Workspace home boundary. Official runtime
images keep executables and package resources in `/usr/local/bin` and
`/opt/tobari`; custom compatible images must not require image-layer files
below the home boundary. Tobari does not copy or remove files that a nested
mount masks.

## Consequences

### Positive

- A project below the host home has the expected home-relative path inside the
  container.
- Host-home isolation remains explicit and testable.
- Existing home-external projects retain their `/workspace` compatibility.
- Runtime image tools remain visible because they are outside the mutable home.

### Negative

- A nested project mount intentionally hides any unrelated per-Workspace home
  entries at the same relative destination while that project is attached.
- The selected root and persistent home now have a nested mount relationship,
  which requires Docker integration coverage.
- Custom runtime image authors must treat `/var/lib/tobari` as replaceable home
  state rather than an image-owned installation directory.

## Security and public-boundary impact

The set of host paths mounted into a CWD-owned runtime does not expand: one
selected root and one per-Workspace home remain the only writable host assets.
The public contract documents the conditional target path, and no credential,
policy, or host-home value is copied into the container.

## Mechanical enforcement

- Pure mapping tests cover home-relative, external, nested, sibling, and
  protected-home cases.
- Project runtime tests use the same mapping for create args and the spec hash.
- Integration creates projects under a synthetic host home and verifies nested
  `pwd`, project files, `HOME`, image-owned `gh`/`aws`, and host-home canary
  isolation.
- Runtime image checks retain the requirement that agent assets stay outside
  `/var/lib/tobari`.

## Compatibility and rollback

Logical roots and persistent homes do not change. Existing containers whose
spec hash uses the old `/workspace` target are recreated on the next
reconciliation; the host project root and home data remain in place. Reverting
the adapter restores the old target mapping without deleting logical state.

## Validation

`task check`, `task public:check`, and the Docker integration profile pass. The
security profile retains four pre-existing gosec findings in presentation
code; no finding points to this change.
