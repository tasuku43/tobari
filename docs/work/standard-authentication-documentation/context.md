# Work Context: Make authentication documentation match the standard profile

## Current behavior

- `docs/07_authentication.md` says ADR 0044 supersedes Broker-first standard and
  defines the standard/release profile with no provider binding, projection,
  handle, vault, root key, companion, Broker service, or `auth` command.
- Standard Claude Code, Codex, GitHub CLI, AWS CLI, TWG, and pup flows use
  client-owned login state inside one Workspace home. The reviewed attachment
  bridge may open one semantically validated host browser target and, for the
  closed callback variants, relay one opaque loopback callback.
- Gateway removes client authentication and cookies from OPA/audit, preserves
  the original value, and forwards it only after the ordinary HTTP effect is
  allowed.
- `internal/cli/auth_catalog_standard.go` deliberately returns no public auth
  commands. Standard-profile tests reject `auth login`, `auth import`, `auth
  logout`, and `auth status`.
- `task build:dev` compiles the experimental Broker capability and `auth`
  commands behind the experimental profile; standard runtime input cannot
  activate them.
- README's “What V1 protects” and Authentication sections still describe
  Broker-required provider bindings, project-bound handles, vault isolation,
  Broker-first routing, and unqualified `tobari auth` commands as standard.
- README's build/release wording also mixes standard release identity with Auth
  Broker publication despite durable docs describing the experimental Broker
  as unsupported and unpublished.

## Relevant structure

- Entry point: README Quick Start, security summary, Authentication section,
  build-profile description, and release checkpoint prose
- Domain rule: authentication mode and capability profile types
- Application use case: no standard auth use case; experimental `authcmd` is
  build-tagged
- Infrastructure boundary: native attachment opener/callback relay, Workspace
  home, Gateway header handling, and experimental Broker adapters
- CLI catalog or presentation: standard/experimental catalog composition,
  version capability profile, Context authentication report, and help
- Existing tests and harness checks: auth catalog standard/experimental tests,
  build identity matrix, README/claim guards, repoguard, contractlint, native
  login fixtures, and security profile

## Constraints

- Standard has no provider extension boundary or runtime switch capable of
  enabling Broker functionality.
- Tool-owned native credentials are readable by every process in the same
  Workspace; documentation cannot claim per-tool secret isolation.
- Host credentials, host CLI homes, token caches, and credential helpers are
  never inherited or copied.
- Browser opening is not network authorization. The ordinary Gateway/OPA policy
  independently decides client bootstrap/API effects.
- Typed bootstrap may import only declared secret-free configuration; it is not
  authentication state.
- Experimental research must not appear supported, published, or invocable by
  the standard executable.
- Public preparation and release changes remain outside scope; stale prose may
  be corrected to the existing durable contract only.

## External facts

No external source is required. The authoritative facts are repository-owned
profile composition, catalog tests, ADRs, and authentication/security contracts.

## Unknowns

- [ ] Inventory every README Broker/provider/release assertion and classify it
      as standard truth, experimental research, or stale removed design.
- [ ] Determine the smallest experimental README section that remains useful
      without duplicating the detailed authentication reference.
- [ ] Confirm the exact standard native command examples against the standard
      Runtime and catalog; avoid implying that an optional custom-runtime
      client is always installed.
- [ ] Identify an existing documentation/profile guard to extend rather than
      introducing a competing README parser.
- [ ] Reconcile any remaining stale Auth Broker publication prose with the
      authoritative release/non-publication contracts without changing release
      tooling.

## Thesis evidence

- Repeated design decision or point of agent confusion: the standard catalog
  mechanically removes auth commands while the primary README instructs users
  to run them.
- User outcome or friction observed in the minimal slice: a standard user
  cannot tell whether login belongs to the Workspace client or a host Broker and
  receives a non-invocable command path.
- Code workaround or exception being considered: retaining both narratives and
  adding a caveat would continue to misstate ownership and support.
- Current thesis that resolves it, or proposed thesis revision: no thesis
  revision is needed; promote the already accepted standard-native contract.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: README, profile docs, documentation claim tests, and human-handoff
  evidence only.

## Reproduction or observation

```sh
rg -n 'Broker-first|broker_auth_required|tobari auth|Brokered reviewed providers|Auth Broker' README.md
go test ./internal/cli -run 'Standard.*Auth|Auth.*Standard|BuildIdentity'
```

Observed 2026-08-21: README presents unqualified Broker auth commands while the
standard catalog and authoritative authentication contract explicitly exclude
the namespace.

## Security and public-boundary notes

- Assets and side effects involved: documentation, build-profile command
  inventory, and static claim tests; no runtime side effect
- Credentials or confidential data involved: no credential values; examples
  must use no real account, token, organization, or private endpoint
- New dependencies, destinations, files, processes, or generated content: no
  runtime dependency/destination/process; one deterministic docs guard may be
  extended
- External schema provenance, publication rights, and drift evidence: all
  affected docs and capability profiles are Tobari-owned
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged
- Publication and licensing concerns: none; no release or external content

## Glossary

- **Native Workspace authentication:** a client creates and owns its credential
  state inside one persistent Workspace home.
- **Native login bridge:** the attachment-scoped, pinned-client host browser and
  closed callback relay; it does not own credentials or policy authority.
- **Experimental Broker profile:** repository-only development capability with
  Context vaults, project-bound handles, and build-tagged `auth` commands.
- **Standard example:** a command invocable by the standard/release catalog and
  base/custom Runtime contract without an experimental executable.
