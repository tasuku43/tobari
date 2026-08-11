# Work Goal: Keep declared reads observational on first use

- Status: Active
- Retention: temporary
- Retention reason: Required public documentation synchronization is deferred by an explicit no-Pages-change instruction
- Governing contract: Project Theses 5 and 7; read-effect invariant
- Review/delete trigger: Synchronize the public architecture-site schema tables, pass `task check`, then delete this packet
- Successor: None
- Owner: Unassigned packet agent
- Target: Explicit and Resumable UX program, state-foundation lane
- Related ADRs: None

## Outcome

Running a declared read command against a fresh installation observes an
explicit absent or synthetic-default state without creating durable Context,
policy, credential, auth, project, or lock files. Initialization remains owned
by declared create/write operations, and later diagnostics do not depend on
which read happened first.

## Non-goals

- Do not remove bounded recovery of already existing interrupted journals.
- Do not make absent default Context equivalent to a persisted valid Context
  for later mutation.
- Do not initialize Docker, policy, auth, or Workspace state from doctor/status/list.
- Do not redesign Context creation or legacy migration beyond separating read
  observation from durable commitment.
- Do not claim zero operating-system metadata outside Tobari-owned paths.

## Acceptance criteria

- [x] Every public `EffectRead` command has a first-use filesystem and external
      call canary proving no new Tobari-owned durable configuration/state files
      except explicitly permitted cleanup of pre-existing journals.
- [x] Context reads can report an absent or task-owned synthetic default without
      persisting it or making it mutation authority.
- [x] Auth status on a fresh installation reports explicit uninitialized/absent
      state without creating provider/vault/key/credential directories or files.
- [x] Lifecycle status/list do not create Context or project locks/records and
      preserve exact empty scope.
- [x] Read-only observation of legacy state never commits migration; the first
      authorized mutation performs validated atomic migration if still required.
- [x] Concurrent reads remain deterministic and cannot race to initialize state.
- [x] Read-only paths work with a read-only synthetic XDG tree when observation
      otherwise permits it.
- [x] Effect/catalog tests fail when a read path calls an initialization port.
- [x] Product, architecture, security, and harness explicitly name the narrow
      pre-existing journal-cleanup exception.
- [ ] `task check` and `task security` pass. (`task security` passes; `task check`
      is deferred solely on forbidden architecture-site schema-table updates.)

## Completion definition

The work is complete when the catalog-wide read-effect canary covers first use,
legacy and concurrency cases, initialization ownership is explicit, required
profiles pass, and this temporary packet is removed.
