# Work Plan: Publish the second-wave architecture presentation

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Create one semantic, responsive `index.html` with one local `styles.css`.
Organize the page as a guided narrative: dependency direction, trust
boundaries, Workspace lifecycle, policy recovery, and Context runtime
customization. Use text-first HTML structures with CSS diagrams and accessible
descriptions so the presentation remains useful without images, JavaScript, or
remote assets. Add a concise documentation-map link and a Pages workflow whose
artifact path is exactly `docs/architecture-site`.

## Alternatives considered

### Alternative A: Static HTML/CSS presentation

Chosen because it is dependency-light, reviewable in the repository, safe to
serve from Pages, and can express the architecture without adding a second
runtime or build tool.

### Alternative B: Client-rendered diagram or slide framework

Not chosen because a remote or bundled JavaScript framework would add a
publication dependency, increase the public attack surface, and make the
architecture less inspectable as plain source. It is also unnecessary for the
five requested relationships.

## Design

### Public contract

This is a public, read-only presentation rather than a CLI capability. It has
no command inputs, opaque-reference producers or consumers, authentication,
mutation effect, pagination, or structured runtime failure contract. Its
recovery behavior is navigational only: stable in-page anchors and a source
note that defers to the numbered documents. The page must not imply that a
display position, previous allow, profile name, or visual label grants
authority.

### Layer changes

- Domain: None.
- Application: None.
- Infrastructure: One GitHub Pages workflow uploads the repository-owned static directory only; no runtime adapter changes.
- CLI and catalog: None. `docs/README.md` receives one concise link to the static presentation.

### Data and control flow

The browser requests `index.html`, which references only the sibling
`styles.css`. The Pages workflow checks out the repository, configures Pages,
uploads `docs/architecture-site`, and deploys that artifact. No page asset is
generated from Go, Docker, policy, credentials, or external network data.

### Error and cancellation behavior

The local E2E fails if HTML parsing, CSS balance, local links, required content,
or the loopback fetch fails. The Pages workflow is complete-artifact
publication with no application retry semantics; its concurrency group cancels
superseded runs. A Pages configuration or environment problem is external to
this repository change and must be reported rather than worked around by
adding a push or deploy command to the agent.

### Security and public boundary

The page contains synthetic architecture language only. No credentials,
private URL, absolute machine path, personal data, vendor endorsement, remote
font, CDN, JavaScript, image, or copied external text is included. Actions are
pinned to immutable public commit SHAs. The workflow token permissions are
limited to `contents: read`, `pages: write`, and `id-token: write`, and the
artifact input is the exact site directory.

## Implementation slices

1. Contract and failing tests: derive page terms from docs/00 through docs/06 and docs/09; define the bounded local E2E assertions.
2. Domain and application behavior: not applicable for documentation-only work.
3. Infrastructure adapter: add the least-scope Pages workflow with pinned actions.
4. CLI catalog and presentation: add static HTML/CSS and the concise docs map link; no catalog change.
5. Harness and documentation: add the evidence packet, run E2E and required gates, and review the exact path set.

## Verification

- Unit and contract tests: `task check` and the repository's existing contract/harness checks.
- Negative side-effect tests: Inspect the workflow path and run a source scan proving no remote asset, script, or unscoped artifact path exists.
- Opaque-reference and complete-pagination tests: Not applicable; the page is read-only and non-interactive.
- Structured output, hostile-output, and recovery tests: HTML/CSS parser checks, internal-anchor checks, fetched-term checks, and no-secret/public-boundary gates.
- Agent-readiness scenario and discovery-round-trip count: The page has zero command discovery round trips and only deterministic section navigation; it does not claim to replace the executable agent-readiness scenario.
- Human-handoff scorecard for setup/authentication candidates: Not applicable; no setup or authentication candidate is introduced.
- Manual observation: Review the rendered local page at loopback and inspect semantic headings, focus order, responsive CSS, and reduced-motion behavior.
- Required profiles: `task check`, `task public:check`, and `task release:check`.
- Generated-diff or artifact checks: `git diff --check`, exact allowed-path review, and Pages artifact path review.

## Rollout and rollback

The workflow runs on `main` changes to the static directory or workflow file
and can be invoked manually. Rollback is a normal reviewed commit reverting
the site/workflow; the agent does not push, deploy, or alter branch history.
No application state or CLI compatibility state changes.

## Documentation promotion

No durable decision requires promotion. The page explicitly points readers
back to the numbered contracts, and this packet retains only publication
evidence for the stated review/delete trigger.
