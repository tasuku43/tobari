# Work Goal: Host-driven brokered CLI authentication

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, and ADR 0020
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Current change
- Related ADRs: 0016, 0019, and 0020

## Outcome

A Context-selected local runtime can contain `kubectl`, `cwk`, `twg`, `pup`,
and `aws` without adding those tools to Tobari's published base image. Static
credentials remain Context-owned Auth Broker records. Refreshable or signing
credentials use a resident trusted-host credential companion, so provider CLI
execution and automatic refresh do not require provider tools in the Auth
Broker image and never expose primary credentials to a Workspace.

The first complete dynamic driver uses a trusted-host AWS CLI IAM Identity
Center session. It refreshes automatically while the upstream session remains
renewable and returns only a request-bound temporary lease to the Auth Broker
after Gateway and OPA have allowed the exact HTTP request.
GitHub login also moves to a reviewed fixed trusted-host GitHub CLI driver; it
does not add a refresh claim or Git transport authentication.

## Why now

The requested tools vary by user and Context. Baking every possible provider
CLI into either the public work base or the Auth Broker image does not scale,
while copying host CLI homes into Workspaces would break the existing
project-bound post-policy credential boundary.

## Non-goals

- Adding kubectl, cwk, TWG, pup, or any new tool to the published base runtime.
- Letting a repository, Context provider manifest, or request choose an
  arbitrary host executable, argument, environment, endpoint, or shell.
- Copying host CLI homes, AWS caches, kubeconfigs containing secrets, or real
  temporary credentials into a Workspace.
- Claiming general TWG OAuth refresh until TWG exposes a bounded typed export
  contract and every credential-bearing authority used by the supported path
  is declared.
- Supporting AWS SigV4a, presigned-query authentication, streaming signatures,
  custom endpoints, China/GovCloud/ISO/sovereign partitions, newer IAM Identity
  Center portal URL forms, or provider-operation policy in this slice.

## Acceptance criteria

- [x] Tobari's published base runtime and its embedded snapshot are unchanged.
- [x] The optional local toolbox pins and validates kubectl, cwk, TWG, pup,
      AWS CLI, and their local-only license/integrity inventory.
- [x] Cluster lifecycle starts, verifies, reports, and stops one resident host
      credential companion without exposing a public hidden command.
- [x] Auth Broker reaches the companion over one authenticated encrypted
      reverse `docker exec` channel that opens no host/container listener and
      is not reachable from a Workspace, Gateway, or OPA.
- [x] AWS login runs on the trusted host, cancellation cannot commit stale
      state, renewable AWS state remains encrypted in the Context vault, and
      refresh has bounded single-flight waiting per credential revision.
- [x] Broker persists a pre-execution encrypted task barrier; outcome-unknown
      survives restart, maps to non-retryable HTTP 409, and requires AWS
      re-login or logout before another refresh.
- [x] GitHub login runs through the reviewed fixed host driver, and the Auth
      Broker image contains no GitHub, AWS, or other provider CLI.
- [x] Gateway still authorizes before any companion call and forwards at most
      once after validating the exact request-bound result.
- [x] Static Chatwork, Datadog, Kubernetes, and bounded delegated TWG examples
      continue through project-bound handles without a host driver claim.
- [x] Existing GitHub and schema-v1 provider behavior remains compatible or is
      migrated with explicit recovery.
- [x] Required focused tests pass; `task check`, `task security`,
      `task public:check`, and `task release:check` decide completion.

## Completion definition

The work is complete when the acceptance criteria have evidence, durable
decisions have been promoted, runtime/Broker/Gateway snapshots agree, required
gates pass, and this temporary packet is removed.
