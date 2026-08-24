import { execFileSync } from "node:child_process";
import { readFile, readdir, stat } from "node:fs/promises";
import { extname, join, relative, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const repositoryRoot = resolve(root, "../..");
const productSnapshot = (
  await readFile(join(root, "source-snapshot.txt"), "utf8")
).trim();
const scannedRoots = ["src", "public"];
const textExtensions = new Set([
  ".astro",
  ".css",
  ".html",
  ".js",
  ".json",
  ".md",
  ".mdx",
  ".mjs",
  ".svg",
  ".ts",
]);
const errors = [];
const evidenceLinks = new Map();
const universalQuestionOpening =
  /^##\s+(?:(?:The question this (?:page|guide) answers)|(?:What question does this page answer\?)|(?:この(?:ページ|ガイド)で答える問い))\s*$/m;
const retiredManifestRoutes =
  /\/(?:ja\/)?(?:guides\/workspace-manifests|how-it-works\/workspace-manifest-workspace-cluster)\/?/;

const v1Sources = {
  manifests: await readFile(
    join(root, "src/content/docs/guides/workspace-templates-and-contexts.mdx"),
    "utf8",
  ),
  manifestsJa: await readFile(
    join(
      root,
      "src/content/docs/ja/guides/workspace-templates-and-contexts.mdx",
    ),
    "utf8",
  ),
  manifestModel: await readFile(
    join(
      root,
      "src/content/docs/how-it-works/workspace-template-context-workspace-cluster.mdx",
    ),
    "utf8",
  ),
  manifestModelJa: await readFile(
    join(
      root,
      "src/content/docs/ja/how-it-works/workspace-template-context-workspace-cluster.mdx",
    ),
    "utf8",
  ),
  auth: await readFile(
    join(root, "src/content/docs/guides/authentication.mdx"),
    "utf8",
  ),
  authJa: await readFile(
    join(root, "src/content/docs/ja/guides/authentication.mdx"),
    "utf8",
  ),
  supplyChain: await readFile(
    join(root, "src/content/docs/security/supply-chain.mdx"),
    "utf8",
  ),
  supplyChainJa: await readFile(
    join(root, "src/content/docs/ja/security/supply-chain.mdx"),
    "utf8",
  ),
};

for (const required of [
  "template list",
  "template default set --id TEMPLATE_REF",
  "context enter --id CONTEXT_REF",
  "workspace delete --id WORKSPACE_REF",
  "Policy Memory",
]) {
  if (
    !v1Sources.manifests.includes(required) ||
    !v1Sources.manifestsJa.includes(required)
  ) {
    errors.push(
      `Workspace Template capability documentation is missing ${required} in one locale`,
    );
  }
}
for (const required of [
  "`tobari-expose`",
  "`tobari-permission`",
  "`check-exposure-helper-source.sh`",
]) {
  if (
    !v1Sources.supplyChain.includes(required) ||
    !v1Sources.supplyChainJa.includes(required)
  ) {
    errors.push(
      `Workspace helper supply-chain documentation is missing ${required} in one locale`,
    );
  }
}
for (const [label, source] of [
  ["English Workspace Template guide", v1Sources.manifests],
  ["Japanese Workspace Template guide", v1Sources.manifestsJa],
  ["English Workspace Template model", v1Sources.manifestModel],
  ["Japanese Workspace Template model", v1Sources.manifestModelJa],
]) {
  const retiredTerm = source.match(
    /\bWorkspace Manifests?\b|--manifest\b|\bworkspace_manifest_id\b|\bmanifest (?:list|show|create|delete|default|runtime)\b/i,
  );
  if (retiredTerm) {
    errors.push(`${label} retains retired public term: ${retiredTerm[0]}`);
  }
}
for (const required of [
  "claude",
  "codex login",
  "agent-ready",
  "release surface",
  "subscriptionType",
  "rateLimitTier",
]) {
  if (
    !v1Sources.auth.includes(required) ||
    !v1Sources.authJa.includes(required)
  ) {
    errors.push(
      `native authentication guide is missing ${required} in one locale`,
    );
  }
}

async function filesBelow(directory) {
  const result = [];
  for (const entry of await readdir(directory)) {
    const path = join(directory, entry);
    if ((await stat(path)).isDirectory())
      result.push(...(await filesBelow(path)));
    else result.push(path);
  }
  return result;
}

const files = [
  join(root, "astro.config.mjs"),
  join(root, "package.json"),
  ...(
    await Promise.all(
      scannedRoots.map((directory) => filesBelow(join(root, directory))),
    )
  ).flat(),
];

for (const file of files) {
  if (!textExtensions.has(extname(file))) continue;
  const label = relative(root, file);
  const source = await readFile(file, "utf8");

  if (retiredManifestRoutes.test(source)) {
    errors.push(`${label} retains a retired Workspace Template public route`);
  }

  if (
    label.startsWith("src/content/docs/") &&
    universalQuestionOpening.test(source)
  ) {
    errors.push(
      `${label} uses the retired universal question-first documentation opening`,
    );
  }

  if (label.startsWith("src/content/docs/")) {
    const retiredV1Claim =
      /\bcompaction\b|static managed adapter|tobari policy compact(?:ions)?\b/i;
    const match = source.match(retiredV1Claim);
    if (match) {
      errors.push(
        `${label} retains retired first-public V1 claim token: ${match[0]}`,
      );
    }
    if (
      /\b(?:safe|read-only)\s+GET\b|\bGET\s+(?:request|effect)s?\s+(?:is|are)\s+(?:safe|read-only)\b/i.test(
        source,
      )
    ) {
      errors.push(`${label} characterizes GET as safe or read-only`);
    }
  }

  if (
    /\b(?:google-analytics|googletagmanager|segment\.com|plausible\.io|mixpanel|hotjar)\b/i.test(
      source,
    )
  ) {
    errors.push(`${label} contains analytics or tracking code`);
  }
  if (/[@]import\s+(?:url\()?['"]?https?:\/\//i.test(source)) {
    errors.push(`${label} imports a runtime stylesheet from the network`);
  }
  if (
    /<(?:script|img|source|iframe|link)\b[^>]*(?:src|href)\s*=\s*['"](?:https?:)?\/\//i.test(
      source,
    )
  ) {
    errors.push(`${label} embeds an external runtime asset`);
  }
  if (/\bfetch\s*\(\s*['"]https?:\/\//i.test(source)) {
    errors.push(`${label} performs an external runtime fetch`);
  }
  if (
    /\b(?:AIza[0-9A-Za-z_-]{35}|ghp_[0-9A-Za-z]{36}|github_pat_[0-9A-Za-z_]{22,})\b/.test(
      source,
    )
  ) {
    errors.push(`${label} appears to contain a credential`);
  }

  const permalinkPattern =
    /https:\/\/github\.com\/tasuku43\/tobari\/(blob|tree)\/([0-9a-f]{40})\/([A-Za-z0-9._~%+@/-]+)/g;
  for (const match of source.matchAll(permalinkPattern)) {
    const [, kind, commit, encodedPath] = match;
    let sourcePath;
    try {
      sourcePath = decodeURIComponent(encodedPath);
    } catch {
      errors.push(`${label} contains a malformed repository permalink path`);
      continue;
    }
    const key = `${kind}:${commit}:${sourcePath}`;
    const labels = evidenceLinks.get(key) || [];
    labels.push(label);
    evidenceLinks.set(key, labels);
  }
}

if (!/^[0-9a-f]{40}$/.test(productSnapshot)) {
  errors.push("source-snapshot.txt must contain one full lowercase commit SHA");
} else {
  for (const [key, labels] of evidenceLinks) {
    const [kind, commit, ...pathParts] = key.split(":");
    const sourcePath = pathParts.join(":");
    if (commit !== productSnapshot) {
      errors.push(
        `${labels[0]} links to product commit ${commit}, expected ${productSnapshot}`,
      );
      continue;
    }
    try {
      const objectType = execFileSync(
        "git",
        ["cat-file", "-t", `${commit}:${sourcePath}`],
        {
          cwd: repositoryRoot,
          encoding: "utf8",
          stdio: ["ignore", "pipe", "ignore"],
        },
      ).trim();
      const expectedType = kind === "tree" ? "tree" : "blob";
      if (objectType !== expectedType) {
        errors.push(
          `${labels[0]} uses ${kind} for ${sourcePath}, but Git reports ${objectType}`,
        );
      }
    } catch {
      errors.push(
        `${labels[0]} links to missing product evidence ${commit}:${sourcePath}`,
      );
    }
  }
}

if (errors.length) {
  console.error(
    `Site source verification failed with ${errors.length} problem(s):`,
  );
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}

console.log(
  `Verified ${files.length} runtime site source files and ${evidenceLinks.size} commit-fixed evidence targets: no CDN, tracking, external fetch, stale permalink, or credential pattern.`,
);
