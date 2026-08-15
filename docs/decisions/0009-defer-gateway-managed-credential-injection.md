# ADR 0009: Select tool-native passthrough by default

- Status: Accepted
- Date: 2026-08-02
- Deciders: Tobari maintainers
- Scope: Product, architecture, and security
- Supersedes: [ADR 0003: Inject credentials after authorization](0003-inject-credentials-after-authorization.md)
- Superseded by: [ADR 0035: Require the Auth Broker for declared provider bindings](0035-require-broker-for-declared-bindings.md)

## Context

Tobari already gives each CWD-owned runtime a persistent home mounted at
`/var/lib/tobari`, and its base/agent images provide ordinary tools that have
their own authentication flows. The initial design instead asked a trusted
Gateway to read host `credentials.json` profiles and inject bearer or fixed
headers after OPA allow. That creates a second credential setup flow and delays
the useful outcome of entering a project and using arbitrary tools. The existing
implementation is valuable as a later mode, so it must remain replaceable
rather than being deleted.

## Decision drivers

- Make the first end-to-end value usable for arbitrary tools and agents.
- Keep provider-specific API adapters, account semantics, and SDK credentials
  out of the product boundary.
- Preserve the Gateway/OPA network and policy enforcement boundary.
- Avoid exposing the host home, host keychain, or host CLI configuration to an
  untrusted Tobari.
- Keep secret values out of OPA input, Gateway audit, denial projections, and
  CLI output.

## Considered options

- Select tool-native passthrough by default and let each tool authenticate inside
  its per-Tobari home, while retaining managed injection behind an adapter.
- Keep host-file credential profiles as the initial path.
- Mount the host home or individual host CLI configuration into Tobari.

## Decision

The initial scope selects a tool-native passthrough credential adapter. A tool or
agent may execute its normal login/configuration flow inside a Tobari and write
its state under `HOME=/var/lib/tobari`. The directory is backed by that
Tobari's exact persistent host home, survives runtime-container recreation, and
is removed by the explicit Tobari delete operation. Different Tobari instances
do not share this state.

Gateway and OPA remain outside Tobari and continue to authorize generic
HTTP/HTTPS effects. The passthrough adapter does not load managed credential
profiles or inject a managed secret. It redacts client authentication and
cookie values from OPA input and audit data, strips only proxy/control headers
that must not be forwarded, and forwards client-authenticated headers after an
ordinary policy allow. The request still has one normalized policy decision and
one upstream attempt.

The existing profile binding and injection behavior remains as a separate
managed adapter. Trusted runtime configuration may select it later; selection
is explicit and there is no hidden fallback from passthrough.

Existing `credentials.json` and `credentials/` inputs remain reserved for the
managed adapter and are not read by the default passthrough adapter. They are
not deleted automatically and may be removed explicitly when no managed mode
requires them.

## Consequences

- Any process inside one Tobari can read and use that Tobari's tool credentials;
  this is intentional same-Tobari trust, not host-wide credential protection.
- A tool credential can exercise whatever authority both the trusted policy and
  the upstream account grant. Allowing a destination remains an explicit
  security decision.
- Native device/browser handoffs remain tool behavior. Tobari does not add a
  browser bridge, token parser, clipboard integration, refresh service, or
  provider-specific login command.
- Selecting the retained managed adapter is a runtime configuration decision
  whose trust boundary, injection semantics, and tests must remain explicit.
  Provider-specific credential adapters still require a new reviewed decision.

## Mechanical enforcement

- Compose/runtime tests prove the selected adapter is explicit and defaults to
  passthrough; managed inputs remain available only for managed mode.
- Gateway tests prove client auth is redacted from policy/log output and is
  forwarded only after allow in passthrough mode; proxy/control headers are not
  forwarded; managed profile injection remains compatible.
- Rego and denial-output tests keep credential-profile semantics secret-free and
  adapter-dependent.
- Runtime integration writes a synthetic auth file below one Tobari home,
  recreates the work container, proves persistence, and proves another Tobari
  cannot read it.
- Delete integration proves the exact home is removed with its owning Tobari.

## Compatibility, security, and validation

This is a pre-v1 narrowing of the initial authentication contract. The generic
OPA input, Gateway decision, audit, and policy-learning boundaries remain
explicit compatibility surfaces. Tests use local synthetic fixtures only. The
managed adapter is available when tool-native state is insufficient, but it must
not become an implicit fallback or a provider-specific credential framework.
