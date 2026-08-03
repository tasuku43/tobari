# Work Plan: CLI catalog audit

- Status: Active (verification complete; commit blocked by repository state)
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Treat `DefaultCatalog().Commands()` as the authoritative finite set. Build a
clean binary, obtain root and scoped human/agent help, and run representative
argv for every public command plus each suspected duplicate or retired surface.
Cross-check each result against the handler, application task, declared effect,
fixed/reference target, output contract, fault recovery, and integration flow.
Classify commands without changing the public contract. Only remove source
definitions that are demonstrably unreachable, have no persisted-state or
supported-test owner, and can be deleted as a small buildable change; keep
public-contract retirement as a follow-up when it needs migration or durable
decision work.

## Alternatives considered

### Alternative A: Delete every similar-looking command

Rejected. Similar text output does not prove the same user task. The product
contract deliberately distinguishes typed denial evidence, machine candidate
discovery, human review, compatibility tail, and exact actions.

### Alternative B: Use only static inventory and unit tests

Rejected. The acceptance rule requires clean-build human/agent help and actual
argv execution with fault/recovery and side-effect-boundary evidence.

### Alternative C: Remove only the unregistered legacy definitions

Selected as the only possible production cleanup. It is disjoint from the
public catalog if compile, catalog, negative-path, and E2E checks prove no
supported route depends on those definitions. The catalog continues to reject
the historical names through its explicit retired-command message.

## Design

### Public contract

No public contract changes are planned by the audit. The public set remains the
catalog's 25 paths unless a safe dead-definition cleanup removes only unregistered
source declarations. Roles/effects and reference flow are documented in the
E2E transcript. Policy review remains a read-only discover command in the
catalog, with a TTY-only confirmation workflow that delegates one unchanged
candidate ID to `policy allow` or `policy deny`.

### Layer changes

- Domain: none planned.
- Application: none planned; legacy application methods remain a separate
  follow-up unless reachability proves a safe shared cleanup.
- Infrastructure: none planned.
- CLI and catalog: inventory and, only if proven safe, removal of unregistered
  legacy CLI spec/handler definitions. The public catalog remains the source of
  truth.

### Data and control flow

```text
argv -> catalog lookup and typed parser -> declared handler
     -> application port -> infrastructure boundary (only for commands whose
        prerequisites reach a configured runtime)
     -> structured output or declared fault/recovery

help argv -> same catalog -> root index or exact/namespace scoped projection
```

The audit never passes a display position as a policy target. Candidate IDs are
read from discovery output and, where mutation is executable, passed unchanged
to the exact reference-bound action.

### Error and cancellation behavior

Invalid or retired paths must fail before an application or Docker call with a
stable usage/retired fault and an exact next command. Runtime-unavailable reads
must produce the catalog-declared fault/recovery. Mutations must preserve the
declared non-retryable reconciliation faults and must not be retried merely
because an E2E environment lacks Docker.

### Security and public boundary

The audit uses synthetic roots, names, and candidate values. No credentials,
host paths, Docker identifiers, private URLs, or external content enter the
packet. Any future removal must use the capability-retirement checklist for
catalog paths, fault/recovery edges, dormant fallbacks, dependencies, and
persisted state.

## Implementation slices

1. Contract and failing/negative-path evidence: inspect catalog tests and
   retired-command routing.
2. Domain and application behavior: map each handler to its task and port.
3. Infrastructure adapter: observe only bounded runtime failures or successful
   synthetic E2E where Docker is available.
4. CLI catalog and presentation: generate root/scoped human and agent help and
   compare it to the declared catalog.
5. Harness and documentation: record transcript, gate outputs, classifications,
   and follow-up docs findings in this packet.

## Verification

- Unit and contract tests: catalog, help, argument, output-schema, and focused
  CLI tests.
- Negative side-effect tests: retired paths, invalid inputs, missing runtime,
  and mutation candidate errors before Docker/policy calls.
- Opaque-reference and complete-pagination tests: catalog reference graph plus
  policy candidate/review/tail -> allow/deny and compactions -> compact.
- Structured output, hostile-output, and recovery tests: representative text,
  JSON, invalid argv, runtime-unavailable faults, and declared next actions.
- Agent-readiness scenario and discovery-round-trip count: root/scoped help plus
  each representative argv; zero undeclared external processing.
- Human-handoff scorecard for setup/authentication candidates: not applicable;
  this packet does not introduce setup or authentication.
- Manual observation: inspect clean-binary outputs and side-effect boundaries.
- Required profiles: `task check`; `task public:check` if public docs/code change.
- Generated-diff or artifact checks: clean binary build and repository status.

## Rollout and rollback

No public behavior rollout is planned. If unregistered dead definitions are
removed, rollback is a source-level revert; no persisted state is deleted or
migrated. Public command retirement remains a separate reviewed change.

## Documentation promotion

- Promote only a durable command-retirement or compatibility decision if the
  evidence requires it.
- Forward the stale README `tobari exec` example to the documentation packet;
  do not edit it here.
- If source cleanup is accepted, preserve the explicit retired-command negative
  test and update the capability-retirement record in the follow-up change.
