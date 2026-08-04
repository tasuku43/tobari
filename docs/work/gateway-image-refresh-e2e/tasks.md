# Work Tasks: Restore the regular Gateway-image integration path

## Understand

- [x] Record the default-path 403 and the explicit-source success.
- [x] Confirm the failure reproduces before the policy decision management
      change.
- [x] Confirm the source snapshot and repository gates pass.

## Investigate

- [ ] Compare pinned image content, labels, digest, and source behavior.
- [ ] Trace the release publication and digest-review path.
- [ ] Determine whether the issue is image freshness, selection, cache, or
      architecture.

## Implement

- [ ] Apply the smallest release, image-selection, or harness-precondition fix.
- [ ] Add a regression check for the identified failure mode.

## Verify and hand off

- [ ] Default integration passes with the pinned image.
- [ ] Explicit source integration still passes as a diagnostic comparison.
- [ ] Required repository gates pass.
- [ ] Durable conclusion is promoted and this temporary packet is removed.
