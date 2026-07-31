# Capability Retirement: Named Tobari lifecycle

## Decision and evidence

- Capability ID: `tobari.lifecycle`
- Previous status and public commands: public; `attach`, `shell`, `exec`, `logs`, and `detach`
- New status: public, superseded by CWD-owned root command, `status`, `list`, and `delete`
- Superseding capability or ADR: CWD-owned Tobari lifecycle work packet
- User, incident, compatibility, security, or maintenance evidence: the requested routine outcome must not require a name, root flag, or opaque ID.
- Last version or revision that supported the old surface: pre-replacement mainline

## Public contract removal

- [x] Command paths, namespaces, help entries, and dispatch bindings are retired.
- [x] Reference edges and required chains are removed with the named-action surface.
- [x] Fault declarations and recovery actions no longer name retired commands.
- [x] Capability ledger and schema compatibility are updated.
- [x] Negative tests prove retired commands are unknown and cannot dispatch.

## Implementation and dependency removal

- [x] Named selectors, Docker volume homes, and user-facing lifecycle state are removed or replaced by shared logic.
- [x] No hidden named-command fallback remains reachable from the public catalog.

## Persisted state

| State | Secret? | Disposition: ignore / migrate / explicit cleanup | Recovery and evidence |
|---|---|---|---|
| Schema-2 named state and home volume | no | explicit cleanup with the matching old binary | new binary rejects legacy state without mutation |

## Verification

- Focused negative tests: `TestRetiredNamedCommandsAreUnknown` and catalog absence assertions.
- Catalog/capability/schema checks: default catalog validation and scoped agent-help tests.
- Persisted-state migration or cleanup tests: project state unknown-field and atomic-write tests.
- Required gate: `task check`, `task security`, and `task public:check`.
