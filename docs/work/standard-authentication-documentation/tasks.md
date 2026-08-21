# Work Tasks: Make authentication documentation match the standard profile

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [ ] Re-read governing docs, ADR 0044 and later native-login ADRs, README,
      build profile composition, and add-capability Skill at implementation
      time.
- [ ] Inventory every README Broker/provider/auth-command/release assertion and
      classify it against standard and experimental contracts.
- [ ] Verify every proposed standard example against standard catalog and
      Runtime availability.
- [ ] Find the existing profile/documentation guard to extend.
- [x] Confirm the public outcome and non-goals. Evidence: product-owner
      approval in the main design session on 2026-08-21.

## Decide

- [x] Make README standard-native and Workspace-owned. Evidence: product-owner
      approval on 2026-08-21.
- [x] State explicitly that standard has no `auth` command. Evidence:
      authoritative catalog plus product-owner approval.
- [x] Retain only a short explicit experimental Broker section using a
      development executable. Evidence: product-owner approval on 2026-08-21.
- [x] Reject standard Broker restoration or a caveat-only edit. Evidence:
      accepted security boundary and approved plan.
- [ ] Accept exact standard examples after standard Runtime/catalog replay.
- [ ] Accept the smallest non-duplicative deterministic documentation guard.

## Implement

- [ ] Add failing README/profile consistency tests.
- [ ] Rewrite standard security, Authentication, and build/profile prose.
- [ ] Replace unqualified Broker commands with native standard examples.
- [ ] Isolate experimental research and use only an explicit development
      executable for auth-command examples.
- [ ] Correct stale README release-identity statements only to match existing
      durable contracts; perform no release work.
- [ ] Update handoff evidence and authoritative links.
- [ ] Do not change production authentication code or profile composition.

## Verify

- [ ] Focused CLI/catalog/docs/README tests pass. Evidence:
- [ ] Standard help contains no auth namespace. Evidence:
- [ ] Experimental help is reachable only through its explicit development
      profile/executable. Evidence:
- [ ] README contains no unqualified standard `tobari auth` example. Evidence:
- [ ] README standard claims contain no Broker-required, vault, or handle
      ownership. Evidence:
- [ ] Human-handoff scorecard agrees with the final examples. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Durable documentation and tests agree with the standard profile.
- [ ] Temporary diagnostics are removed.
- [ ] The packet is removed in the same completion commit.
- [ ] The implementation is one concern-specific commit on main.
