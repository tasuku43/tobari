# Presentation Evidence: Recommended first-use confirmation

This file fixes the reviewed interface and its semantic source. Implementation
must replace placeholders with repository fixture paths, hashes, byte counts,
and generated goldens.

## Typed fixture

```text
task: root first use
context_collection: known empty
project_root: synthetic canonical project root
context_name: default
source_access: read-write
network_mode: guided
native_readiness: enabled
other_requests: exact review
private_unsafe_ceiling: deny
runtime: standard@1
workspace_bootstrap: absent
session_kind: bash
```

The same fixture must generate the screen and the exact Context-create input.
The project-root label, Runtime display ordinal, and menu position are not
authority.

## After: recommended screen

```text
Tobari will create an isolated Workspace for:
  <canonical project root>

Project files
  Read-write · changes are made directly

Network
  Claude Code and Codex routine traffic   allowed
  Other requests                          exact review
  Private and unsafe destinations         denied

Tools
  standard@1

Host configuration
  Not imported

Session
  Open Bash

❯ Start Workspace
  Customize
  Cancel
```

For an exact child, only the Session value changes:

```text
Session
  Run claude directly
```

Later argv is deliberately absent from repeated presentation. It remains in the
already parsed exact session request.

## Before: ordinary six-stage path

```text
1 of 6 · Name
2 of 6 · Filesystem
3 of 6 · Network
4 of 6 · Runtime
5 of 6 · Workspace bootstrap
6 of 6 · Review & Create
```

The six-stage interface remains the Customize path and standalone Context
creation surface. The change is which surface root first use presents first,
not removal of detailed review.

## Answer key

- Start submits exactly the displayed recommended draft and then continues root
  composition only after confirmed Context creation.
- Customize performs no mutation and opens the existing wizard with the draft
  preselected.
- Cancel performs no mutation and restores the terminal.
- Host configuration is not read before an explicit Configure-from-host choice
  inside Customize.
- Existing Context collections bypass the new screen.
- Direct child argv and final child status remain exact.

## Negative-inference canaries

- A displayed Context name does not identify an existing Context or permit
  adoption after a concurrent change.
- A displayed project path does not create Workspace identity; the typed
  canonical root remains authoritative.
- `standard@1` does not authorize a Runtime by label; the compiled standard
  identity remains authoritative.
- The selected menu row does not authorize values different from the rendered
  typed draft.
- "Not imported" cannot be rendered after host discovery has already occurred.
- The presence of a managed Runtime does not change the recommendation.
- A command label does not permit shell expansion or mutation of later argv.

## Evidence to add during implementation

- Semantic fixture and answer-key paths and hashes
- Raw and line-mode before and after goldens
- Encoded byte counts and any terminal-width assumptions
- Exact Context-create input captured from Start
- Zero-side-effect call ledger for Cancel and every pre-create failure
- Concurrent-change result and recovery snapshot
- Customize transcript proving no host discovery before selection
- First-use Bash and exact-child integration results
- Product-owner compatibility decision for final wording
