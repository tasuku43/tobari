# Work Context: First-use friction repair

This file records verified facts and unresolved questions. Do not present a desired design as current reality.

## Current behavior

- Source-level repair evidence:
  - The embedded and canonical Gateway source now classify an intercepted
    `PUT https://example.com/quickstart` as scheme `https` when the client
    connection has TLS established.
  - A clean source-Gateway replay returned
    `permission_review_available` for `example.com:443 PUT /quickstart`.
  - `tobari policy review --tail 100 --format json` then listed one pending
    candidate with the same host, port, method, and path.
  - The learnable denial message told the user to leave the Workspace with
    `exit` before running host-side `tobari policy review`.
  - `tobari cluster down --purge` after cleanup rendered `Cluster removed`.
- Remaining image-boundary evidence:
  - Ordinary `cluster up` still uses the reviewed pinned Gateway image from
    `internal/infra/runtimeassets/assets/versions.env`.
  - The currently pinned image does not contain the source markers added in
    this change and still returns `permission_review_unavailable` for the
    README PUT request.
  - Refreshing the published Gateway image and digest is release-boundary work
    already tracked by `docs/work/gateway-image-refresh-e2e`.

## Behavior observed before repair

- `task build` creates `bin/tobari`; in this shell, `command -v tobari` resolved to that path after build.
- In a clean XDG state, `tobari cluster up` succeeded and `tobari` entered a Workspace.
- In the same clean XDG state, `curl -X PUT https://example.com/quickstart` returned `permission_review_unavailable`, and `policy review` reported no pending network permissions.
- `tobari cluster denials --format json` showed that request as `learnable:false` with reason `request did not match an allow rule`; no human or JSON output explained why it was not learnable.
- In an existing local state, policy commands failed with `policy_data_invalid`, while `doctor` reported OPA policy tests as passing and did not diagnose the learned policy data read failure.
- Inside the Workspace, running the Gateway-suggested `tobari policy review` command failed with `bash: tobari: command not found`.
- `runtime build` can succeed while a Workspace exists, but its human output does not say that existing Workspaces keep their stored image.

## Relevant structure

- Entry point: `cmd/tobari` delegates to `internal/cli`.
- Domain rule: policy learnability must remain exact and deny-by-default.
- Application use case: policy discovery, doctor diagnostics, Context runtime build, and Workspace entry.
- Infrastructure boundary: Gateway audit projection and OPA/Rego policy, XDG policy data store, Docker runtime build.
- CLI catalog or presentation: human text renderers, structured errors, scoped help, README quick-start.
- Existing tests and harness checks: Rego tests, Gateway tests, policy-learning integration, Context runtime tests, CLI rendering tests, `task check`.

## Constraints

- Public first-use documentation and CLI-authored prose are English.
- Policy learning must not approve body-bearing, unavailable-body, credential-binding, scheme, port, cluster, or project-principal failures.
- Policy mutations must remain opaque-reference-bound host actions.
- `doctor`, `status`, and `list` must remain read-only.
- Temporary work packet material must be removed before final completion.

## External facts

No external sources are required.

## Unknowns

- [x] Which exact OPA boundary field makes the README quick-start request non-learnable.
      Evidence: the pinned Gateway image passed `scheme:"http"` with port `443`;
      OPA correctly kept that as non-learnable. Source Gateway now normalizes
      intercepted TLS requests to `https`.
- [x] Whether stale `policy_data_invalid` is caused by schema migration, unsafe ownership/mode, malformed JSON, or prior development data.
      Evidence: the first observed local state had unsafe or invalid learned
      data; the repair is a read-only `doctor` diagnostic rather than an
      automatic migration.
- [x] Whether build/PATH ambiguity belongs in README, Task output, or both.
      Evidence: both the README source-install path and `task build` output now
      state the post-build PATH/install expectation.

## Thesis Evidence

- Repeated design decision or point of agent confusion: The host/Workspace split appears in README, Gateway JSON, and session close output, but the actionable denial command was still easy to run in the wrong environment.
- User outcome or friction observed in the minimal slice: The deny-review-allow-retry loop could not be completed from the README path.
- Code workaround or exception being considered: None; fixes should preserve the exact learned-rule boundary.
- Current thesis that resolves it, or proposed thesis revision: Thesis 0 and Thesis 8 already require a useful denial and a fixed host-side review cue; no thesis revision is currently expected.
- Downstream product, architecture, security, Skill, catalog, and harness impact: Add tests and presentation updates so the existing thesis is executable in the first-use route.

## Reproduction or observation

```sh
task build
mkdir -p first-run-project
cd first-run-project
XDG_CONFIG_HOME=... XDG_STATE_HOME=... XDG_DATA_HOME=... tobari cluster up
XDG_CONFIG_HOME=... XDG_STATE_HOME=... XDG_DATA_HOME=... tobari
curl -sS -w '\nhttp=%{http_code}\n' -X PUT https://example.com/quickstart
exit
XDG_CONFIG_HOME=... XDG_STATE_HOME=... XDG_DATA_HOME=... tobari policy review --tail 100
XDG_CONFIG_HOME=... XDG_STATE_HOME=... XDG_DATA_HOME=... tobari cluster denials --tail 10 --format json
```

Expected: the denial is learnable and `policy review` can make one exact host decision.

Observed: the denial was not learnable and the review queue was empty.

## Security and public-boundary notes

- Assets and side effects involved: XDG configuration/state/data, Docker cluster and Workspace resources, OPA policy data, Gateway audit logs.
- Credentials or confidential data involved: none; examples use `example.com`.
- New dependencies, destinations, files, processes, or generated content: none expected.
- External schema provenance, publication rights, and drift evidence: not applicable.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency, and cancellation facts: policy candidate and denial commands remain bounded-window complete outputs; policy mutations remain exact reference-bound writes.
- Publication and licensing concerns: none expected.

## Glossary

- First-use route: the README path from source build through cluster startup, Workspace entry, one denied request, host review, and retry.
- Learnable denial: a validated Gateway denial that OPA marks eligible for one exact learned allow or deny rule.
