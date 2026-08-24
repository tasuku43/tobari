# Work Context: Give physical-host loopback an honest private authority

## Verified integrated facts

- Independent branch `codex/wp05-host-loopback-name` is cleanly rebased on
  exact `583d4e1e32b74107b6347b8addd622c44e6fb48e`; WP03 `922fa792`, WP04
  `cc5d14b`, WP07 `77c5607`, and accepted WP05 concern `0bbd9deb` are ancestors.
- ADR 0084 replaces predecessor conversion and `migrate apply` with a final-only
  clean break. `ConfirmNoPreReleaseLegacyAuthority` treats the Host Loopback,
  interactive-attachment, principal, auth, and service-exposure roots as
  first-initialization-only presence. It observes path presence only and does
  not decode, migrate, rename, clean, or delete predecessor content.
- Final entry uses `BeginFinalWorkspaceSession`. The private
  `interactiveWorkspacePrincipal` projects final ContextID, WorkspaceID,
  Context presentation, and ProjectRoot into frozen compatibility wires.
  TemplateID and Policy Memory do not cross this boundary.
- WP07 owns `Runtime.withInteractiveAttachmentLock`,
  `Runtime.permissionSessionActive`, and
  `Runtime.reconcileExpiredInteractiveSessionsLocked`. Its public global
  absence consumer is `ConfirmNoFinalWorkspaceSessions`. WP05 neither reads nor
  modifies ingestion endpoint, nonce, lease, ACK, wait registry, or mount/profile.
- Final entry establishes or borrows the canonical interactive session before
  Host Loopback. Host Loopback and service exposure remain separate dependent
  capabilities.
- Current implementation still uses `host.tobari.test`, public capability
  schema 1, private registry schema 1, and route ID domain
  `tobari-host-loopback-route-v1` without the hostname. Gateway has no checked-in
  TLS terminal hook yet.
- Gateway currently resolves principal before Host Loopback classification and
  `_permission_wait_effect` excludes only the old hostname. The retired HTTP
  guard must precede principal, credential, registry, OPA, wait, DNS/upstream,
  and relay consumers; both current and retired names must be ineligible for
  permission resume.
- The Gateway aggregate selects the Host Loopback branch from
  `destination.kind == "host_loopback"`; it needs an exact current hostname,
  HTTP, and bounded-port predicate so crafted sibling input cannot borrow the
  branch.
- Gateway source, embedded Gateway snapshot, helper source snapshot, and
  integration fixtures are mechanically checked and must change coherently via
  the repository sync scripts.
- The first accepted concern already added ADR 0083, durable docs, README, and
  a narrow public-guard exception for exact bounded-port
  `http://host.tobari.internal:{port}` with sibling/wildcard/TLS/private-host
  negative tests.

## Verified external facts

- ICANN Board resolution 2024.07.29.06 permanently withholds `.INTERNAL` from
  DNS-root delegation for private-use applications.
- The IANA Special-Use registry updated 2026-05-22 does not list `internal.`;
  no localhost-like resolver behavior is assumed.
- RFC 2606 reserves `.test` for testing and `.invalid` for deliberately invalid
  names. RFC 6761 gives `localhost.` descendants caller-loopback semantics.
- Pinned mitmproxy 12.1.2 exposes `tls_clienthello` with parsed SNI/raw
  extensions and `tls_start_client` with mutable `ssl_conn`. Its TLS layer
  closes when no truthy TLS context is supplied. Accepted disposable evidence
  proved that an exact terminal classification followed by a false no-cert
  context stops leaf generation/cache growth and all upstream/HTTP hooks; an
  ordinary control creates one leaf.

## Product Owner-fixed decisions

- Sole public/routable name: `host.tobari.internal`; retired name is a permanent V1 terminal negative guard.
- Public capability schema stays V1; private route/grant registry is V2. Route ID V2 directly includes exact hostname; grant ID remains hostname-bound and is freshly issued.
- Frozen `context_id`, `project_id`, `context` tokens remain private compatibility spellings whose values are ContextID, WorkspaceID, and Context presentation.
- Host header remains the exact new authority. Physical `127.0.0.1`, synthetic Gateway IP, and `host.docker.internal` are transport only.
- No CA rotation, broad cache deletion, alias, redirect, Host rewrite, or ordinary-policy fallback.
- Release client floor is curl/libc, Python, applicable Go pure/cgo, and Node; Java/browser are optional unless present.
- Host Loopback is not Runtime use evidence and cannot borrow WP07 permission-wait, service exposure, or research-auth authority.

## Assessment and remaining observations

- The mechanism is a hard authority replacement, not a compatibility rename.
  Schema-V2 validation and hostname-bound IDs make all prior references stale.
- ADR 0084 removes the former lifecycle -> interactive-attachment ->
  host-loopback migration transaction from WP05. The final presence guard is
  the only predecessor-state seam; runtime cleanup remains exact attachment
  teardown for fresh final records.
- Supported transparent-network and Runtime-client Docker canaries remain to be
  observed. Record standard image client versions, CA digest stability, leaf
  cache non-growth for rejected TLS, and exact Host at a synthetic local service.
- If a supported client bypasses Tobari synthetic DNS, rewrites Host, or causes
  a leaf/cache insert before the terminal hook, that is a completion blocker.
  Unsupported Java/browser/provider shapes are observation only.
