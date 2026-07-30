# Authentication and Credential Injection

Tobari does not authenticate its CLI to a service API in the MVP. Instead, the
trusted Gateway injects one static credential only after OPA has authorized the
actual HTTP request.

## Selected method

Credential profiles support:

- `bearer`: read one token from a Gateway-only file and set
  `Authorization: Bearer <value>`;
- `header`: read one value from a Gateway-only file and set the configured
  header.

Every profile has an exact list of normalized destination hosts. Profile names,
types, hosts, header names, and secret container paths are non-secret
configuration. Secret values are never accepted in CLI arguments, environment
variables, or configuration files.

## Runtime flow

1. Tobari sends an HTTP/HTTPS request through Gateway.
2. Gateway removes inbound secret headers and creates redacted OPA input.
3. OPA returns allow/deny, an optional credential profile name, and a
   non-authorizing exact-rule learnability classification for denial UX.
4. Gateway rejects an unknown profile or a host/profile mismatch.
5. Gateway reads the already-mounted secret file, validates its bounded value,
   injects the configured header, and sends the authorized request once.
6. Audit output records only the profile name; a learnable candidate repeats
   that non-secret name so the user can see credential-bearing intent before
   approving network access.

There is no fallback profile and no probing of another secret after failure.
Missing, unreadable, empty, oversized, or malformed secret material fails
closed before upstream forwarding.

## Storage and permissions

The host credential directory is below the Tobari configuration directory by
default. Secret files must be regular files, not symlinks, and have no group or
other permission bits on Unix. Docker mounts individual files read-only into
Gateway under `/run/tobari/credentials`. Tobari and OPA receive neither the
directory nor its contents.

On non-Unix hosts, the CLI can validate regular-file shape but portable mode
bits do not prove ACL ownership. The limitation is reported by `doctor`.

## Deliberate exclusions

MVP does not implement OAuth, refresh, PAT discovery, provider account
selection, AWS SigV4, GitHub App tokens, macOS Keychain, or dynamic short-lived
credentials. Adding any of them requires a new accepted ADR, infrastructure-only
credential implementation, bounded refresh policy, and dedicated tests.

## Verification

- Unit tests cover profile validation, host binding, secret-header stripping,
  and redacted failures.
- Gateway tests prove injection only after allow and absence from decisions and
  logs.
- Docker integration tests prove the mock secret reaches only the allowed mock
  host and is unavailable from Tobari files, environment, and process arguments.
