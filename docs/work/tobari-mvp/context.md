# Work Context: Deliver the Tobari MVP

## Current behavior

- The repository was bootstrapped from the foundry template to identity-ready
  Tobari on 2026-07-29.
- The inherited CLI provides only template help, version, doctor, and sample
  commands; no Docker runtime exists yet.
- Bootstrap `fast` and `public` gates pass with Go 1.26.5.

## Relevant structure

- Entry point: `cmd/tobari/main.go`
- Domain: `internal/domain`
- Application: `internal/app`
- Infrastructure: `internal/infra`
- CLI catalog: `internal/cli/catalog.go`
- Gate: `scripts/check.sh`

## Constraints

- One read-write root and one long-lived Realm.
- Docker Engine only; no Docker Desktop-specific APIs.
- Explicit proxy and deny-by-default OPA.
- Realm is untrusted and receives no Docker socket, host credentials, or
  Gateway credentials.
- Trusted repository prose remains English.

## External facts

- mitmproxy, “How mitmproxy works,” checked 2026-07-29: explicit HTTPS begins
  with HTTP CONNECT, then mitmproxy terminates client TLS and establishes a
  separate upstream TLS session:
  https://docs.mitmproxy.org/stable/concepts/how-mitmproxy-works/
- mitmproxy, “Certificates,” checked 2026-07-29: intercepted clients must trust
  the installation-specific mitmproxy CA:
  https://docs.mitmproxy.org/stable/concepts/certificates/
- mitmproxy 12.2.3 was the current release, but its official arm64 image
  reproducibly exited with SIGILL before addon loading on the supported local
  Colima VM. The immediately preceding stable 12.1.2 image runs with Python
  3.13.7 on the same VM, so the MVP pins its multi-platform manifest digest.
- Git, “git-config,” checked 2026-07-29: Git supports explicit HTTP proxy and
  proxy CA configuration:
  https://git-scm.com/docs/git-config

## Unknowns

- [ ] Confirm the locally available Docker Engine and integration-test platform.
- [ ] Pin current stable image versions/digests from official sources.
- [ ] Validate `gh`, Git, curl, Go, and Python trust the Realm CA in practice.

## Thesis evidence

- HTTPS L7 inspection is the critical slice; the topology must implement
  CONNECT plus two TLS sessions, not replace the upstream protocol with HTTP.
- Network denial, not process cooperation, is the proxy-bypass enforcement.

## Security notes

- Assets and accepted risks are promoted to `docs/03_security_model.md` and
  `docs/THREAT_MODEL.md`.
- All tests use synthetic credentials and local upstreams.
- No provider schema or external account is required.

## Glossary

- Realm: the single untrusted execution container.
- Gateway: trusted mitmproxy policy enforcement point.
- OPA: trusted policy decision point.
- Control network: internal network shared only by Gateway and OPA.
