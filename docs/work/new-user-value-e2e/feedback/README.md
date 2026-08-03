# Scenario feedback intake

The parent agent writes one redacted feedback file per completed or blocked
scenario here after receiving the child agent's report. Official child agents
must not read or change repository documentation, scenario definitions,
production files, or other packet files. The parent owns feedback intake and
commits.

Historical protocol-v1 pilot files:

- `long-01-safe-first-success.md`
- `long-02-least-privilege-multi-project.md`
- `medium-01-human-permission-inbox.md`
- `medium-02-workspace-bootstrap-reuse.md`

Official re-run files:

- `official/long-01-safe-first-success.md`
- `official/long-02-least-privilege-multi-project.md`
- `official/medium-01-human-permission-inbox.md`
- `official/medium-02-workspace-bootstrap-reuse.md`

Each file records the raw-PTY capture digest/location outside Git, readable
screen checkpoints, exact typed keys and timings, value signal, blockers,
discovery count, cleanup, and command-surface candidates. A missing or blocked
run is still recorded; it is never silently omitted or counted as success.

Official child inputs contain only the desired outcome and safe sandbox
boundary. They do not receive the paths above or the parent-authored route.
