# ADR 0068: Color structured Workspace output without reformatting

- Status: Accepted
- Date: 2026-08-20
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, terminal presentation, runtime, and harness
- Revises: None
- Related: ADR 0055 and ADR 0073
- Revised by: None
- Superseded by: None

## Context

An attached Workspace shell currently forwards Docker's interactive streams
unchanged. JSON responses from commands such as `curl` are therefore readable
but visually undifferentiated. Pretty-printing or shell aliases would change
command output, pipelines, and line behavior, and would create a hidden
command adapter.

The child PTY is also part of the interactive contract. A presentation writer
wrapped around Docker stdout would make Docker observe a pipe, which can lose
window-size and resize behavior. Structured output remains untrusted child
text and must not become a policy, command, credential, or recovery authority.

## Decision

On reviewed Unix hosts, an interactive Workspace entry may use a host-side PTY
relay. Docker's attached child still owns the terminal-facing shell; the relay
forwards input and window-size changes and reads the PTY master for display
only. Stdout is passed through a bounded streaming projection that recognizes
complete JSON objects/arrays and conservative YAML mappings/sequences.

The projection adds fixed SGR token wrappers only. It preserves original bytes,
whitespace, ordering, and visible content; it never pretty-prints, reindents,
sorts, or changes command input. Incomplete, invalid, oversized, ambiguous,
control-bearing, already-styled, tagged, anchored, or aliased candidates are
passed through unchanged. Shell prompts and other non-candidate partial lines
remain responsive. Stderr uses its original host stream and is not colored.

The feature is enabled only when the caller's input and stdout are terminals,
the presence-only `NO_COLOR` preference is absent from both host and declared
child environment, and the platform has the reviewed PTY implementation.
Machine-readable Tobari output, redirected output, and all non-interactive
paths retain their existing byte-clean behavior. Unsupported platforms use
direct pass-through.

## Consequences

- Interactive JSON and common multi-line YAML are easier to scan without a
  formatting or schema contract change.
- A small infrastructure dependency and Unix PTY lifecycle are now part of the
  attached-session path.
- A candidate may be held briefly until it is complete or disproven, but the
  buffer has a fixed bound and flushes on child completion or cancellation.
- The conservative YAML detector intentionally leaves ambiguous flat prose
  and colon-delimited logs uncolored.

## Mechanical enforcement

- Terminal-style unit tests prove token insertion, visible-byte preservation,
  fragmented writes, prompt responsiveness, bounded fallback, hostile output,
  and destination failure behavior.
- PTY tests prove stdout-only color, stderr separation, input forwarding after
  idle periods, distinct polling and streaming raw-read modes with restoration,
  initial window-size propagation, `NO_COLOR`, non-TTY bypass, and child exit
  handling.
- Product, architecture, security, and harness contracts keep machine,
  redirected, and non-interactive output free of new ANSI.
- `task check` and `task security` remain the completion gates; the PTY module
  is used only in infrastructure and its MIT license is reviewed.

## Compatibility and migration

This is a display-only change to the pre-public interactive Workspace path.
There is no persisted state, catalog input, output schema, reference flow,
provider API, or migration. Removing the relay and returning to direct Docker
streams is a compatible rollback for machine and redirected callers.

## Security and public-boundary impact

The relay never parses input or turns child output into authority. Actual
terminal controls are never copied into generated SGR; escaped controls remain
ordinary value text. The bounded projection rejects YAML tags, anchors, and
aliases and does not execute or interpret external text. No credential,
network, provider, or persistent Workspace boundary is added.
