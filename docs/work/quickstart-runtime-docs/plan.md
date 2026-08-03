# Work Plan: Quick Start runtime documentation

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Rewrite the README Quick Start as one short, host/agent-labelled journey over
the existing catalog: prepare a synthetic project directory, run `doctor` and
explicit `cluster up`, enter the CWD-owned Tobari, issue one learnable denied
`curl`, leave the session, review the exact candidate on the host, allow it by
opaque ID, retry the same request, then initialize and edit the active Context
Dockerfile, run explicit `runtime build`, and enter the custom runtime. Keep
advanced topology, authentication, policy, and compatibility material outside
the primary path. Record the real harness replay and gates in the packet.

## Alternatives considered

### Alternative A: Keep the existing split Quick Start sections

Rejected because a new user must infer the order among cluster startup, the
in-Tobari denial, host policy recovery, and Context runtime customization. The
existing facts are correct, but the safe outcome is not a single runnable path.

### Alternative B: Document direct Docker build and image selection

Rejected because it creates a second runtime authority, makes image identity a
routine user input, and bypasses the active Context's explicit `runtime init` /
`runtime build` boundary. The README keeps direct image selection as advanced
compatibility material only.

## Design

### Public contract

No public CLI contract changes. The README uses these existing commands:

- `tobari doctor --root .` and `tobari cluster up` for host preparation;
- `tobari` for fixed current-directory entry;
- in-Tobari `curl -X PUT https://example.com/quickstart` for a synthetic,
  bodyless learnable denial;
- host `tobari policy review`, read-only JSON review, and exact
  `tobari policy allow --id ID` or `policy deny --id ID`;
- `tobari runtime init`, `tobari context show`, the active Context Dockerfile,
  explicit `tobari runtime build`, and a final `tobari` entry; and
- `tobari delete` plus `tobari cluster down --purge` for cleanup.

`policy review` and `policy candidates` remain discover/read surfaces;
`policy allow` and `policy deny` remain exact reference-bound host actions;
`runtime init` and `runtime build` remain command-bound fixed-target host
actions. The README does not expose IDs as lifecycle inputs or claim that an
upstream HTTP status is controlled by Tobari after policy allow.

### Layer changes

- Domain: none.
- Application: none.
- Infrastructure: none.
- CLI/catalog: none.
- Documentation packet and README: clarify the existing workflow and record
  evidence only.

### Data and control flow

```text
trusted host
  -> doctor -> cluster up
  -> tobari fixed CWD entry
untrusted process inside Tobari
  -> curl -> Gateway/OPA -> secret-free 403 denial
trusted host
  -> policy review (read-only discovery)
  -> one exact opaque policy allow/deny
  -> same in-Tobari request retry
trusted host
  -> runtime init -> edit active Context Dockerfile
  -> explicit runtime build -> validated image promotion
  -> tobari fixed CWD entry with the selected Context runtime
```

The agent never receives the policy path, Docker authority, Context manifest
authority, credentials, or a candidate ID from the denial response. The host
chooses the exact action from bounded read-only evidence.

### Error and cancellation behavior

- Missing or incompatible Gateway startup stops before shared resource
  mutation; recover with `tobari doctor`, `tobari cluster status`, and a retry
  of `tobari cluster up`. `--gateway-source` is explicit and limited to its
  declared source/recovery use.
- A non-TTY root invocation stops with `tty_required`; rerun `tobari` in an
  interactive terminal. A canceled or stale Workspace choice performs no
  logical or Docker mutation; rerun `tobari` and choose again.
- A learnable denial remains `403` until the host explicitly confirms one
  exact allow or deny. `policy review --format json` is read-only; a failed or
  stale action is not replayed blindly. Retry only after a successful host
  action.
- `runtime_recipe_missing` recovers through `tobari runtime init`. Existing
  recipes are never overwritten. A failed/incompatible `runtime build` directs
  the user to `tobari context show` and leaves the prior Context image active.
- A session exit detaches without deleting the Workspace. Cleanup is the exact
  CWD-local `tobari delete`, then `tobari cluster down --purge` once no Tobari
  remains.

### Security and public boundary

The README names the host/agent trust boundary without suggesting that the
agent can administer policy, Docker, Context stores, or host credentials. The
curl is bodyless and synthetic; the runtime example adds only the harmless
`tree` package to the Context Dockerfile. No credential, secret, private URL,
machine path, or shell history is recorded. The explicit runtime build may
obtain its declared base only because the user invoked that host action.

## Implementation slices

1. Read governing contracts and the existing catalog/harness.
2. Add the packet from `docs/work/_template` and rewrite only the permitted
   README journey.
3. Replay help, integration, and runtime customization evidence; record exact
   results or the exact environment stop.
4. Run `git diff --check`, `task check`, `task public:check`, and relevant
   integration/runtime checks.
5. Verify the allowed-path diff and create one scoped commit on `main`.

## Verification

- Unit/contract tests: `task check` passed in a clean detached checkout; no
  source contract changes.
- Negative/recovery review: inspect README commands against scoped catalog help
  and the recorded failure paths.
- Opaque-reference proof: record policy discovery followed by unchanged
  `pcy_...` input to `policy allow --id` in the E2E transcript.
- Runtime proof: record `runtime init`, active Dockerfile edit, explicit
  `runtime build`, Context promotion, and final `tobari` entry or the exact
  environment blocker.
- Agent-readiness: use the existing integration journey; routine successful
  policy recovery has zero undeclared provider parsing, source inspection,
  exploratory calls, or automatic retry.
- Required profiles: `task check` and `task public:check` passed; the relevant
  policy/Gateway/base runtime profiles passed; aggregate integration/runtime
  profiles stopped at the existing PTY helper with exit 130 and are not claimed
  green.
- Manual observation: inspect the final README for concise ordering, host/agent
  ownership, exact recovery commands, synthetic values, and no stale retired
  command paths.
- Artifact/diff check: `git diff --check` passed and the final stage/commit uses
  an explicit allowed-path list while preserving unrelated dirty paths.

## Rollout and rollback

No runtime, configuration, or public command behavior changes. Reverting this
single README/packet commit restores the previous documentation without
changing any Tobari state, image, policy, or Docker resource.

## Documentation promotion

No durable thesis, product, architecture, security, release, or ADR change is
needed. The packet records evidence for an existing contract; if later replay
shows a contract mismatch, revise the governing document and its enforcement in
a separate reviewed change before changing the docs again.
