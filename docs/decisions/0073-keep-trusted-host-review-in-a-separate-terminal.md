# ADR 0073: Keep trusted-host review in a separate terminal

- Status: Accepted
- Date: 2026-08-21
- Deciders: Tobari maintainers
- Scope: Product, terminal ownership, architecture, security, policy review,
  and harness
- Revises: None
- Related: ADR 0046, ADR 0053, ADR 0055, ADR 0068, and ADR 0072
- Revised by: None
- Superseded by: None

## Context

The Permission Inbox is host-owned authority, but opening it from an attached
Workspace terminal would reduce the separate-terminal step in the denial loop.
The reviewed proposal reserved a sequential `Ctrl+]`, then `r` host prefix,
used repeated `Ctrl+]` for one literal byte, and temporarily transferred the
same terminal from the Workspace child to trusted-host review.

That proposal changes more than keyboard presentation. Today Docker owns the
attached child's terminal-facing streams, PTY, signals, and raw input. The
optional Unix relay forwards input and resize events and applies only bounded
display color to child output. A same-terminal review switch would instead
make Tobari a terminal multiplexer responsible for preserving arbitrary child
presentation while a different host UI is visible.

A bounded synthetic PTY experiment exercised the proposed boundary on macOS
and compiled the same Unix path for Linux. It established that:

- a byte-defined fragmented prefix, literal `0x1d`, and unknown pairs can be
  forwarded deterministically;
- Bash, a raw byte reader, exact nonzero child status, resize, ordinary
  signals, review cancellation, and a quiet child exit can retain their
  process semantics;
- a continuously writing child can stop on finite kernel PTY backpressure
  while review progresses independently, then resume draining after Back;
  one 4 MiB synthetic payload completed without byte loss; and
- these successes do not solve alternate-screen ownership.

When a child already owns DEC private mode 1049, opening another 1049 screen
does not create a portable stack. It replaces the same alternate buffer on
non-stacking terminals, while rendering review inline overwrites the child's
active buffer. A PTY transports bytes but retains no screen grid, cursor,
scrollback, or terminal modes from which the prior presentation can be
reconstructed. Replaying all prior output is unbounded and may repeat terminal
effects; bounded reconstruction requires interpreting terminal output.
Forcing `SIGWINCH` is also not a restoration contract because an arbitrary
child may ignore it or redraw only a delta.

The experiment therefore could not prove honest alternate-screen restoration
without a terminal emulator/parser or a weaker product promise.

## Decision

V1 preserves transparent child terminal ownership and the separate trusted-host
Permission Inbox:

- `tobari` reserves and intercepts no child input prefix, including `Ctrl+]`;
- the ordinary attached streams remain Docker-owned, and the optional Unix PTY
  relay remains display-only and forwards every input byte unchanged;
- a user keeps the Workspace and agent session running, opens a separate
  trusted-host terminal, runs `tobari policy review` or
  `tobari policy review --watch`, confirms any decision there, and deliberately
  retries the original request in the same Workspace;
- Tobari adds no terminal emulator, terminal-output parser, shortcut, command,
  configurable binding, dependency, policy path, or new trust boundary for
  same-terminal review; and
- terminal output, OSC sequences, Workspace processes, and the attachment
  browser channel cannot open or control Permission Inbox.

The measured prefix mechanism is not retained as dormant code or an
experimental switch. The accepted work packet is closed as a product-shaped
no-go result.

## Consequences

### Positive

- Arbitrary shells, raw readers, Vim-compatible `Ctrl+]` use, agent CLIs, and
  full-screen programs keep their existing input and terminal contracts.
- Tobari makes no screen-restoration claim it cannot enforce across supported
  terminals.
- Permission mutation remains visibly host-owned and reuses the existing
  bounded Inbox, typed staging, fresh Apply validation, and authoritative
  receipts without a second path.
- No dependency, persisted state, schema, command, or migration is added.

### Negative

- The ordinary denial loop still requires a separate trusted-host terminal.
- A user must move focus between the running Workspace and Permission Inbox.
- Future attachment-owned approval workflows cannot depend on an inline
  terminal switch and require their own reviewed host interaction design.

## Mechanical enforcement

- PTY relay tests pass a literal `0x1d` byte to a raw child unchanged and keep
  delayed input, resize, status, and restoration coverage.
- Root human help tests reject a reserved `Ctrl+]` or Trusted Host Review
  shortcut; the public catalog retains no same-terminal review command.
- Policy-review PTY tests continue to own their independent raw terminal,
  alternate-screen lifecycle, staging, cancellation, and confirmed Apply.
- The capability ledger classifies same-terminal trusted-host review as
  excluded and requires it to remain absent from the catalog.
- Product, architecture, security, harness, README, and architecture-site text
  continue to direct users to a separate trusted-host terminal.

## Reconsideration trigger

Reconsider only after an explicit product and trust-boundary decision makes
Tobari the owner of terminal emulation or multiplexing. That decision must
select and review the parser/emulator dependency and licensing, define bounded
screen/scrollback and backpressure semantics, preserve raw bytes and child
status, cover alternate-screen and hostile control sequences across supported
macOS and Linux terminals, and supply compatibility and security evidence for
shells, agent CLIs, raw readers, and full-screen applications. A desire for one
fewer terminal switch is not by itself sufficient.
