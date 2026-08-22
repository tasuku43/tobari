# Work Plan: Align trusted-host review commands

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Make `review` a pure catalog namespace with two registered leaf commands:
`review permissions` and `review services`. Move the existing Permission Inbox
contract and handler from `policy review` to `review permissions`. Replace the
root selector with direct `review services` routing to the existing service
review flow. Let the generic catalog namespace renderer own bare `tobari
review`.

Keep `policy` as the lower-level permission resource namespace and `service` as
the lower-level service request/exposure resource namespace. Remove the two
superseded pre-public routes rather than preserving aliases.

## Alternatives considered

### Keep root `review` as a menu plus child commands

This would preserve the one-key selector, but the canonical catalog prohibits
one path from being both a registered command and a namespace. Weakening that
invariant or adding special routing would make help and dispatch ambiguous.

### Use subject-first top-level commands

`permission review` and `service review` are grammatically regular but spread
one trusted-host task family across two namespaces. They also make the existing
resource namespaces carry both human workflow and lower-level operations.

### Use an `inbox` namespace

This describes pending work but is weaker for immediate service decisions and
introduces a new product noun when `review` already names the user task.

## Design

### Public contract

```text
tobari review
Commands in namespace review:
  permissions  Review pending network permissions
  services     Review Workspace service exposure requests

tobari review permissions [--tail N] [--format text|json]
                           [--watch] [--notify auto|osc9|bel|off]
tobari review services
```

- `review permissions` retains the existing discover-plus-TTY-fixed-target-
  apply role, inputs, output schema, bounded collection, faults, staged
  decisions, explicit Apply, watch, notifications, exit behavior, and
  permission lifetime.
- `review services` retains the existing discover-plus-TTY-reference-bound-
  action role, fresh exhaustive service-request output, exact opaque request
  selection, explicit confirmation, immediate Allow once or Deny, exit
  behavior, and attachment lifetime.
- Bare `review` is generic namespace discovery, not a registered capability,
  mutation, or output schema.
- `policy candidates`, `policy rules`, `policy allow`, `policy deny`, and
  `policy reset` remain unchanged resource operations.
- `service requests`, `service allow`, and `service deny` remain unchanged
  resource operations and preserve their opaque-reference graph.
- Retire `policy review` and the former registered root `review` selector with
  no aliases. Existing persisted policy rules, denial evidence, service
  requests, exposures, and attachment records are unchanged and need no
  migration.
- Human headings should frame the shared navigation while preserving semantic
  distinction, for example `Tobari · Review · Permissions` and `Tobari ·
  Review · Services`; exact wording remains subject to existing presentation
  foundations and golden evidence.

### Layer changes

- Domain: none expected.
- Application: none expected; reuse the existing use cases and typed results.
- Infrastructure: none.
- CLI and catalog: move the Permission Inbox spec, replace the unified selector
  spec with the service leaf spec, remove selector routing, and update all
  catalog-owned help, recovery, rendering, and tests.

### Data and control flow

```text
review permissions -> existing policy review handler -> existing read/apply ports
review services    -> existing service review loop   -> existing request/action ports
review             -> catalog namespace renderer     -> no task adapter call
```

### Error and cancellation behavior

All error kinds, retryability, confirmation, cancellation, and cleanup remain
owned by the existing leaf workflows. Exact next actions and denial guidance
must name `review permissions` or `review services` as appropriate. Bare
namespace invocation performs no task read or mutation.

### Security and public boundary

No authority, route, target binding, asset, credential, destination,
dependency, or trust boundary changes. Permission decisions remain durable and
host-owned; service decisions remain attachment-local and host-owned. Exact old
paths are removed from public and generated artifacts.

## Implementation slices

1. Add failing catalog, dispatch, help, and retirement tests.
2. Move the Permission Inbox spec and exact navigation strings.
3. Replace the unified selector with direct Service review and delete the
   special selector path.
4. Update presentation fixtures, readiness evidence, docs, and ADR wording.
5. Regenerate derived artifacts, run gates, delete this packet, and commit one
   atomic implementation change.

## Verification

- Unit and contract tests: exact catalog paths, inputs, roles, outputs,
  namespace listing, dispatch, and scoped help for both leaves
- Negative side-effect tests: bare namespace and invalid/retired paths perform
  no application or infrastructure call
- Opaque-reference and complete-pagination tests: existing policy candidate
  and service request round trips remain unchanged; pagination remains not
  applicable
- Structured output, hostile-output, and recovery tests: existing JSON and
  visible-projection corpora with updated command identity; all next actions
  route through catalog-valid paths
- Agent-readiness scenario and discovery-round-trip count: root or `review`
  scoped help discovers both tasks; a known leaf needs one scoped help request
- Human-handoff scorecard: not applicable; no setup or authentication change
- Manual observation: run bare namespace, both scoped help forms, redirected
  leaf forms, and retired paths
- Required profiles: focused Go tests, `task check`, `task public:check`; run
  `task security` only if the final diff changes security behavior or claims
- Generated-diff or artifact checks: catalog/site generation and repository
  status audit

## Rollout and rollback

This is a pre-public command replacement. There is no alias period and no state
migration. A rollback restores the previous catalog paths and selector code;
it does not alter persisted state. Capability retirement evidence below proves
that no hidden fallback survives.

## Documentation promotion

- Product and thesis text names the task-level `review` namespace and both
  leaves.
- Architecture and security text retain separate-host ownership and describe
  distinct durable versus attachment-local authority.
- Harness and readiness contracts fix exact paths and negative retirement
  canaries.
- ADR 0074, and ADR 0073 where exact Permission paths appear, are revised or
  superseded so durable decisions do not name removed commands.
