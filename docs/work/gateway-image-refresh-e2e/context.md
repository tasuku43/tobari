# Work Context: Restore the regular Gateway-image integration path

## Verified facts

- The policy decision management change is committed on `main` as `b6327e1`.
- The previous source-built Gateway replay completed with `integration: OK`,
  including Allow reset, Deny reset, default-deny checks, and re-review. That
  public fallback has now been replaced by the contributor-only `task
  build:dev` / `bin/tobari-dev` path.
- The default `./scripts/test-integration.sh` fails at
  `GET http://mock-upstream:8080/allowed` with HTTP 403 and Gateway audit
  reason `request did not match an allow rule`.
- Gateway and OPA are healthy when the failure occurs; OPA returns a successful
  decision response and the Gateway logs a normal deny audit.
- The default failure reproduces from the pre-feature commit `4f087fd`.
- `./scripts/check-gateway-source.sh` and the full `task check` pass, so the
  checked-in source and embedded snapshot are internally equal.
- The explicit local development path is `task build:dev`; it must not become
  an implicit fallback for ordinary startup.
- During the first-use repair, ordinary `cluster up` with the pinned image
  returned `permission_review_unavailable` for
  `PUT https://example.com/quickstart`, while the current Gateway source
  returned `permission_review_available` for the same request and produced a
  `policy review` candidate.
- The running pinned Gateway container did not contain the new source markers
  for TLS-established scheme normalization or the host-only review message.
- Static OPA evaluation with `scheme:"https", port:443, method:"PUT",
  path:"/quickstart"` is learnable; static OPA evaluation with `scheme:"http",
  port:443` is intentionally non-learnable.

## Constraints

- Preserve deny-by-default and the verified-image boundary.
- Keep the regular integration gate meaningful: it must exercise the pinned
  image, not a local dev image hidden inside the test.
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
task build:dev
bin/tobari-dev cluster up
```

Expected after repair: the default integration reaches and passes the same
policy-learning assertions as the contributor local Gateway image path.
