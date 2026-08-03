# Work Plan: Keep the auth-broker experiment detached from `main`

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Treat deferral as a governance boundary, not an implementation task. Inspect
both refs without checkout, run the supported checks against a temporary exact
`main` archive, assert public Catalog/help/path absence, and assert positive
branch-only object reachability. Then commit only this child packet.

## Alternatives considered

### Merge or cherry-pick now

Rejected because it would make provider-facing auth public before product,
architecture, security, and harness consequences are accepted.

### Record the decision only in the coordinator

Rejected because a coordinator note is not a mechanical guard against a future
branch or Catalog merge. This child packet must be independently inspectable.

## Design

### Public contract

`main` keeps its existing public command set and capability ledger.
Auth-broker commands, auth-profile authoring, and broker runtime selection are
not public, discoverable, or invocable from `main`. Explicit Git inspection of
the experiment ref is not a user-facing Tobari capability. This packet adds no
command, output field, reference kind, effect, mutation, or auth requirement.

### Layer changes

- Domain: none.
- Application: none.
- Infrastructure: none.
- CLI/Catalog: none; negative assertions prove absence from `main`.
- Documentation: only these four temporary packet files.

### Error and security behavior

Any failed assertion is a failed deferral verification. Do not compensate by
checking out a ref, copying branch code, changing a ref, or weakening a
negative pattern. If the required governance E2E is environment-blocked, keep
the packet Active and record the rerun command. A future broker requires
explicit secret custody, exact project-principal/host binding, redaction,
failure-closed behavior, revocation, cleanup, cross-project canaries, and
public-safe output before promotion.

## Implementation slices

1. Ref inventory: complete.
2. Domain/application behavior: not applicable; no auth implementation.
3. Infrastructure adapter: not applicable; no broker runtime activation.
4. CLI/Catalog: complete as a negative E2E.
5. Harness/documentation: complete after the required commit.

## Verification

- `task check`, `task public:check`, and `task build` passed on the exact `main` archive.
- Ref reachability and branch-only `git cat-file` checks passed.
- Built `main` help/Catalog/capability/path negative checks passed.
- `task security` was rerun on the current main line and passed; the auth
  branch remains detached and no security exception is needed for the
  deferral.
- The required docs-only main commit is created after the governance E2E; the
  auth implementation remains on `codex/auth-broker` and is not staged.

## Future restart gate

A new reviewed packet may reconsider the branch only when product evidence
shows material bounded-autonomy value; the thesis explicitly accepts the
provider scope; four-layer/Catalog/effect/reference contracts are defined;
security defines custody, binding, redaction, failure, revocation, cleanup, and
isolation; public/help/readiness contracts are updated; and the candidate passes
`task check`, `task security`, `task public:check`, and relevant runtime E2E.
Until then, `codex/auth-broker` remains an explicit experiment only.

## Commit handoff

The docs-only evidence was committed on `main` as `93e5cac`; no further Git
metadata operation is required for this packet. On explicit resumption, create
a new reviewed packet rather than reopening this accepted deferral.

Historical handoff command, already completed:

```sh
git add -- docs/work/auth-broker-deferral/goal.md docs/work/auth-broker-deferral/context.md docs/work/auth-broker-deferral/plan.md docs/work/auth-broker-deferral/tasks.md
git commit --only -m 'docs: record auth broker deferral' -- docs/work/auth-broker-deferral/goal.md docs/work/auth-broker-deferral/context.md docs/work/auth-broker-deferral/plan.md docs/work/auth-broker-deferral/tasks.md
```

The commit changed exactly four paths and the packet remains Accepted because
the implementation itself is deliberately not complete or merged.
