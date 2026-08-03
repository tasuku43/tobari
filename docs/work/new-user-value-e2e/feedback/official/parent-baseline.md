# Parent baseline: safe first success and runtime customization

- Status: parent baseline completed; this is an entry criterion, not one of the four child-agent acceptance records.
- Subject: a clean tracked snapshot of `cc-bash-guard` at revision `e045d15`.
- Boundary: disposable project copy on a Docker-VM-shared host path, isolated Tobari XDG state, synthetic `example.com` traffic, and no source or harness changes in the subject.
- Interaction: desktop PTY sessions with interactive host and Workspace processes. The raw PTY bytes remain in the parent execution log rather than Git; the checkpoints below are a redacted readable projection.

## Result

The core value loop works when the runtime is prepared before the first
Workspace registration:

1. `doctor --root .` passed for Docker CLI, Engine, context, Compose, root
   sharing, credentials, and the empty owned-resource check.
2. `cluster up` prepared the environment, started Gateway and OPA, and reached
   the `Cluster ready` signal.
3. `runtime init` followed by `runtime build` selected a ready Context image.
4. A real `tobari` PTY entered a new Workspace and presented a Bash shell.
5. Inside the Workspace, a synthetic `curl -X PUT https://example.com/...`
   returned the secret-free `policy_denied` JSON and HTTP 403. The response
   named the host-side `tobari policy review` recovery command.
6. After exiting, the host Permission Inbox showed the exact host, port,
   method, and path. Entering detail, choosing `a`, and confirming with `y`
   produced `Permission allowed`, `Testing policy passed`, and `Applying exact
   rule applied`.
7. Re-entering the same Workspace and repeating the same request reached the
   upstream Example Domain and returned HTTP 405, rather than another Tobari
   policy denial. This is the first clear safety/value signal.
8. Adding `tree` to the active Context Dockerfile and running `runtime build`
   produced a new ready image. Re-entering the existing Workspace continued
   to use the old image, so `tree` was absent. After deleting and recreating
   that disposable Workspace, `tree --version` returned `tree v2.1.0` and
   `printf 'shell=%s\\n' "$0"` returned `shell=/bin/bash`.
9. `exit`, `delete --force`, and `cluster down --purge` completed. No owned
   Tobari containers remained.

## Readable checkpoints

| Checkpoint | Visible result | Interpretation |
|---|---|---|
| Bootstrap | `Cluster ready`, Gateway/OPA healthy | The host boundary is operational. |
| First request | `policy_denied`, `permission_review_available`, `http=403` | The agent sees a bounded denial and a human-owned next action. |
| Review detail | `This allows exactly this host, port, method, and path.` | The approval scope is concrete and least-privilege shaped. |
| Review completion | `Permission allowed` / `Testing policy passed` / `Applying exact rule applied` | The human action has a visible, bounded outcome. |
| Retry | Example Domain body, `http=405` | The approved request crossed the Tobari boundary and reached upstream. |
| Runtime recreation | `tree v2.1.0`, `shell=/bin/bash` | A custom runtime is observable inside a newly created Workspace. |
| Cleanup | `Tobari deleted`, `Cluster not configured` | The run left no active project or shared cluster. |

## Friction and follow-up candidates

### Runtime must be prepared before first registration

When the parent entered a fresh project before the Context runtime was ready,
Tobari registered the project with `runtime: missing` and returned
`runtime_reconcile_failed`. Running `runtime init` and `runtime build` later did
not repair that already-created instance; deleting and registering again was
required. This is a product/recovery finding, not an environment failure.

Candidate disposition: **integrate or narrow the recovery path**. Either the
first `tobari` entry should make the missing-runtime state actionable without
leaving a broken registration, or the documented bootstrap must make runtime
preparation an explicit prerequisite. The catalog currently treats `tobari`
as the fixed current-directory act and `runtime init/build` as separate host
actions, so this needs a reviewed product decision before implementation.

### Raw review confirmation is timing-sensitive

The Permission Inbox rendered correctly and the immediate `a` -> `y` sequence
worked. If the parent paused after `a` while the raw terminal confirmation
reader was polling, the command exited with `undeclared_fault_contract` before
the confirmation could be entered. The pending candidate remained intact and
could be reviewed again; no unintended policy mutation occurred.

Candidate disposition: **keep the separate review and exact action commands,
fix the interactive timeout contract**. The human workflow is valuable, but a
normal human pause must not become an undeclared fault. This finding should be
replayed in a focused PTY regression before any child result is classified as
an agent or documentation problem.

### Runtime build does not refresh an existing Workspace

`runtime build` selected the new Context image, but a reusable Workspace that
already existed continued to run the old image on re-entry. Deleting and
recreating the Workspace consumed the new image successfully.

Candidate disposition: **docs-only until the lifecycle contract is decided**.
The current command separation is understandable once observed, but the
Quick Start must say whether runtime customization applies to new Workspaces
only, or the lifecycle should offer an explicit safe refresh/recreate action.
Do not silently replace a live Workspace from `runtime build`.

### Environment boundary

The first disposable copy was under `/tmp`; the Docker VM could not consume
the generated policy directory reliably. A host-shared disposable path under
the user's home directory worked. Official child runs must use a Docker-VM-
shared path and treat a failed bind/share diagnostic as an environment
blocker, not as product value evidence.

## Parent conclusion

The denial-to-review-to-retry loop is sufficiently proven to begin blind child
runs. The child protocol must not include these commands, this packet, the
thesis, or the findings above. The three friction points are retained as
parent hypotheses for comparison; child agents must be allowed to discover
whether they recur on their own.
