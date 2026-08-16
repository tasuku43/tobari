# ADR 0044: Make native Workspace authentication standard

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, release, and harness
- Revises: ADR 0019, ADR 0025, ADR 0033, ADR 0035, ADR 0038, and ADR 0043
- Superseded by: ADR 0045 for all Tobari-owned image publication and release-lock scope

## Context

Claude Code completed its native OAuth token exchange inside a Workspace, but
the standard provider projection then classified its own
`GET https://api.anthropic.com/api/oauth/profile` Authorization header as a raw
credential at a Broker-required binding. Gateway returned
`broker_auth_required`; Claude consequently observed null `subscriptionType`
and `rateLimitTier` and `/status` misreported `Claude API account`. Repeated
provider-schema and acquisition fixes also made standard login diverge from
the supported client experience without improving the ordinary network-policy
boundary.

## Decision

The standard profile has no provider binding, provider projection, Auth Broker
service, Broker socket, handle projection, root key, vault, companion, or
`auth` command. Claude Code, Codex, and other tools authenticate natively inside
one persistent Workspace home. Gateway redacts authentication before OPA and
audit, then forwards the original header only after the ordinary exact HTTP
effect is allowed. Host credentials are never inherited or copied.

`builtin/agent-ready` includes the exact native-login effects required by the
pinned clients. For Claude these include `GET platform.claude.com/v1/oauth/hello`,
`POST platform.claude.com/v1/oauth/token`, and
`GET api.anthropic.com/api/oauth/claude_cli/roles`; the existing
`GET api.anthropic.com/api/oauth/profile` remains granted. For Codex they include
`POST auth.openai.com/oauth/token` plus the two POST device-auth endpoints under
`/api/accounts/deviceauth`. Browser navigation does not cross Workspace egress
and is not a policy grant.

The Broker implementation remains compile-time-only in `task build:dev`. It
uses a three-service Compose override and an experimental Gateway layer. It is
unsupported, is not present in the standard catalog or Gateway image, has no
release lock entry, and is never published.

## Consequences

- A process in one Workspace can read that Workspace's native agent credential
  and use it only through policy-allowed network effects. Tobari does not claim
  to distinguish the authenticating CLI from another same-Workspace process.
- Native provider account metadata and client UX remain provider-owned; Tobari
  no longer decodes or re-encodes their credential schema in standard.
- Deleting a Workspace deletes its persistent native auth state. Recreating or
  switching Workspaces requires native login in that Workspace.
- The standard shared cluster has exactly Gateway and OPA. The release publishes
  exactly Gateway; the standard component lock is Gateway-only.

## Mechanical enforcement

- Standard build-tag tests require no auth commands, provider environment,
  provider mount, Broker Compose service, Broker status field, or Broker image
  authority.
- Standard Gateway image inputs exclude Broker modules. The native-login
  Gateway regression proves Claude/Codex Authorization values are absent from
  OPA input and preserved after allow without `broker_auth_required`.
- Policy-preset tests pin every Claude and Codex native-login method/host/path.
- Experimental tests compile both `tobari_dev` and `tobari_experimental` and
  retain the closed Broker contracts without creating a runtime switch in the
  standard binary.
