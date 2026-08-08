# Work Context: Publish and pin the trusted authentication images

## Verified facts

- Local `main` equals `origin/main` at `dcb73b5` before publication and holds the complete Auth Broker change as uncommitted work.
- Gateway source and its embedded snapshot changed in the same worktree.
- The current pinned Gateway digest predates those source changes.
- The Gateway GHCR package is publicly readable through an anonymous registry token flow.
- The Auth Broker package does not yet exist publicly; anonymous token acquisition returns 403.
- Both workflows publish `latest`, `main`, and `sha-<commit>` only on a push to `main`, with package-write permission confined to the publish job.
- Routine startup already requires immutable digest references and validates image API/role labels, non-root execution, entrypoint, platform, and resolved digest.

## Constraints

- The initial implementation commit must retain `AUTH_BROKER_IMAGE=unpublished`; its successful workflow output is the authoritative digest input for the promotion commit.
- The promotion commit may trigger another development image publication because `versions.env` is a workflow input. The reviewed first image remains valid immutable authority; moving tags are not authority.
- Registry checks must use an anonymous bearer-token flow, not a locally authenticated Docker configuration.
- No credential, key, vault, handle, device code, or authenticated output may enter source, image layers, logs, or evidence.

## Unknowns to resolve

- [ ] Exact workflow run IDs and manifest digests for the implementation commit.
- [ ] Whether both packages inherit public repository visibility automatically.
- [ ] Whether official-image integration passes with both promoted digests.
