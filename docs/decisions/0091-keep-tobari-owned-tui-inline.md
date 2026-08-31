# ADR 0091: Keep Tobari-owned TUI inline

- Status: Accepted
- Date: 2026-08-29
- Deciders: Tobari product owner and maintainers
- Scope: CLI, terminal presentation, interactive selectors, first use, review,
  accessibility, and harness
- Revises: The selector presentation mechanism described by ADR 0072 and the
  Configurator handoff presentation in ADR 0090
- Related: ADR 0073, ADR 0087, and ADR 0090
- Superseded by: None

## Context

Tobari's interactive selectors are intentionally styled, keyboard-driven, and
more legible than a sequence of plain prompts. Their implementation entered the
terminal's DEC private-mode alternate screen so redraws could avoid logical-row
cursor movement when long rows wrapped.

That mechanism has a product cost. The main-screen history disappears while a
selector is active and returns only after exit, so the user experiences a
screen takeover rather than a continuous CLI journey. This is particularly
misleading at the Configurator boundary: a Tobari selector disappears and the
selected agent's native sign-in UI appears, making container execution easy to
misread as a host agent launch. Removing TUI interaction would also remove a
well-liked part of Tobari's interface and is not required to solve the problem.

## Decision

Tobari-owned interactive terminal presentation is **inline by default and by
contract**. It keeps the existing styled TUI, raw-key navigation, semantic
colors, and line-input fallback, but it does not enter or exit DEC private mode
1049 and does not clear main-screen scrollback.

The shared selector renderer:

1. measures the current terminal and the frame's wrapped physical-row height,
   reserves that region, and only then saves its main-screen cursor origin, so
   a bottom-of-screen scroll cannot invalidate the saved origin;
2. redraws only after interaction state changes by restoring that origin,
   erasing below it, and reserving the newly measured physical region before
   saving the next origin;
3. retains the dimensions that authorized an origin; after resize it appends a
   complete frame and establishes a new origin without restoring the old one;
4. never locates an already-rendered frame with a logical-line cursor-up count;
5. degrades an unknown-size output or a frame taller than the viewport to
   complete append-only frames rather than guessing dimensions, restoring an
   invalid origin, or hiding content;
6. budgets every non-ASCII code point conservatively so ambiguous width,
   variation selectors, combining sequences, and emoji cannot under-reserve;
7. writes raw-mode line endings as CRLF, leaves the final selected or canceled
   frame in terminal history, and restores a visible column-zero cursor exactly
   once on every exit path; and
8. treats render, finish, and terminal-restore failures as command failures
   before any choice can authorize later mutation.

Interactive views remain bounded and own scrolling or paging inside that
inline frame. If raw mode is unavailable, the existing numbered line-input
fallback remains the compatibility path. A Tobari-owned view does not switch
to alternate-screen presentation merely because it is dense.

An attached shell, Codex, Claude Code, or another native child may independently
use its own terminal modes after explicit handoff. Tobari does not overlay,
parse, emulate, or attempt to restore an arbitrary live child's screen. It
reclaims presentation only after the child returns, then writes a normal
main-screen outcome or review boundary.

## Consequences

- The user retains one continuous scrollback narrative from selection through
  handoff, child return, review, and Apply.
- TUI quality and keyboard efficiency remain; “inline” does not mean “plain
  line-oriented prompts.”
- Shared renderer tests reject `ESC[?1049h` and `ESC[?1049l`; model a
  bottom-of-viewport wrapped frame, resize, unknown size, and redraw; and prove
  conservative Unicode width, one-owner column-zero cursor restoration, and
  fail-closed finish errors.
- Individual selector tests assert one continuous inline session rather than
  alternate-screen entry/exit.
- Long interactive content must use its existing bounded scrolling instead of
  relying on a full-screen takeover.

## Rejected alternatives

### Replace every TUI with numbered prompts

This preserves history but gives up navigation, hierarchy, and visual quality
that are independent from the alternate-screen mechanism.

### Keep alternate screen and add more prose

Correct prose still disappears at the most important ownership transition and
does not create a continuous CLI mental model.

### Parse or frame the selected agent's native TUI

That would create a terminal-emulation boundary, risk input/resize fidelity,
and conflict with ADR 0073. The selected child owns the terminal after handoff.
