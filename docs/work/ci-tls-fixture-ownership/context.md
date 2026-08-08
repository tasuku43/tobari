# Work Context: Make the synthetic TLS fixture owner-consistent

- GitHub CI run `31260248390`, job `93109840457`, failed before the brokered
  HTTP scenario because `ssl.SSLContext.load_cert_chain` received
  `PermissionError` for the mounted synthetic key.
- The key is intentionally mode `0600` and its parent is mode `0700`, owned by
  the GitHub runner UID that generated it.
- The mock container inherited the image's default UID instead of the fixture
  owner UID. The local macOS/Colima mount did not expose the mismatch, while
  the Linux bind mount enforced it.
- The production Auth Broker and Gateway identities are unrelated to this
  synthetic test-container identity.
- A local validation attempt separately reproduced delayed positive network
  membership after service health. The initial three-second convergence bound
  was insufficient on Colima, so the positive read uses the harness-standard
  thirty-second bound; negative attachment checks remain immediate.
