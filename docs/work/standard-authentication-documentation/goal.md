# Work Goal: Make authentication documentation match the standard profile

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Standard native Workspace authentication, capability profiles, README, and documentation harness
- Review/delete trigger: Delete after standard/experimental authentication claims are corrected and mechanically enforced
- Successor: None
- Owner: Tobari maintainers
- Target: Before remaining pre-public UX work
- Related ADRs: ADR 0044, ADR 0048, ADR 0051, ADR 0053, ADR 0055, ADR 0057, ADR 0058, ADR 0063, and ADR 0065

## Outcome

README and nearby entry-point documentation describe the standard binary's
actual authentication model: each tool performs its native login inside one
persistent Workspace home, reviewed browser/callback flows may bridge to the
host, host credentials are never inherited, and the standard catalog has no
`auth` namespace. Legacy Auth Broker commands and provider acquisition are
clearly labeled as unsupported experimental development research and use only
an explicitly experimental executable in examples.

## Why now

The authoritative authentication document and standard catalog already define
native Workspace-owned login with no Broker, vault, handle, companion, or
`auth` command. README still presents Broker-first routing, provider bindings,
`tobari auth login`, brokered reviewed providers, and Broker release identities
as the standard V1 path. This sends users to commands that do not exist and
misstates credential ownership and the trust boundary.

## Non-goals

- Changing native login, browser/callback bridging, credential storage,
  Gateway redaction/forwarding, or Workspace deletion behavior.
- Adding an `auth` command, Broker, vault, handle, provider binding, or shared
  credential to the standard profile.
- Removing or redesigning the repository-only experimental Broker code.
- Publishing an experimental binary, Broker image, provider artifact, or
  release.
- Expanding the reviewed native-login client or URL union.
- Changing provider credentials, fixtures, or live login compatibility.
- Rewriting the complete detailed authentication reference when it is already
  authoritative and correct.

## Acceptance criteria

- [ ] README's primary security and Authentication sections state that standard
      credentials are tool-owned inside each Workspace home, readable by that
      Workspace, and never inherited from host CLI homes.
- [ ] Standard examples use invocable native commands such as `tobari --
      claude`, `tobari -- codex`, or `tobari -- gh auth login`; no standard
      example contains `tobari auth`.
- [ ] README explains that reviewed native browser/callback bridging does not
      make Tobari the credential owner and grants no HTTP policy authority.
- [ ] Experimental Broker material is short, explicitly repository/development
      only, unsupported and unpublished, and every auth-command example names
      an experimental executable such as `bin/tobari-dev`.
- [ ] Stale Broker-first standard/release claims are removed or corrected
      wherever the README authentication narrative depends on them, without
      performing release work.
- [ ] README, `docs/01_product_contract.md`, `docs/07_authentication.md`, build
      profile descriptions, and catalog capability matrix agree about the
      absence/presence of the `auth` namespace.
- [ ] A deterministic documentation/profile contract rejects unqualified
      `tobari auth` examples and Broker-required/vault/handle claims in the
      standard README path while allowing explicitly experimental material.
- [ ] The human-handoff scorecard shows that the standard path requires no
      Tobari credential command, host credential transfer, or manual fixed-value
      re-entry beyond provider-owned flows.
- [ ] No production command, build tag, capability profile, state schema,
      authentication boundary, or release artifact changes.
- [ ] Focused documentation/profile tests, `task check`, and `task security`
      pass.

## Governing documents

- Thesis: standard Workspace-owned authentication and attachment-scoped native
  browser bridge in `docs/00_theses.md`
- Product contract section: standard capability profile, native Workspace
  authentication, browser/callback bridge, and experimental auth commands
- Architecture or security invariant: credentials remain Workspace-owned;
  Gateway redacts before policy and forwards only after allow; standard has no
  Broker activation path
- Existing ADR: ADR 0044 supersedes Broker-first standard; later native-login
  ADRs fix the closed browser/callback union

## Completion definition

The work is complete when every routine authentication claim and example is
truthful for the standard executable, experimental Broker text cannot be
mistaken for standard support, profile/documentation checks prevent regression,
required gates pass, and this temporary packet is removed.
