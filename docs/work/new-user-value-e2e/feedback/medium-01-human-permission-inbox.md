# Medium 01: Blind human Permission Inbox trial

## Scope

This is a blind new-user trial. The first attempt must use only the host
terminal and the CLI's own visible help or screens. Do not consult command
procedures, README guidance, the parent packet, source, or harness before the
first attempt. The trial is limited to synthetic/public-safe values and the
user's temporary Workspace resources.

## Pre-trial environment and expectation

- Repository: [redacted; pilot ran against the current checkout]
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

## Pilot status update

- Status: incomplete pilot; not official acceptance evidence.
- The child reached a healthy synthetic Workspace, issued several synthetic
  requests, exited the Workspace, and observed a visible pending-permission
  summary with a host review cue.
- The child did not reach allow/deny/cancel screen actions. No policy mutation
  was reported. The Workspace and cluster remained at the time of the status
  report; the parent subsequently removed the exact scenario-owned state with
  supported CLI cleanup.
- Raw PTY evidence was retained outside Git with a redacted digest prefix
  `cf4cbf8c…9377aa2`. The full capture is not committed.
- The child reported that it had inspected CLI help and read-only queue views;
  this pilot therefore cannot be treated as a pure no-document beginner run.
- The parent received the status update before deciding the next action. This
  timing/communication behavior is retained as orchestration evidence, not as
  a product success claim.
