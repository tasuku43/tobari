# Capability Retirement: Named Tobari lifecycle

## Decision and evidence

- Capability ID: `tobari.lifecycle`
- Previous status and public commands: public; `attach`, `shell`, `exec`, `logs`, and `detach`
- New status: public, superseded by CWD-owned root command, `status`, `list`, and `delete`
- Superseding capability or ADR: CWD-owned Tobari lifecycle work packet
- User, incident, compatibility, security, or maintenance evidence: the requested routine outcome must not require a name, root flag, or opaque ID.
- Last version or revision that supported the old surface: pre-replacement mainline

## Public contract removal

- [ ] Command paths, namespaces, help entries, and dispatch bindings are retired.
- [ ] Reference edges and required chains are removed with the named-action surface.
- [ ] Fault declarations and recovery actions no longer name retired commands.
- [ ] Capability ledger and schema compatibility are updated.
- [ ] Negative tests prove retired commands produce only the explicit migration error.

## Implementation and dependency removal

- [ ] Named selectors, Docker volume homes, and user-facing lifecycle state are removed or replaced by shared logic.
- [ ] No hidden named-command fallback remains reachable.

## Persisted state

| State | Secret? | Disposition: ignore / migrate / explicit cleanup | Recovery and evidence |
|---|---|---|---|
| Schema-2 named state and home volume | no | explicit cleanup with the matching old binary | new binary rejects legacy state without mutation |

## Verification

- Focused negative tests:
- Catalog/capability/schema checks:
- Persisted-state migration or cleanup tests:
- Required gate:
