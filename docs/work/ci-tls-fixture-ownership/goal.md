# Work Goal: Make the synthetic TLS fixture owner-consistent

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: docs/04_harness.md
- Review/delete trigger: Delete after local runtime and GitHub container gates pass
- Successor: None
- Owner: Tobari maintainers
- Target: CI run 31260248390, job 93109840457
- Related ADRs: None

## Outcome

The synthetic authenticated-upstream fixture consumes its owner-only private
key under the same explicit UID/GID that created it on Linux and macOS Docker
hosts.

## Non-goals

- Do not broaden the private-key or parent-directory permissions.
- Do not change production container identities.

## Acceptance criteria

- [ ] The synthetic TLS key remains mode `0600` below a mode `0700` directory.
- [ ] The mock upstream starts with the fixture owner UID/GID.
- [ ] Local runtime validation and the GitHub container job pass.

