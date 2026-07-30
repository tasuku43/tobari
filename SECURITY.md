# Security Policy

Tobari treats security statements as contracts that identify both a trust
boundary and an enforcement mechanism. The concrete assets, actors, side
effects, storage, and network destinations are defined in the
[security model](docs/03_security_model.md).

## Supported versions

| Version | Security support |
|---|---|
| Latest release | Supported |
| `main` | Supported for fixes before the next release |
| Older releases | Not supported |

Until Tobari publishes a release, `main` is the only supported version.

## Report a vulnerability

Use GitHub's private vulnerability reporting flow from the repository's **Security** tab. Do not include vulnerability details, credentials, private URLs, or personal data in a public issue.

If private reporting is unavailable, contact the repository owner at the
address recorded in `.harness/project.json`. A public issue may be used only to
ask how to contact maintainers and must contain no sensitive details.

Include, when possible:

- affected version and platform;
- preconditions and impact;
- minimal reproduction steps;
- whether secrets or user data may have been exposed;
- any suggested mitigation.

Maintainers should acknowledge a complete report within three business days, coordinate disclosure with the reporter, and avoid promising a release date before the impact is understood.

## Security invariants

| Claim | Primary enforcement |
|---|---|
| Layer boundaries cannot be bypassed accidentally | `tools/archlint`, architecture tests, `task check` |
| Every public operation has a declared effect | `operation.Effect`, catalog validation, negative tests |
| Mutation declarations cannot omit their required target shape | `operation.Intent`, `operation.TargetRef`, domain validation and negative tests |
| Unknown or inconsistent effects fail closed | domain validation and mutation-path tests |
| Public commands come from one catalog | `cli.Catalog` contract tests |
| Credentials and private identifiers are not committed | `tools/repoguard`, synthetic fixtures, `task security` |
| Realm has no direct external route | Docker topology and integration tests |
| HTTP and HTTPS requests fail closed through Gateway and OPA | Gateway, Rego, and integration tests |
| Managed credentials enter only after authorization | Gateway credential-binding tests and secret canaries |
| A release is built from reviewed source and checked artifacts | release profile and release workflow contracts |
| Public readiness is an explicit gate | `task public:check` |

## Trust boundaries

Tobari assumes:

- CLI arguments and environment values are untrusted input.
- Filesystem, process, network, and credential operations are side effects.
- Infrastructure responses are untrusted until validated.
- Build dependencies and release automation are part of the supply chain.
- Repository content and Git history are public once pushed to a public remote.

## Out of scope

Tobari does not:

- protect a compromised operating system or developer account;
- isolate files below the selected read-write root from Realm processes;
- authorize non-HTTP protocols or transparent proxy bypass;
- provide code signing, notarization, or artifact attestation;
- authorize publication of code copied from another repository;
- replace independent review for high-impact or regulated systems.
