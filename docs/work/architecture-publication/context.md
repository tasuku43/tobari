# Work Context: Publish the second-wave architecture presentation

This file records verified facts and publication constraints for the static
architecture presentation. It does not redefine the product contract.

## Current behavior

- The repository is on `main` at the start of this packet; the initial worktree is clean.
- The numbered contracts define the current product line: CWD-first Workspace selection, one shared OPA-backed Gateway for supported HTTP/HTTPS, default-deny policy learning, and one host-selected Context with physically separated stores.
- `docs/README.md` is the durable documentation map and currently links the numbered contracts and work-packet template.
- No `docs/architecture-site/` directory or `docs/work/architecture-publication/` packet exists at the start of this change.
- After the initial clean status, unrelated shared-worktree changes appeared outside this packet: `README.md`, first-wave work packets under `docs/work/cli-catalog-audit/`, `policy-review-tty/`, `runtime-bash-shell/`, and `work-packet-retirement/`, plus a new `docs/work/quickstart-runtime-docs/` packet. They were preserved and never staged.

## Relevant structure

- Entry point: `docs/architecture-site/index.html`, served as a static page.
- Domain rule: No production domain or application behavior changes; the page mirrors existing contract vocabulary.
- Application use case: Not applicable; this is documentation-only publication.
- Infrastructure boundary: `.github/workflows/architecture-pages.yml` uses the official Pages artifact/deploy boundary and does not build or publish runtime artifacts.
- CLI catalog or presentation: No catalog change; the page explicitly remains a presentation layer rather than a second contract.
- Existing tests and harness checks: `task check`, `task public:check`, and `task release:check`; a bounded local Python E2E validates the static artifact and a local HTTP fetch.

## Constraints

- Product and compatibility: Preserve the documented terms Workspace, Gateway, OPA, Context, canonical root, default deny, exact candidate reference, and explicit host action.
- Architecture and security: Show CLI → Application/Domain and Infrastructure → Domain without inventing an Application → Infrastructure dependency; show the agent as untrusted, Gateway/OPA as trusted, and direct egress absent.
- Platform, dependency, or release: Use plain HTML/CSS, system fonts, no JavaScript/CDN, and pin workflow actions to immutable commits. GitHub Pages is the only publication destination described.
- Public repository: English, synthetic, no credentials/private URLs/machine paths/vendor endorsements/copied external material, and local Markdown links must resolve to repository files.

## External facts

- GitHub Actions `configure-pages` tag `v5` resolved to commit `983d7736d9b0ae728b81ab479565c72886d7745b`, `upload-pages-artifact` tag `v3` to `56afc609e74202658d3ffba0e8f6dda462b719fa`, and `deploy-pages` tag `v4` to `d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e`. Checked on 2026-08-03 with `git ls-remote --refs --tags` against the corresponding public `github.com/actions/*` repositories; the immutable SHAs are used in the workflow.

## Unknowns

- [x] GitHub Pages project settings are an external repository-owner concern; this packet only adds the least-scope workflow and does not enable or deploy Pages from the agent.
- [x] Browser-specific visual QA is outside the bounded repository E2E; semantic HTML, CSS balance, local links, local serving, and fetched content are checked deterministically.

## Thesis evidence

- Repeated design decision or point of agent confusion: The same architecture claims are distributed across theses, product, architecture, security, harness, release, and readiness documents; a visual map can reduce discovery cost only if it clearly defers to those sources.
- User outcome or friction observed in the minimal slice: The publication request calls for a second-wave map of trust, lifecycle, policy recovery, and runtime customization rather than another command reference.
- Code workaround or exception being considered: None. The page uses a static presentation and does not add a runtime or CLI escape hatch.
- Current thesis that resolves it, or proposed thesis revision: Thesis 0 and Thesis 7: bounded autonomy should be easier to understand, and claims must remain executable and contract-backed.
- Downstream product, architecture, security, Skill, catalog, and harness impact: No durable contract or catalog change; the packet adds static publication evidence and local artifact validation only.

## Reproduction or observation

```sh
# Minimal deterministic command
git status --short --branch
git log -1 --oneline --decorate
find docs/architecture-site docs/work/architecture-publication -maxdepth 2 -type f -print
```

Expected initial result: `main`, a clean worktree, and no files under the two
new paths. The final bounded E2E and gate results are recorded in
[e2e-transcript.md](e2e-transcript.md).

## Security and public-boundary notes

- Assets and side effects involved: Public HTML/CSS and a GitHub Pages artifact upload/deploy action. No runtime, Docker, policy, credential, or user data side effect is added.
- Credentials or confidential data involved: None. The workflow uses the standard GitHub token permissions required by Pages; no repository secret is read or printed.
- New dependencies, destinations, files, processes, or generated content: One local stylesheet, one local README, one workflow, and one temporary evidence packet. The local E2E starts only a loopback Python HTTP server.
- External schema provenance, publication rights, and drift evidence: No external schema or copied content. The Pages action SHAs are pinned public workflow dependencies; their tag-to-SHA checks are recorded above.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency, and cancellation facts: The site is a complete static artifact with no pagination or mutation API. The workflow uses Pages artifact/deploy semantics and cancels superseded runs through its concurrency group.
- Publication and licensing concerns: Page copy is original synthetic prose and CSS. It uses no third-party assets and does not imply vendor endorsement.
- Gate blocker: The clean-checkout `task release:check` run returns 201 after reaching ShellCheck on the existing `scripts/test-integration.sh:238` `SC2183`/`SC2016` warnings. That production script is outside the allowed scope, so it was not modified; public and full gates pass.

## Glossary

- **Workspace:** The human-facing name for one CWD/canonical-root-bound Tobari; its logical existence survives a child session exit.
- **Gateway:** The trusted HTTP/HTTPS enforcement point that normalizes and forwards only after policy approval.
- **OPA:** The trusted policy decision point shared by the installation-local cluster.
- **Context:** One host-selected execution setup that references separated agent, policy, credential, and runtime inputs.
