# Medium-02 blind rerun: lifecycle recovery and cleanup

- Status: functional E2E complete; no child-returned raw artifact.
- Date: 2026-08-04
- Subject: fresh disposable copy of `cc-bash-guard` with isolated Tobari state.
- Delegation: the child received only the desired outcome, a disposable
  boundary, a real pseudo-TTY requirement, and progress/cleanup expectations.
  It did not receive the README, thesis, `AGENTS.md`, work packets, source,
  harness, scenario route, or command names before its first attempt. It made
  no repository change and created no commit.

## Desired outcome

Reach an isolated Workspace for the project, verify that the project path is
isolated, leave and reuse the same Workspace, test a nested location without
creating an unnecessary second environment, cancel safely, and clean up all
project/shared state.

## Observed journey

1. The child attempted the primary project entry before preparation. The
   visible result identified that the shared environment was not configured.
2. From the visible recovery surface it discovered the shared-environment
   preparation action, completed it, and entered an isolated Workspace.
3. `pwd` showed the mirrored project path inside the environment.
4. The child exited and re-entered from the same project, confirming reuse
   rather than creating a second environment.
5. From a nested project location it observed the existing nearest-ancestor
   Workspace, returned to it, and did not create a new environment.
6. The child removed the project environment and verified that the shared
   environment was also no longer configured.

## Discovery and evidence

- Discovery: the child recovered from the first visible bootstrap failure and
  found the cleanup path through the visible product surface. It reported two
  recovery/cleanup discoveries; no source inspection, JSON route, direct
  Docker/OPA operation, or non-interactive replay was used.
- Terminal: real OS pseudo-TTY, 80x24, `TERM=dumb`.
- Child artifact: no raw-byte digest, timestamped key log, or external bundle
  path was returned. This is an evidence limitation, not a functional failure.
- Parent supporting capture: the same validated integration boundary was
  captured separately by the parent-owned PTY harness at
  `/private/tmp/tobari-pty-integration-20260804/capture`. The external bundle
  recorded exit `0`, 120x40, `TERM=xterm-256color`, elapsed `74354 ms`,
  checkpoints `spawn` and `exit`, raw SHA-256
  `db2b7f75ae62ab98e3ef56853267ec1dcc00ffaaaaa89bc07a86223c7ead257b`, and
  redacted SHA-256
  `0dc2b769ab9092f848b638348d1de6d39f49f596930b2f312e8a4c1c1fbf8d72`.
  This parent artifact verifies the capture boundary but is not presented as
  a child-returned artifact.

## Feedback

- **Keep:** visible bootstrap recovery, same-root reuse, nearest-ancestor
  selection, safe cancellation, and separate project/shared cleanup. These
  surfaces supported the desired outcome without teaching the child a route.
- **Clarify:** the terminal's initial size/`TERM` can vary across real human
  sessions; the user-facing lifecycle should remain understandable without
  assuming one terminal profile.
- **Narrow/deprecate:** none evidenced by this run. No command was redundant
  for the requested lifecycle outcome.
- **Environment noise:** the child reported a shell-startup warning related to
  local Kubernetes prompt state; it did not affect the Tobari path.

## Acceptance boundary

The blind child independently completed the declared lifecycle outcome and
left no repository changes. The child did not return the stronger raw-evidence
bundle required by the original four-scenario protocol; the parent-owned
capture and harness contract test now prove that the bundle format works for a
real integration run. Future blind runs should keep the same outcome-only
prompt while the parent collects the external artifact at the orchestration
boundary.
