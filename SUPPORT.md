# Support

Agentic CLI Foundry is maintained on a best-effort basis. This file explains where to ask for help and what information maintainers need. A derived project must replace this generic policy with support promises appropriate to its users and release maturity.

## Where to ask

- Use a GitHub issue for a reproducible bug or a focused template improvement.
- Use the repository's discussion channel, when enabled, for usage and design questions.
- Use a pull request for a reviewed implementation tied to a clear outcome.
- Use the private process in [SECURITY.md](SECURITY.md) for vulnerabilities or sensitive security details.
- Use the private maintainer contact described in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for conduct concerns.

Do not put credentials, private URLs, personal data, confidential logs, or embargoed details in an issue or discussion.

## Information to include

- Template or derived-project version and commit.
- Operating system and architecture.
- Go and Task versions when the problem concerns development.
- Exact command, expected result, actual result, and exit status.
- A minimal reproduction using synthetic data.
- Relevant bounded output with secrets and personal data removed.
- Whether `task check:fast` or another profile fails.

## Support boundary

The template supports its documented runnable defaults, bootstrap behavior, architecture contracts, and repository gates. It does not provide support for an arbitrary derived project's private integrations, credentials, deployments, external services, or modified release process.

Maintainers do not guarantee a response time, long-term support for old releases, or private implementation consulting. Security-report acknowledgement targets are stated separately in `SECURITY.md`.

## Before requesting help

1. Read [README.md](README.md) and the [documentation map](docs/README.md).
2. Check existing issues and accepted decisions.
3. Run the smallest relevant verification profile.
4. Reduce the problem to a public, synthetic reproduction.
5. Confirm the behavior belongs to this template rather than a derived integration.

Clear evidence makes support faster and helps turn one report into a lasting test or documentation improvement.
