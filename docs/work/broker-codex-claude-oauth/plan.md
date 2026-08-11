# Work Plan: Closed Codex and Claude authentication plans

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Implement the accepted ADR as two closed built-in authentication plans. Keep
host acquisition, Broker storage/refresh, Gateway post-policy projection, and
Workspace consumption as separate trust-boundary stages. Describe provider
variation with strict reviewed manifests while keeping every credential shape,
helper, refresh flow, and supplemental header plan compiled and closed.

## Provider plans

### OpenAI Codex

The host driver runs the pinned Codex authorization-code flow with bounded
terminal and callback handling. The Broker stores the acquired OAuth state,
refreshes it only through the exact reviewed OpenAI transport, and exposes a
project-bound handle. After policy allow, Gateway resolves that handle and
injects the exact authorization and ChatGPT account headers required by the
declared HTTP binding.

### Anthropic Claude

The host driver runs the pinned Claude Code setup-token flow in a protected
PTY, validates bounded output, and imports the token through the Broker control
path. The Broker treats it as static inference credential material with no
refresh plan. After policy allow, Gateway injects only the exact Anthropic
headers declared by the reviewed binding.

## Layer ownership

- Domain owns reviewed provider identities and invariant vocabulary.
- Application owns the login task and the smallest host-acquisition port.
- Infrastructure owns CLI processes, PTYs, credential parsing, Broker/Gateway protocols, secrets, network transport, and runtime assets.
- CLI owns catalog grammar, terminal selection, presentation, and dependency wiring.

## Failure behavior

Reject unsupported methods, malformed provider output, timeouts, cancellation,
invalid or stale handles, provider mismatch, HTTP-binding mismatch, refresh
failure, and API-label mismatch before credential use. Preserve structured
mutation outcomes and provide read-only reconciliation guidance when a result
cannot be classified.

## Verification

1. Run focused Go tests for domain, application, CLI, credential-host, Docker runtime, and runtime compatibility.
2. Verify canonical and embedded Broker/Gateway source equality.
3. Run Broker and Gateway unit/image suites.
4. Run repository security checks and the full four-container integration suite.
5. Run the repository gate and record the exact Pages-only deferral without editing GitHub Pages sources.
6. Before publication or release, build immutable API 4/3 images and record both live-provider login journeys required by ADR 0025.

## Rollout and rollback

The new providers are built-in reviewed plans, not an extensible executable
surface. Runtime compatibility labels fail closed. Rolling back requires
removing the built-ins and restoring matching runtime and image API pins; it
does not permit API 4/3 clients to run against older images.

## Documentation promotion

Durable decisions live in ADR 0025 and numbered documents 00 through 09.
README and Auth Broker documentation describe supported operator behavior.
GitHub Pages changes are intentionally deferred.
