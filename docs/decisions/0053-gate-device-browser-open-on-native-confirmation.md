# ADR 0053: Gate manual-code browser handoff on native confirmation

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, runtime, and harness
- Revises: ADR 0048 and ADR 0050
- Related: ADR 0046
- Revised by: ADR 0055
- Superseded by: ADR 0055

## Context

GitHub CLI and TWG device login display a short provider-owned code in the
Workspace before navigating to a host browser. Opening the browser as soon as
the activation URL appears can move focus away from the terminal before the
user copies that code. GitHub CLI already has an Enter handoff and TWG already
has an `Open in browser?` confirmation, but Tobari's earlier bridge opened from
the pre-confirmation URL output.

Claude, Codex, and GitHub's loopback fallback have different transfer
directions. Their reviewed URLs carry the callback or browser-side state needed
to continue and do not require a Workspace-displayed code to be copied before
navigation. Delaying those flows would add ceremony without preserving any
user transfer.

A Tobari-owned `c` shortcut would require the observer to consume or multiplex
child input. That would violate the current raw-input contract in which Docker
and the child retain terminal input ownership while Tobari observes only an
output copy.

## Decision

The native-login bridge distinguishes manual-code handoffs from immediate URL
handoffs. Claude, Codex, and GitHub's callback-bearing fallback retain their
reviewed immediate open behavior. GitHub CLI's device target and TWG stage a
validated activation target only in bounded session memory and release it after
a provider-owned confirmation transition.

Exact default `gh auth login` restores GitHub CLI's native prompt and sets
`GH_BROWSER` to one fixed image-owned marker executable. GitHub CLI invokes
that adapter only after Enter. The marker performs no browser, network, shell
evaluation, or clipboard action; it emits one exact post-confirmation line
containing GitHub CLI's target. A leading newline prevents concatenation with
the client's no-newline prompt. The host observer independently validates the
URL, verifies the selected Workspace, and then opens it. The marker is framing,
not URL authority.

For TWG CLI 1.2.5, the observer stages only the strict validated `Visit:` URL.
It opens that target only after the exact fallback line emitted after TWG's
native browser confirmation. A missing, reordered, duplicated, ambiguous, or
changed target or confirmation opens nothing. Staged values are cleared after
confirmation, ambiguity, or session end and are never logged or persisted.

Tobari does not intercept Enter, `c`, or any other child input and does not copy
a provider code to the host clipboard. Users may select and copy the visible
provider code before accepting the client's native confirmation. A future
shortcut requires a separate input-multiplexing decision rather than a
provider-specific exception.

## Consequences

- GitHub device login and TWG keep terminal focus until the user accepts their
  existing native browser prompt.
- Claude, Codex, and GitHub's callback-bearing fallback retain immediate host
  navigation.
- Exact default `gh auth login` once again requires its native Enter, while
  every other GitHub CLI argument remains pass-through.
- The bridge temporarily retains one already validated activation URL in
  session memory, but no code, credential, callback, or durable state.
- Provider output or marker drift fails closed and leaves native fallback text
  visible.

## Mechanical enforcement

- Runtime source/snapshot checks fix the GitHub wrapper environment, marker
  installation, one-argument contract, leading newline, fixed output framing,
  pass-through, and absence of network, browser, URL, or shell-evaluation
  authority.
- Fragmented GitHub tests prove the device pre-confirmation prompt has zero
  browser and listener effects, the exact marker opens once, and malformed,
  neighboring, ambiguous, or replayed markers open nothing.
- Fragmented TWG tests prove `Visit:` alone has zero browser effects, the exact
  post-confirmation line opens once, and missing, reordered, ambiguous, or
  replayed transitions open nothing.
- Existing Claude, Codex, GitHub callback, PTY-byte, resize, ownership,
  opener-failure, and session-cleanup tests remain required.

## Compatibility and migration

No public command, flag, policy rule, Gateway protocol, Context snapshot, or
Workspace credential format changes. The canonical runtime image changes, so
`tobari cluster up` rebuilds and activates the new GitHub wrapper assets. TWG
uses the installed Tobari observer and its compatible custom runtime without a
Context recreation.

## Security and public-boundary impact

The strict host URL union and Workspace network grants do not grow. GitHub's
fixed marker adds only post-confirmation output framing and no opener of its
own. TWG adds bounded session-memory staging of a synthetic-shape activation
URL. Repository fixtures contain no live code, token, account, callback, or
authenticated transcript.
