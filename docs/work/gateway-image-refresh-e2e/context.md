# Work Context: Restore the regular Gateway-image integration path

## Verified facts

- The policy decision management change is committed on `main` as `b6327e1`.
- `TOBARI_INTEGRATION_GATEWAY_SOURCE=1 ./scripts/test-integration.sh` completes
  with `integration: OK`, including Allow reset, Deny reset, default-deny
  checks, and re-review.
- The default `./scripts/test-integration.sh` fails at
  `GET http://mock-upstream:8080/allowed` with HTTP 403 and Gateway audit
  reason `request did not match an allow rule`.
- Gateway and OPA are healthy when the failure occurs; OPA returns a successful
  decision response and the Gateway logs a normal deny audit.
- The default failure reproduces from the pre-feature commit `4f087fd`.
- `./scripts/check-gateway-source.sh` and the full `task check` pass, so the
  checked-in source and embedded snapshot are internally equal.
- The explicit source-build path is the documented development/recovery path;
  it must not become an implicit fallback for ordinary startup.

## Constraints

- Preserve deny-by-default and the verified-image boundary.
- Keep the regular integration gate meaningful: it must exercise the pinned
  image, not a source build hidden inside the test.
- Treat image publication, digest changes, and release metadata as separate
  authority-sensitive work.

## Unknowns

- Whether the pinned digest in `internal/infra/runtimeassets/assets/versions.env`
  was published before the current Gateway policy-input contract.
- Whether the failure is caused by image content, digest metadata, or a local
  Docker image cache/architecture selection.
- Which release workflow should refresh the image and update the reviewed
  digest.

## Reproduction

```sh
./scripts/test-integration.sh
TOBARI_INTEGRATION_GATEWAY_SOURCE=1 ./scripts/test-integration.sh
```

Expected after repair: both commands reach and pass the same policy-learning
assertions; source mode remains an explicit diagnostic comparison.
