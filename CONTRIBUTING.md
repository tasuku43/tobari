# Contributing

Thank you for improving Agentic CLI Foundry. Contributions are welcome when they keep the repository runnable, understandable by a new contributor or coding agent, and safe to publish.

## Before you begin

- Read [AGENTS.md](AGENTS.md) and the documents it lists.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- Search existing issues and decisions before proposing a second solution to the same problem.
- For a security vulnerability, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.
- For usage help, follow [SUPPORT.md](SUPPORT.md).
- Do not submit confidential code, URLs, credentials, personal data, or material you do not have the right to license.

## Development setup

Install the exact Go version declared by `go.mod`, Git, `gofmt`, and [Task](https://taskfile.dev/). The canonical gate uses `GOTOOLCHAIN=local`, so the selected PATH, `GOROOT`, `GOTOOLDIR`, and compiler must all belong to that exact installation. Then run:

```sh
task check:fast
```

The required pre-merge `task check` also runs the complete security, release, and public profiles. Local execution needs ShellCheck 0.9.0 or newer, Ruby, `tar`, `unzip`, either `sha256sum` or `shasum`, plus network access or a pre-populated Go module cache for pinned Go-based linters. Preflight reports missing tools or a mixed Go installation before the long-running profiles begin. See [Harness](docs/04_harness.md) for the composition and diagnostic contract.

The default repository must remain runnable as `github.com/tasuku43/agentic-cli-foundry` with the `agentic-cli-foundry` binary. Identity changes belong in a derived repository through the bootstrap workflow, not in a contribution to the reusable template.

## Propose the outcome first

For a substantial change, open an issue or include a work packet starting from the [`goal.md` work-packet template](docs/work/_template/goal.md). State:

- the user outcome;
- what is explicitly out of scope;
- the product, architecture, security, and compatibility constraints;
- the considered alternatives;
- objective acceptance criteria.

If the change introduces a durable trade-off or supersedes an earlier decision, add an ADR from [the decision template](docs/decisions/0000-template.md).

## Implement and verify

- Keep domain, application, infrastructure, and CLI responsibilities separate.
- Add tests that state the behavior and failure boundary.
- Keep help, routing, and the public command list derived from `cli.Catalog`.
- Update documentation in the same change when a promise or invariant changes.
- Use synthetic data in examples and fixtures.

Before opening a pull request, run:

```sh
task check
task public:check
```

Run `task release:check` as well when packaging, Formula, version metadata, or release automation changes.

## Pull requests

A pull request should be small enough to review as one decision and should include:

- the user-visible outcome and rationale;
- relevant work packet or ADR links;
- tests added or changed;
- commands used for verification;
- compatibility, security, and public-boundary impact;
- generated changes clearly separated from hand-written changes.

Reviewers evaluate conformance to the theses and invariants, not only whether the happy path works. A check may be green while the product decision is still wrong.

## Licensing contributions

This repository is licensed under the MIT License. By submitting a contribution, you represent that you have the right to submit it and agree that it is licensed under the same terms. Do not copy code or documentation from a private or incompatibly licensed source.
