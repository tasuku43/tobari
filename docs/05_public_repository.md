# Public Repository Boundary

Public publication is irreversible in practice. Removing a secret, private URL,
personal record, or proprietary file in a later commit does not remove it from
clones, caches, logs, or forks. This guide treats source, fixtures, history,
licensing, workflows, and release metadata as one public boundary.

## Material that must not cross the boundary

- Credentials, tokens, root keys, encrypted vaults, project handles, device
  codes, certificates, cookies, or authenticated URLs.
- Private domains, tenant names, repository names, organization identifiers, or internal documentation links.
- Real customer, employee, account, calendar, message, file, or operational data.
- Private incident details, vulnerability information, or security assumptions useful to an attacker.
- Proprietary source, generated output, schemas, examples, or screenshots without publication rights.
- Internal deployment steps, access groups, approval routes, and contact lists.
- Local absolute paths, usernames, shell history, editor state, build caches, or debug logs.

Use `example.com`, synthetic identifiers, fixed timestamps, and invented content in fixtures and documentation.

## Executable public guard

`task public:check` scans publishable regular files and fails before reading
repository-controlled content when a symbolic link or special file is present.
It rejects configured forbidden identifiers, unresolved repository
placeholders, secret-like values, and missing required public files.

Repository shape checks also reject Claude-specific policy paths and root-level
binary build artifacts. Tobari has one canonical `AGENTS.md` policy and a Codex
harness; a parallel `CLAUDE.md` or `.claude/` tree is a failed hygiene check.
The full-tree shape walk does not treat deliberately ignored local files such
as `.env` as publishable content, but symbolic links and special files still
fail closed.

Every local Markdown link must use a canonical repository-relative path that stays inside the repository and resolves to a publishable regular file without crossing a symbolic link. External URLs, `mailto:` links, same-document fragments, and examples inside fenced code blocks are outside this local-file check. External link availability still requires review because network state is not reproducible inside the repository gate.

The security scan recognizes common token formats, credential-bearing URLs, authorization headers, and secret assignments, including quoted JSON keys and values. Example values are exempt only when they use an explicit whole-value convention such as `dummy-value`, `example-token`, `${ACCESS_TOKEN}`, `env.ACCESS_TOKEN`, `null`, or `[redacted]`. A marker embedded in a plausible real value, such as `production-dummy` or `contest-token`, is not an exemption.

These checks are a repository-specific backstop, not a claim that regular expressions can prove the absence of secrets. Public history and artifacts still require the approved full-history secret scanner and human confidentiality review.

## History and secret review

For a new public repository, review the complete history, not only `HEAD`.

- Confirm the first commit contains only reviewed public content.
- Scan all refs and generated artifacts with the approved secret scanner.
- Search for forbidden identifiers defined in repository policy.
- Inspect unusually large or binary objects.
- Verify ignored local files were never committed.
- Review workflow logs and release artifacts before making them public.

If sensitive material entered history, stop publication. Coordinate revocation and history remediation before any push; deleting the working-tree file is insufficient.

## Rights and license review

Before publication:

- Confirm who owns every copied or generated component.
- Choose and commit an explicit project license.
- Review dependency and bundled-asset licenses.
- Decide the inbound contribution policy, such as MIT inbound licensing, DCO, or CLA.
- Add required notices and attribution.
- Confirm names, logos, and examples do not imply unauthorized endorsement.

Tobari is licensed under MIT as recorded in `LICENSE` and project metadata.

## Security disclosure readiness

- Enable GitHub private vulnerability reporting or publish another private channel.
- State supported versions and response expectations in `SECURITY.md`.
- Do not ask reporters to disclose sensitive details in public issues.
- Document project-specific assets, trust boundaries, and limitations.
- Ensure maintainers can revoke credentials and pull or replace a release if needed.

## Community health

A public repository should provide, as appropriate:

- README with supported use and maturity status;
- LICENSE;
- CONTRIBUTING;
- CODE_OF_CONDUCT;
- SECURITY;
- support expectations;
- issue and pull-request templates;
- ownership and review rules;
- versioning and deprecation policy.

Tobari's community documents must contain current contacts, ownership, and
support promises before maintainers invite external users.

## Dependency and automation review

- Pin third-party workflow actions to immutable revisions.
- Pin security and generation tools to reviewed versions.
- Grant workflow tokens the minimum required permissions.
- Separate untrusted pull-request execution from privileged release jobs.
- Do not expose secrets to forked pull requests.
- Verify dependency integrity, licenses, and known vulnerabilities.
- For the Auth Broker image, verify the pinned GitHub CLI version, both Linux
  architecture checksums, bundled MIT license, and third-party notice; never
  substitute a network-fetched checksum during the build.
- Keep the pinned `auth-provider.v1` schema fixture repository-authored,
  synthetic, MIT-licensed, and digest-matched. It must contain no real account,
  hostname, file path, or credential.
- Treat automated dependency or schema pull requests as untrusted changes that must pass the same checks.

## Public release review

Before each public release, verify:

- the tag points to reviewed source;
- all required profiles pass;
- version and commit metadata are correct;
- supported-platform artifacts are complete;
- checksums and any provenance or signatures are present and verified;
- archives contain only intended files;
- installation instructions work in a clean environment;
- release notes disclose compatibility, security, and migration impact;
- no artifact, Formula, URL, log, or metadata contains a forbidden identifier.

For an official OCI image publication, also verify the canonical image source,
parent/base digest lock, supported architectures, runtime labels, license
metadata, downloaded-artifact notices, and the exact moving-versus-immutable
tag behavior. The main-channel base workflow is a continuous development
publication; it must not be described as a stable SemVer release or grant the
image any authority beyond its declared root filesystem.

The Auth Broker is a credential-bearing runtime, so its public image requires
additional negative evidence: no credential, live account fixture, GitHub CLI
configuration, root key, vault, runtime-issued handle, device code, or
authenticated output is present in source, layers, workflow artifacts, logs,
or notices. Deterministic synthetic handle canaries are permitted only in
tests. Its canonical
source/snapshot drift check, pinned GitHub CLI checksums and license, fixed
non-root labels/entrypoint, and Linux amd64/arm64 build must pass. Pull-request
validation is cache-only and has no package-write permission; only the
main-push job may publish moving `latest`/`main` and immutable
`sha-<commit>` development identities. Routine CLI startup must use a reviewed
manifest digest rather than those moving tags.
The first Auth Broker publication is complete. Public and release validation
now require its reviewed `ghcr.io/tasuku43/tobari/auth-broker@sha256:...`
manifest reference; an unpublished marker, invented digest, wrong repository,
or moving identity is a public-boundary failure. Future promotion still
requires independent platform, metadata, anonymous-readability, and digest
review rather than trusting a moving tag or successful workflow alone.

See [Release](06_release.md) for the artifact workflow.

## Automated and manual gates

`task public:check` is required, but it cannot decide ownership, confidentiality context, trademark use, or whether an example reveals an internal process. The release owner records manual review evidence in the release work packet.

The public documentation is built from `docs/architecture-site/` into static
HTML, CSS, and repository-owned assets. It loads no runtime CDN, web font,
analytics, or tracker. CLI and component-version references are generated from
the Catalog at the immutable product snapshot named in
`docs/architecture-site/source-snapshot.txt`; every page displays that product
snapshot separately from the documentation build commit. Stale generated data
or product evidence linked to a different commit is a failed gate. Pull
requests receive read-only build, internal-link,
accessibility, no-JavaScript, theme, motion, keyboard, mobile, and base-path
checks. Pages upload and deployment are separate jobs: only a successful push
to `main` uploads generated `dist/`, and only the deploy job receives
`pages: write` and `id-token: write`. No workflow secret is required.

Minimum first-public-push checklist:

- [ ] Repository history and all refs were reviewed.
- [ ] Theses and product contract are concrete.
- [ ] Security model covers every real side effect and credential.
- [ ] License and contribution terms were approved.
- [ ] Private reporting and maintainer contacts exist.
- [ ] Fixtures and docs contain only synthetic data.
- [ ] Auth Broker source, image layers, tests, and manual validation evidence
      contain no real account material, device code, handle, key, or vault.
- [ ] Full history and artifacts passed secret and identifier review.
- [ ] `task check`, `task security`, and `task public:check` passed.
- [ ] A human reviewer approved publication.
