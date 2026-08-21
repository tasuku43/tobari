# Work Context: Switch from an attached Workspace to trusted-host review

## Current behavior

- `internal/infra/dockerruntime/project_runtime.go` starts the interactive
  Workspace child with `docker exec -i -t`; ordinary attachment streams and
  the container PTY are Docker-owned.
- `docs/00_theses.md`, `docs/01_product_contract.md`,
  `docs/02_architecture.md`, and `docs/03_security_model.md` explicitly state
  that Tobari does not consume child input. The optional existing PTY relay
  forwards input and resize events but is presentation-only and does not parse
  input or become authority.
- `policy review --watch` already owns a raw-terminal Permission Inbox with
  bounded refresh, stable-ID staging, one confirmed reviewed-set Apply,
  alternate-screen restoration, and fixed OSC 9 or BEL attention cues.
- A learnable denial currently directs the user to `tobari policy review` in a
  separate trusted-host terminal. The Workspace-visible navigation is advisory
  and carries no candidate ID or mutation authority.
- The native browser bridge already uses a binary-owned read-only opener, an
  unpredictable attachment-local Unix socket, and a separate non-TTY Docker
  exec agent. Its narrow schema transports a browser request, while the host
  independently validates the closed authorization target union. It neither
  observes terminal output nor consumes terminal input.
- The accepted follow-on product direction is to use a separate, similarly
  bounded attachment channel for Workspace service-exposure requests. That
  capability needs the trusted-host review surface defined here but is not
  implemented by this packet.

## Relevant structure

- Entry point: root catalog command `tobari [--context NAME] [-- COMMAND...]`
- Domain rule: typed policy candidates, reviewed decision sets, and Workspace
  session requests under `internal/domain/tobari`
- Application use case: Workspace entry and existing policy-review reads and
  reviewed-set Apply under `internal/app/tobaricmd`
- Infrastructure boundary: Docker interactive runner, host PTY relay, terminal
  mode/resize handling, and browser channel under `internal/infra`
- CLI catalog or presentation: root command composition, Permission Inbox
  selector, terminal notifier, and agent/human help under `internal/cli`
- Existing tests and harness checks: attached-session PTY tests,
  `policy_review_selector` tests, resumable Permission Inbox fixtures,
  browser-channel protocol tests, and the terminal/security profiles described
  in `docs/04_harness.md`

## Constraints

- The selected shortcut deliberately revises the current “Tobari never
  consumes child input” contract. This cannot be implemented as an undocumented
  exception; a durable ADR and thesis/product/architecture/security propagation
  are prerequisites.
- The trusted host must remain the sole policy writer. Terminal appearance,
  Workspace prose, and Workspace-originated control bytes are never authority.
- Policy review semantics remain unchanged: staging grants nothing, Apply is
  explicit, candidate IDs remain opaque and unchanged, and the denied request
  is never automatically retried.
- The prefix state machine must be byte-defined and deterministic across split
  reads. It must not infer user intent from timing, labels, or child output.
- The public human notation must avoid the ambiguous compact form
  `Ctrl-] r`; use “press `Ctrl+]`, then `r`.”
- `Ctrl+]` is already meaningful to programs such as Vim/Neovim, so one
  discoverable literal-prefix escape is mandatory.
- A terminal switch can hide neither unbounded child output nor terminal state
  uncertainty. Bounded buffering that can overflow silently is not acceptable.
- The common transport/lifecycle work may be reusable by a later typed service
  request, but this packet must not introduce a generic operation discriminator
  or arbitrary host adapter.
- The repository documentation locale is English. Examples and fixtures must
  use synthetic roots and data.

## External facts

- OpenBSD `telnet(1)`, checked 2026-08-21:
  <https://man.openbsd.org/man1/telnet.1>. Telnet uses `^]` (`Ctrl+]`) as its
  initial local command-mode escape character. This is the closest established
  semantic precedent for returning control from an attached environment to the
  local owner.
- tmux Getting Started, checked 2026-08-21:
  <https://github.com/tmux/tmux/wiki/Getting-Started#the-prefix-key>. tmux uses
  a control prefix followed by a separately pressed command key, and documents
  the space-separated notation as sequential rather than simultaneous.
- tmux FAQ, checked 2026-08-21:
  <https://github.com/tmux/tmux/wiki/FAQ#why-is-c-b-the-prefix-key-how-do-i-change-it>.
  Repeating the prefix is the established way to send the literal prefix to the
  attached program.
- OpenSSH `ssh(1)`, checked 2026-08-21:
  <https://man.openbsd.org/ssh.1#ESCAPE_CHARACTERS>. Its local `~` escapes must
  follow a newline, which makes that form unsuitable for an always-available
  switch from a raw full-screen child.
- Docker `container attach`, checked 2026-08-21:
  <https://docs.docker.com/reference/cli/docker/container/attach/>. Docker uses
  the sequential `Ctrl+P`, `Ctrl+Q` detach sequence and permits an override;
  Tobari must observe the actual Docker exec interaction rather than assume its
  own reader is the only prefix consumer.
- Neovim user manual, checked 2026-08-21:
  <https://neovim.io/doc/user/usr_29/>. `Ctrl+]` jumps to the tag under the
  cursor, so literal delivery cannot be omitted or treated as an edge case.

## Unknowns

- [ ] Can the existing host PTY relay own one deterministic input prefix for
      every interactive attachment without changing child TTY identity or
      introducing idle latency?
- [ ] How can child output be isolated while host review is visible and then
      restored for normal, raw, and alternate-screen children without silent
      loss, unbounded memory, deadlock, or an undeclared terminal emulator?
- [ ] Does nested alternate-screen behavior remain portable across the
      supported macOS and Linux terminals, or must the presentation use a
      different host-owned surface?
- [ ] What exact result occurs when the child exits or the attachment owner
      closes while trusted-host review is active?
- [ ] Does Docker's current exec detach-key handling need an explicit setting
      to keep the Tobari prefix grammar deterministic?
- [ ] Which current terminal identities and JIS/US keyboard layouts emit the
      expected `0x1d` byte for the documented `Ctrl+]` operation?

## Thesis evidence

- Repeated design decision or point of agent confusion: Permission review and
  future service-exposure approval both need trusted-host authority without
  moving the user to an unrelated terminal.
- User outcome or friction observed in the minimal slice: the current safe
  policy loop preserves the Workspace session but still requires terminal
  switching and remembering a second command.
- Code workaround or exception being considered: intercepting a key locally
  while leaving governing documents claiming that no child input is consumed.
- Current thesis that resolves it, or proposed thesis revision: revise the
  terminal-transparency thesis narrowly so one documented, literal-escapable
  host prefix is owned by the attachment; every other input remains child-owned.
- Downstream impact: root command catalog/help, Workspace entry application
  composition, interactive infrastructure, policy review presentation,
  browser-channel non-authority language, PTY harness, capability ledger,
  README, architecture site, security table, and agent-readiness scenario.

## Reproduction or observation

```sh
go run ./cmd/tobari help tobari
go run ./cmd/tobari help policy review
go test ./internal/cli -run 'PolicyReview|PTY'
go test ./internal/infra/dockerruntime -run 'WorkspaceBrowser|Interactive|PTY'
```

Before implementation, add a synthetic child that emits ordinary lines, raw
bytes, alternate-screen transitions, and resize-sensitive frames while the
host transition is entered and left. Record exact bytes, terminal modes, child
status, buffer bounds, and supported platform facts without recording user
terminal content.

## Security and public-boundary notes

- Assets and side effects involved: host terminal input ownership, PTY and
  Docker exec streams, existing bounded policy reads, and the existing
  reviewed-set policy mutation.
- Credentials or confidential data involved: none. Policy evidence remains
  secret-free under its existing typed projection.
- New dependencies, destinations, files, processes, or generated content: no
  dependency is accepted by this packet. A terminal-emulator/parser dependency
  requires separate license, architecture, and security review.
- Output delivery, collection coverage, timeout, retry, idempotency, and
  cancellation facts: the review read retains the existing bounded complete
  snapshot semantics; policy Apply retains its existing one-attempt confirmed
  mutation semantics; the host switch is interactive-only and has no JSON
  output or automatic retry.
- Publication and licensing concerns: only primary public manuals are cited;
  no source or UI is copied.

## Glossary

- **Host prefix:** the exact two-step keyboard grammar beginning with the
  `Ctrl+]` byte and interpreted by the trusted attachment process.
- **Trusted Host Review:** the host-owned terminal state in which Workspace
  input is paused and typed host review workflows may run.
- **Literal prefix:** one `Ctrl+]` byte delivered to the Workspace by pressing
  `Ctrl+]` twice.
- **Review source:** a closed typed host-owned task shown in Trusted Host
  Review. The first and only source in this packet is Permission Inbox.
