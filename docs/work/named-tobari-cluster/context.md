# Work Context: Named Tobari cluster

## Current behavior

- `up --root` persists one schema-1 root and rejects a second root.
- One Compose project creates Realm, Gateway, and OPA together.
- XDG policy is mounted read-only at `/policy`; an existing `up` restarts OPA to reload it.
- `shell`, `exec`, and `down` act on the fixed target `realm-default`.

## Relevant structure

- Entry point: `cmd/tobari`
- Domain rule: `internal/domain/realm`
- Application use case: `internal/app/realmcmd`
- Infrastructure boundary: `internal/infra/dockerruntime`
- CLI catalog or presentation: `internal/cli/runtime_catalog.go` and `internal/cli/realm.go`
- Existing tests and harness checks: Go contract tests plus policy, Gateway, and Docker integration profiles

## Constraints

- Individual destructive actions must consume one opaque reference unchanged.
- Gateway is the only component allowed to join a Tobari network and the egress network.
- Each Tobari receives only its selected root, its exact home volume, and the public CA volume.
- OPA does not need host write permission for live policy reload; read-only bind plus watch preserves least privilege.
- Public documentation remains English and pre-v1 compatibility changes must be explicit.

## External facts

No external content is required. The pinned OPA runtime and Docker CLI are the
executable sources used to verify watch and network behavior.

## Unknowns

- [ ] Verify the pinned OPA image accepts `run --watch /policy`.
- [ ] Verify the Gateway remains healthy after dynamically joining more than one internal network.

## Thesis evidence

- Repeated design decision or point of agent confusion: a singleton Realm makes the policy-learning environment compete with ordinary work roots.
- User outcome or friction observed in the minimal slice: users want to attach isolation wherever work occurs and edit policy from a separately attached XDG root.
- Code workaround or exception being considered: repointing the singleton root would discard or conflate persistent environments.
- Current thesis that resolves it, or proposed thesis revision: replace the single-root thesis with one shared enforcement cluster and independently named Tobari.
- Downstream impact: theses, product, architecture, security, catalog, capability ledger, runtime state, integration tests, and README.

## Reproduction or observation

```sh
rg -n 'single|one Realm|realm-default|tobari-realm' docs internal README.md
```

The current contracts and implementation consistently encode one fixed Realm.

## Security and public-boundary notes

- Assets and side effects involved: Docker containers, networks, volumes, XDG state, XDG policy, selected roots, and Gateway credentials.
- Credentials or confidential data involved: no new credentials; existing Gateway-only mounts remain unchanged.
- New dependencies, destinations, files, processes, or generated content: no dependency or destination change.
- External schema provenance, publication rights, and drift evidence: not applicable.
- Output delivery and coverage: `list` is complete and exhaustive for local state; status reads are point observations; no pagination or retry.
- Publication and licensing concerns: none beyond normal public-boundary checks.

## Glossary

- **Tobari:** one named, untrusted execution container attached to one root.
- **cluster:** the installation-local shared Gateway, OPA, configuration, and CA lifecycle.
- **Tobari ID:** an opaque CLI-owned reference emitted by `list`.
