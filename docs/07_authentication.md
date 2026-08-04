# Authentication handling

Tobari does not interpret provider-specific login protocols. Each tool or agent
may run its normal login/configuration flow inside a Tobari and persist its own
state below `HOME=/var/lib/tobari`. Gateway authentication handling is
pluggable; the default is tool-native passthrough, while the existing managed
profile-injection behavior remains available as a separate adapter.

## Selected adapter

The trusted Gateway runtime selects `TOBARI_CREDENTIAL_ADAPTER`:

- `passthrough` is the default. It never loads managed credential profiles. It
  redacts authentication and cookie values from OPA input and audit data, then
  forwards the original client/tool authentication only after policy allow.
- `managed` preserves the existing static profile behavior. It validates the
  selected profile against the host-issued project principal and normalized
  request host, then injects a bounded bearer or fixed-header secret only after
  policy allow.

Selection is infrastructure configuration, not a user-facing provider login
command in the current slice. There is no implicit fallback from passthrough to
managed injection. If managed selection becomes user-facing later, it should
become Context metadata so the adapter choice and its credential stores switch
together; the adapter implementation and secret values must remain
infrastructure-owned.
The default path applies equally to GitHub CLI, AWS CLI, Claude, Codex, and any
other tool that owns its authentication flow; these names are examples, not a
provider-specific product boundary.

The active Context is the host-facing composition boundary for managed
credential metadata and secret paths. A Context never contains tool-native
login state, and its display name or credential-profile name is not an
authority. Project-principal and normalized-host binding checks remain the
source of managed credential authority.

## Deferred auth-broker experiment

The provider-facing auth-broker experiment is not part of the supported
`main` product surface. Its implementation and branch-only decision records
remain on `codex/auth-broker`; no broker command, provider login flow, or
broker-owned credential authority is present in the current Catalog. This is
an explicit deferral while Tobari's bounded-autonomy and policy-learning value
is established, not a partially supported authentication mode.

Resuming the experiment requires a new reviewed work packet and fresh product,
architecture, security, public-boundary, and harness decisions. The detached
branch must not be merged or cherry-picked as a routine documentation or
authentication update.

## Tool-native runtime flow

1. The runtime mounts one private persistent home at `/var/lib/tobari` for the
   selected Tobari.
2. The user runs the tool's normal login or configuration flow inside it.
3. The tool writes its state below that home; Tobari does not copy host home,
   keychain, SSH-agent, environment, or CLI configuration into it.
4. The tool sends a request through the explicit Gateway proxy.
5. The passthrough adapter keeps the client authentication out of OPA input and
   audit, removes proxy/Tobari control headers after allow, and forwards the
   authenticated request once.

The per-Tobari home survives shell exit and runtime-container recreation. It is
removed only by the exact Tobari `delete` operation. Every process inside one
Tobari can read that home by design; another Tobari cannot access it through the
supported mounts or network topology.

## Retained managed adapter

The managed adapter supports:

- `bearer`: read one bounded token from a Gateway-only file and set
  `Authorization: Bearer <value>`;
- `header`: read one bounded value from a Gateway-only file and set the
  configured header.

Every profile has an exact list of normalized destination hosts and an explicit
project-principal binding. Profile names, types, hosts, header names, and
container paths are non-secret configuration. Secret values are never accepted
in CLI arguments, Tobari environment variables, or OPA input.

The managed adapter checks the project binding before OPA and again immediately
before reading the secret. Missing, unreadable, empty, oversized, malformed,
host-mismatched, or project-mismatched material fails closed. It does not probe
another profile after failure.

Managed configuration remains a trusted Gateway-side compatibility input. The
default passthrough adapter does not load it, and tool-native authentication
does not depend on it.

## Deliberate exclusions

Tobari does not implement OAuth, refresh, PAT discovery, provider account
selection, AWS SigV4, GitHub App tokens, macOS Keychain, dynamic short-lived
credentials, browser bridges, token parsers, or provider-specific login
commands. A tool may implement its own native flow inside its Tobari home.
Adding a new Gateway/provider adapter requires a separate reviewed thesis,
architecture, security, and harness decision.

## Verification

- Gateway tests prove passthrough forwards client authentication only after
  allow and excludes it from policy/log output.
- Gateway tests retain managed profile validation, project/host binding, and
  injection coverage.
- Docker integration tests prove a synthetic tool-auth file persists through
  runtime recovery, is isolated from another Tobari, and is removed by delete.
- Integration tests prove an allowed client-authenticated request reaches the
  mock upstream while the credential value stays out of Gateway/OPA/CLI output.
