# Work Goal: Switch from an attached Workspace to trusted-host review

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, and the accepted terminal-ownership ADR required by this packet
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Pre-public self-use
- Related ADRs: ADR 0055 and a new terminal-ownership ADR required before mechanism implementation

## Outcome

During an interactive `tobari` attachment, a user can press `Ctrl+]`, release
it, and then press `r` to enter an unmistakable trusted-host review surface in
the same terminal. The first slice exposes the existing Permission Inbox and
its unchanged typed staging and confirmed Apply behavior. Returning restores
the Workspace child session, terminal state, window size, signal behavior, and
child exit status without leaking host-review keystrokes into the Workspace.

## Why now

Permission review currently requires a separate trusted-host terminal even
though the user normally wants to remain in the active agent session. The same
host-review transition will later be needed to approve attachment-scoped
Workspace service-exposure requests. Establishing and testing one honest
terminal ownership model before adding that second authority avoids two
different review experiences and avoids coupling a new relay to an unproven
terminal mechanism.

## Non-goals

- Do not add Workspace service exposure, a service-request queue, a host port
  listener, or a Workspace-side exposure shim in this packet.
- Do not add a general Workspace-to-host RPC bus, plugin surface, command
  executor, browser authority, clipboard feature, or arbitrary host control.
- Do not add a second policy engine, mutation path, candidate identity, or
  remembered policy format. The existing Permission Inbox and canonical
  reviewed-set Apply remain authoritative.
- Do not let Workspace output, a Workspace process, an OSC sequence, or a
  service request force the terminal into host review.
- Do not make the shortcut available for redirected input/output or non-TTY
  direct commands.
- Do not add a configurable keybinding before repeated self-use demonstrates
  that the fixed prefix is unsuitable.
- Do not claim exact screen restoration for a child interaction mode that the
  implementation and PTY evidence cannot preserve.

## Acceptance criteria

- [ ] Human help and attachment guidance describe the operation as “press
      `Ctrl+]`, then `r`”; they do not imply `Ctrl+R` or a simultaneous
      three-key chord.
- [ ] In normal attachment mode, every input byte is forwarded unchanged
      except the finite host prefix grammar: `Ctrl+]` then `r` opens review,
      `Ctrl+]` then `Ctrl+]` forwards one literal `Ctrl+]`, `Ctrl+]` then `?`
      shows host shortcut help, and any other second byte forwards both bytes
      unchanged.
- [ ] Only host keyboard input can activate trusted-host review. Workspace
      stdout/stderr, control sequences, the browser channel, and Workspace
      processes cannot activate it or confirm a decision.
- [ ] The host-review surface states that Workspace input is paused and that
      keystrokes remain on the trusted host. Review keys never reach the child.
- [ ] The first review source is the existing installation-wide Permission
      Inbox. It preserves exact/template staging, one-Context Apply, opaque
      candidate IDs, fresh revalidation, authoritative receipts, and no
      automatic retry.
- [ ] Back or cancellation performs no mutation and restores the child
      session. A child exit, host-review failure, resize, signal, or attachment
      cancellation has one specified outcome and cannot turn an unknown write
      into permission to retry.
- [ ] Synthetic PTY evidence covers Bash, a raw byte reader, a continuously
      writing child, a synthetic alternate-screen TUI, window resize, ordinary
      signals, literal-prefix delivery, host-review cancellation, child exit
      during review, and exact nonzero child status propagation.
- [ ] A bounded runtime experiment proves that hidden child output cannot
      corrupt the trusted-host surface, disappear, deadlock the child, or
      produce a false claim of restored presentation. If this cannot be proved
      without a terminal-emulator dependency or a broader input/output parser,
      implementation stops and the trade-off returns for design review.
- [ ] The public catalog/help, capability ledger, README, theses, architecture,
      security model, ADR, and harness agree on the one reserved prefix and the
      changed terminal-ownership contract.
- [ ] Routine entry into review requires no second terminal, output parser,
      opaque-reference transcription, or undeclared external processing step.
- [ ] Focused tests, relevant Docker/runtime integration, `task check`,
      `task security`, and `task public:check` pass.

## Governing documents

- Thesis: North Star; Theses 0, 5, 7, and 8
- Product contract section: Primary operating loop, root `tobari` attachment,
  Permission Inbox, and output/exit contract
- Architecture or security invariant: Four-layer dependency direction, direct
  Docker terminal ownership, browser-channel separation, trusted-host-only
  mutation, and external-text non-authority
- Existing ADR: ADR 0055, which deliberately removed child-input interception
  from the browser bridge

## Completion definition

The work is complete when the prefix and trusted-host transition have
product-shaped PTY and security evidence, the existing Permission Inbox works
through the new surface without semantic duplication, the terminal ownership
trade-off is accepted in a durable ADR and propagated through governing
contracts, all required gates pass, and this temporary packet is removed.
