# Work Context: Closed Codex and Claude authentication plans

## Verified baseline

- The repository already owns a local Auth Broker, Gateway, root key, encrypted vault, project-bound handle protocol, and reviewed built-in provider manifests.
- `auth login` performs host-side acquisition and sends the acquired credential to the Broker over the protected local control path.
- Gateway resolution occurs after OPA allow and replaces a recognized handle only for its exact HTTP binding.
- Canonical Broker and Gateway sources are embedded into the Go runtime assets and guarded by byte-equality checks.
- ADR 0025 is accepted and defines the exact Codex and Claude flows, pins, API revisions, destinations, and validation obligations.

## Change surface

- Domain and application: reviewed provider vocabulary, login method validation, and provider-specific acquisition state.
- Infrastructure: Codex authorization-code driver, Claude setup-token PTY driver, strict built-in manifests, Broker refresh, and runtime wiring.
- Gateway: provider-specific supplemental headers and removal of caller-supplied credential headers.
- CLI: installed-provider selection and unchanged fixed-target authentication mutation semantics.
- Runtime recipes: pinned Codex CLI 0.146.0 and Claude Code 2.1.220 integration.
- Harness: source snapshots, image contracts, unit tests, hostile-output tests, and four-container integration scenarios.

## Security boundaries

- Codex primary refresh material and Claude setup tokens enter only through trusted host or Broker infrastructure paths.
- Workspaces receive only versioned opaque handles and cannot read the Broker vault, root key, or provider primary secret.
- OpenAI refresh uses one fixed token endpoint and exact client identity; Claude has no refresh transport.
- Gateway introspection is non-secret, resolution is deny-before-resolve, replacement happens exactly once after allow, and invalid handles never fall back to caller credentials.
- External text is treated as untrusted data and credentials are excluded from user-visible output, diagnostics, and logs.

## Compatibility and versioning

- Gateway API is version 4.
- Auth Broker API is version 3.
- Runtime compatibility checks reject mismatched image labels before startup.
- Provider acquisition contracts are tied to the reviewed Codex and Claude CLI pins; pin changes require a new review and manual evidence.

## Current evidence

- `task security`: passes.
- `task authbroker:test`: passes, including source/snapshot equality, image contract, and 101 Python tests.
- `task gateway:test`: passes all 82 Gateway tests.
- `task integration:test`: passes the four-container suite after aligning the
  new provider assertions with the current human policy-result contract.
- Focused Go auth/runtime tests and `go test -race -count=1 ./internal/...`: pass.
- `go vet ./...`, `go mod tidy -diff`, and `git diff --check`: pass.
- `task check`: repository, architecture, and contract checks pass; it currently stops because the separately maintained GitHub Pages JSON-schema table markers are stale. This packet deliberately does not modify `docs/architecture-site/**`.

## Integration observation

The first integration replay reached both new provider denial paths but stopped
when their approval assertions still expected the superseded `applied: true`
text. Existing scenarios already asserted the current human contract: `Policy
rule updated` plus the exact HTTP target. The new scenarios now assert the same
contract and the complete integration suite passes.

## Unknowns and release-only evidence

- Live OpenAI browser authorization must be replayed against the pinned Codex client before publication or release.
- Live Claude setup-token acquisition must be replayed against the pinned Claude Code client before publication or release.
- Reviewed immutable image digests for API 4/3 are publication artifacts and are not produced by this implementation commit.

## Related work

`docs/work/auth-login-provider-selector/` owns the human provider-selection UX.
This packet owns the broader credential acquisition, brokering, projection,
runtime, and security capability. They share one integration path but retain
separate completion criteria.
