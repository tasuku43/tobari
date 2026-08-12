# Work Goal: Select direct source access per Context

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Context capability-envelope decision and direct-root security boundary
- Review/delete trigger: Delete after durable contracts are promoted and the first public V1 change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before first public V1 release
- Related ADRs: ADR 0010, ADR 0013, ADR 0014, ADR 0018, and ADR 0027
- Parent: [Context capability envelope](../context-capability-envelope/goal.md)

## Outcome

A user can create a Context whose selected project source bind is permanently
`read-only` or `read-write`. Every Workspace bound to that Context receives the
declared direct access, while its Tobari-owned home and temporary files remain
writable and the product makes no clone, snapshot, or apply-back claim.

## Why now

Direct read-write source access is the largest accepted host-integrity risk in
the current execution model. A direct read-only bind is a narrow Docker-backed
control that fits the existing canonical-root-plus-Context identity without
introducing Git, copy, overlay, synchronization, or result-application
semantics.

## Non-goals

- Clone, overlay, copy-on-write, snapshot, branch, commit, diff, apply, or
  cherry-pick workflows.
- Making the entire Workspace filesystem read-only.
- Protecting source confidentiality or preventing network exfiltration.
- Protecting the source from host processes or Workspaces in read-write
  Contexts.
- Changing source access after Context creation.
- Per-Workspace or per-session source-access overrides.
- Adding write allowlists, subpath permissions, filesystem approval, or
  organization-level filesystem policy.

## Acceptance criteria

- [ ] `context create` accepts optional `--source-access
      read-only|read-write`, defaults to `read-write`, and validates it before
      Context persistence or Docker activity.
- [ ] Context manifest, summary, report, catalog help, JSON/text output,
      generated documentation, and schema ledger carry an explicit required
      source-access value.
- [ ] Source access is immutable for the Context lifetime and contributes to
      the project runtime spec hash and reconciliation evidence.
- [ ] A read-only Context emits an exact direct bind with Docker read-only
      enforcement; a read-write Context retains the current direct bind.
- [ ] Read-only integration proves source read succeeds, source create/change/
      delete and Git metadata writes fail, Workspace home and tmpfs writes
      succeed, and no second writable source alias exists.
- [ ] Same-root read-only/read-write Context integration proves separate
      Workspaces and honestly demonstrates that the read-only view observes
      host/read-write-Context changes.
- [ ] Threat model, security docs, help, and errors distinguish selected-source
      integrity from snapshot integrity, source confidentiality, and whole-
      Workspace read-only behavior.
- [ ] `task check`, `task security`, and `task public:check` pass.

## Governing documents

- Thesis: CWD-owned Workspace and Context composition in `docs/00_theses.md`
- Product contract section: project-root selection/mount and Context creation
- Architecture or security invariant: one selected source bind, fixed mount
  set, invoking host UID/GID plus read-only-rootfs/cap-drop runtime, no Docker
  socket
- Existing ADR: ADR 0010 direct read-write root, revised to permit immutable
  Context-selected direct access without adding a second binding mechanism

## Completion definition

The work completes when both modes have product-shaped integration evidence,
all claims are promoted and executable, required gates pass, the parent Context
packet incorporates the result, and this temporary packet is removed.
