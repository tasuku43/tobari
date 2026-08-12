# Tobari Auth Broker boundary

The first-public-V1 Auth Broker is a locked, non-root static-secret daemon with
no TCP listener. It serves strict newline-delimited JSON schema 1 over two
Unix sockets:

- `/run/tobari-auth/runtime/broker.sock`: `health`, `introspect`, `resolve`
- `/run/tobari-auth/control/broker.sock`: `health`, `status`, `unlock`,
  `import`, `login`, `logout`, `issue_handle`, `binding_status`

Every frame is at most 64 KiB and contains exactly the fields for its operation.
`unlock` adds exactly 32 raw key bytes after its JSON newline. `import` and
GitHub `login` add one declared bounded raw secret. Secrets never use argv or
environment variables.

Vaults live at `/var/lib/tobari-auth/contexts/<stable-context-id>/vault.enc`.
Schema-1 vaults use AES-256-GCM with a random 12-byte nonce and associated data
binding schema plus stable Context ID. The encrypted payload accepts only
static primary-secret records. Raw project handles persist only inside the
authenticated ciphertext; the live lookup table contains SHA-256 handle
hashes. Every other schema or record kind is rejected.

## Control command

The installed control executable is internal to bounded `docker exec` calls:

```text
tobari-auth-control health
tobari-auth-control status --context-id <uuidv7> --provider <provider>
tobari-auth-control unlock                         # exactly 32 stdin bytes
tobari-auth-control import --context-id <uuidv7> --provider <provider>  # secret on stdin
tobari-auth-control login --context-id <uuidv7> --provider github --account-label <login>  # token on stdin
tobari-auth-control logout --context-id <uuidv7> --provider <provider>
tobari-auth-control issue_handle --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --bindings <json>
tobari-auth-control binding_status --context-id <uuidv7> --project-id <uuidv7> --provider <provider> --revision <revision> --bindings <json>
```

`login` accepts only `provider: "github"`; `import` is the static owner-provider
path. Login stores its bounded verified non-secret account label. Import has no
account label. Login/import replacement and logout atomically revoke all old
handles for the Context/provider.

Runtime `introspect` has exact keys `schema_version`, `op`, `handle`,
`context_id`, `project_id`, `provider`, `target`, `source_header`, and
`source_format`. `resolve` repeats every field and adds the exact credential
`revision` returned by introspection. Both operations validate the complete
Context/project/provider/revision/HTTPS/header binding. Introspection returns
only non-secret metadata. Resolution returns one static primary secret only
after Gateway has independently obtained OPA allow.

The Broker starts locked and retains the installation root key only in memory.
It contains no GitHub CLI or other provider executable and performs no provider
network call. GitHub acquisition happens on the trusted host through fixed
API-only GitHub CLI argv, a private temporary home, a fixed device page/manual
fallback, bounded token capture, executable digest checks, and checked cleanup.

First public V1 has no managed adapter, AWS, Datadog, OpenAI, Anthropic,
Chatwork, dynamic record, OAuth refresh, signer, supplemental header, task
barrier, credential companion, companion socket/protocol, arbitrary helper, or
exact-client-version driver. Unknown operations and retired record kinds fail
closed; there is no compatibility reader or fallback.

The canonical component contracts are Auth Broker API 1 and Gateway API 1.
Official immutable V1 indexes remain unavailable until reviewed
multi-architecture digests replace the paired `unpublished` marker. Canonical
source and the embedded runtime snapshot must remain byte-identical.
