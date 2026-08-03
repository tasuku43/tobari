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

Future parent captures use `python3 scripts/pty-evidence.py` with an events
schedule and an output directory outside the repository. The bundle's
`metadata.json` is the parent-owned intake boundary: it carries terminal
dimensions, `TERM`, typed-input timing, checkpoint offsets/tails, exit status,
and raw/redacted digests. `transcript.raw` stays outside Git; only a reviewed
redacted projection may be copied into feedback. The four official runs above
predate this helper, so their missing raw digests remain recorded rather than
being backfilled or treated as newly captured evidence.

Official child inputs contain only the desired outcome and safe sandbox
boundary. They do not receive the paths above or the parent-authored route.
