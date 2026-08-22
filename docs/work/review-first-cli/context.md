# Work Context: Align trusted-host review commands

## Current behavior

- `internal/cli/runtime_catalog.go` registers `policy review` as the complete
  human Permission Inbox with `tail`, `format`, `watch`, and `notify` inputs.
- `internal/cli/service_exposure_catalog.go` registers root `review` as a
  selector between Permission requests and Service requests.
- `internal/cli/service_exposure.go` implements the selector itself; its
  redirected path renders only the Service request snapshot, while its TTY
  path asks the user to choose the two review kinds.
- `cli.Catalog` rejects a registered command path that is also a word-boundary
  namespace. Therefore a registered `review` command cannot coexist with
  registered `review permissions` and `review services` children.
- Bare namespaces already have catalog-derived behavior. For example, bare
  `tobari policy` lists the commands in that namespace and exits successfully.
- ADR 0073 keeps Permission Inbox in a separate trusted-host terminal. ADR 0074
  gives service review the same trusted-host placement but a different,
  attachment-local authority and lifetime.

## Relevant structure

- Entry point: `cmd/tobari` through the shared CLI dispatcher
- Domain rule: permission evidence/decisions and service request/exposure
  references remain unchanged
- Application use case: existing policy review and service exposure use cases
- Infrastructure boundary: existing Gateway, policy, owner rendezvous, and
  relay adapters remain unchanged
- CLI catalog or presentation: `internal/cli/runtime_catalog.go`,
  `internal/cli/service_exposure_catalog.go`,
  `internal/cli/policy_review_selector.go`, and
  `internal/cli/service_exposure.go`
- Existing tests and harness checks: catalog validation, policy review PTY
  corpus, service review interaction tests, agent help, presentation fixtures,
  docs generation, contractlint, and public guard

## Constraints

- The catalog remains the only public command, parsing, routing, and help
  registry; do not add a special dispatcher for bare `review`.
- Command paths must remain disjoint from their word-boundary namespaces.
- Permission review stages a bounded set and applies it only after explicit
  confirmation; service review performs one immediate exact attachment-local
  Allow once or Deny after confirmation.
- Redirected and machine-readable review remains read-only.
- A Workspace cannot invoke or control host review authority.
- This is pre-public software, so the accepted replacement removes the old
  paths rather than adding aliases.
- Existing structured results retain their field and reference semantics; only
  their command identity and navigation strings move.

## External facts

None. This decision follows the repository's catalog behavior and the observed
Tobari CLI.

## Unknowns

- [ ] Audit whether an ADR revision is sufficient or a short new ADR is needed
      to record `review` as the stable task namespace.
- [ ] Audit generated and experimental-profile surfaces for exact old command
      strings before declaring retirement complete.

## Thesis evidence

- Repeated design decision or point of agent confusion: permission review and
  service review are sibling host decisions, but their current paths expose
  different conceptual depths.
- User outcome or friction observed in the minimal slice: `policy review` plus
  generic `review` makes it unclear whether review is a task, a menu, or a
  policy-only operation.
- Code workaround or exception being considered: retaining root `review` as a
  command while adding children would require weakening the catalog's
  command/namespace invariant or inventing special dispatch.
- Current thesis that resolves it, or proposed thesis revision: public CLI
  paths should express and close user tasks; a pure `review` namespace gives
  the two host review tasks one level without a new router.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: command vocabulary and recovery text change; authority and adapter
  behavior do not.

## Reproduction or observation

```sh
go run ./cmd/tobari policy
go run ./cmd/tobari help review
go run ./cmd/tobari help policy review
```

Observed on 2026-08-22: the first command renders a namespace list; the second
describes the registered unified selector; the third describes the separate
Permission Inbox. Catalog validation rejects command/namespace collisions.

## Security and public-boundary notes

- Assets and side effects involved: existing read and reviewed mutation paths
  only; no new side effect
- Credentials or confidential data involved: none
- New dependencies, destinations, files, processes, or generated content: none
- External schema provenance, publication rights, and drift evidence: not
  applicable
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged for both moved leaf tasks
- Publication and licensing concerns: public docs and generated help must not
  retain stale pre-public paths

## Glossary

- Review namespace: the task-level catalog namespace containing trusted-host
  review workflows; it is not itself an executable workflow.
- Permission review: durable Context-and-Workspace policy decisions prepared in
  the Permission Inbox and activated as one reviewed set.
- Service review: one immediate attachment-local Allow once or Deny for a live
  Workspace service request.
