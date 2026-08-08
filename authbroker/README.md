# Tobari Auth Broker boundary

The Auth Broker is a locked, non-root daemon with no TCP listener. It serves
newline-delimited JSON schema 1 over two owner-only Unix sockets:

- `/run/tobari-auth/runtime/broker.sock`: `health`, `introspect`, `resolve`
- `/run/tobari-auth/control/broker.sock`: `health`, `status`, `unlock`,
  `import`, `login`, `logout`, `issue_handle`, `binding_status`

Every JSON frame is at most 64 KiB and must contain exactly the fields for its
operation. `unlock` adds exactly 32 raw key bytes after its JSON newline;
`import` and the internal result of `login` add the declared bounded raw secret
bytes. Those values never use argv or environment variables.

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

## Control command and request schemas

The installed control executable is internal to bounded `docker exec` calls:

```text
tobari-auth-control health
tobari-auth-control status --context-id <uuidv7> --provider <provider>
tobari-auth-control unlock                         # exactly 32 stdin bytes
tobari-auth-control import --context-id <uuidv7> --provider <provider>  # secret on stdin
tobari-auth-control login github --context-id <uuidv7>                 # interactive TTY
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
- `login`: the import fields plus the bounded, non-secret `account_label`
  obtained from reviewed `gh auth status --json hosts` output
- `issue_handle`: those keys plus `context_id`, `project_id`, `provider`,
  `bindings`
- `binding_status`: those keys plus `context_id`, `project_id`, `provider`,
  `revision`, `bindings`; it returns only `ready`, `missing`, or `stale`

Runtime `introspect` has exact keys `schema_version`, `op`, `handle`,
`context_id`, `project_id`, `provider`, `target`, `source_header`, and
`source_format`. `resolve` repeats every one of those fields and adds the exact
credential `revision` returned by introspection.

The built-in GitHub control login is API-authentication-only. It runs
`gh auth login --hostname github.com --web` with prompts and container browser
launch disabled, writes only to the private login tmpfs, and requests no Git
protocol or credential helper. The trusted host CLI opens only the fixed GitHub
device URL when available; Auth Broker receives no host browser, GUI socket,
Git configuration, or Git executable requirement.
