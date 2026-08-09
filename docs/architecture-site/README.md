# Tobari public documentation

This directory contains the Astro and Starlight source for Tobari's public,
English-language documentation site. The output is static HTML, CSS, and small
client-side scripts. It must work both at `/` for local review and at the GitHub
Pages project base `/tobari/`.

The site explains the current public contract; it is not a second product
specification. Read the repository `AGENTS.md` and the applicable numbered
documents under `docs/` before changing a claim. Product behavior, security
preconditions, limitations, and recovery guidance must agree with the governing
documents, implementation, and tests.

## Toolchain and install

Use exactly Node.js `24.18.0`. `package.json` also records npm `11.16.0`, and
`package-lock.json` is the committed dependency authority. From the repository
root:

```sh
cd docs/architecture-site
npm ci
```

Do not replace `npm ci` with an unlocked install in CI. The documentation
generators also require the Go version declared by the repository's `go.mod`.
The browser suite uses Chromium; install the matching Playwright browser once
on a development machine if it is not already available:

```sh
npx playwright install chromium
```

## Preview and build

Run the local development server at the root base:

```sh
npm run dev
```

Build static output for a root-hosted preview:

```sh
npm run build
```

Build the exact GitHub Pages shape and verify its static artifact:

```sh
npm run build:pages
npm run check:dist
```

Both builds write `dist/`. The directory is ignored and must not be committed;
CI rebuilds it from source and uploads only that generated artifact to Pages.
Set `SITE_ORIGIN` only when checking a different deployment origin. The local
base, Pages base, browser-test origin, and pinned product snapshot are
centralized in `site.config.mjs` and `source-snapshot.txt`.

## Generated reference data

The CLI, fault, component-version, and canonical provider-manifest example data
is derived from repository authorities, not copied into Markdown. Regenerate
it after changing the CLI Catalog, component versions, runtime metadata, a
schema authority, or the synthetic provider fixture:

```sh
npm run generate
```

That script runs:

```sh
go run ../../tools/sitegen --write
```

It updates the committed files under `src/generated/`. Do not edit those JSON
files by hand. Check that they are current without writing:

```sh
npm run generate:check
# equivalent generator invocation:
go run ../../tools/sitegen --check
```

CI fails when generated data is missing or stale.

### Product snapshot and documentation build

The site records two commit identities on every page:

- **Product source snapshot** is the full commit SHA in `source-snapshot.txt`.
  Product claims, implementation evidence, CLI data, component versions, and
  schema versions are derived from that immutable tree.
- **Documentation build** is the commit that contains the site source being
  built. Page-source, Contributing, and Security Policy links use this commit.

This distinction lets a documentation-only commit accurately describe the
last internally consistent product tree. It also prevents an unrelated dirty
working tree from leaking unfinished commands or schemas into a public build.
The generator exports the product snapshot into a temporary directory, builds
that commit's CLI, and reads its machine help; it never maintains a second
command registry.

Advance `source-snapshot.txt` only after the selected product commit's
governing documents, implementation, tests, Catalog, and runtime snapshots
agree. Then run `npm run generate`, review every changed claim and evidence
link, and run the complete gates. Do not point the snapshot at an uncommitted
tree or a commit that contains only part of a product change.

## Checks

The complete site gate is:

```sh
npm test
```

It runs, in order:

1. `npm run generate:check` — generated Catalog, faults, versions, and schemas;
2. `npm run check:source` — runtime-CDN, tracking, external-fetch, and
   credential-pattern guard;
3. `npm run format` — Prettier over site source;
4. `npm run typecheck` — Astro and TypeScript diagnostics;
5. `npm run build` plus `npm run check:dist:root` — root-base static build;
6. `npm run build:pages` — production build under `/tobari/`;
7. `npm run check:dist` — required pages, document structure, internal links,
   fragments, assets, base-path ownership, local-path leaks, print/reduced-motion
   CSS, and absence of external runtime resources or tracking code; and
8. `npm run test:browser` — Playwright and axe checks for representative pages,
   console/runtime errors, System/Light/Dark persistence, reduced motion,
   keyboard sequence controls, the no-JavaScript transcript, 360 px layout, and
   `/tobari/` asset/link behavior.

Run an individual stage while iterating, but run the complete gate before
handoff. The browser command serves the already-built Pages artifact through
`scripts/serve-dist.mjs`; build it first when running that command alone.

## Maintaining content and evidence

Page source lives under `src/content/docs/`. Reusable diagrams and sequence
data live under `src/data/`; interactive and static sequence presentations must
continue to consume the same data. Keep introductions plain, introduce one new
boundary at a time, and place exact paths, schemas, fault codes, and versions in
Reference rather than the first-use learning path.

Every material product or security claim should link to commit-fixed evidence:

- the governing contract;
- the relevant architecture or security section;
- the enforcement implementation; and
- tests that exercise the stated success and failure behavior.

Do not use a moving `main` URL as the only evidence and do not manually freeze
large sets of line numbers. Every page separately renders its product snapshot
and documentation build commit. Use synthetic examples only; never add
credentials, private URLs, personal data, or copied private history. Runtime
pages must not load fonts, scripts, styles, or images from a CDN, analytics
service, or tracker.

Most importantly, do not document active work packets, dirty-worktree ideas,
accepted mechanisms that are not implemented, or planned provider/runtime
changes as current functionality. A feature belongs on the public site only
after its governing contracts, implementation, tests, and repository gates
agree. If those sources conflict, resolve the higher-level product decision
before choosing convenient wording for the site.

## GitHub Pages workflow

`.github/workflows/architecture-pages.yml` owns validation and publication.
Pull requests build and test the site but never deploy it. A successful `main`
run may build the `/tobari/` artifact, upload only `dist/`, and deploy through
GitHub Pages. The deploy job uses the Pages environment with only the required
`pages: write` and `id-token: write` permissions; checkout/build jobs remain
read-only. External Actions are pinned to full commit SHAs, installs use the
lockfile, and workflow concurrency prevents overlapping publications.

Do not push, publish Pages, or change a release merely to preview documentation.
Local builds and pull-request checks are the normal review path.
