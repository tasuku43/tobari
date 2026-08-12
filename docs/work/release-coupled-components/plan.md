# Plan

1. Define and validate a strict component-lock format.
2. Make the published resolver consume link-injected lock data while repository
   source retains only parent/base image inputs and API requirements.
3. Give development component images stable embedded-source-hash tags and
   build them only when missing.
4. Integrate component build/evidence/lock generation before CLI packaging in
   the release workflow.
5. Update release gates and governing documents, then run the full repository
   completion gate and release-specific profiles.

## Rejected alternatives

- Always build components on installed-user startup: slower and less reliable,
  and it weakens the common reviewed-image identity.
- Commit generated component digests: preserves the manual source/image/source
  cycle this change is intended to remove.
- Moving tags: do not provide immutable release authority.

## Risks

- Linker quoting of JSON is fragile; inject separate validated scalar fields.
- Multi-architecture digest creation must remain paired and source-revision
  bound; the lock tool rejects partial or mismatched evidence.
