# Work Goal: Named Tobari cluster

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Lead engineer
- Target: Current implementation
- Related ADRs: `docs/decisions/0004-use-docker-as-the-mvp-isolation-primitive.md`, `docs/decisions/0008-keep-gateway-and-opa-outside-realm.md`

## Outcome

A developer starts one installation-local Gateway and OPA cluster, then attaches
multiple named Tobari isolation containers to arbitrary host directories. Each
Tobari has its own internal network and persistent home, while all HTTP and HTTPS
effects pass through the shared Gateway and live host-edited XDG policy.

## Why now

The first single-Realm slice proved the proxy and policy boundary, but it makes
users destroy or repurpose one environment whenever they change work roots. The
requested product loop instead treats isolation as something users can attach
freely to ordinary work directories and to Tobari's own XDG configuration
directory.

## Non-goals

- Selecting an individual Tobari for actions by an ambiguous display name
- Giving OPA write access to trusted host policy files
- Transparent proxying, non-HTTP protocols, or per-Tobari policy engines
- Migrating schema-1 singleton state automatically

## Acceptance criteria

- [ ] `cluster up` creates or reconciles one shared Gateway and OPA without creating a Tobari.
- [ ] `attach --name NAME --root PATH` creates one named Tobari with a dedicated internal network and home volume.
- [ ] `list` returns every configured Tobari with an opaque ID usable unchanged by `shell`, `exec`, `logs`, and `detach`.
- [ ] Host edits under the XDG policy directory are watched by OPA without restarting the cluster.
- [ ] Direct egress, OPA access, cross-Tobari access, broad host mounts, and unowned cleanup remain denied.
- [ ] Agent discovery through scoped help plus `list` is sufficient; routine success needs no external parser or source inspection.
- [ ] The pre-v1 singleton command and state break is explicit in the product contract and README.
- [ ] `task check`, `task security`, `task public:check`, and the runtime profile pass.

## Governing documents

- Thesis: 1 through 8, especially topology, lifecycle ownership, and policy learning
- Product contract section: Public vocabulary, commands, configuration, and side effects
- Architecture or security invariant: Four layers, exact labels, separate networks, and fail-closed egress
- Existing ADR: Docker isolation, explicit proxy, and trusted Gateway/OPA separation

## Completion definition

The work is complete when the acceptance criteria have evidence, durable
decisions have been promoted to numbered documentation, required profiles pass,
temporary diagnostics are removed, and this temporary packet is removed.
