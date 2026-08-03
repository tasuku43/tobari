# Scenario feedback intake

The parent agent writes one redacted feedback file per completed or blocked
scenario here after receiving the child agent's report. Child agents must not
silently change the parent scenario definitions or production files.

Expected files:

- `long-01-safe-first-success.md`
- `long-02-least-privilege-multi-project.md`
- `medium-01-human-permission-inbox.md`
- `medium-02-workspace-bootstrap-reuse.md`

Each file records the raw-PTY capture digest/location outside Git, readable
screen checkpoints, exact typed keys and timings, value signal, blockers,
discovery count, cleanup, and command-surface candidates. A missing or blocked
run is still recorded; it is never silently omitted or counted as success.
