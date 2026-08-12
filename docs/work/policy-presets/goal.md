# Work Goal: Create Contexts from bounded policy presets

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Context capability envelope and HTTP effect authorization
- Review/delete trigger: Delete after durable contracts are promoted and the first public V1 change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before first public V1 release
- Related ADRs: ADR 0013, ADR 0018, ADR 0024, ADR 0026, ADR 0027, and ADR 0028
- Parent: [Context capability envelope](../context-capability-envelope/goal.md)
- Prerequisite: [Policy compaction retirement](../policy-compaction-retirement/goal.md)
- Related retirement: [V1 capability retirement](../first-public-release-core/capability-retirement.md)

## Outcome

A user can inspect a small set of versioned built-in network-policy presets,
create and validate strict local custom presets, and snapshot one preset into a
new Context. The Context report makes the selected revision and effective
guardrail visible. Terminal guardrails cannot be exceeded by baseline grants,
learned exact permissions, or Advanced Rego.

## Why now

Deny-by-default is safe but does not by itself give users reusable, legible
security postures. Current Context creation silently copies one example policy,
while learned prefix compaction proposes authority growth from observations.
Explicit presets provide a safer answer for repeated fixed patterns because
the authority ceiling is chosen and reviewable before any Workspace exists.

## Non-goals

- Automatically allowing observed traffic or automatically retrying a denied
  request.
- Prefix-compacting learned rules; `policy compactions` and `policy compact`
  remain retired by the parent packet.
- Calling GET, HEAD, or any HTTP method semantically read-only or safe.
- A built-in `open`, `get-anywhere`, provider/package-manager baseline, wildcard
  host, private destination, or auto-updating endpoint list.
- Live propagation from custom preset changes to existing Contexts.
- Remote/organization preset distribution, inheritance, composition between
  presets, or signed third-party preset registries.
- Secrets, credentials, executable adapters, shell, arbitrary Rego, refresh,
  signing, or external fetches in a preset.
- Temporary/session/allow-once permissions.

## Acceptance criteria

- [ ] `policy.presets` is a public capability with catalog-derived
      `policy preset list`, `policy preset show`, `policy preset init`, and
      `policy preset validate` commands and complete machine contracts.
- [ ] Tobari owns exactly three built-ins:
      `builtin/offline`, `builtin/reviewed-exact`, and
      `builtin/get-only-reviewed`, each with immutable normalized bytes,
      revision digest, visible guarantee, and limitations.
- [ ] `offline` terminally denies every HTTP/HTTPS effect; `reviewed-exact`
      grants nothing initially and preserves exact review eligibility;
      `get-only-reviewed` grants nothing initially, makes only eligible `GET`
      effects reviewable, and terminally denies `HEAD` and every non-GET
      method.
- [ ] A strict owner-only schema-V1 custom preset can declare a bounded method
      and destination ceiling, optional exact Context-wide baseline grants,
      baseline denies, and exact GraphQL classification points, with every
      grant proven to be inside the ceiling.
- [ ] Custom presets reject unknown/duplicate fields, symlinks, unsafe modes,
      oversized input, noncanonical names/hosts/paths, wildcard/IP/private
      destinations, code, secrets, and unsupported composition.
- [ ] Context creation validates and snapshots normalized preset bytes plus
      name/revision before any Context mutation. Later source changes or
      deletion have no effect on existing Context authority.
- [ ] Guardrail and deny precedence are enforced before built-in/custom
      baseline grants, learned rules, and Advanced Rego. Terminal denials
      produce no candidate, DNS, credential resolution, or upstream attempt.
- [ ] `context show` reports preset origin/revision, method/destination ceiling,
      Context-wide grant count, baseline deny count, and project-bound learned
      decision count without inferring semantic safety.
- [ ] Cloud-agent/model traffic receives no implicit bypass. Manual readiness
      evidence records expected startup failure when its required POST is
      outside `offline` or `get-only-reviewed`.
- [ ] `task check`, `task security`, and `task public:check` pass.

## Governing documents

- Thesis: authorize effects at the generic HTTP boundary; Context owns policy
  composition; observation alone never grants authority
- Product contract section: Context create/show and exact policy review
- Architecture or security invariant: Tobari-owned system router, deny
  precedence, strict data source, complete atomic projection, no pre-allow I/O
- Existing ADR: ADR 0028 exact domain source, ADR 0026 transparent gateway,
  ADR 0024 atomic activation; new ADR extends them with a pre-allow guardrail

## Completion definition

The work completes when built-in and custom preset journeys have executable
evidence, compaction is absent, guardrails cannot be bypassed, durable contracts
and generated docs agree, required gates pass, the parent packets incorporate
the result, and this temporary packet is removed.
