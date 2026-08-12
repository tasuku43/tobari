# Work Context: Create Contexts from bounded policy presets

## Current behavior

- New Contexts copy embedded domain pairs for `api.github.com`, `example.com`,
  and `mock-upstream`. Their authority/method entries immediately allow some
  GET/HEAD and POST traffic, so initial authority is implicit and test-oriented.
- The guided system evaluator defaults to deny. HTTPS requests can be eligible
  for exact learning even without a declared authority; plain HTTP needs a
  declared authority or GraphQL endpoint.
- An ordinary authority/method entry is a Context-wide baseline grant.
  Learned allow/deny rules bind stable Context and project plus exact HTTP
  effect; baseline deny takes terminal precedence.
- Advanced Rego is namespaced per Context, while a Tobari-owned system router
  chooses the decision entrypoint and handles GraphQL routing.
- Domain policy source is strict schema 1 at
  `policy/domains/<host>/{allow,deny}.json`, uses exact canonical hosts, and is
  promoted as one journaled complete generation.
- Learned prefix rules and the public compaction workflow still exist in code
  and contracts but are selected for retirement by the parent release packet.
- There is no preset store, preset identity/revision, or preset command
  namespace.

## Relevant structure

- Entry point: new `policy preset *` commands and `context create
  --policy-preset`
- Domain rule: new preset/guardrail types beside Context and policy vocabulary
- Application use case: new `internal/app/policypresetcmd` task-owned ports;
  Context create consumes one validated snapshot
- Infrastructure boundary: owner-only preset store, embedded built-ins,
  normalization/digest, Context snapshot creation, domain source generation,
  aggregate/system evaluator
- CLI catalog or presentation: new command specs/capability and Context report
  policy fields
- Existing tests and harness checks: policy source strict parsing, aggregate
  atomicity, Rego tests, candidate learnability, denial zero-call canaries,
  catalog/reference graph, public guard

## Constraints

- A preset must separate authority ceiling from current grants. Calling both
  “allow” would let users misread `get-only-reviewed` as immediate GET access.
- `get-only-reviewed` means only GET may proceed to exact review; it does not
  mean GET is safe, private, body-free, query-free, or automatically allowed.
- A Context-wide baseline grant applies to every project bound to that Context;
  reports and review must make that scope explicit.
- Project-bound learned rules cannot exceed the Context guardrail even if they
  predate a policy change; snapshots are immutable so this is chiefly a
  malformed-state and Advanced-bypass invariant.
- Preset source is non-secret data but remains owner-only because changing it
  can create future authority.
- Existing Contexts never re-read the origin preset.
- Exact V1 has no migration/default reader for old Context policy source.

## External facts

No external endpoint catalog is used. Built-ins are Tobari-owned deterministic
data and custom presets are local owner-authored files. This avoids drift and
licensing authority from third-party provider lists.

## Unknowns

- [ ] Freeze the exact custom schema after hostile-fixture review; field names
      below are semantic requirements, not yet a published JSON contract.
- [ ] Measure first-run denial volume for each built-in during the retained
      agent-readiness scenarios.
- [ ] Record which supported cloud agents fail to start under offline/GET-only
      because their control traffic requires POST; document without adding a
      bypass.

## Thesis evidence

- Repeated decision: deny-by-default needs manageable explicit policy without
  turning observed traffic into automatic authority.
- User outcome: name a reusable “offline,” “reviewed exact,” or “GET-only
  reviewed” posture and attach it to a Context before entry.
- Avoided workaround: copying/editing example policy directories or widening
  learned rules through compaction.
- Proposed revision: selected preset snapshot supplies the Context-wide
  guardrail and optional baseline; exact learned decisions remain below it.
- Downstream impact: new capability/catalog, preset store/schema, Context
  manifest/report, system Rego, aggregate data, compaction retirement,
  docs/harness/readiness.

## Reproduction or observation

```sh
find internal/infra/runtimeassets/assets/opa/policy/domains -type f -maxdepth 2 -print
sed -n '1,180p' internal/infra/runtimeassets/assets/opa/policy/tobari.rego
sed -n '180,260p' internal/infra/dockerruntime/context_store.go
go run ./cmd/tobari help policy --format agent
```

Observed 2026-08-12: example domains are copied implicitly; the evaluator has
default deny plus exact learning and terminal baseline denies; the public
catalog has compaction but no preset capability.

Observed on the integrated V1 branch after recreating an exact-schema Context:
Context policy preflight ran the embedded guided evaluator before aggregate
preset routing and exposed one stale test expectation. A declared GraphQL
endpoint without an exact learned rule is review-eligible at that layer, while
the immutable aggregate preset guardrail remains responsible for any terminal
method or destination rejection before candidate projection. Naming that raw
decision terminal contradicted both the evaluator and the system-router
ownership boundary and prevented `cluster up` before container reconciliation.

Observed on the next integrated V1 journey: the ordinary HTTP Gateway denial
audit omitted `scheme` even though the accepted exact-policy identity and OPA
rule both require it. `policy allow` therefore activated a rule with an empty
scheme that could never match the denied HTTPS request. The unpublished V1
reader now rejects every absent or non-HTTP(S) scheme at the domain boundary,
the Gateway records the canonical request scheme, and the typed review fixture
pins the complete identity. The journey then passed brokered exact approval,
post-allow retry, Context/project handle isolation, and invalid-handle canaries.
It next exposed a separately scoped release-journey expectation that still
assumed the retired implicit `mock-upstream` initial grant.

Read-only review of that journey then found that custom-preset
`graphql_endpoints` were normalized and snapshotted but only legacy policy
source endpoints entered the aggregate OPA boundary and Gateway projection.
The aggregate now forms one validated, deduplicated, deterministic union of
both sources. Preset endpoints must declare POST because Gateway's bounded V1
GraphQL classifier has no GET/subscription transport contract; accepting and
then silently dropping another method would make owner data misleading.

The same reproduction found that reopening the Context store reconstructed a
builtin manifest for an already-persisted `default` Context. A default Context
created from a custom snapshot therefore failed later commands because the
reconstructed revision disagreed with its immutable preset. Store completion
now uses the validated persisted default manifest when one exists; builtin
defaults are seed data only for a genuinely absent store.

The integrated Docker journey then exposed two enforcement-boundary gaps that
focused projection tests could not reveal. First, denial/candidate/rule domain
objects retained the exact request scheme while their CLI JSON, TSV, and human
projections omitted it, making `http` and `https` effects impossible for an
agent to distinguish without inference. Those public surfaces and catalog
fields now carry the scheme explicitly. Second, the deferred GraphQL request
path retained the scheme in metadata but failed to restore it before emitting
a denied audit and response. The resulting exception occurred after OPA had
denied and allowed mitmproxy to continue upstream. The Gateway now restores the
trusted scheme before any GraphQL policy outcome, with a negative unit test
and the full Docker journey proving denial creates exact per-root candidates
while committing no upstream request.

## Security and public-boundary notes

- Assets: outbound HTTP authority, denial/candidate semantics, Context policy
  source, aggregate OPA bundle, custom preset files.
- Credentials: preset contains no secret or provider credential. A terminal
  guardrail denial remains before broker resolution.
- New dependencies/destinations/processes: none; parser and built-ins use the
  standard library and embedded reviewed data.
- Side effects: preset init creates one exact owner-only local file; Context
  create snapshots it; list/show/validate are read-only.
- Retry/cancellation: validation makes no policy activation; Context creation
  follows fixed-target mutation semantics; a later permission change never
  retries the denied request.
- Publication: synthetic domains only; no copied provider endpoint catalog.

## Glossary

- **Preset origin:** canonical built-in/custom selector used at Context
  creation.
- **Preset revision:** SHA-256 digest of canonical normalized preset bytes.
- **Guardrail:** terminal method/destination ceiling checked before all allows.
- **Baseline grant:** explicit Context-wide initial HTTP route grant below the
  guardrail, distinct from project-bound learned permission.
- **Baseline deny:** trusted terminal deny that cannot become a candidate.
