# Tobari Auth Broker boundary

The Auth Broker is a locked, non-root daemon with no TCP listener. It serves
newline-delimited JSON schema 1 over two owner-only request sockets and one
private credential-companion socket:

- `/run/tobari-auth/runtime/broker.sock`: `health`, `introspect`, `resolve`,
  `introspect_signing`, `sign_sigv4`
- `/run/tobari-auth/control/broker.sock`: `health`, `status`, `unlock`,
  `import`, `login`, `logout`, `issue_handle`, `binding_status`
- `/run/tobari-auth/companion/bridge.sock`: one authenticated, encrypted,
  reverse host-companion session; this socket is not mounted from the image

Every JSON frame is at most 64 KiB and must contain exactly the fields for its
operation. `unlock` adds exactly 32 raw key bytes after its JSON newline;
`import` and host-completed `login` add a declared bounded raw secret or opaque
AWS CLI state. Those values never use argv or environment variables. The
Broker image contains no GitHub CLI, AWS CLI, provider network client, browser,
or provider-native login helper.

Runtime introspection and resolution both bind the caller-supplied Context ID,
project ID, provider ID, HTTPS target, source header, and semantic source format
to the encrypted handle record. Introspection returns only revision and
credential-projection metadata. Resolution repeats the checks and returns the
opaque primary secret only after the Gateway has independently obtained OPA
authorization.

Vaults are stored at
`/var/lib/tobari-auth/contexts/<stable-context-id>/vault.enc`. Schema-1 vaults
use AES-256-GCM with a random 12-byte nonce and associated data binding the
schema and Context ID. Raw project handles are durable only inside this
authenticated ciphertext; the daemon's live lookup table contains SHA-256
handle hashes.

The encrypted payload is schema 2. Schema-1 payloads are strictly validated and
migrated in memory; the next mutation writes schema 2. Static records contain a
`static_primary_secret`. AWS records instead contain typed
`aws_sso_session` opaque host-driver state, its fixed driver ID and executable
revision, and a monotonic state generation. They have no resolvable
primary-secret field.
The outer AES-GCM envelope and socket protocol remain schema 1.

## Control command and request schemas

The installed control executable is internal to bounded `docker exec` calls:

```text
tobari-auth-control health
tobari-auth-control status --context-id <uuidv7> --provider <provider>
tobari-auth-control unlock                         # exactly 32 stdin bytes
tobari-auth-control import --context-id <uuidv7> --provider <provider>  # secret on stdin
tobari-auth-control login --context-id <uuidv7> --provider github --account-label <login>  # token on stdin
tobari-auth-control login --context-id <uuidv7> --provider aws --account-label <account> --driver-id aws_cli_sso --driver-revision <sha256>  # opaque state on stdin
tobari-auth-control companion_prepare --epoch-id <companion-e1_epoch>
tobari-auth-control companion_status
tobari-auth-control logout --context-id <uuidv7> --provider <provider>
tobari-auth-control issue_handle --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --bindings <json>
tobari-auth-control binding_status --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --revision <revision> --bindings <json>
```

Control requests have these exact schema-1 keys:

- `health`: `schema_version`, `op`
- `status` and `logout`: those keys plus `context_id`, `provider`
- `unlock`: those keys plus `key_length: 32`, followed by 32 raw bytes
- `import`: those keys plus `context_id`, `provider`, `secret_length`, followed
  by exactly that many raw bytes; an import has no account label
- GitHub `login`: `context_id`, `provider: "github"`, the bounded non-secret
  `account_label` obtained by the reviewed host driver, and `secret_length`,
  followed by the raw token captured on the host
- AWS `login`: `context_id`, `provider: "aws"`, the 12-digit non-secret
  `account_label`, `driver_id: "aws_cli_sso"`, lowercase SHA-256
  `driver_revision`, and `state_length`, followed by canonical opaque host
  driver state
- `companion_prepare`: `epoch_id`; the Broker derives an epoch key from the
  in-memory installation key and returns `prepared`
- `companion_status`: no additional fields; it returns `absent`, `prepared`,
  or `ready` plus the exact current epoch ID
- `issue_handle`: those keys plus `context_id`, `project_id`, `provider`,
  `bindings`
- `binding_status`: those keys plus `context_id`, `project_id`, `provider`,
  `revision`, `bindings`; it returns only `ready`, `missing`, or `stale`

Runtime `introspect` has exact keys `schema_version`, `op`, `handle`,
`context_id`, `project_id`, `provider`, `target`, `source_header`, and
`source_format`. `resolve` repeats every one of those fields and adds the exact
credential `revision` returned by introspection.

`introspect_signing` takes `handle`, `context_id`, `project_id`, `provider`, an
actual HTTPS `target`, and the full normalized `aws_sigv4` binding. It returns
only non-secret binding metadata and a revision. After OPA allows that ordinary
HTTP effect, `sign_sigv4` repeats the identity, revision, and full binding and
accepts only a normalized request description containing the body SHA-256, not
the body. The broker refreshes AWS SSO state when required, obtains role
credentials in memory, and returns only the final SigV4 authorization, date,
session-token, and optional content-hash headers. AWS state and role credentials
can never pass through `resolve`.

The reviewed GitHub and AWS login drivers run on the trusted host before any
Broker mutation begins. GitHub uses a private temporary `GH_CONFIG_DIR` and
fixed API-only commands; AWS uses a private temporary home and one fixed IAM
Identity Center device-code profile. A successful driver sends only its final
bounded token or opaque state through control stdin. Failure leaves the prior
Context credential unchanged. Neither host CLI home nor a browser/GUI socket
is mounted into the Broker or a Workspace.

AWS refresh is post-policy and on demand. After the Gateway obtains OPA allow
for the exact ordinary HTTP effect, the Broker snapshots the AWS record and
releases its global state lock. A per-record single-flight call sends the
opaque state and request-bound digests through the authenticated companion
session. The host runs only fixed `aws configure export-credentials --format
process` argv against the digest-pinned executable. The Broker commits returned
state only after record/revision/generation/request correlation still matches,
then uses the temporary lease to sign that unchanged request. Logout or login
rotation wins over a late result; an unobservable provider outcome is never
blindly replayed.

The companion is the current Tobari executable in a private same-binary mode.
Its only container path is fixed `docker exec -i --user <uid:gid>
<container-id> python3 -m authbroker.companion_bridge`. Bootstrap key material
crosses child stdin, and the reverse byte stream uses a root-key-derived epoch,
challenge authentication, direction-separated AES-256-GCM keys, exact
monotonic sequences, strict schemas, and finite frame/time bounds. It opens no
host listener and receives no provider-selected executable, argv, or
environment.
