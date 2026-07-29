## Outcome

Describe the user or maintainer outcome. Explain why this change belongs in the
project; do not only list edited files.

## Thesis impact

- [ ] The change follows the current theses without changing them.
- [ ] I updated `docs/00_theses.md` and propagated the decision into downstream
      architecture, contracts, tests, or agent guidance.

State which thesis guided the design and note any tension or trade-off:

## Contracts and safety boundaries

Describe changes to command discovery, opaque ID producer/consumer flows,
effects, external destinations, output schemas, compatibility, or release
artifacts. Write `None` when there is no contract change.

## Public repository check

- [ ] This PR contains no secret, credential, private hostname, internal URL,
      employee/customer data, or proprietary identifier.
- [ ] Examples use synthetic values and public domains.
- [ ] New safety or security claims are mapped to enforceable checks.

Do not paste sensitive evidence into this PR. Follow `SECURITY.md` for private
vulnerability reporting.

## Validation

- [ ] `./scripts/check.sh fast`
- [ ] `./scripts/check.sh full`
- [ ] `./scripts/check.sh security` when dependencies, auth, I/O, or trust
      boundaries changed
- [ ] `./scripts/check.sh release` when packaging or distribution changed
- [ ] `./scripts/check.sh public` when metadata, docs, examples, or repository
      configuration changed

List any intentionally skipped profile and why:
