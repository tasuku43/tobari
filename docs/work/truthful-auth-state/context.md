# Work Context: Report authentication state and change truthfully

## Verified facts

- The Context provider grant revision is authoritative for configuration.
- A logical project's stable `ProjectID`, canonical root, and `ContextID` come
  from the project store. `ProjectID`, not root or row order, is identity.
- The project auth registry supplies its own exact `ProjectID`, provider
  revision, and binding digest. The Broker `binding_status` observation is
  correlated with the exact provider and registry revision before use.
- Infrastructure returns bounded, secret-free observation facts only.
  Application/domain validate request Context, project, and revision correlation
  and derive public activation, action, and change semantics.
- Provider configuration and Workspace projection activation are separate.
- The Auth Broker logout response already supplies an authoritative `changed`
  boolean. `changed=false` crosses no activation-change observation boundary.
- Auth JSON advances from schema 3 to schema 4 because activation detail and a
  mutation `change` state alter the public result contract.
- No external API capability, destination, authentication plan, or secret
  ingress changes; `docs/08_external_api_contracts.md` therefore remains unchanged.

## Constraints and bounds

- Provider configured state is Context-wide; Workspace handles are project-bound.
- Locked/unavailable Broker state cannot be interpreted as not configured.
- Enumeration coverage is explicit: `exhaustive`, `unavailable`, or
  `not_applicable`; exhaustive zero is distinct from unavailable.
- Activation output has at most 1,024 Workspace rows, 64 providers per row,
  1,024 Broker binding checks, and 4,096 bytes per canonical root.
- The 1,024-call policy is intentionally smaller than the storage layer's broad
  structural ceiling: the public task must remain bounded in routine time and
  output and cannot inherit a 65,536-entry host-store maximum as an external
  call budget.
- Auth mutation never changes network policy or a running process.
- Modified or unowned complete-file projections must not be overwritten or removed.
- Human presentation uses `human-presentation-foundation` cancellation semantics.

## Semantic decisions

- Aggregate activation states are `ready`, `workspace_reentry_required`,
  `not_applicable`, `unavailable`, and `unresolved`.
- Provider projection states are `current`, `missing`, `stale`, `unavailable`,
  and `not_applicable`.
- Exact re-entry is only emitted for a fully known stale/missing Workspace and
  is the pair of canonical working directory plus
  `tobari --context <validated-context>` argv.
- Mixed re-entry and unavailable facts are `unresolved` and carry no action.
- A confirmed mutation receipt requires the Broker `changed` result plus
  authoritative post-mutation facts. Advisory projection-observation failure is
  explicit unavailable coverage and never creates replay permission.
- A no-op logout is `no_change`, carries no provider/Workspace activation facts,
  and claims neither removal, revocation, nor re-entry.
- Unsupported login recovery is `auth status`, which exposes the installed
  provider collection, rather than unsupported `auth import`.

## Evidence and fixtures

- Pinned typed corpus covers current, missing, stale, configured-with-zero-
  Workspace, enumeration unavailable, locked-but-enumerable, mixed unresolved,
  changed login, changed logout, and no-op logout.
- Its answer key records routine-success external processing count zero and
  negative-inference canaries.
- Domain tables reject mismatched Context, project identity, digest, binding
  provider/revision, duplicate projects, discarded non-persisted facts, and
  discarded no-change activation facts.
- CLI typed tests cover configured selector marker/warning and neutral cancel;
  fixture tests cover result/status interpretation and exact actions.
- Implementation was developed in a clean detached worktree to preserve
  unrelated dirty worktree edits. It was moved out of a temporary directory
  before Docker integration because Colima cannot write that bind mount.
- Required committed-base harness repairs aligned the Auth Broker image checker
  with authoritative API version 3, the integration OPA mount assertion with
  the owned read-only `tobari-policy-bundle` volume at `/bundle`, and stale
  policy human-output assertions with the current typed presentation. These
  repairs change no production behavior and retain the same security checks.
- Focused Go suites, the 82-test Auth Broker suite, Docker integration, and the
  security profile pass. The full gate reaches contract lint and fails only
  because both generated Pages JSON-schema tables under the explicitly excluded
  `docs/architecture-site/**` subtree contain committed-base stale versions for
  `report`, `contexts`, `context`, `auth`, and `status`. This packet does not
  alter that prohibited subtree.

## Security and public-boundary notes

- Assets/effects: encrypted Context provider record and project-bound handle/projection metadata.
- Credentials: only secret-free state/revision/account labels; no value reads in CLI.
- New dependencies/destinations: none.
- Publication: synthetic provider/account labels, IDs, roots, and revisions only.

## Glossary

- Provider configuration: whether one Context owns a readable provider grant.
- Workspace activation: whether a particular eligible Workspace has the
  expected current project-bound projection.
- No-op logout: requested provider record is already absent and no authority or
  projection changes.
