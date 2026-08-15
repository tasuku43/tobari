# ADR 0042: Upgrade the canonical Codex client to 0.147.0

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Runtime images, policy compatibility, security, harness, release, and public boundary
- Revises: The Codex 0.146.0 pin in ADR 0039
- Superseded by: None

## Context

ADR 0039 made the canonical base client pins and the finite
`builtin/agent-ready` effect catalog one compatibility contract. The canonical
base pinned Codex 0.146.0, while the maintainer's default custom runtime had to
install stable Codex 0.147.0 over that base. This duplicated a large standalone
package and made the effective client depend on custom-layer order.

On 2026-08-16, OpenAI's official stable channel identified 0.147.0 as latest.
Its release metadata, checksum manifest, and independently hashed Linux amd64
and arm64 package bytes agree with the repository locks adopted by this
decision. The standalone package retains the reviewed entrypoint, companion,
search, and Linux sandbox resource layout.

The existing 0.147.0 default-runtime use did not require a new core Codex HTTP
effect. There is therefore no evidence supporting broader baseline authority.

## Decision

The canonical combined base and retained Codex integrity fixture pin Codex
0.147.0. The coupled policy compatibility version changes to 0.147.0, but the
finite `builtin/agent-ready` exact grant set does not change. Any new or
unmatched client effect remains denied or enters the ordinary exact review
path according to the Context guardrail.

The base continues to install the complete official standalone package outside
the mutable Workspace home and verifies the checked Linux amd64 and arm64
SHA-256 locks before extraction. The combined artifact remains build-only with
`NOASSERTION`; this upgrade does not approve redistribution or publication.

## Consequences

- Custom Context runtimes based on the current canonical base no longer need to
  reinstall Codex merely to obtain 0.147.0.
- The canonical base and the host stable client can use the same version during
  local acceptance.
- No HTTP authority is inferred from the version bump. A future observed route
  still requires an explicit baseline review or normal permission decision.
- Existing immutable Context policy snapshots remain valid because the exact
  rule bytes are unchanged.

## Mechanical enforcement

- `runtimes/codex/runtime.lock.json` binds version, asset names, byte sizes, and
  SHA-256 values for both Linux architectures.
- `tools/runtimecheck` couples that lock to
  `tobari.AgentReadyCodexVersion`, both canonical Dockerfiles, runtime metadata,
  and the embedded base snapshot.
- Runtime smoke checks execute `codex --version` with a fresh Workspace home.
- Policy tests retain the exact core grants and continue to reject optional
  plugin, MCP, connector, transfer, evaluation, release, and update surfaces.
- Current product, architecture, security, release, harness, and readiness
  documents name Codex 0.147.0 as the reviewed client.

## Security and public-boundary impact

This change replaces integrity-pinned executable bytes without widening the
Gateway policy, credential projection, Broker plan, container privileges,
mounts, or network topology. No credentials or authenticated provider output
are recorded. Publication remains mechanically disabled while bundled-agent
redistribution and license review is pending.
