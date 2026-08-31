# ADR 0088: Separate editable resource sources from active authority

- Status: Accepted
- Date: 2026-08-26
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, clean break,
  harness, and public boundary
- Revises: ADR 0084 at its repository-authored configuration seam
- Related: ADR 0087, ADR 0080, and ADR 0092
- Superseded by: None

## Context

ADR 0084 correctly assigned Template, Context, and Policy Memory ownership, but
made one private atomic envelope the only persistent Template and Context
representation. The expected author is often an AI agent. It needs exact,
ordinary files that can be discovered, edited, diffed, and reviewed without
decoding an internal snapshot. This boundary must exist before the future
semantic provider-policy language is added.

## Decision

### Concept-separated installation sources

The installation-owned XDG configuration tree is:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/tobari/
  templates/<template-id>/template.yaml
  templates/<template-id>/policy.yaml
  contexts/<context-id>/context.yaml
  runtimes/<runtime-id>/runtime.yaml
  runtimes/<runtime-id>/source/

${XDG_STATE_HOME:-$HOME/.local/state}/tobari/
  runtimes/<runtime-id>/runtime.json
  runtimes/<runtime-id>/revisions/
  authority/
    templates/<template-id>/<object-digest>.json
    contexts/<context-id>/<object-digest>.json
    policy-memory/<context-id>/<object-digest>.json
    workspaces/<workspace-id>/<object-digest>.json
    generations/<generation-digest>.json
    active.json
    journal/
```

Child directories use stable IDs, never display names. Display names remain in
documents. There are no symlink aliases. Template and Context machine-readable
show output exposes the canonical absolute `source_path`; callers pass opaque
resource references and never reconstruct IDs or paths.

Dynamic state stays concept-separated below XDG state. Authority objects are
immutable typed concept objects; a content-addressed generation manifest binds
one coherent cross-resource view, and the small regular-file `active.json`
pointer selects exactly one verified last-known-good generation. Publication
writes changed objects, then the complete manifest, then atomically replaces
the pointer. Runtime revisions, Workspace homes, and cluster receipts retain
their own concept roots. There is no monolithic editable or active registry.

Installed source is not a portable interchange contract. It contains local
stable IDs, exact Runtime authority, canonical roots, and revision bookkeeping.
A future portable package is separately versioned reusable semantic content;
import creates fresh local IDs and resolves one exact Runtime revision. Portable
import/export is outside this decision.

### Strict current schemas and closed file sets

`template.yaml` owns Template ID, display name, direct Project source access,
exact Runtime ID and immutable revision, session/Workspace defaults, and nullable Template-wide
`base_revision`. `policy.yaml` owns the same Template ID, Method Boundary, and
static semantic policy. Apply snapshots and validates both as one source; no
partial activation exists. `base_revision` is concurrency bookkeeping and is
excluded from the semantic revision digest.

`context.yaml` schema v2 owns only Context ID and Template ID. Both values are
immutable after creation. Project root belongs only to Workspace authority; a
different Template needs a fresh Context ID, and Policy Memory is never
transferred implicitly.

Every document has one exact schema version. Ordinary reads reject unknown
fields and unsupported schemas and never rewrite source. A future explicit
migration uses a dedicated old-schema decoder, writes only current desired
source without semantic widening, and never activates it. Normal Apply remains
required.

The original decision introduced `tobari.dev/template-policy/v1alpha1` as a
lossless installation bridge and reserved the final token. ADR 0089 now
supersedes this seam: current ordinary source is
`tobari.dev/template-policy/v1`, while alpha is decoded only by explicit
non-activating `template migration plan/apply`. Relabeling alpha bytes as V1
still fails. Migration updates desired source only; normal Plan/Apply remains
required afterward.

The Template directory contains exactly `template.yaml` and `policy.yaml`; the
Context directory contains exactly `context.yaml`. Owner/mode, regular-file,
hard-link, symlink, replacement-race, size, duplicate-key, alias, merge-key,
tag, non-string-key, unknown-field, and closed-set violations fail closed.
YAML parsing stays in infrastructure; `cmd` and `internal/cli` gain no third-
party dependency.

### Desired source, explicit Apply, and active authority

Source files are desired state. The last successfully applied snapshot is
active authority. Observation reports exactly `in_sync`, `modified`, `invalid`,
or `missing`, with desired and active revisions. `cluster up` consumes only
active last-known-good authority and never applies YAML.

Every Template Apply begins with read-only `template plan --id
<template-ref>`. The plan binds Template identity, active/base revision, exact
source fingerprint, exact Runtime revision, bound Context set and relevant
Memory revisions, affected running Workspaces, and a widening/reducing/mixed/
no-op classification. Non-TTY Apply consumes the opaque plan unchanged through
`template apply --plan <template-change-plan-ref>`. Apply revalidates every
bound fact, snapshots both files, and rechecks their exact bytes immediately
before atomic generation publication. Stale/null-for-existing input or a
concurrent edit fails with zero activation. After success Tobari advances only
`base_revision`, preserving intervening edits and unrelated source text.

Template/Context creation, and Template copy, issue a stable ID and write an
unpublished draft source; they create no active logical authority. A draft has
no cluster projection, Workspace, or Policy Memory and cannot be a Context
binding target. First Context activation likewise uses `context plan --id
<context-ref>` followed by `context apply --plan
<context-activation-plan-ref>`. The plan binds source bytes, canonical root,
current Template revision, duplicate observation, exact Runtime and effective
policy. After activation the Context ID/root/Template tuple is immutable;
same-content reapply is a no-op.

Deleting or losing a source file/directory is drift, never logical deletion.
Reads and cluster lifecycle preserve active Templates, Contexts, Policy Memory,
Workspaces, and receipts and never regenerate source. Apply, copy, and future
promotion that require a missing source block. Only explicit reference-bound
delete commands logically delete resources.

Bound Contexts retain Template-ID moving-head behavior. Runtime builds create
revisions only and never propagate. Adopting a Runtime revision requires a
Template edit and Apply. There is no `latest` Runtime binding.

Template display name is revision metadata: rename through Plan/Apply retains a
complete metadata-only revision without changing policy, Runtime, session, or
Workspace-default slice digests. Method Boundary is likewise editable only
through planned Template Apply. Tightening atomically supersedes every
conflicting remembered Allow in bound Contexts with typed provenance/history;
remembered Denies remain. Loosening grants no semantic authority by itself.

### Exactly two Template semantic write paths

Ordinary semantic authoring is direct human/agent editing followed by Template
Apply. Granular shell, Git, bootstrap, and Runtime setter commands are retired;
they neither mutate active authority nor silently rewrite source.

The ADR 0089 alpha-to-V1 schema migration is not an additional semantic
authoring path: it accepts only one deterministic non-widening conversion of
exact in-sync predecessor bytes, leaves active authority unchanged, and still
requires ordinary Template Plan/Apply for any semantic publication.

The sole additional writer reserved for later work is explicit reviewed Policy
Memory promotion. It is limited to `policy.yaml`, starts only from an `in_sync`
Template, binds the exact source fingerprint, base/active Template revision,
and Policy Memory revision, preserves unrelated rules/comments or fails, and
atomically publishes one Template revision with supersession of selected Memory
Allows. It retains provenance/history and never promotes Denies implicitly.
Promotion implementation remains out of scope here. The semantic AWS language
and the rest of the closed module taxonomy are decided by ADR 0089.

Future policy collections are semantic sets: order has no meaning, exact
duplicates are invalid, and canonical digests ignore reorder-only edits while
source order is preserved. Rule identity is content-addressed from effect,
provider/protocol, and canonical matcher dimensions; users author no IDs.
Static precedence and provider-specific matcher semantics remain governed by a
later policy-language decision.

The final `tobari.dev/template-policy/v1` policy schema was constrained here
and is implemented by ADR 0089. Its top
level has required `boundary` and `semantic` containers. V1 Method Boundary is
only `boundary.methods.deny`: exact uppercase ASCII tokens terminal-deny while
unlisted methods merely continue. `semantic` has closed, non-fallback sibling
taxonomies `protocols.http.<allow|deny>.rules` and
`providers.aws.<allow|deny>.rules`; an AWS-classified request never falls back
to generic HTTP. Absent individual HTTP/AWS namespaces mean known-none; a
present namespace requires both effects and explicit non-null arrays.

Generic HTTP rules require explicit scheme, port, one uppercase method,
`host` XOR `hosts`, and `path` XOR `path_template`. Plurals contain at least
two distinct canonical lowercase DNS names; IP literals and wildcards are not
allowed. The only template placeholder is one full non-empty `{id}` segment.
Raw observed path bytes are matched without percent/dot/backslash
normalization, query is outside identity, and ambiguous/encoded segments do not
match a template. AWS rules use `service` XOR `services`, closed lowercase
`query|json` protocol, exact case-sensitive operation plus version/namespace,
and at most one terminal operation-prefix `*`. Static Deny precedence is fixed:
exact Allow/Deny equality, fully Deny-shadowed Allow, and Method-Boundary-
shadowed Allow are compile errors; narrower Deny carve-outs and partial overlap
remain valid and reportable. ADR 0089 owns the executable compiler, common
projection, and per-module tests that make these constraints an accepted source
schema.

### Rego remains internal

No `.rego` file may appear in a user XDG root or Workspace. ADR 0087's fixed
evaluator remains embedded and is assembled in memory into Docker-managed
bundle material. Editable source contains only typed data.

### Explicit installed-state migration and recovery

Ordinary read, startup, upgrade, and cluster lifecycle never migrate state. The
only accepted predecessor is the exact currently supported typed final
`authority.json`, with no Advanced/Rego marker or unsupported schema. Its
presence returns `installation_migration_required`. `installation migration
plan` is a read-only byte-digest and generation-bound discovery operation;
`installation migration apply --plan <installation-migration-plan-ref>` passes
that opaque plan unchanged, revalidates it without rediscovery, writes the
canonical source documents and a complete verified generation whose
content-addressed manifest commits the exact migration plan authority, atomically
selects it, then retires the old root. Ordinary non-migration generation manifests
carry no migration provenance. Stale input causes zero mutation.
Pre-verification swap/sync/read-back failure restores the byte-identical old
store and removes the rejected generation from the canonical path. Unsupported,
unsafe, partial, Advanced/Rego, or ambiguous predecessor state is rejected,
not decoded or transformed. Before retiring the outer transaction, Apply
durably publishes an exact plan/generation/revision accepted receipt inside the
active authority journal. Same-plan retries return that confirmed result only
after loading the exact plan provenance through the selected generation's verified
pointer/manifest digest chain and requiring receipt equality; receipt-local hashes
cannot substitute another otherwise self-consistent plan. The receipt is immutable retained authority
provenance: automatic retirement is forbidden because it would reopen an
unrecoverable success-before-response window; only a future explicit
history-retention mutation may remove it.

Active last-known-good state remains usable when only desired source is missing
or invalid. Immutable authority revision/provenance history has no automatic
GC. Explicit dependency-checked resource deletion purges its source and owned
history/state and retains only a minimal immutable tombstone.

Active-first/source-bookkeeping interruption is reported as partial recovery;
recovery never overwrites intervening edits. Source fingerprint failure before
active publication has zero effect. Unknown mixed files are never guessed away.

## Consequences

- AI agents receive deterministic editable resources without executable policy.
- Static Template source, immutable Context binding, dynamic Policy Memory,
  Runtime source, and active receipts have visibly different owners.
- Explicit Apply is the only ordinary publication boundary; cluster lifecycle
  cannot widen authority from an edit.
- ID-based Runtime paths remain stable across display-name evolution.
- Strict multi-file and recovery handling adds implementation and test cost.

## Mechanical enforcement

- Domain tests prove source ownership, identity, exact Runtime binding, and
  desired/active state vocabulary.
- Infrastructure tests prove strict YAML, closed files, filesystem safety,
  fingerprint fencing, comment-preserving bookkeeping, and ID-based paths.
- Catalog tests prove Apply workflows and absence of granular setters.
- Existing whole-tree checks prove no Rego below user XDG roots or Workspaces.
- `task check` and `task security` decide completion.
