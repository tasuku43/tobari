# Presentation Evidence: Workspace service exposure

This evidence remains incomplete until implementation. It fixes the reviewed
human state transitions and the typed facts that presentation may display.

## Typed semantic fixture

```text
attachment: current and active
workspace_label: /projects/app
target: 127.0.0.1:3000
host_binding: IPv4 loopback only
host_port_selection: automatic
browser_open: false
lifetime: current attachment
request_state: pending
```

The Workspace label is presentation only. Request identity is one fresh opaque
host-owned reference and is never reconstructed from the label, target port,
list position, or review copy.

## Reviewed transcript

```text
workspace$ tobari-expose 3000
Exposure requested.
Waiting for trusted-host approval...

Host review available - press Ctrl+] then r
```

Trusted Host Review:

```text
Trusted Host Review
Workspace input is paused. Review keys stay on the trusted host.

Workspace    /projects/app
Service      127.0.0.1:3000
Host access  Loopback only
Host port    Selected automatically
Browser      Will not open
Lifetime     Current attachment

[a] Allow once  [d] Deny  [b] Back
```

Successful helper result:

```text
Available on the host:
  <generated per-attachment loopback URL>
Lifetime: current attachment
```

Unavailable service response:

```text
Workspace service is not available yet.
Start the service, then reload.
```

## State transitions to preserve

- Pending: no listener and no data path exist.
- Back: review closes, request remains pending, and no mutation occurs.
- Denied: no listener exists; the waiting helper returns a distinct failure.
- Active and listening: host listener exists; this is not an application health
  claim.
- Relaying: one or more active connections exist.
- Last Workspace connection failed: the latest reviewed request could not
  reach the exact target; this is not a periodic health result.
- Stopped: listener and streams are closed before authority is removed.
- Attachment closed: all pending and active state is removed.

## Negative-inference canaries

- A matching Workspace label does not establish attachment identity.
- A matching target port does not let another attachment reuse or stop an
  exposure.
- Review order and selection position do not identify a pending request.
- Workspace terminal output that prints the host cue or review copy cannot open
  review or approve the request.
- The `.localhost` label is not trusted by itself; exact listener ownership and
  authority validation remain required.
- Listener open does not mean the development server is healthy.
- An HTTP 200 does not prove application health, and an HTTP 404 or login page
  does not prove failure.

## Evidence to add during implementation

- Frozen typed fixture and answer-key file paths, hashes, and byte counts
- Before and after human golden snapshots derived from the same fixture
- Exact helper help and error output snapshots
- Terminal cue, review, Back, Deny, Allow, stop, and attachment-close traces
- Vite, Next.js, Storybook, and Jupyter Host and Origin observations
- HTTP and WebSocket compatibility results
- Hostile-copy and wrong-authority rejection evidence
- Product-owner compatibility decision for any failed representative tool
