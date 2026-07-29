# Release Model

Tobari follows Semantic Versioning and initially supports source builds on
Linux and macOS with Docker Engine. Colima is the documented macOS runtime;
Docker Desktop-specific behavior is not required.

## Artifacts

The Go CLI remains a pure-Go binary for Linux and macOS on amd64 and arm64.
Windows CLI archives are buildable by the inherited packaging harness but the
MVP runtime is not supported on Windows because bind mounts, Unix ownership,
TTY behavior, and container networking have not been validated there.

Runtime assets are embedded in the binary and materialized into versioned state
before Docker builds. The embedded Realm, Gateway, OPA policy, and compose
inputs are therefore bound to the CLI source revision. Container base images
are pinned to reviewed immutable versions or digests.

## Compatibility

Before v1.0, breaking changes require release notes but not a deprecation
window. The stable boundaries are command paths, exit meanings, Docker labels,
state schema, configuration keys, OPA input/decision schemas, audit fields, and
preservation of the Realm home volume by default.

## Publication

Tags use `vMAJOR.MINOR.PATCH`. Release publication is create-only and runs the
full, security, release, public, policy, Gateway, and Docker integration gates
from the exact tagged revision. GitHub Releases publish checksums with each CLI
archive. Container images are built locally by `tobari up` in the MVP; no
registry publication is promised.

Tobari does not yet claim code signing, notarization, SBOM attestation, or
externally verifiable build provenance. Checksums protect selected artifact
integrity but do not identify the builder.

## Ownership and security

The repository owner performs release and confidentiality review. Vulnerability
reports use GitHub private vulnerability reporting as documented in
[Security Policy](../SECURITY.md). A broken or vulnerable release is replaced
by a new version; assets for an existing tag are not overwritten.

## Required gates

Before tagging:

```sh
task check
task security
task release:check
task public:check
task policy:test
task gateway:test
task integration:test
```

The first public release also requires a clean-environment Colima or Linux
Quick Start run and a human review of history, dependencies, licenses, and
generated artifacts.
