# Medium 01: Blind human Permission Inbox trial

## Scope

This is a blind new-user trial. The first attempt must use only the host
terminal and the CLI's own visible help or screens. Do not consult command
procedures, README guidance, the parent packet, source, or harness before the
first attempt. The trial is limited to synthetic/public-safe values and the
user's temporary Workspace resources.

## Pre-trial environment and expectation

- Repository: `/Users/tasuku/work/github.com/tasuku43/tobari`
- Date: 2026-08-03 (Asia/Tokyo)
- Initial branch: `main`
- Initial repository revision: `96160610bec0ab56e948f126594123f6dff4d447`
- Initial worktree: clean before this feedback file
- Interaction contract: host and Workspace actions through a real OS
  pseudo-TTY, 120x40, `TERM=xterm-256color`, raw ANSI capture, checkpoints,
  typed keys/timing, and exit status; no bulk pipe
- Time bounds: 90 seconds per stage, stop after 30 seconds without output,
  four minutes overall
- Expected value: from the host terminal alone, find a way to create or
  surface three synthetic pending outbound permission requests, inspect them
  as a human, intentionally allow exactly one, intentionally deny a different
  one, cancel the third without changing it, and understand the next action
  plus the allow/deny/cancel distinction from the screen
- Expected safety property: only the deliberately selected request changes;
  rejection and cancellation do not silently grant permission

## Initial checkpoint

Created before the blind trial. The post-trial observations will be appended
to this same file and committed separately as the scoped feedback change.

