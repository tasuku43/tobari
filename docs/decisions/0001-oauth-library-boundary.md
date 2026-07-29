# ADR 0001: Keep OAuth behind an optional reviewed infrastructure adapter

- Status: Accepted
- Date: 2026-07-18
- Deciders: Template maintainers
- Scope: Architecture, security, harness, and public boundary
- Supersedes: None
- Superseded by: None

## Context

Derived API CLIs commonly need either OAuth 2.0 or a personal access token (PAT). OAuth contains security-sensitive protocol behavior such as state validation, PKCE, token exchange, expiry, refresh, and authenticated transport. Reimplementing those details in each derived CLI creates avoidable security and interoperability risk.

Adding an unused OAuth module to the template core would create a different problem: every derived project would inherit code, updates, and supply-chain exposure even when it uses only a PAT or another mechanism. The core contract therefore needs to be reusable without pretending that every product has the same OAuth flow.

## Decision drivers

- Do not implement security-sensitive OAuth protocol machinery from scratch.
- Keep raw credentials out of command arguments, domain values, application values, logs, help, and command output.
- Avoid unused runtime dependencies in the template baseline.
- Make a selected dependency reviewable, replaceable, and mechanically checked.
- Leave provider endpoints, grants, scopes, account selection, storage, refresh, and revocation to the derived security model.

## Considered options

### Option A: Implement OAuth in the template

This avoids a third-party module but makes the template responsible for protocol correctness, provider quirks, security updates, and long-term interoperability. That responsibility is larger and riskier than the dependency it removes.

### Option B: Include an OAuth module in every generated project

This provides immediate implementation code, but imposes an unused dependency on PAT-only projects and chooses a flow before the product and security model have established one.

### Option C: Fix the boundary now and add a reviewed library only when selected

The template supplies secret-free authentication requirements and session metadata, a non-serialized ephemeral binding to the exact infrastructure-owned authentication record, failure taxonomy, a fail-closed application gate, tests, and an infrastructure boundary. Authenticated application task ports pass that binding unchanged so infrastructure can resolve and revalidate the same private record immediately before I/O. A derived project that selects OAuth adds a concrete library behind that boundary.

## Decision

Choose Option C. The template does not contain an OAuth protocol implementation and does not depend on an OAuth module by default. When a derived project's accepted security model selects OAuth 2.0, [`golang.org/x/oauth2`](https://pkg.go.dev/golang.org/x/oauth2) is the default candidate because it is maintained by the Go project and supplies the relevant protocol and transport primitives. The project imports only the required packages and wraps them inside `internal/infra`; domain and application code never depend on the library or receive its token types.

A PAT implementation uses the same template authentication boundary. Its secret source and authenticated transport also remain in `internal/infra`; the PAT itself never becomes a domain or application value.

A provider-maintained SDK may replace `golang.org/x/oauth2` only when the derived ADR shows that it is necessary, reviews its transitive dependency and license surface, and preserves the same boundary.

## Consequences

### Positive

- OAuth protocol machinery is not rewritten by each project.
- PAT-only projects retain a small dependency surface.
- Replacing or upgrading an OAuth library does not change use cases or public output.
- The authentication method remains a product/security decision rather than a template accident.

### Negative

- OAuth projects perform an explicit dependency review before their first authenticated vertical slice.
- `golang.org/x/oauth2` has not reached v1, so upgrades can require adapter maintenance.
- The template cannot provide a runnable login flow without provider-specific decisions.

### Risks and mitigations

- **Compromised or vulnerable dependency:** pin the selected module version in `go.mod`/`go.sum`; retain checksum database verification; run `go mod verify`, `govulncheck`, dependency review, and the full gate.
- **Excess transitive code:** import only required subpackages, inspect `go mod graph`, and reject a provider SDK when its added surface is not justified.
- **Credential disclosure:** keep token-bearing types in infrastructure, redact upstream errors, and test public errors and output with secret-like canaries.
- **OAuth implementation error:** require state validation and PKCE where the provider supports the authorization-code flow; test callback mismatch, cancellation, expiry, refresh failure, and account/scope mismatch.
- **Unreviewed update:** automated update proposals must pass the same gate and receive security review; do not use floating versions in builds or tools.

## Mechanical enforcement

- The four-layer architecture prevents application and domain packages from importing external OAuth modules.
- The authentication gate accepts only secret-free requirement and session metadata; its opaque binding is non-secret correlation metadata, not a credential or bearer capability.
- Architecture lint rejects production binding issuance outside infrastructure; each derived adapter's contract tests must prove that authenticated task ports resolve the exact binding and reject stale, mismatched, or cross-session records before provider I/O.
- `go.sum`, `go mod verify`, `govulncheck`, dependency update automation, and public repository checks are part of the one gate.
- Adapter contract tests use fake credentials and assert that secret canaries never reach errors or output.
- A derived OAuth project adds an accepted ADR naming the grant, callback model, PKCE/state behavior, token source, storage, refresh/cache rules, scopes, account selection, and revocation behavior.

## Compatibility and migration

This decision adds no runtime dependency and changes no public command. A derived project can move from PAT to OAuth, or replace its OAuth library, behind the authentication and API adapter ports. Any user-visible login, credential storage, configuration, or account-selection change still requires a product and security migration plan.

## Security and public-boundary impact

The template stores no credential, client secret, private endpoint, callback registration, or real account identifier. Derived projects must use publishable fixtures and fake tokens in tests. OAuth or PAT setup documentation must describe credential destinations and redaction behavior without publishing real values.

The template tracks the latest stable Go patch release rather than an older compatibility floor. At acceptance time that version is [`1.26.5`](https://go.dev/doc/devel/release#go1.26.5); it includes the current security fixes and is newer than the fixed release for the 2026 [`cmd/go` checksum-validation vulnerability](https://pkg.go.dev/vuln/GO-2026-4984). CI resolves the exact version from `go.mod` so local, CI, security, and release workflows converge on the same toolchain.

## Validation

Run:

```text
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
./scripts/check.sh full
```

For a derived OAuth project, also inspect the dependency delta and run the authentication adapter's protocol, cancellation, redaction, and zero-downstream-call tests.

## Reconsideration signals

Create a superseding ADR if the Go project deprecates `golang.org/x/oauth2`, a selected provider requires a materially different standard, the dependency's maintenance/security posture changes, or evidence shows that a provider SDK has a smaller and safer total implementation surface.
