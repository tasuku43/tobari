# Work Goal: Enter Tobari workspaces through Bash

- Status: Active
- Retention: temporary
- Retention reason: Evidence for the base-runtime shell contract and its end-to-end verification
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md
- Review/delete trigger: Delete after the runtime shell contract and E2E evidence are durable in tests and documentation
- Successor: None
- Owner: Delegated runtime maintainer
- Target: Base runtime image and interactive `tobari` entry
- Related ADRs: None

## Outcome

A developer who runs `tobari` enters the selected Workspace through an
interactive Bash shell. The canonical base runtime contains Bash and declares
the `tobari` user shell as `/bin/bash`; the host-controlled Workspace lifetime
entrypoint and `sleep infinity` lifetime command remain unchanged. The same
contract is present in the embedded runtime snapshot used by the CLI.

## Why now

The requested routine experience is explicit: Bash must be available in the
base image, and the ordinary `tobari` entry path must open Bash. The repository
already appears to contain both pieces, but completion requires a fresh
end-to-end image and entry verification rather than relying on source inspection
alone.

## Non-goals

- Do not make the image `CMD` own Workspace lifetime.
- Do not replace the fixed Tobari entrypoint or the host-controlled
  `sleep infinity` process.
- Do not add a shell selector, arbitrary command escape hatch, or new public
  CLI command.
- Do not change host credential, network, mount, or privilege boundaries.

## Acceptance criteria

- [x] The canonical and embedded base images contain an executable `/bin/bash`,
  and the `tobari` user login shell is `/bin/bash`.
- [x] A real or deterministic runtime E2E enters a Workspace through
  `tobari` and proves the interactive exec command is `/bin/bash` with a TTY.
- [x] The Workspace remains reusable after the child Bash exits; its lifetime
  remains owned by the infrastructure `sleep infinity` command.
- [x] The agent-facing and human-facing contract remains discoverable without
  a new input or recovery path, and the discovery/retry journey needs no extra
  parser or command guess.
- [ ] Required runtime, implementation, security, and public-boundary checks
  pass. The shared-worktree packet-lint blocker is recorded in `tasks.md` and
  the E2E transcript without claiming this criterion passed.
- [ ] The delegated sub-agent creates an intentional scoped commit and reports
  its SHA after the E2E and required gates pass.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially bounded autonomy,
  CWD-owned Workspace lifecycle, custom-image isolation, and executable claims
- Product contract: [Product contract](../../01_product_contract.md), runtime
  image and interactive root entry contracts
- Architecture: [Architecture](../../02_architecture.md), runtime assets and
  fixed Workspace lifetime command
- Security: [Security model](../../03_security_model.md), untrusted image and
  fixed runtime boundary
- Harness: [Harness](../../04_harness.md), runtime and integration profiles

## Completion definition

The work is complete when the acceptance criteria have evidence in the child
packet, the canonical and embedded runtime sources agree, the end-to-end shell
journey has been replayed, the required gates pass, and the delegated sub-agent
has created a scoped commit. If the packet only confirms behavior that was
already present, the commit may contain the packet's durable evidence and
regression coverage; no unnecessary production change is required.
