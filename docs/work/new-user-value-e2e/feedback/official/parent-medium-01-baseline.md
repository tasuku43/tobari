# Parent baseline: three pending Permission Inbox decisions

- Status: parent baseline completed; this is an entry criterion and not one of
  the four blind child-agent acceptance records.
- Subject: a fresh disposable `cc-bash-guard` snapshot with isolated Tobari
  state and synthetic `example.org` requests.
- Interaction: host and Workspace commands used real pseudo-TTY sessions. The
  raw bytes remain in the parent execution log rather than Git; the evidence
  below is a redacted readable projection. No repository source or harness was
  changed.

## Result

The three-decision Permission Inbox journey is functionally available:

1. The parent prepared the cluster and runtime, entered a new Workspace, and
   issued three distinct synthetic requests: `PUT /medium-a`, `POST /medium-b`,
   and `DELETE /medium-c` on `example.org:443`.
2. Each request returned secret-free `policy_denied` JSON with HTTP 403 and the
   visible host-side recovery cue `tobari policy review`.
3. Exiting the Workspace displayed `3 pending network permissions are waiting
   for review` and identified the latest request.
4. The host Permission Inbox listed all three exact candidates. Its detail
   screen stated `Current Tobari only` and showed the host, port, method, path,
   reason, status, and observed time.
5. The parent allowed the first candidate and confirmed it. The visible result
   included `Permission allowed`, `Testing policy passed`, and `Applying exact
   rule applied`; the queue refreshed to two candidates.
6. The parent denied the second candidate and confirmed it. The visible result
   included `Permission denied` and `Applied yes`; the queue refreshed to one
   candidate.
7. The parent pressed `q` at the remaining list. The visible result was
   `Permission review canceled` and `No permissions changed.` A read-only JSON
   review immediately afterward contained exactly the remaining
   `DELETE /medium-c` candidate, proving cancellation did not mutate it.
8. Workspace deletion and cluster cleanup completed, and the owned-container
   check was empty.

## Readable checkpoints

| Checkpoint | Visible result | Interpretation |
|---|---|---|
| Denial queueing | Three `policy_denied` responses, each `http=403` | Distinct requests become reviewable candidates. |
| Inbox list | `3 pending permissions` with three exact request rows | The queue is visible without JSON or source inspection. |
| Detail | `Scope Current Tobari only` and exact request dimensions | The decision scope is understandable and bounded. |
| Allow | `Permission allowed` / `Testing policy passed` / `Applying exact rule applied` | One exact candidate is applied. |
| Deny | `Permission denied` / `Applied yes` | One exact candidate is explicitly rejected. |
| Cancel | `Permission review canceled` / `No permissions changed.` | Leaving the queue preserves the remaining candidate. |
| Read-only refresh | JSON contains only the third candidate | Queue state agrees with the visible decisions. |
| Cleanup | Workspace deleted; cluster is unconfigured; no owned containers | The run leaves no active resources. |

## Friction and follow-up candidates

### Confirmation is timing-sensitive

In an initial parent attempt, pressing `a` and waiting before entering `y`
produced `undeclared_fault_contract`. The candidate remained pending and no
policy mutation occurred. A second run with the short human sequence `a` then
`y` succeeded, and the deny path succeeded with the corresponding immediate
`d` then `y` sequence.

Candidate disposition: **keep** the Inbox and separate exact decision
semantics, but create a focused PTY contract-fix packet. A normal human pause
must not be converted into an undeclared failure, and the confirmation prompt
must remain available until the user responds or explicitly cancels.

### Queue review is a coherent surface

The list, detail, exact action, confirmation, refresh, and cancel states form a
single understandable human task. This baseline provides no evidence that
`policy review` should be split further. The JSON form remains useful as a
read-only inspection surface, while the TTY form owns mutation decisions.

Candidate disposition: **keep** the Permission Inbox as the primary human
workflow; do not collapse it into raw `policy allow`/`policy deny` commands.

## Parent conclusion

The Medium-01 product path itself is sufficiently proven for one more blind
child run. The child should receive only the desired outcome—resolve three
independent blocked outbound requests by making one allow, one deny, and one
cancel decision—and must discover the path independently. This baseline and
its timing workaround must not be included in the child prompt.
