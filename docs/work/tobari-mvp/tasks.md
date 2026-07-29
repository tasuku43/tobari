# Work Tasks: Deliver the Tobari MVP

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand and decide

- [x] Read governing documents and both required skills.
- [x] Bootstrap and verify Tobari identity.
- [x] Record product, architecture, security, authentication, and API contracts.
- [x] Select explicit CONNECT interception with mitmproxy and network-enforced bypass denial.
- [ ] Pin external image/tool versions and record ADRs.

## Implement

- [ ] Add Gateway addon and tests.
- [ ] Add Rego policy and tests.
- [ ] Add Realm and Gateway container assets.
- [ ] Add domain and application lifecycle behavior.
- [ ] Add Docker and state infrastructure.
- [ ] Register and implement the public catalog.
- [ ] Add credential injection and redaction.
- [ ] Add Docker integration scenarios.
- [ ] Update capabilities, harness, CI, README, and operating docs.

## Verify

- [ ] Go focused tests pass. Evidence:
- [ ] Gateway tests pass. Evidence:
- [ ] Rego tests pass. Evidence:
- [ ] Docker integration passes. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] Quick Start is replayed. Evidence:
- [ ] Diff and cleanup are reviewed. Evidence:

## Hand off

- [ ] Promote every durable decision and delete this temporary packet.
- [ ] Commit the intended change.
- [ ] Push and open a draft PR when repository authentication permits.
