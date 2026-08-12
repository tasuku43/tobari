# Work Goal: Narrow brokered authentication for first public V1

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Brokered credentials remain post-policy and project-bound
- Review/delete trigger: Delete after retirement evidence is promoted and the parent release closes
- Successor: None
- Owner: Tobari maintainers
- Target: Before integrated V1 security verification
- Related ADRs: ADR 0009, ADR 0019, ADR 0020, Datadog ADR 0021, ADR 0023, ADR 0025, and ADR 0027
- Parent: [First public release core](../first-public-release-core/goal.md)
- Prerequisite: [Context capability envelope](../context-capability-envelope/goal.md)

## Outcome

Brokered V1 retains the static project-bound Auth Broker core, strict owner
stdin import, and one reviewed GitHub acquisition helper. Managed injection,
AWS, Datadog, OpenAI, Anthropic, Chatwork, companion execution, refresh,
signing, and exact-version host drivers are absent.

## Non-goals

- Adding providers, arbitrary acquisition helpers, executable owner adapters,
  remote manifests, provider business-operation policy, or credential-mode
  selection.
- Removing Workspace-owned tool-native credentials from persistent home.
- Weakening deny-before-resolution, exact binding/replacement, rotation,
  revocation, or no-fallback behavior.

## Acceptance criteria

- [ ] Public auth surface is `auth login --provider github`, strict static
      `auth import OWNER_PROVIDER`, brokered status, and logout.
- [ ] Only static primary-secret broker plans and the GitHub acquisition helper
      remain; every retired selector, state shape, adapter, driver, process,
      endpoint, fault, recovery, dependency, and fallback is absent.
- [ ] GitHub acquisition retains fixed argv, private temporary home, bounded
      strict output, canonical executable digest-before/after, and cleanup.
- [ ] Static flow re-proves project-bound opaque handles, deny before
      resolution, exact post-allow replacement, one resolution, rotation,
      revocation, no invalid-handle forwarding, and secret-free output/logs.
- [ ] Tool-native state is described as Workspace-owned, not brokered.
- [ ] `task check`, `task security`, and `task public:check` pass on the
      integrated branch.

## Completion definition

The packet completes when the retained static broker flow is the only brokered
runtime path, retirement evidence is concrete, and one local commit explains
only V1 authentication narrowing.
