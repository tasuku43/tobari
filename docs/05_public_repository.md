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
- Standard native agent login is performed by the provider's own pinned CLI;
  Tobari publishes no provider credential acquisition or refresh adapter.

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
- Pin `github.com/creack/pty` v1.1.24 for the reviewed Unix attached-terminal
  relay. Its MIT text is retained in root `THIRD_PARTY_NOTICES`, and release
  archives include that file beside the project license.
- Experimental Auth Broker source and tests remain subject to the public secret
  guard, but no Broker image or provider adapter is a release artifact.
- For the agent-ready base runtime, retain GitHub CLI and AWS CLI checks and
  bind Claude Code 2.1.220 and Codex 0.147.0 to their per-platform artifact
  locks. The base workflow remains validation-only, and the protected Release
  workflow has no base publication path. The base declares `NOASSERTION` and
  is built by the user from the pinned embedded recipe. Custom Context runtime
  contents are outside the public base inventory and do not create
  redistribution evidence.
- Keep the pinned `auth-provider.v1` schema fixture repository-authored,
  synthetic, MIT-licensed, and digest-matched. It must contain no real account,
  hostname, file path, or credential.
- Treat automated dependency or schema pull requests as untrusted changes that must pass the same checks.

## Public release review

Before each public release, verify:

- the tag points to reviewed source;
- all required profiles pass;
- `version --format json` reports the release version, full source commit,
  embedded resolver, selected pin APIs, and compatibility expected by the
  release gate, with empty repository-development recovery fields;
- supported-platform artifacts are complete;
- checksums and any provenance or signatures are present and verified;
- archives contain only intended files;
- installation instructions work in a clean environment;
- release notes disclose contract and security impact;
- no artifact, Formula, URL, log, or metadata contains a forbidden identifier.

Tobari publishes no OCI images. Validation workflows remain cache-only and
have no package-write, registry-login, or push path. Routine CLI startup builds
or reuses source-derived local Gateway and runtime images, then checks their
component API, role, non-root user, entrypoint, and architecture before use.
All Tobari-owned component APIs are V1; source and release metadata record no
generated image digest authority.

Source-build identity may contain only the deterministic public fields above.
Artifact and repository scans reject local absolute paths, usernames, branch
names, dirty content, credentials, or invented digest authority; the literal
`unknown` commit remains visibly incompatible rather than becoming a release
identity.

Contributors use `task build` for the local agent-ready runtime,
content-addressed local service images, and `bin/tobari`. Release packaging
fails if a component lock, Tobari GHCR reference, package-write permission, or
registry push path reappears.

See [Release](06_release.md) for the artifact workflow.

Stable CLI distribution targets the shared `tasuku43/homebrew-tap` repository.
The protected release workflow propagates the exact audited Formula asset into
a Formula-only pull request using a GitHub App token restricted to that one
repository. It does not render a second Formula authority in the tap or push
tap `main` directly. Dry runs and prereleases cannot obtain the tap token or
create that pull request.

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

English remains the canonical root locale for public product claims. The site
also publishes one reviewed Japanese counterpart for every route below
`/ja/`; it performs no runtime translation. Publication fails when either
locale is missing a same-topic route, when Japanese has an orphan route, or
when the built language metadata and alternate links do not preserve the topic
under both `/` and `/tobari/` bases.

Minimum first-public-push checklist:

- [ ] Repository history and all refs were reviewed.
- [ ] Theses and product contract are concrete.
- [ ] Security model covers every real side effect and credential.
- [ ] License and contribution terms were approved.
- [ ] Private reporting and maintainer contacts exist.
- [ ] Fixtures and docs contain only synthetic data.
- [ ] Auth Broker source, image layers, tests, and manual validation evidence
      contain no real account material, SSO/token/role state, device code,
      Codex or Claude credential state, signed authorization field, handle,
      key, or vault.
- [ ] The agent-ready base's tool archives, checksums, licenses, notices, and
      both architectures were reviewed; unlisted custom-Context tools are absent.
- [ ] The combined Codex and Claude base remains permanently local/cache-only;
      its registry publication path is absent and a local build or passing
      version smoke test is not redistribution evidence.
- [ ] Native Anthropic account-login distribution has explicit legal/product
      approval and any provider approval required by the applicable terms.
- [ ] Full history and artifacts passed secret and identifier review.
- [ ] `task check`, `task security`, and `task public:check` passed.
- [ ] A human reviewer approved publication.
