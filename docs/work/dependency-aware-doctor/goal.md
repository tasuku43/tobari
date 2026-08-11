# Work Goal: Make doctor diagnose prerequisites without false blame

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Project Theses 0, 5, 6, and 7
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: dependency-aware-doctor packet agent
- Target: Explicit and Resumable UX program, recovery lane
- Related ADRs: None

## Outcome

`doctor` always reports its complete bounded diagnostic set, distinguishes a
failed check from a check blocked by an unmet prerequisite, avoids blaming
policy or authentication state that could not be observed, and gives a
concrete recovery step before the next diagnostic run.

## Why now

With the Docker CLI present but Engine unavailable, hands-on review reported
Docker Engine and Compose failures plus an OPA policy failure that suggested
broken Rego/XDG sharing, even though OPA could not run. The only next action was
to run `tobari doctor` again. With Docker absent, doctor stopped after one
check and omitted independently observable host/configuration checks, contrary
to the complete-report contract.

## Non-goals

- Do not start Docker, reconcile the cluster, initialize policy, create a root
  key, unlock the broker, or repair state.
- Do not turn warnings into failure or hide real independent failures.
- Do not add platform-specific shell execution or arbitrary external commands.
- Do not make doctor part of the happy-path first session.

## Acceptance criteria

- [x] The diagnostic model represents pass, warning, fail, and blocked/skipped
      with one typed prerequisite reason and no presentation inference.
- [x] Every invocation returns the complete declared check inventory; one
      missing dependency does not suppress independent checks.
- [x] A dependent check cannot report policy syntax, broker corruption, or
      runtime failure when its prerequisite was not available.
- [x] Summary status and exit remain unhealthy when a required check fails,
      while blocked checks do not duplicate the root failure.
- [x] Recovery names the unmet prerequisite in plain language and gives the
      exact next Tobari command to run after it is corrected; it is not a bare
      self-loop.
- [x] Docker missing, Engine unavailable, cluster absent, invalid policy,
      broker locked, and healthy-with-warnings fixtures preserve distinct facts.
- [x] Doctor remains read-only and creates no Context, policy, credential,
      root-key, lock, or runtime state.
- [x] Text, TSV, JSON, agent help, and catalog fault declarations agree.
- [ ] `task check` and `task security` pass.

## Governing documents

- Thesis: `docs/00_theses.md`, Theses 0, 5, 6, and 7
- Product contract section: Input and path contract, Output and exit contract
- Architecture/security: doctor composition and observational mutation policy
- Existing ADR: None

## Completion definition

The work is complete when all dependency combinations have deterministic
fixtures, doctor remains observational, recovery is actionable, required
profiles pass, and this temporary packet is removed.

## Verification state

Implementation verification is complete and `task security` passes. `task
check` is deferred only at the repository's protected Pages JSON-schema table
drift: the English and Japanese Pages tables remain stale for Doctor report
schema 2 and several independently advanced schemas. This packet remains
Active because its scope forbids changing `docs/architecture-site/**`.
