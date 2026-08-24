# Work Plan: Give physical-host loopback an honest private authority

## Chosen approach

Implement one hard-cut authority replacement while preserving the existing
attachment-owned capability model.

1. Rebaseline the packet and source map on final WP11 identity. Treat final
   ContextID/WorkspaceID as route/grant principal values and keep Template and
   Policy Memory outside the attachment branch.
2. Change the canonical public hostname/URL, retain public capability schema
   V1, advance private route/grant registries to strict schema V2, and bind
   route ID V2 directly to hostname. Rename Go route/grant fields to final
   Context/Workspace vocabulary while preserving exact JSON wire keys.
3. Update Gateway registry readers, exact branch selection, OPA router, helper
   snapshots, and tests as one coherent consumer concern. No dual current/old
   reader exists.
4. Add an exact retired HTTP classifier immediately after authority
   normalization. Return one fixed secret-free terminal fault before principal,
   credential, registry, OPA, permission-wait, DNS/upstream, relay, or retry.
5. Add TLS ClientHello classification for exact current/retired spelling
   variants plus absent/malformed SNI and ECH-unobservable authority. The
   paired start-client hook supplies no certificate context, causing terminal
   close before leaf generation. Later HTTP Host/SNI drift remains fail closed
   through the existing normalized-authority equality check.
6. Keep synthetic DNS non-recursive. Preserve exact Host while dialing the
   authenticated relay to physical `127.0.0.1`; add branch and call-count
   canaries for sibling/private names.
7. Use ADR 0084's existing presence guard for predecessor registries. Do not
   add migration, cleanup, zero-owner cutover, or cache mutation.
8. Run focused standard/research tests, sync/check source snapshots, then one
   bounded isolated Docker evaluator covering transparent routing, standard
   clients, retired HTTP, TLS pre-leaf/cache behavior, CA stability, and
   teardown. Finish all repository gates, promote evidence, and delete packet.

## Trust and data flow

```text
final ContextID + WorkspaceID + Context presentation
                  |
                  v
canonical interactive session (WP07 owner / epoch)
                  |
                  v
fresh schema-V2 route + exact reviewed grant
                  |
Workspace synthetic DNS -> Gateway exact Host -> OPA Host Loopback branch
                  |
                  v
authenticated relay -> physical 127.0.0.1:same-port
```

Template revision, Policy Memory, permission-ingestion transport, service
exposure, research Broker state, and Runtime lifecycle are outside this flow.

## Compatibility and state

- `host.tobari.test` is negative compatibility only: an explicit terminal deny.
- Schema-V1 route/grant registries are unsupported predecessor presence, not
  migration input. They block fresh final initialization without mutation.
- Fresh final attachment creates schema-V2 empty stores and fresh hostname-bound
  IDs. No old route, grant, candidate, relay token, attachment epoch, or cache
  entry is adopted.
- The shared root CA is preserved. Certificate cache contents are non-authority
  and the final binary does not clean predecessor cache entries.

## Alternatives rejected

- Keep `.test`: wrong routine-product meaning.
- Use `.localhost`: may resolve to Workspace loopback before Tobari.
- Use `.invalid`: clients may reject before Tobari classification.
- Use `host.tobari`: globally delegable/search-path ambiguity.
- Use `gateway.tobari.internal`: names an implementation hop, not destination.
- Route old and new names or rewrite Host: duplicates authority and widens the trust boundary.
- Translate predecessor registries: contradicts final-only clean break.
- Merge with WP07 registry or service exposure: different owners, directions, and authority.

## Risks and gates

- Most dangerous: TLS hook order could generate a leaf before terminal close.
  Prove cache/leaf count and zero server/request hooks in pinned mitmproxy 12.1.2.
- High risk: a current-name denial could enter permission resume. Exclude both
  current and retired names in the pure effect projection and add call counts.
- High risk: route/grant writer/reader mismatch across Go, Gateway, embedded
  source, and helper. Land schema/ID/hostname changes coherently and run both
  standard and research snapshot tests.
- High risk: ordinary policy could authorize a forged `host_loopback` kind.
  OPA requires exact current hostname, HTTP, and port bounds in addition to kind.
- Completion requires focused race/tests, isolated Docker evidence,
  `task check`, `task security`, `task public:check`, and `task release:check`.

## Fixed finite evaluator

This matrix is fixed before the first resumed production edit. `RED` means the
integrated baseline does not yet satisfy or prove the cell. `N/A` is closed by
design and cannot later be converted into new scope. Implementation may only
turn these 18 RED cells green; adding a row or widening a cell requires a new
Product Owner decision.

| ID | Contract | Standard focused evidence | Research / equality evidence | Race or isolated integration evidence |
|---|---|---|---|---|
| A | New HTTP exact host/port/effect plus live session epoch | **RED A-S:** domain, policy, Gateway allow/deny and Host-preservation tests | **RED A-R:** research Gateway uses the identical branch and frozen principal | **RED A-I:** transparent synthetic-DNS relay, reviewed grant, exact target and teardown |
| B | Retired `.test` HTTP/TLS terminal before principal, credential, route, OPA, upstream | **RED B-S:** early HTTP fault plus zero-call canaries | **RED B-R:** Broker-enabled addon proves zero credential/Broker/resume work | **RED B-I:** transparent retired HTTP/TLS reaches no physical service or external resolver |
| C | TLS ClientHello, ECH/unobservable SNI, and no-cert terminal close | **RED C-S:** pure hook cases for current/retired/case/trailing-dot/absent/malformed/ECH | **RED C-R:** canonical/embedded Gateway source equality retains the same hook | **RED C-I:** pinned mitmproxy 12.1.2 leaf/cache count, zero server-connect and HTTP hooks |
| D | Private route/grant schema V2 and public capability schema V1 | **RED D-S:** Go writer/validator and Gateway strict-reader schema/ID tests | **RED D-R:** release/research helper projections retain identical capability V1 | **RED D-X:** concurrent writer/read identity and stale schema-V1/reference rejection |
| E | Final ContextID/WorkspaceID projection plus canonical owner/liveness/borrower cleanup | **RED E-S:** final session bridge and route/grant projection tests | **RED E-R:** research composition adds no alternate owner or Template/Policy-Memory field | **RED E-X:** owner/borrower/teardown race and exact epoch non-inheritance |
| F | Gateway/OPA canonical, embedded, helper, and integration equality | **RED F-S:** Gateway and aggregate focused tests plus source snapshot checks | **RED F-R:** research package/source snapshot and frozen-wire equality | **RED F-I:** integration fixture consumes exact hostname/schema/Host contract |
| G | Clean-break legacy presence rejects with zero mutation | **RED G-S:** existing final presence guard extended with schema-V1 Host Loopback fixture | **N/A:** the guard is one compile-time-independent final-store precondition | **RED G-I:** tree-before/after proof; no lock/store/Docker/cache cleanup side effect |

Initial evaluator state: **18 RED, 1 N/A, 0 GREEN**. The bounded comprehensive
evaluation is complete only at **0 RED**; full repository gates then decide
completion without creating additional evaluator scope.
