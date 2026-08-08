# Work Plan: Publish and pin the trusted authentication images

1. Run the implementation, security, and public-boundary gates while the broker remains in its honest unpublished bootstrap state.
2. Commit the complete Auth Broker slice and push it to `main` to trigger both trusted-image workflows.
3. Wait for the Auth Broker and Gateway publish jobs, then independently inspect the immutable tags, OCI index media type, Linux amd64/arm64 members, labels, entrypoints, users, and anonymous readability.
4. Replace the Auth Broker marker and stale Gateway reference with the reviewed immutable digests; update bootstrap-only durable documentation and tests.
5. Run official-image integration and all completion/publication/release gates.
6. Commit and push the digest promotion, verify the resulting main checks, then remove temporary packets in the final handoff commit.

## Risks and controls

- A failed workflow leaves routine startup fail-closed; do not invent a digest.
- A private package is not a public runtime artifact; verify anonymously.
- A moving tag can change after promotion; record only immutable digest references.
- Digest promotion changes embedded runtime identity; rerun integration rather than relying on local development-image evidence.
