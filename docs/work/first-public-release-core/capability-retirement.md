# Capability Retirement: Narrow policy learning and brokered authentication for V1

This record covers a coordinated pre-public narrowing. The public capability
IDs remain, but commands, provider variants, adapters, state, dependencies,
and security claims inside them are removed.

## Decision and evidence

| Capability ID | Previous status and public surface | New status | Superseding decision |
|---|---|---|---|
| `policy.learning` | Public; exact rules, batch review, `policy compactions`, and `policy compact` | Public, narrowed to exact candidate/rule and explicit batch-review flows | New V1-scope ADR from this packet |
| `authentication.broker` | Public; static managed injection, static and dynamic broker plans, GitHub/AWS/Datadog/OpenAI, helper-acquired static Anthropic, Chatwork, and companion execution | Public, narrowed to static broker plans, owner stdin import, and one GitHub acquisition helper | New V1-scope ADR from this packet |
| `context.composition` | Public; every new Context silently copies one embedded example policy and every Workspace source bind is read-write | Public, revised to immutable direct source access plus one explicit snapshotted policy preset | Context capability-envelope ADR and child packets |

- User, incident, compatibility, security, or maintenance evidence: there is no
  public release, component images remain `unpublished`, dynamic providers
  create a large externally drifting manual matrix, managed injection
  duplicates the static broker outcome, and learned prefix compaction widens
  authority without public denial-volume evidence.
- Last version or revision that supported the old surface: no public version;
  development revision `f73c9c6` on 2026-08-12.
- Superseded ADRs or clauses: dynamic/companion decisions in ADR 0020,
  `0021-add-datadog-pup-oauth.md`, ADR 0023, and ADR 0025; retained-managed
  clauses in ADR 0009; expanded built-ins in ADR 0019; compaction clauses in
  theses/product/security contracts. ADR 0019's static project-bound
  post-policy broker core and the unrelated
  `0021-context-owned-narrow-host-projections.md` remain accepted.

## Public contract removal

- [ ] `policy compactions`, `policy compact`, their help, examples, dispatch,
      output envelope, faults, and recovery actions are removed.
- [ ] The `policy-compaction` produced/consumed reference graph is absent, and
      every retained required-reference chain still leads to an invocable
      producer.
- [ ] `auth login` accepts only explicit GitHub; AWS `--method`, provider
      selector rows, dynamic-provider faults/recoveries, and companion status
      fields are removed.
- [ ] Managed-adapter selectors, paths, fields, and help are removed rather than
      deprecated or retained as internal configuration.
- [ ] Implicit `api.github.com`, `example.com`, and `mock-upstream` Context
      policy initialization is removed; no unnamed initial authority survives
      outside the selected preset snapshot.
- [ ] Capability and schema ledgers record narrowed V1 state and reason.
- [ ] The exact-V1 compatibility impact is explicit: development state from
      the old source snapshot is unsupported and is recreated, not migrated.
- [ ] Negative tests prove every retired command, provider, plan kind, adapter,
      state field, and configuration selector is rejected with no undocumented
      fallback.

## Implementation and dependency removal

- [ ] Compaction domain types, IDs, application ports/use cases,
      infrastructure grouping/activation paths, presentation, and tests are
      removed.
- [ ] Context initialization generates policy only from the selected normalized
      preset; embedded integration/example domains remain test fixtures only
      and cannot become production authority through a fallback.
- [ ] Managed credential profile binding, storage, Gateway injection,
      configuration, policy input, mounts, and tests are removed.
- [ ] AWS, Datadog, OpenAI, helper-acquired Anthropic, and Chatwork built-ins
      plus dynamic plan, refresh, signing, and projection variants are removed.
- [ ] AWS/Codex/Claude/Datadog host drivers, executable/version contracts,
      provider-state parsers, network clients, PTY/shim code, and manual release
      checks are removed.
- [ ] Companion application/infrastructure code, bridge/session crypto,
      resident process, broker socket/API, status/doctor fields, image assets,
      environment values, and tests are removed.
- [ ] Provider libraries, protocol helpers, transitive modules, imports,
      generated files, image packages, and CI/release steps are removed when no
      retained capability owns them.
- [ ] No dormant transport, raw route, helper ID, provider manifest field,
      adapter selector, environment value, or legacy image can reactivate the
      retired behavior.
- [ ] Documentation, repository-local Skills, generated architecture data, and
      examples describe the retained product rather than the historical scope.

## Persisted state

ADR 0027 supplies the governing disposition: no public user depends on the
development snapshot, no compatibility reader is added, and old state is
explicitly removed and recreated. Unrelated commands do not silently delete
legacy state.

| State | Secret? | Disposition | Recovery and evidence |
|---|---|---|---|
| Prefix-compacted Context policy source and compaction journals/proposals | No | Explicit cleanup | Before running narrowed V1, export any desired exact decisions as reviewed notes, remove the development Context, and recreate exact rules. New readers reject the old shape; negative tests prove no prefix fallback. |
| Contexts initialized from the unnamed embedded example policy | No | Explicit cleanup | Remove and recreate the development Context with an explicit built-in/custom preset. Exact-V1 readers reject a manifest without source-access and preset snapshot fields; no implicit example-policy default is injected during read. |
| Static managed `credentials.json` and `credentials/` files | Yes | Explicit cleanup | Use the old snapshot to remove or securely archive values, then remove the development Context state. Narrowed V1 never reads or mounts these paths. Files are not deleted by an unrelated command. |
| AWS, Datadog, OpenAI, Anthropic, or Chatwork encrypted vault records | Yes | Explicit cleanup | Use the old snapshot to `auth logout` each provider, delete affected development Workspaces, and stop the old cluster before removing/recreating installation/Context state. Narrowed V1 rejects the old vault shape and does not log or decode records for migration. |
| Dynamic refresh barriers, provider driver state, and companion session material | Yes | Explicit cleanup/discard | Complete provider logout and cluster shutdown with the old snapshot. Ephemeral session keys disappear with the process; encrypted durable state is removed only as part of explicit development-state cleanup. |
| Retired provider handles projected into Workspace environment or complete files | Handle, not primary secret | Explicit cleanup | Delete the development Workspace with the old snapshot before upgrade. New Workspace reconciliation issues only retained-provider handles and has no retired binding. |
| Owner static provider manifests | No | Keep if schema V1 static | Strict static manifests remain supported. Any dynamic/helper-bearing owner manifest is rejected and must be rewritten as a static import or removed. |
| GitHub static vault record and project handles | Yes | Recreate for exact V1 | Log in/import again after development-state recreation. Tests re-prove revision, rotation, logout, binding, and stale-handle rejection. |
| Workspace-owned tool-native credential state | Yes | Explicit user choice | It is outside broker migration. Deleting a development Workspace removes its home; retaining unsupported old Workspace state is not a V1 compatibility guarantee. |

- [ ] Cleanup instructions use only bounded existing actions from the old
      development snapshot plus explicit removal of the exact development
      installation/Context paths; no broad or inferred deletion target is
      introduced.
- [ ] No dependency is retained only to decode, migrate, or delete legacy
      provider state.
- [ ] Removed credentials cannot leak through cleanup errors, logs, fixtures,
      public history, SBOMs, or release artifacts.

## Verification

- Focused negative tests: retired catalog paths; provider IDs; helper/plan
  kinds; managed selectors and headers; companion sockets/state; prefix policy
  fields; unsupported V1 state; zero side effects on every rejection.
- Catalog/capability/schema checks: `cli.Catalog`, reference graph, fault/recovery
  graph, `.harness/capabilities.json`, `.harness/schemas.json`, generated
  catalog, and exact API/schema V1 checks.
- Dependency and import diff: Go modules, Python imports, OCI package inventory,
  embedded/snapshotted sources, helper assets, manual-release matrix, licenses,
  and SBOMs have one retained owner each.
- Persisted-state migration or cleanup tests: no migration success path;
  unsupported state rejects exactly; retained static V1 state works only after
  explicit development-state recreation; cleanup documentation names exact
  targets and uses no secret output.
- Required gate: `task check`, `task security`, `task public:check`, and
  `task release:check`.
- Rollback or reintroduction policy: before publication, reviewed source revert
  plus development-state recreation; after publication, a provider, dynamic
  plan, managed adapter, companion, or compaction requires evidence, a new ADR,
  full security/external-contract work, and compatible V1 addition or explicit
  V2 migration. No hidden flag or old image is a rollback mechanism.
