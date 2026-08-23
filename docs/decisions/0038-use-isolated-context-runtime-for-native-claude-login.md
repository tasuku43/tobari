# ADR 0038: Use an isolated Context runtime for native Claude login

- Status: Accepted
- Date: 2026-08-15
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, harness, runtime images, and public boundary
- Revises: ADR 0025, ADR 0031, and ADR 0036
- Revised by: ADR 0041
- Superseded by: ADR 0044 for the release surface; retained only by the research surface

## Context

The prior Anthropic plan ran `claude setup-token` on the trusted host and stored a static inference-only automation token. That surface is portable, but it is not the ordinary Claude Code account-login experience and cannot reproduce the native renewable session that Claude Code itself maintains.

Runtime observation of exactly Claude Code 2.1.220 on Linux established a bounded alternative. `claude auth login --claudeai` opens a browser authorization URL and supports a paste-code fallback when a callback cannot reach the process. On success it writes one private `.claude/.credentials.json` containing a renewable `claudeAiOauth` session. The same exact client accepts a complete credential file whose access-token field contains a Tobari handle, allowing Workspace projection without transferring the real access or refresh token.

Claude's full account session has no documented portable host export comparable to Codex's file-store override. A fresh container using the selected Context image can nevertheless run the same exact Claude version and observe its own native Linux output without reading a host credential store or depending on a host installation. This makes the selected Context image an explicit acquisition authority for this provider only, which must be narrower than ordinary Workspace execution.

Anthropic's published terms and authentication guidance can constrain third-party use of Claude.ai account credentials. Technical support in the repository is not a representation that every distribution or deployment is authorized. Public release remains gated on explicit legal/product review and any required provider approval.

## Decision

`tobari auth login --provider anthropic` resolves the selected existing Context and starts one fresh, random-name, one-shot container from its compatible image. The container receives no bind mount or volume, no project or Workspace path, no Context home, no Auth Broker or Gateway socket, and no Docker socket. It runs as the image's non-root user with all capabilities dropped, `no-new-privileges`, bounded PIDs, memory, CPU, output, and login time.

Before login, Tobari observes exactly `/usr/local/bin/claude --version` as `2.1.220 (Claude Code)` and streams the executable bytes through `docker cp` to compute the host-side SHA-256 digest. It records the exact image ID, executable path, digest, and product version. A hash command from the custom image is not trusted.

The fixed login argv is:

```text
/usr/local/bin/claude auth login --claudeai
```

The Claude child owns OAuth state, PKCE, provider exchange, paste-code validation, and non-echoing terminal input. Tobari recognizes only the exact current opening, OSC 8 authorization-link, browser-result, and no-newline paste-prompt events and renders a fixed Tobari UI. It asks the trusted host to open the exact validated HTTPS URL once. Success reports the host open without repeating the long URL; failure retains that exact URL for manual navigation and does not by itself fail provider login. The prompt is emitted immediately rather than waiting for a newline and includes fixed guidance that authorization may take a moment after Enter. Tobari passes the original terminal reader unchanged to Docker and Claude rather than wrapping or inspecting submitted input. Successful child exit produces fixed Context-credential-validation feedback before capture. Neither fixed message contains input or provider state. Tobari emits no entered code and does not guess that the next real child failure is an echo. The one-shot container overrides the ordinary Workspace CA-waiting entrypoint with fixed `/usr/bin/tini -- /usr/bin/sleep infinity`; the acquisition boundary intentionally provides no Workspace CA mount. While Docker holds the interactive terminal in raw mode, every fixed and control-safe pass-through line uses explicit CRLF rather than relying on disabled newline post-processing. Exact cursor hide/show events are consumed as presentation state and closed with one Tobari-owned cursor-show instead of being rendered as escaped text. Invalid, changed, duplicated, or hostile provider output remains control-safe visible data and never becomes browser authority.

After successful child exit, Tobari copies only `/var/lib/tobari/.claude/.credentials.json` and requires one owner-only regular tar entry. The native file is an acquisition input, not Tobari's durable schema: Tobari extracts only the exact `accessToken`, `refreshToken`, `expiresAt`, and granted `scopes` values required by Broker. It does not compile Claude scope names. Instead it validates bounded RFC OAuth scope-token syntax and uniqueness for the authorization request and grant, requires the grant to be a subset of the exact set observed in the browser URL, normalizes provider ordering to Tobari's canonical order, and discards `clientId`, subscription, rate-limit, refresh-expiry, and any additive provider-owned metadata. Duplicate JSON keys still fail closed. It canonicalizes the four values with the observed image and executable identity into a strict versioned Tobari-owned record. Any version, required-field, output, capture, or cleanup failure preserves the previous Context credential. Capture failure publishes only one fixed stage name for credential export, archive envelope, file permissions, document encoding/JSON, OAuth record/core/token/expiry/scope set, or canonical record; raw causes and values remain private. Checked container removal is a commit precondition.

Auth Broker stores the strict Tobari-owned record as `anthropic_claude_oauth_session` in the Context-bound encrypted vault. It selects an unexpired access token or performs one same-record, single-flight refresh against only `https://platform.claude.com/v1/oauth/token` with the compiled fixed Claude client and the persisted canonical scope set; it never derives refresh authority from optional native-file metadata. A refresh response must preserve the same scope set. Redirects and proxies are disabled. A refresh result with unknown outcome leaves the existing durable no-replay barrier and requires reconciliation or login.

Gateway retains only the exact `api.anthropic.com:443` bearer binding. A Workspace receives a minimal Tobari-owned `.claude/.credentials.json` whose access token is the project-bound handle, whose refresh token is empty, whose expiry is a fixed non-secret native-shape sentinel, and whose scopes are the dynamically granted non-secret set returned with handle issuance. It receives neither the primary access token nor the renewable session. Gateway removes the handle, asks OPA about the ordinary request, and resolves one same-revision bearer only after allow.

Exact Claude 2.1.220 keeps interactive onboarding completion in a separate
private `.claude.json`; a valid credential alone makes `claude auth status`
authenticated but does not close the first-run login-method selector. The
Anthropic built-in therefore declares a second non-secret `merge_json`
projection containing only `hasCompletedOnboarding: true`. Runtime preserves
all unrelated top-level values, rejects unsafe filesystem and JSON shapes, and
removes only that registered field when the Anthropic projection is removed.
Owner manifests cannot select mutable JSON merging.

## Consequences

### Positive

- Anthropic login follows Claude Code's native account flow and retains renewable-session behavior.
- Host Claude installation and host Claude credential-store formats are no longer part of the acquisition contract.
- The custom runtime can carry the same pinned Claude build used by the Workspace while the one-shot login container remains project-free.
- Workspace Claude sees its expected credential-file shape but only receives a scoped Tobari handle.
- Authenticated interactive Claude starts directly without a redundant login-method selector.

### Negative

- For Anthropic acquisition only, the operator-selected Context image becomes trusted to run the provider login and sees the newly issued native credential before Broker capture.
- The current contract is pinned to one exact Claude version, executable path, OAuth client, four required native field names, authorization URL safety shape, OAuth scope-token grammar, and refresh response subset. Provider scope names remain dynamic. It deliberately does not pin or persist the complete private Claude file schema.
- Interactive startup additionally pins one non-secret top-level onboarding boolean for the exact client while leaving every other mutable Claude value client-owned.
- Browser authorization uses Claude's paste-code fallback because the isolated container callback is not exposed to the host.
- Public distribution may require provider approval independent of technical correctness.

## Mechanical enforcement

- Container tests prove exact image selection, version observation, host-side executable hashing, fixed argv, resource limits, absence of every mount and privileged surface, bounded capture, and cleanup-before-commit.
- A versioned synthetic visible-output fixture and browser tests accept only the exact Claude events, fragment the no-newline prompt, prove it is visible before flush, hide the successful URL, retain one exact manual fallback, preserve the first real provider failure/status after the prompt, consume exact cursor state with one owned restore, and reject host/client/state/challenge drift plus malformed or duplicate dynamic scope tokens.
- Container argv tests require fixed `/usr/bin/tini -- /usr/bin/sleep infinity` instead of the Workspace CA-waiting entrypoint. Live diagnostic evidence reproduced the old mount-free entrypoint exit and Claude exec status 137 at about ten seconds, then proved the fixed path remained input-bound until the outer bounded deadline.
- Fault tests distinguish container setup, Claude authorization, bounded output, timeout, staged strict native-state capture, and checked cleanup. They prove each published stage remains secret-free. When timeout and cleanup both fail, the joined result retains timeout as the primary user recovery while preserving cleanup evidence internally.
- Go and Python state tests use only synthetic tokens and prove four-field extraction, additive provider-metadata removal, duplicate/core-field rejection, dynamic scope-token bounds, future-name acceptance, granted-subset enforcement, scope-order normalization, strict Tobari canonical schema, executable binding, redaction, fixed refresh transport without scope drift, single-flight state replacement, durable unknown-outcome barriers, rotation, and logout behavior.
- Gateway and projection tests fix the native handle-only file shape, dynamically reproduce the granted non-secret scope set, retain the exact Anthropic bearer binding, merge and remove only the onboarding field, preserve unrelated Claude values, and reject malformed, duplicate-key, oversized, symlinked, or non-private mutable state.
- Source/snapshot checks include both Anthropic refresh modules, and Auth Broker image checks continue to prove no provider CLI exists in Broker.
- Manual release validation records only exact version/digest and pass/fail. It never records an authorization URL, paste code, account identity, access token, refresh token, native credential file, handle, or authenticated transcript.

## Compatibility and migration

This pre-public V1 change provides no compatibility reader or state migration.
Existing static Anthropic records and setup-token projections are invalid under
the new contract and must be discarded by recreating local development state.
The operator then runs `auth login --provider anthropic` and re-enters each
Workspace to receive the new complete-file projection. No conversion or
rollback path transforms either credential shape into the other.

## Security and public-boundary impact

This decision adds one deliberately isolated provider-acquisition trust boundary and one fixed Broker egress endpoint. It does not grant the login container project access, persistent home state, Docker authority, Broker access, or Workspace network policy authority. Documentation and tests contain only synthetic credentials and reconstructed contract examples; live provider artifacts remain prohibited repository content.
