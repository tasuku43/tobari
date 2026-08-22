# Work Context: Create a Context from an explicit Base

This file records verified facts and unresolved questions. It does not make a
desired design current reality.

## Current behavior

- `context create` is one fixed-target `EffectCreate` action over the local
  Context catalog. Its catalog inputs are name, exact Runtime, policy mode,
  source access, native-readiness selection, optional AWS/EKS bootstrap, and
  format (`internal/cli/runtime_catalog.go`).
- Interactive creation can seed supplied values, collect omitted settings, and
  finish at one complete Review & Create screen with Create Context, Edit
  settings, and Cancel (`internal/cli/context_create_wizard.go`).
- Context creation already creates through the application-owned canonical
  mutation invoker and validates the confirmed report
  (`internal/app/contextcmd/service.go`).
- Context has a stable identity and immutable Boundary. The exact Runtime
  binding may change after review; shell and Git are late-bound session
  defaults; Workspace bootstrap affects future Workspace creation. Existing
  Workspaces stay bound to their original Context
  (`docs/decisions/0071-define-context-as-a-stable-work-mode.md`).
- Context and Workspace are distinct public resources. Remembered permissions,
  home state, and native tool authentication belong to one Context/Workspace;
  Attachment authority has a still shorter lifetime.
- At packet creation, unrelated uncommitted edits exist in
  `internal/cli/runtime_catalog.go`, `internal/cli/service_exposure.go`, and
  `internal/cli/service_exposure_catalog.go`. They rename review commands and
  currently make the catalog validation test fail because recovery paths have
  not all moved together. This packet does not modify or classify those edits.

## Relevant structure

- Entry point: `cmd/tobari` through catalog-derived `context create` dispatch.
- Domain rule: `internal/domain/tobari.ContextCreateComposition`, Context
  manifests/reports, Boundary and Workspace-default value types.
- Application use case: `internal/app/contextcmd.Service.CreateWithComposition`.
- Infrastructure boundary: Context store/runtime implementation behind the
  application-owned Context runtime port.
- CLI catalog or presentation: `contextCreateSpec`, `runContextCreate`, and the
  terminal Context-create wizard.
- Existing tests and harness checks: Context domain/application tests, catalog
  validation, CLI direct/wizard/raw/line/cancel tests, presentation goldens,
  completion tests, contractlint, agent-readiness replay, and `task check`.

## Constraints

- Base is a draft initializer, not persisted inheritance or mutation authority.
- Base selection is a higher-level step before name and individual settings; it
  must not appear as a peer customization section.
- The current Context may be initially highlighted but `context use` must not be
  invoked or required to select a different creation Base.
- A mutation must still carry one complete fixed Context-catalog target with
  explicit empty `target_inputs`; `--base` is not a target ID or parent input.
- A Base read must become one complete validated creation composition before
  the controlled mutation boundary. Presentation cannot infer copied settings
  from labels, ordering, or proximity.
- The new Context receives a new ID. Existing Contexts and Workspaces remain
  unchanged, and no lower-lifetime authority crosses into the new Context.
- Existing Context JSON compatibility must be audited before adding Base-only
  presentation facts. No stored schema should gain lineage merely to explain
  the draft source.
- Preserve unrelated worktree edits and do not weaken catalog validation around
  their temporary failure.

## External facts

None. This capability is installation-local and uses no new external service,
schema, dependency, or network destination.

## Unknowns

- [ ] Audit whether the existing atomic creation adapter can initialize shell,
      Git, and bootstrap defaults together with Boundary and Runtime, or whether
      its input contract must be extended without exposing partial creation.
- [ ] Audit the complete set of mutable Workspace-default revisions needed to
      freeze one reviewed draft and decide whether a private draft digest is
      needed for same-process consistency.
- [ ] Confirm how an Advanced-mode policy snapshot is copied into a complete
      standalone creation composition without retaining a source-store link.
- [ ] Confirm whether recommended settings need a direct non-interactive
      selector; do not reserve a valid Context name or invent a pseudo-Context
      value without a product decision.
- [ ] Re-run catalog/help observation from a clean baseline after the unrelated
      review-command rename is completed or separated.

## Thesis evidence

- Repeated design decision or point of agent confusion: treating Base as one
  item beside Project files, Network, and Runtime mixed two abstraction levels.
- User outcome or friction observed in the minimal slice: creating a nearby
  Context currently requires remembering or revisiting settings that already
  exist in another Context.
- Code workaround or exception being considered: changing the current Context
  before creation would overload `context use` with unrelated global mutation.
- Current thesis that resolves it, or proposed thesis revision: Context is one
  reusable work mode and creation reviews one complete draft; Base selection
  initializes that draft but does not create inheritance.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: product/architecture/ADR wording, catalog `--base` input and
  completion, domain/application composition, presentation corpus, and
  zero-mutation/authority-isolation evidence. No new trust boundary is expected.

## Reproduction or observation

```sh
go test ./internal/cli -run 'TestContextCreate|TestDefaultCatalogIsValidAndUnique'
```

Expected from a clean baseline: existing direct, line, raw, review, and cancel
creation behavior passes. Current observed packet-creation worktree: the
unrelated review-command rename causes `TestDefaultCatalogIsValidAndUnique` to
fail before Context tests because one recovery path still names `policy review`.

## Security and public-boundary notes

- Assets and side effects involved: owner-only Context policy/configuration
  stores and one new Context-catalog creation.
- Credentials or confidential data involved: none; tool-owned login state and
  host credentials are explicitly excluded from copying.
- New dependencies, destinations, files, processes, or generated content: no
  dependency, network destination, or helper process is expected.
- External schema provenance, publication rights, and drift evidence: not
  applicable.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: scalar complete creation result; no
  pagination; mutation remains non-idempotent by name and must preserve
  confirmed-output and unclassified-outcome rules; cancellation before action
  is zero mutation.
- Publication and licensing concerns: public CLI/help/docs change only; no new
  licensed asset.

## Glossary

- **Base:** one existing Context snapshot, or Tobari recommended settings, used
  only to initialize a complete Context creation draft.
- **Draft:** the complete standalone settings reviewed before creation.
- **Inheritance:** a persisted relationship in which later Base changes affect
  a derived Context; explicitly not supported.
