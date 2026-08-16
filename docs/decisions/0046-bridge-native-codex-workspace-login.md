# ADR 0046: Bridge reviewed native Workspace logins to the host browser

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, runtime, and harness
- Revises: ADR 0036 for standard Workspace login
- Related: ADR 0044
- Superseded by: Device-flow readiness revised by ADR 0048

## Context

ADR 0044 made native authentication inside the persistent Workspace standard. Claude Code completes its supported browser/paste-code flow there, but pinned Codex 0.147.0 uses the local-machine browser flow by default: it binds a loopback listener, opens an OpenAI authorization URL whose redirect URI selects that port, and owns OAuth state, PKCE, exchange, and credential storage. Pinned GitHub CLI 2.96.0 may use the same local-machine shape when its device flow is unavailable: its web-application fallback binds a random Workspace-loopback port, prints one GitHub.com authorization URL whose redirect selects that port, and owns state validation, token exchange, and credential storage.

Inside a Workspace, those listeners belong to the container loopback namespace while the browser's loopback belongs to the host. A direct runtime probe proved that Docker port publication does not reach a service bound only to container loopback. Permanent publication also conflicts across Workspaces, and host networking would violate the resident Workspace boundary. Forcing a different login mode changes the ordinary native local experience.

The host `tobari` process already remains attached for the complete interactive Workspace session and carries the child terminal streams. A live replay exposed an important distinction: replacing Docker stdout with a generic `io.Writer` made `os/exec` insert a pipe, Docker no longer detected a terminal, and Codex stopped after terminal capability queries. A controlled intermediate-PTY replay restored its native UI. The bridge must therefore preserve terminal identity and resize semantics as well as output bytes. This gives the selected Workspace a bounded session owner without adding a resident service or client wrapper.

## Decision

During one interactive `tobari` session, the Docker runtime attaches Docker stdout to a raw session-scoped intermediate PTY, inherits the caller terminal size, propagates resize signals, and relays the PTY master byte-for-byte to the original stream and bounded observer. Docker retains the original stdin and stderr terminal files, owns raw input mode and signal forwarding, and continues to allocate the container PTY. The observer accepts only the closed reviewed authorization-contract union: Codex may originate on a bounded direct line or one complete bounded ANSI synchronized-update frame, while GitHub CLI may originate in its complete bounded no-newline browser prompt. Frame recognition removes terminal controls without interpreting cursor positions, wording, or layout. The GitHub prompt trailer supplies only a complete-stream boundary; the recovered URL still passes its independent semantic validator. Surrounding account prompts, project output, ordering, terminal width, and cursor positions are not browser authority. It opens no browser and binds no listener for partial, oversized, duplicate, ambiguous, malformed-control, or invalid output.

Tobari accepts only these provider contracts:

- Codex: strict `https://auth.openai.com/oauth/authorize`, the reviewed client ID, a nonempty `openid`-containing subset of its reviewed scope ceiling, PKCE method, bounded base64url challenge/state, and an HTTP `localhost` or `127.0.0.1` callback on `/auth/callback`. Reviewed auxiliary fields may be omitted but retain their exact values when present. The optional originator is exactly direct CLI `codex_cli_rs` or interactive TUI `codex-tui`.
- GitHub CLI: strict `https://github.com/login/oauth/authorize`, GitHub CLI's fixed OAuth client `178c6fc778ccc68e1d6a`, required `repo`, `read:org`, and `gist` scopes with optional `workflow` only, an exact 20-character lowercase hexadecimal state, and an HTTP `127.0.0.1` callback on `/callback`. GitHub Enterprise hosts, SSH-key-upload scope, caller-added scopes, and unknown query fields are outside this contract.

Scope order and permitted reduction, bounded dynamic state, callback port, and presentation framing are syntax rather than authority. Each callback port is derived once from the validated URL and must be non-privileged. Tobari then:

1. verifies that the selected container still has the exact Tobari owner, project ID, and work role;
2. binds host `127.0.0.1` on that validated callback port for the login only;
3. opens the exact validated URL with the existing fixed host browser adapter;
4. accepts at most one host-loopback connection;
5. relays its opaque bytes through a fixed Docker exec program plus the validated port to the same port on `127.0.0.1` in the selected Workspace; and
6. closes the listener after the connection, browser-open failure, or interactive-session end.

The originating CLI remains the OAuth implementation and input owner. Tobari does not construct the authorization URL, inject or consume GitHub CLI's Enter input, inspect callback HTTP bytes, compare state, implement PKCE, exchange the code, read or write credentials, or log/persist the URL, callback, code, state, or token. Callback bytes exist only in the host relay's bounded in-memory stream. ADR 0048's later exact-default-argv device compatibility path disables the pinned client's prompt before this callback branch, so it requires no Enter injection and does not revise the relay's input ownership.

The bridge is not a generic browser or port-forwarding service. It is available only while the owning `tobari` process is attached, only for the reviewed Codex and GitHub CLI OAuth authority contracts, only on a URL-selected non-privileged host-loopback port, and only to the exact selected owned Workspace. Each client ID and scope ceiling remains reviewed authority: changing a client or expanding a ceiling requires an explicit compatibility/security decision. ADR 0048 separately adds GitHub CLI's exact host-open-only device target and its finite Workspace authentication effects without callback authority.

## Consequences

### Positive

- A user signs in from ordinary `codex`, `codex login`, or the reviewed GitHub.com HTTPS `gh auth login` flow inside the Workspace with no manual URL or callback transfer.
- Ordinary Codex full-screen rendering, terminal negotiation, and resize behavior remain native while the bridge is enabled.
- Codex continues to own OAuth semantics and native credential state.
- No resident Workspace receives host networking, a published port, capability, Docker socket, host credential, or general host opener.
- Multiple detached Workspaces reserve no callback port; only an active login may hold its validated port.

### Negative

- The trusted host process transports one opaque authorization callback and validates the dynamic authorization URL, expanding the standard runtime boundary beyond pure terminal attachment.
- Concurrent reviewed browser logins that choose the same host port contend. The second attempt fails closed and retains the child's visible recovery.
- Client prose, callback-port, permitted scope ordering/reduction, and reviewed auxiliary-field omission do not require a Tobari change, but OAuth authority changes such as client identity, scope expansion, authorization origin/path, callback host/path, or changed flow-flag values remain deliberate review events.
- Native callback login outside a supported interactive `tobari` session is not provided.

## Mechanical enforcement

- URL tests fix each provider's authorization host/path, client, scope ceiling, callback host/path/port bound, state, query-key allowlist, and hostile-host constraint; Codex additionally fixes PKCE and reviewed originators.
- Fragmented-stream tests prove byte-for-byte child output, Codex direct-line and ANSI synchronized-frame recognition, GitHub CLI no-newline prompt recognition, bounded buffering and controls, ambiguity/replay rejection, and one trigger per unique authorization URL.
- A subprocess PTY regression proves the observed child sees terminal stdout, exact relayed bytes, the inherited initial size, and a propagated resize.
- Relay tests prove exact owned-container verification, URL-derived host and Workspace port equality, one listener, one browser target, fixed Docker exec program plus validated port, opaque bidirectional bytes, and listener cleanup.
- Runtime tests prove the bridge lifetime is nested inside interactive entry and changes neither child argv nor exit status.
- Security and agent-readiness checks keep callback transport separate from ADR 0048's host-open-only device flow and require a manual live pinned-client pass/fail replay without retaining an authenticated transcript.

## Compatibility and migration

No public command, flag, Context schema, Workspace state, credential file, policy effect, or Gateway contract changes. Existing Workspaces acquire the behavior on their next interactive entry. No persisted migration exists.

## Security and public-boundary impact

The host browser opens one strict authorization shape from a closed reviewed provider union derived from untrusted output. The callback relay handles authorization material in memory but never interprets or persists it. Tests use synthetic URLs and callback bytes only. Live evidence records pinned client versions and pass/fail, never URLs, challenges, state, codes, tokens, account identifiers, or authenticated transcripts.
