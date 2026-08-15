# ADR 0036: Use Codex's native trusted-host browser login

- Status: Accepted
- Date: 2026-08-15
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, harness, and public boundary
- Revises: ADR 0025 and ADR 0032
- Superseded by: None

> The Claude limitation described here is revised by ADR 0038. The Codex decision remains unchanged.

## Context

ADR 0025 selected Codex's explicit `login --device-auth` flow because it exposed a fixed page and a portable private file credential store. Runtime observation later showed that this makes `tobari auth login --provider openai` differ from ordinary local Codex use: direct `codex login` starts a localhost callback server, asks the host browser to complete authorization, and writes the same reviewed file-backed session state. OpenAI documents the browser flow as the default local path and device authentication as a beta alternative for remote or headless environments.

The trusted-host Codex driver already runs the canonical executable directly on the host, forces an owner-only temporary `HOME` and `CODEX_HOME`, forces file credential storage, bounds the process and visible output, validates the complete captured state, rechecks executable identity, performs checked cleanup, and commits only after every postcondition succeeds. It does not need a Tobari-owned listener or OAuth implementation to use the native local flow.

Claude Code does not have an equivalent portable full-login export. Its documented `setup-token` surface is a long-lived inference-only credential for scripts and CI, while normal login uses platform-dependent native credential storage. This decision therefore changes Codex acquisition only and keeps the Claude limitation explicit.

## Decision

The reviewed OpenAI host driver runs fixed `codex login` argv with the existing fixed configuration overrides:

```text
login
-c cli_auth_credentials_store="file"
-c check_for_update_on_startup=false
```

The verified Codex child owns the native browser-login lifecycle. It alone binds the loopback callback listener, creates and validates OAuth state and PKCE values, asks the operating system to open the browser, receives the callback and authorization code, exchanges that code, and writes the private `auth.json`. Tobari does not bind a callback port, proxy the callback, parse or reconstruct the authorization URL, receive an authorization code, or implement the OpenAI OAuth client.

The child continues to run in the sanitized trusted-host environment and owner-only temporary home. Tobari retains the existing ten-minute timeout, 64 KiB visible-output bound, control-safe projection, canonical executable and digest checks, strict schema-1 auth-state parser, checked cleanup, canonical Broker record, and previous-credential preservation on every failure.

Codex's dynamic authorization URL is untrusted visible text, not browser authority for Tobari. Tobari never passes that URL to its shared host-browser opener. Codex owns its own open attempt; if it cannot open a browser, its bounded visible guidance remains available for manual navigation. Port collision, listener setup failure, browser failure, callback failure, timeout, cancellation, executable drift, invalid state, or cleanup failure prevents credential commit through existing stable failures.

The Anthropic plan remains fixed `claude setup-token`. Public documentation names it as an inference-only automation credential and does not imply parity with normal Claude subscription login, refresh, Remote Control, or platform credential storage.

## Consequences

### Positive

- Local OpenAI login follows the ordinary Codex CLI experience.
- OAuth callback, state, PKCE, and code handling stay inside the reviewed provider CLI rather than expanding Tobari into an OAuth server.
- The Broker record and Workspace handle projection remain unchanged, so no persisted-state migration is required.
- A dynamic authorization URL cannot become a Tobari-selected executable or browser target.

### Negative

- The host loopback port required by Codex must be available for routine success.
- Native browser behavior becomes part of the reviewed Codex host-login contract and requires manual release replay when Codex changes it.
- OpenAI and Anthropic acquisition remain intentionally asymmetric because their stable external credential surfaces differ.

## Mechanical enforcement

- Host-driver tests fix exact `login` argv, sanitized environment, private file storage, bounded streams, strict captured state, digest recheck, cleanup, cancellation, and failure preservation.
- Visible-output tests use a synthetic native-login transcript and prove the dynamic OpenAI authorization URL remains visible while causing zero Tobari browser-open calls.
- Browser allowlist tests reject every OpenAI authorization URL; only existing fixed GitHub and reviewed AWS targets remain selectable by Tobari.
- Contract and documentation tests keep the native callback owner and Claude setup-token limitation aligned across product, architecture, security, authentication, external API, harness, and public-site surfaces.
- Manual trusted-host release validation confirms that a reviewed stable Codex CLI opens the browser, receives its own localhost callback, produces the strict private file state, and leaves no retained temporary home or transcript.

## Compatibility and migration

The public command, flags, fault codes, result schema, encrypted record, Gateway plan, and Workspace projection do not change. Existing configured credentials remain valid. An operator can roll back to the previous device flow without transforming persisted state.

## Security and public-boundary impact

The trusted host gains no Tobari listener or new network client. The provider CLI already owns the OpenAI authorization and token exchange; this decision changes which reviewed public login mode it runs. Tests and documentation use only synthetic URLs and state and never retain a real authorization URL, code challenge, state, authorization code, account identifier, token, or authenticated transcript.
