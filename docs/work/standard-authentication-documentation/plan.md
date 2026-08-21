# Work Plan: Make authentication documentation match the standard profile

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Make README standard-first. Replace Broker-first security/authentication prose
with the existing native Workspace-owned contract and invocable direct/shell
examples. Retain only a short, explicit experimental research section that uses
`bin/tobari-dev` and links to the detailed authoritative authentication doc.
Extend existing profile/documentation enforcement so stale standard Broker
claims cannot return.

## Alternatives considered

### Add a caveat above the existing Broker-first section

This leaves most of README describing an unavailable standard command and the
wrong credential owner. Rejected.

### Remove all experimental authentication documentation

This is simple for standard users but makes repository contributor research
undiscoverable. Rejected in favor of one short explicitly experimental section
and the existing detailed reference.

### Restore Broker commands to standard

This would reverse the accepted trust boundary and capability profile to make
stale docs true. Rejected categorically.

## Design

### Public contract

The standard README path says:

```text
Authentication belongs to each Workspace.

Run Claude Code, Codex, GitHub CLI, AWS CLI, or another installed tool inside
the Workspace and use that tool's normal login. A reviewed browser flow opens
on the host when supported. Credentials remain in that Workspace home. Host
credentials and host CLI homes are never imported. The standard Tobari binary
has no `auth` command.
```

Representative standard examples:

```sh
tobari -- claude
tobari -- codex
tobari -- gh auth login
tobari
```

Examples are retained only after standard Runtime/catalog verification. The
experimental section uses `task build:dev` and `bin/tobari-dev auth ...`, states
that the profile is unsupported/unpublished and absent from standard/release,
and links to `docs/07_authentication.md`.

### Human-handoff scorecard

| Handoff | Stale README | Correct standard path |
|---|---:|---:|
| Tobari credential command | 1 `tobari auth ...` | 0 |
| Host credential export/import | described as Broker acquisition/import | 0 |
| Workspace client login | treated as unsupported fallback | 1 normal client flow |
| Browser transfer | Broker/provider-driver dependent | host opens reviewed native target |
| Fixed-value manual re-entry | provider/driver dependent | only provider-owned native flow when applicable |
| Login-state owner | Context Broker/vault | one Workspace client home |
| Steady-state Tobari commands | auth status/handle reconciliation | 0; re-enter Workspace normally |

The native provider may still ask for its own confirmation or device code.
Tobari neither adds a clipboard step nor claims to eliminate provider-owned
handoffs.

### Layer changes

- Domain: none
- Application: none
- Infrastructure: none
- CLI and catalog: no production change; profile/catalog tests supply the
  executable command authority for documentation checks
- Documentation/harness: README rewrite, concise experimental section, links,
  and deterministic standard-versus-experimental claim enforcement

### Data and control flow

```text
standard catalog + capability profile + native authentication contract
        ↓ documentation/profile consistency check
README standard examples and ownership claims

experimental catalog + explicit development executable
        ↓
short isolated research section
```

### Error and cancellation behavior

Unchanged. Native tools retain their own errors, cancellation, logout, refresh,
and state. Browser/opener failures keep existing manual fallback. Experimental
Broker faults remain visible only in the experimental catalog/docs.

### Security and public boundary

The change removes false claims; it does not weaken the standard boundary.
Credential values remain absent from examples, logs, fixtures, OPA, and audit.
The browser bridge remains purpose-limited and post-policy credential
forwarding remains exact.

## Implementation slices

1. Inventory and classify every README authentication/Broker/release claim.
2. Add failing standard/experimental documentation consistency tests.
3. Rewrite standard security/authentication/build prose and examples.
4. Reduce and explicitly isolate experimental Broker research text.
5. Verify links, profile matrix, handoff scorecard, and required gates.

## Verification

- Unit and contract tests: standard catalog lacks auth namespace; experimental
  catalog retains only its reviewed commands/providers
- Negative side-effect tests: not applicable to docs; existing profile tests
  prove no standard activation path
- Opaque-reference and complete-pagination tests: unchanged
- Structured output, hostile-output, and recovery tests: unchanged
- Agent-readiness scenario and discovery-round-trip count: standard login
  guidance selects direct Workspace entry without an unavailable command guess
- Human-handoff scorecard: update observed values above and preserve rationale
- Manual observation: standard `help` has no auth namespace; experimental help
  exposes it only through explicit development binary
- Required profiles: focused CLI/docs tests, `task check`, and `task security`
- Generated-diff or artifact checks: README links and any generated capability
  matrix/reference

## Rollout and rollback

Documentation-only correction with no state or runtime migration. Rollback is a
source revert, though restoring the stale Broker-first standard narrative would
reintroduce a known contract defect.

## Documentation promotion

- README standard security and Authentication sections.
- README build/profile and stale release-identity prose that depends on the old
  Broker-first model.
- Existing documentation/profile harness claims; no new durable auth design is
  needed because `docs/07_authentication.md` and ADR 0044 are authoritative.
