# Long-01 attempt 01: environment-blocked before value signal

- Status: blocked attempt; not acceptance evidence for Long-01.
- Scenario: parent-defined Long-01 safe first success.
- Subject: clean disposable `cc-bash-guard` snapshot; no repository edits or
  feedback commit were made by the child.
- Interaction: child reported a real PTY session and exited it cleanly after
  cleanup. No raw capture or value checkpoint was returned, so no human-path
  success is claimed.

## Child report

The child stopped at the environment boundary rather than because of elapsed
time:

- `tobari cluster up` failed with `gateway_image_unavailable`.
- `tobari doctor` reported Docker Engine unavailable, Docker Compose v2
  unavailable, and policy tests unable to use the isolated XDG directory
  through the Docker VM.
- The PTY session exited cleanly after cleanup; no user-facing Workspace value
  signal was reached.

## Classification

This attempt is an orchestration/environment blocker. It does not establish a
Tobari product failure, a command-surface finding, or a scenario acceptance
result. The parent must provision a Docker-VM-shared isolated state boundary
and a known-good Docker context before restarting Long-01. The child must be
restarted from a fresh project and must not be given Tobari usage instructions
or this finding.

Discovery count, readable screen checkpoints, raw digest, and command-surface
feedback are **not assessed** because the run stopped before the first product
value signal.
