# Work Plan: First-use friction repair

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Repair the existing first-use workflow without changing Tobari's trust boundaries: make the initialized policy classify the README request consistently, add diagnostics for learned policy data read failures, clarify host-only denial guidance, warn about existing Workspaces during runtime build, and align the build/install text with the actual source build contract.

## Alternatives considered

### Change the README example only

This would avoid policy code churn, but it would leave the product's advertised deny-review-allow-retry slice dependent on a hidden request shape. The README example should be covered by executable tests.

### Make all default-deny requests learnable

This would simplify the queue but weaken the body, scheme, port, project, and credential safety gates. The chosen approach keeps the existing exact-rule preconditions.

## Design

### Public contract

No new public command is added. Existing public capabilities keep their role and effect declarations. Human and Gateway outputs become more actionable, but JSON schema fields remain compatible unless investigation shows a missing field is already contractually required. Policy denials remain bounded-window evidence, and policy actions still consume opaque IDs produced by discovery.

### Layer changes

- Domain: add or adjust pure validation/classification only if learnability currently misclassifies a safe empty-body request.
- Application: expose policy data read failures through read-only diagnostics if needed.
- Infrastructure: adjust OPA/Rego, Gateway denial text, or XDG policy data inspection at the owning boundary.
- CLI and catalog: adjust human presentation and README text without adding a competing command registry.

### Data and control flow

Gateway records one validated denial. CLI discovery reads bounded denial evidence and active policy data. If OPA marks the denial learnable and no learned decision already covers it, discovery emits one opaque candidate; the TTY review delegates that ID unchanged to allow or deny. Diagnostics read the same safe policy data state without mutating it.

### Error and cancellation behavior

Read-only commands remain read-only. Policy data failures are non-retryable rejections until the trusted host repairs or replaces unsafe data. Runtime build remains a host-side write; failure leaves the previous selected image active.

### Security and public boundary

No new secrets, mounts, network destinations, external schemas, or dependencies. Tests must preserve non-learnability for unsafe denial causes.

## Implementation slices

1. Contract and failing tests
2. Policy learnability fix
3. Doctor and presentation guidance
4. Runtime/build and README guidance
5. Verification and work-packet cleanup

## Verification

- Unit and contract tests: focused Go/Rego/Gateway tests around changed behavior.
- Negative side-effect tests: confirm non-learnable causes stay outside the candidate queue.
- Opaque-reference and complete-pagination tests: unchanged policy candidate/action round trips where relevant.
- Structured output, hostile-output, and recovery tests: updated rendering snapshots if present.
- Agent-readiness scenario and discovery-round-trip count: first-use denial reaches `policy review` without source inspection.
- Human-handoff scorecard for setup/authentication candidates: first-use route should not require Docker/OPA policy knowledge.
- Manual observation: clean XDG first-use replay if Docker remains available.
- Required profiles: `task check`; `task security` only if security boundary changes.
- Generated-diff or artifact checks: no temporary XDG/Docker evidence remains in repository.

## Rollout and rollback

State compatibility should be preserved. If stale policy data cannot be migrated safely, diagnostics should point to a trusted-host repair path rather than mutating it automatically. Rollback is the previous binary plus unchanged existing XDG data.

## Documentation promotion

Durable conclusions should be reflected in README and, if a contract meaning changes, the numbered product/security/harness documents.

## Residual release boundary

The source and embedded Gateway assets are repaired in this change, but ordinary
`cluster up` still runs the pinned published Gateway image. Updating that image
and the reviewed digest is governed by the existing
`docs/work/gateway-image-refresh-e2e` packet and release contract.
