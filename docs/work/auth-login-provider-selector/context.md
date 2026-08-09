# Work Context: Interactive auth login provider selection

## Current behavior

- `auth login` declares required positional `provider` and usage `auth login <provider>` in `internal/cli/auth_catalog.go`.
- Running `go run ./cmd/tobari auth login </dev/null` fails in the shared argv parser with `invalid_arguments: provider is required` before the login handler.
- `auth status` already returns the complete installed provider collection for the selected Context as typed `authbroker.StatusResult` data.
- `authcmd.Service.Login` and `dockerruntime.Runtime.LoginAuth` both validate terminal streams and the selected provider before host-driver acquisition.

## Relevant structure

- Entry point: `internal/cli/auth.go`
- Domain rule: `internal/domain/authbroker/result.go`
- Application use case: `internal/app/authcmd/service.go`
- Infrastructure boundary: `internal/infra/dockerruntime/auth_runtime.go`
- CLI catalog or presentation: `internal/cli/auth_catalog.go` and the terminal menu machinery in `internal/cli/context_configuration_wizard.go`
- Existing tests and harness checks: `internal/cli/auth_catalog_test.go`, `internal/cli/auth_test.go`, catalog validation, and `task check`

## Constraints

- Provider IDs are human selectors, not opaque action references or credential authority.
- The command remains a fixed-target write to the installation credential catalog.
- Omitted-provider selection must be terminal-only; redirected input must not be consumed.
- Only reviewed built-in providers with compiled login helpers may appear.
- `--method` remains AWS-specific, requires `--provider`, and must not make an omitted provider ambiguous.
- Existing auth JSON schema, secret-free output, broker ordering, and mutation outcome handling remain unchanged.

## External facts

None. The change uses only repository-owned contracts and existing provider metadata.

## Unknowns

- [x] Whether installed providers are already available without a new infrastructure API: `auth status` provides the complete typed collection.
- [x] Whether a reusable dependency-free terminal menu exists: the Context configuration wizard owns raw-terminal and numbered-line selection.

## Thesis evidence

- Repeated design decision or point of agent confusion: Human setup commands already use terminal-only choice when a complete direct input group is omitted.
- User outcome or friction observed in the minimal slice: Omitted provider currently fails before any guided choice.
- Code workaround or exception being considered: None; reuse the catalog parser, typed status result, and existing terminal menu boundary.
- Current thesis that resolves it, or proposed thesis revision: Thesis 0 prefers lower setup friction; Thesis 3 retains explicit reviewed-provider selection; no thesis revision is needed.
- Downstream product, architecture, security, Skill, catalog, and harness impact: Product/authentication/readiness docs, catalog contract, CLI interaction tests; no security boundary or external adapter change.

## Reproduction or observation

```sh
go run ./cmd/tobari auth login --help
go run ./cmd/tobari auth login </dev/null
go run ./cmd/tobari help auth login --format agent
```

Observed on 2026-08-10: help marks `provider` required and omission returns exit
2 with `invalid_arguments` before runtime interaction.

## Security and public-boundary notes

- Assets and side effects involved: Existing Context/provider status read followed by the existing fixed-target auth login mutation.
- Credentials or confidential data involved: No new material; selection contains provider IDs only.
- New dependencies, destinations, files, processes, or generated content: None.
- External schema provenance, publication rights, and drift evidence: Not applicable.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency, and cancellation facts: Auth result remains complete/non-collection. Selection cancellation is a pre-mutation retryable cancellation. Provider login retains its existing bounded driver behavior and no automatic retry.
- Publication and licensing concerns: None.

## Glossary

- Reviewed login provider: An installed built-in provider whose helper is closed over compiled Tobari behavior (`github`, `aws`, or `datadog`).
