# Tobari Auth Broker boundary

The Auth Broker is a locked, non-root daemon with no TCP listener. It serves
strict schema-1 traffic over two owner-only request sockets and one private
credential-companion socket:

- `/run/tobari-auth/runtime/broker.sock`: `health`, `introspect`, `resolve`,
  `introspect_signing`, `sign_sigv4`
- `/run/tobari-auth/control/broker.sock`: `health`, `status`, `unlock`,
  `import`, `login`, `logout`, `issue_handle`, `binding_status`
- `/run/tobari-auth/companion/bridge.sock`: authenticated encrypted reverse
  host-companion session; never a host or Workspace mount

Every frame is at most 64 KiB and contains exactly the fields for its operation.
`unlock` adds exactly 32 raw key bytes after its JSON newline. `import` and
host-completed `login` add a declared bounded static secret, setup token, or
opaque AWS/pup/Codex state. Credential values never use argv or environment.

Vaults live at `/var/lib/tobari-auth/contexts/<stable-context-id>/vault.enc`.
Schema-1 vaults use AES-256-GCM with a random 12-byte nonce and associated data
binding schema plus stable Context ID. The encrypted payload accepts the closed
reviewed record union: static primary secret, opaque AWS driver state, Datadog
OAuth state, OpenAI Codex OAuth state, and Anthropic Claude OAuth state, with
durable operation barriers.
Raw project handles persist only inside authenticated ciphertext; the live
lookup table contains SHA-256 handle hashes. Every other record kind is
rejected.

## Control command

The installed control executable is internal to bounded `docker exec` calls:

```text
tobari-auth-control health
tobari-auth-control status --context-id <uuidv7> --provider <provider>
tobari-auth-control unlock                         # exactly 32 stdin bytes
tobari-auth-control import --context-id <uuidv7> --provider <provider>  # secret on stdin
tobari-auth-control login --context-id <uuidv7> --provider <reviewed-provider> --account-label <label>  # typed credential bytes on stdin
tobari-auth-control logout --context-id <uuidv7> --provider <provider>
tobari-auth-control issue_handle --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --bindings <json>
tobari-auth-control binding_status --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --revision <revision> --bindings <json>
```

`login` accepts only the closed reviewed GitHub, AWS, Datadog, OpenAI, and
Anthropic plans; `import` is the static owner-provider path, including the
Chatwork built-in. Login stores only bounded verified non-secret account
metadata. Login/import replacement and logout atomically revoke all old
handles for the Context/provider.

`issue_handle` returns only ordinary handle metadata for static, Datadog,
OpenAI, and AWS plans. For Anthropic it additionally returns the canonical
non-secret `oauth_scopes` array from the encrypted session so the Workspace
credential projection follows the granted set without compiling Claude scope
names or exposing either renewable token.

Runtime `introspect` has exact keys `schema_version`, `op`, `handle`,
`context_id`, `project_id`, `provider`, `target`, `source_header`, and
`source_format`. Post-policy operations repeat the exact credential revision
returned by introspection and validate the complete
Context/project/provider/HTTPS/header or signing binding. Introspection returns
only non-secret metadata. Static resolution returns one primary secret;
Datadog/OpenAI select or refresh one same-record token; AWS obtains one private
companion export and signs locally.

The Broker starts locked and retains the installation root key only in memory.
It contains no provider CLI. Acquisition happens on the trusted host through
purpose-limited GitHub/AWS/pup/Codex/Claude drivers. Broker egress is limited
to the fixed Datadog US1, OpenAI, and Anthropic refresh transports. Managed profiles,
manifest-selected helpers, arbitrary executables, compatibility readers, and
unknown record kinds remain rejected without fallback.

The canonical component contracts are Auth Broker API 1 and Gateway API 1.
Release assembly generates one source-bound lock for their reviewed immutable
multi-architecture indexes; generated image digests are not committed to
`versions.env`. Canonical source and the embedded runtime snapshot must remain
byte-identical.
