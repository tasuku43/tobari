# ADR 0011: Bind project authority to host-issued network identity

- Status: Accepted
- Date: 2026-08-01
- Revised: 2026-08-08
- Deciders: Tobari maintainers
- Scope: Product, architecture, and security
- Supersedes: The project-principal limitation recorded in the threat model
- Superseded by: None

## Context

Tobari keeps one shared Gateway and OPA cluster while allowing several
current-directory projects to run at once. A work container can control its
own environment and can send arbitrary proxy headers. Therefore a session
header, a project ID in request data, or a credential profile name cannot be
used as the source of project authority. Without a trusted binding, one
project's learned permission or managed credential could become a confused
deputy for another project.

The current-directory workflow remains intentional: the user may run Tobari
from any accessible project directory. An exact canonical root selects the
logical record directly; when only ancestors contain the directory, Tobari
lists them nearest-first and lets the user explicitly choose reuse or creation.
Location is selection context, not authentication.

## Decision

The host writes an owner-only, atomically replaced schema-v2 principal registry
for currently materialized project networks. Each binding contains one stable
UUIDv7 project ID, one stable UUIDv7 Context ID, the diagnostic Context name
and canonical root, one exact Docker network name, and the Gateway interface
address on that network. Docker network ownership, project state, permanent
Context binding, and labels are verified before a binding is written.

Gateway derives the Context/project principal from mitmproxy's local socket
address for the incoming proxy connection and the host-issued registry. Caller
headers, Context strings, URLs, environment, session metadata, and profile
names remain untrusted metadata. Missing, malformed, ambiguous, stale,
mismatched, or unregistered bindings deny before OPA and before upstream I/O.

Credential profiles are selected within one stable Context and must list the
project IDs to which they are bound. Gateway checks Context/project binding
before OPA and repeats it before reading a Context-scoped secret. Learned
denials, opaque candidates, learned rules, compaction groups, and audit evidence
retain both IDs; rule matching and reference identity include both. A rule or
secret approved for one Context/project cannot authorize another.

The initialized host policy remains an installation-wide baseline chosen by the
trusted host. It is not presented as a per-project allowlist. Project-specific
authority means that mutable learned grants and managed credential profiles
cannot cross the host-issued principal boundary. A future per-project static
baseline can use the same principal field without changing the Gateway
identity mechanism.

The registry is a runtime contract, not a Docker API exposed to the Gateway.
Docker is the current producer of the network/address binding; a stronger
sandbox adapter may produce the same host-issued contract without giving the
untrusted process a selector or transport escape hatch.

## Consequences

- Recreating a project network or Gateway attachment requires registry
  reconciliation before requests can succeed.
- Deleting a project removes its binding; cluster down clears all bindings.
- Editing or deleting the owner-only registry is a fail-closed configuration
  error, not a way to select another project.
- Existing credential profiles and learned rules without complete Context/project IDs are
  rejected rather than silently inherited.
- Shared Gateway and OPA resource ceilings, no-secret/no-socket mounts, and
  the CWD-owned selection boundary remain unchanged; only ambiguous ancestor
  entry now exposes the candidate choice explicitly.

## Mechanical enforcement

- Go tests validate registry schema 2, Context/project bindings, uniqueness,
  atomic update/remove, malformed-state rejection, and missing-file failure.
- Gateway tests prove local-network principal derivation, forged Context/session
  resistance, unknown-principal denial, and cross-Context/project credential denial
  before OPA.
- Domain and Rego tests prove candidate/rule identity and learned-rule matching
  retain Context/project identity.
- Docker integration creates same-root and overlapping-root Context-bound
  projects, checks distinct Gateway addresses, verifies learned-permission
  denial across Contexts/projects, then exercises restart, recovery, and exact
  deletion cleanup.
