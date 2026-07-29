# Agentic CLI Foundry

Agentic CLI Foundry is a runnable, public-ready foundation for building task-oriented Go command-line tools with coding agents. It starts as a small `agentic-cli-foundry` binary and gives a derived project an explicit product thesis, a four-layer architecture, a machine-readable agent contract, typed side-effect and external-API boundaries, one verification gate, and a documented path to a public release.

The default repository is intentionally real and buildable:

- Go module: `github.com/tasuku43/agentic-cli-foundry`
- Binary: `agentic-cli-foundry`
- Display name: `Agentic CLI Foundry`

The bootstrap tool replaces those exact defaults with validated project values. The defaults are not placeholder syntax, so the template can be built and tested before it is customized.

## What this template optimizes for

- A project-specific thesis that lets contributors and agents resolve ambiguous design choices.
- User tasks as the public vocabulary, instead of leaking transport or vendor APIs into the CLI.
- Explicit utility, discover, and act roles with opaque IDs passed unchanged between tasks.
- Pure domain rules, application use cases, infrastructure adapters, and a thin CLI composition root.
- Explicit `read`, `create`, and `write` effects with typed intent, target, and impact information.
- Structured command prerequisites, inputs, outputs, delivery, collection coverage, failures, and recovery actions for agents.
- Policy-neutral foundations for OAuth/PAT, pagination, timeout, retry/idempotency, and mutations that derived projects make concrete.
- A single command catalog as the source of truth for routing and help.
- Executable architectural, security, release, and public-repository claims.
- A clean public boundary: no inherited organization names, private URLs, credentials, or internal history.

The template fixes reusable vocabulary and enforcement points, not provider or user-experience policy. For authentication, it fixes secret-free requirements and sessions, a fail-closed application gate, an ephemeral infrastructure-issued binding passed unchanged through task ports, and exact credential-record revalidation before I/O. A derived project still chooses OAuth versus PAT, its OAuth flow and reviewed library, credential input and storage, account and refresh behavior, API budgets, and mutation approval policy. [Authentication](docs/07_authentication.md) and [External API Contracts](docs/08_external_api_contracts.md) define that boundary in detail.

## Start a derived project

Create a new repository from this template, then work from the new repository. Do not copy this repository's `.git` directory into an unrelated project.

For Codex, invoke [`$bootstrap-derived-cli`](.agents/skills/bootstrap-derived-cli/SKILL.md) first. It gathers the project identity, uses the same transactional tool described below, verifies imports and gates, and hands off to project-specific thesis work. The manual equivalent is:

1. Edit [`.harness/project.json`](.harness/project.json) with the new project
   identity and an explicit `public_guard.documentation_locale`. Existing
   schema-1 derived repositories must choose the locale in their thesis or
   product contract before moving the configuration to schema 2; no default is
   applied.
2. Preview the exact replacements:

   ```sh
   go run ./tools/bootstrap --dry-run
   ```

3. Apply the validated bootstrap:

   ```sh
   go run ./tools/bootstrap
   ```

4. Replace the generic project reasoning with concrete decisions, in this order:

   - [theses](docs/00_theses.md)
   - [product contract](docs/01_product_contract.md)
   - [security model](docs/03_security_model.md)
   - [authentication decision](docs/07_authentication.md)
   - [external API contracts](docs/08_external_api_contracts.md)
   - [release model](docs/06_release.md)

5. Run the canonical gates:

   ```sh
   task check
   task public:check
   ```

The bootstrap changes repository identity; it does not invent the product. A derived project is not ready merely because all names were replaced. Its north star, supported tasks, trust boundaries, and release promises must be made specific before implementation expands.

The stored bootstrap profile value `ready` means **identity-ready only**. A derived repository may deliberately retain the same GitHub owner or license as the template, but must replace its repository, Go module, binary, display name, Formula class, description, and security contact. Bootstrap status uses “identity ready” to avoid implying product completion.

## Run the default CLI

```sh
go run ./cmd/agentic-cli-foundry --help
go run ./cmd/agentic-cli-foundry help --format agent
go run ./cmd/agentic-cli-foundry help sample --format agent
go run ./cmd/agentic-cli-foundry doctor
go run ./cmd/agentic-cli-foundry sample list --format json
go run ./cmd/agentic-cli-foundry sample read --id <sample-id> --format json
go run ./cmd/agentic-cli-foundry --error-format json sample read --id <sample-id>
```

The default `doctor` task is a minimal utility slice through the domain, application, infrastructure, and CLI layers. The synthetic `sample list` and `sample read --id` pair demonstrates discover-to-act composition: copy the lowercase `id` emitted by `sample list` unchanged into `sample read`. Keep these examples as references while adding the first real capability, then remove or rename them only when the replacement has equivalent architectural and catalog tests.

`doctor`, `sample list`, and `sample read` default to stable TSV and also support versioned JSON. The list result contains only `id` and `name`; read adds `content`. Success data is written to stdout only after complete delivery has been bounded and rendered; collection coverage is declared separately so a bounded window cannot look exhaustive. Failures go to stderr as stable text or schema-versioned JSON and distinguish invalid input, authentication, permission, missing or ambiguous targets, rate limits, temporary failures, policy rejection, cancellation, unsupported work, contract violations, and internal faults with dedicated exit statuses. Schema-6 root agent help is a compact outcome/capability index whose machine-readable `scope_request` points to exact-command or namespace help. Only that scoped response returns the complete typed input, I/O, output, error, role, prerequisite, authentication, mutation, and grouped reference-workflow contracts. The catalog-owned parser validates argv before dispatch, and hierarchical human help is generated from the same input declarations.

## Repository map

```text
cmd/agentic-cli-foundry/                 thin executable entry point
internal/domain/             pure types, faults, effects, API envelopes
internal/app/                task use cases, auth/pagination/execution gates
internal/infra/              concrete adapters for external systems
internal/cli/                catalog, routing, rendering, composition root

docs/                        durable product and engineering reasoning
docs/decisions/              accepted and superseded architecture decisions
docs/work/                   bounded work packets for active changes
tools/                       repository-aware linters and bootstrap tooling
scripts/                     canonical checks and release helpers
.harness/project.json        project identity and machine-readable policy
.agents/skills/              first-run bootstrap and capability workflows
```

Read [the documentation map](docs/README.md) for the intended order and ownership of each document. Contributors and coding agents must also read [AGENTS.md](AGENTS.md).

For community participation and help, see the [Code of Conduct](CODE_OF_CONDUCT.md), [Contributing Guide](CONTRIBUTING.md), [Support Policy](SUPPORT.md), and [Security Policy](SECURITY.md).

## Verification profiles

All entry points delegate to `./scripts/check.sh`:

| Command | Purpose |
|---|---|
| `task check:fast` | Formatting, architecture, and focused tests for short feedback loops |
| `task check` | The implementation gate: fast, vet/race, tidy, and diff checks |
| `task security` | Credential, dependency, egress, and public-boundary checks |
| `task release:check` | Packaging and release-contract checks |
| `task public:check` | Public-readiness and template-sanitization checks |

CI is the authority. Pull-request CI runs the implementation and security/public boundary gates in parallel. Optional local automation must call the same script rather than reimplementing policy.

### Local gate prerequisites

Install the exact Go version declared by `go.mod`, Git, `gofmt`, and [Task](https://taskfile.dev/). The gate deliberately sets `GOTOOLCHAIN=local`; select the exact installation in PATH instead of relying on automatic toolchain download. It validates the Go binary, reported version, `GOROOT`, `GOTOOLDIR`, and compiler before doing long work and prints one remediation block when installations are mixed.

`task release:check` additionally needs ShellCheck 0.9.0 or newer, Ruby, `tar`, `unzip`, either `sha256sum` or `shasum`, and network access or a pre-populated Go module cache for the pinned action-lint tool. Specialized profiles remain explicit completion evidence for the boundaries they govern.

## Public template policy

This repository uses public-safe runnable defaults and synthetic examples. A derived project must keep confidential material out of source, fixtures, documentation, generated files, build logs, and Git history. Review [the public repository guide](docs/05_public_repository.md) before the first push to a public remote.

## License

Agentic CLI Foundry is available under the [MIT License](LICENSE). Derived projects must make an explicit license choice; keeping MIT is allowed, but it must not happen accidentally.
