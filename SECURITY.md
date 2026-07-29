# Security Policy

Agentic CLI Foundry treats security statements as contracts that must identify both a trust boundary and an enforcement mechanism. A derived project must replace the generic model with the concrete assets, actors, side effects, storage, network destinations, and release guarantees of that tool.

## Supported versions

| Version | Security support |
|---|---|
| Latest release | Supported |
| `main` | Supported for fixes before the next release |
| Older releases | Not supported unless a derived project documents otherwise |

Until this template publishes a release, `main` is the only supported version.

## Report a vulnerability

Use GitHub's private vulnerability reporting flow from the repository's **Security** tab. Do not include vulnerability details, credentials, private URLs, or personal data in a public issue.

If private reporting is unavailable in a derived repository, its maintainers must publish an alternative private security contact before the first public release. A public issue may be used only to ask how to contact maintainers and must contain no sensitive details.

Include, when possible:

- affected version and platform;
- preconditions and impact;
- minimal reproduction steps;
- whether secrets or user data may have been exposed;
- any suggested mitigation.

Maintainers should acknowledge a complete report within three business days, coordinate disclosure with the reporter, and avoid promising a release date before the impact is understood.

## Template security invariants

| Claim | Primary enforcement |
|---|---|
| Layer boundaries cannot be bypassed accidentally | `tools/archlint`, architecture tests, `task check` |
| Every public operation has a declared effect | `operation.Effect`, catalog validation, negative tests |
| Mutation declarations cannot omit their required target shape | `operation.Intent`, `operation.TargetRef`, domain validation and negative tests |
| Unknown or inconsistent effects fail closed | domain validation and mutation-path tests |
| Public commands come from one catalog | `cli.Catalog` contract tests |
| Discovery references reach actions without reinterpretation | catalog producer/consumer graph, canonical ID validation, exact round-trip tests |
| Credentials and private identifiers are not committed | `tools/repoguard`, synthetic fixtures, `task security` |
| A release is built from reviewed source and checked artifacts | release profile and release workflow contracts |
| Public readiness is an explicit gate | `task public:check` |

The default catalog contains read-only commands. The template also supplies a policy-neutral mutation execution boundary that validates and snapshots intent, applies one injected policy, and executes one logical mutation. A derived project must supply and test its concrete policy before exposing create or write commands; approval, confirmation, dry-run, operating-system authentication, and authorization reuse are deliberately not selected here. These mechanisms do not prove that an arbitrary derived project is secure. New adapters, protocols, credential stores, file writes, subprocesses, and network destinations require project-specific threat analysis and tests.

## Trust boundaries

The base template assumes:

- CLI arguments and environment values are untrusted input.
- Filesystem, process, network, and credential operations are side effects.
- Infrastructure responses are untrusted until validated.
- A successful process launch or interactive terminal is not proof of human authorization.
- Build dependencies and release automation are part of the supply chain.
- Repository content and Git history are public once pushed to a public remote.

See [the security model](docs/03_security_model.md) for the required derived-project analysis.

## Out of scope for the base template

The template does not, by itself:

- choose an authentication or authorization system;
- protect a compromised operating system or developer account;
- guarantee the security of project-specific adapters;
- provide code signing, notarization, or artifact attestation unless a derived release model enables them;
- authorize publication of code copied from another repository;
- replace independent review for high-impact or regulated systems.

When a derived project makes a stronger claim, its documentation must name the added mechanism and its verification path.
