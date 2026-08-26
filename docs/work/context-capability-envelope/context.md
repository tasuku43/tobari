# Work Context: Make the Context Boundary explicit

## Current behavior

ADR 0071 reconciles this packet's original whole-Context immutability framing:
Context is now the stable reusable work mode, while only its creation-time
source/network Boundary is the immutable capability envelope. The unfinished
supported-platform source-access evidence below remains owned by this packet
and its child.

- `internal/domain/tobari.ContextManifest` schema V1 stores stable ID, name,
  agent profile, image, canonical typed policy-data snapshot, runtime recipe record,
  shell environment, and Git identity. It has no source-access or policy-preset
  fields.
- ADR 0013 already calls Context a logical composition whose policy,
  credentials, runtime, and profile remain physically separated.
- A canonical root plus stable Context ID is the Workspace key. The same root
  may have separate Workspaces in separate Contexts.
- `context use` changes only the current/default Context marker and cannot
  retarget an existing Workspace.
- Every Workspace currently receives a direct read-write source bind.
- Every new Context currently copies the same three embedded policy-domain
  examples, including immediate authority/method grants. The selection is not
  named or shown as a revisioned user choice.
- The fixed Tobari-owned evaluator executes canonical typed policy data; legacy
  executable-policy markers are detected at the storage boundary and rejected.

## Relevant structure

- Entry point: `context create`, `context list`, `context show`, `context use`,
  and root `tobari --context`
- Domain rule: `internal/domain/tobari/context.go`
- Application use case: `internal/app/contextcmd`
- Infrastructure boundary: `internal/infra/dockerruntime/context_store.go`,
  project runtime reconciliation, aggregate policy construction
- CLI catalog or presentation: Context specs and report output in
  `internal/cli/runtime_catalog.go`
- Existing tests and harness checks: Context domain/application/catalog tests,
  store safety tests, same-root/multi-Context integration, schema and claim
  ledgers

## Constraints

- Context remains a logical composition, not a new secret or code-loading
  boundary.
- Names are selectors; stable IDs and host-owned principal bindings carry
  authority.
- A Context fact that can expand authority cannot change implicitly because a
  source preset file or current/default marker changed.
- Exact V1 readers reject missing, unknown, or malformed required fields; no
  pre-public migration reader is added.
- Reports must distinguish the selected source bind from other writable
  Workspace mounts and preset origin from the current effective learned state.
- No user-authored executable source can override a preset guardrail; the fixed
  evaluator remains the only executable policy authority.

## External facts

No external product contract is required. Docker mount behavior is verified in
the source-access child packet; network semantics are owned by Tobari policy.

## Unknowns

- [ ] Confirm the final machine field names and JSON nesting against the finite
      Context report variants before changing schema V1.
- [ ] Confirm whether `context list` carries the full guardrail summary or only
      source access, preset identity, and revision; `context show` must carry
      the complete summary.

## Thesis evidence

- Repeated design decision or point of agent confusion: Context is described as
  a bundle, yet direct source authority and implicit initial network authority
  are not visible in that bundle.
- User outcome or friction observed in the minimal slice: a user wants named
  `review` and `development` Contexts whose filesystem and network constraints
  are knowable before entry.
- Code workaround or exception being considered: adding independent flags to
  runtime and policy paths without a shared Context invariant.
- Current thesis that resolves it, or proposed thesis revision: Context is the
  stable host-owned work mode; its creation-time Boundary is the immutable
  capability envelope, while mutable Runtime/default components and all
  enforcement stores retain their separate lifecycles.
- Downstream impact: manifest/report schema, create inputs, project spec hash,
  policy projection, catalog/help, security claims, harness, and readiness.

## Reproduction or observation

```sh
go run ./cmd/tobari context create --help
go run ./cmd/tobari context show --help
rg -n 'type ContextManifest|PolicyDataIdentity|PolicyEvaluatorIdentity' internal/domain/tobari/context.go
rg -n 'type=bind,src=.*instance.Root' internal/infra/dockerruntime
```

Observed 2026-08-12 before the envelope retirement: create exposed an image
and a now-retired executable selector; the current manifest instead binds
canonical typed policy data to the fixed evaluator, and the project runtime
emitted one unconditional direct read-write root bind.

## Security and public-boundary notes

- Assets and side effects involved: Context manifest, separate policy source,
  project source bind, aggregate OPA projection, Workspace lifecycle.
- Credentials or confidential data involved: only exposure-mode/status facts;
  no credential value or vault path enters the manifest.
- New dependencies, destinations, files, processes, or generated content: none
  at the decision layer.
- Output delivery and mutation: Context reads remain complete; creation remains
  one fixed-target create against the Context catalog with all inputs validated
  before filesystem or Docker side effects.
- Publication concerns: terminology and examples must not imply stronger
  isolation than the child packets enforce.

## Glossary

- **Capability envelope:** the immutable creation-time source/network Boundary
  applied to Workspaces bound to one stable Context.
- **Authority ceiling / guardrail:** a terminal upper bound that no later allow
  source can exceed.
- **Preset snapshot:** normalized policy-preset bytes copied into Context-owned
  state and bound by digest at Context creation.
- **Projection:** purpose-limited generated data mounted into a trusted or
  untrusted component; not the whole Context store.
