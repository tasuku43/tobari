import { readFile, readdir, stat } from "node:fs/promises";
import {
  dirname,
  extname,
  join,
  normalize,
  relative,
  resolve,
  sep,
} from "node:path";
import { parse } from "parse5";
import { productSnapshot, projectBase } from "../site.config.mjs";
import { collectLocaleRoutes, localeRouteProblems } from "./locale-routes.mjs";

const root = resolve(process.argv[2] || "dist");
const requestedBase = process.argv[3] || projectBase;
const base = `/${requestedBase.split("/").filter(Boolean).join("/")}${requestedBase === "/" ? "" : "/"}`;
const errors = [];
const contentRoot = resolve(import.meta.dirname, "../src/content/docs");
const localeRoutes = await collectLocaleRoutes(contentRoot);
errors.push(...localeRouteProblems(localeRoutes).map(({ message }) => message));
const requiredRoutes = [...localeRoutes.canonical.keys()].flatMap((route) => [
  { route, lang: "en" },
  { route: route ? `ja/${route}` : "ja", lang: "ja" },
]);

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

function walk(node, callback) {
  callback(node);
  for (const child of node.childNodes || []) walk(child, callback);
  if (node.content) walk(node.content, callback);
}

function attr(node, name) {
  return node.attrs?.find((attribute) => attribute.name === name)?.value;
}

function outputPathFor(route) {
  return join(root, route, "index.html");
}

function localTarget(currentFile, value) {
  const withoutQuery = value.split("?")[0];
  const [pathname, fragment = ""] = withoutQuery.split("#");
  if (!pathname) return { path: currentFile, fragment };
  let decoded;
  try {
    decoded = decodeURIComponent(pathname);
  } catch {
    return { error: `invalid URL encoding: ${value}` };
  }
  let target;
  if (decoded.startsWith("/")) {
    if (base !== "/" && !decoded.startsWith(base)) {
      return {
        error: `root-absolute URL escapes configured base ${base}: ${value}`,
      };
    }
    const stripped =
      base === "/" ? decoded.slice(1) : decoded.slice(base.length);
    target = join(root, stripped);
  } else {
    target = resolve(dirname(currentFile), decoded);
  }
  if (!target.startsWith(root + sep) && target !== root)
    return { error: `path escapes output root: ${value}` };
  if (extname(target) === "") target = join(target, "index.html");
  return { path: normalize(target), fragment };
}

for (const { route } of requiredRoutes) {
  try {
    await stat(outputPathFor(route));
  } catch {
    errors.push(`required route missing: ${base}${route ? `${route}/` : ""}`);
  }
}

const files = await filesBelow(root);
const htmlFiles = files.filter((file) => file.endsWith(".html"));
const htmlByPath = new Map();
for (const file of htmlFiles) {
  const source = await readFile(file, "utf8");
  htmlByPath.set(file, { source, document: parse(source) });
  if (/file:\/\/|\/Users\/|[A-Za-z]:\\Users\\/.test(source))
    errors.push(`${relative(root, file)} contains a local absolute path`);
  if (
    /google-analytics|googletagmanager|segment\.com|plausible\.io|mixpanel|hotjar/i.test(
      source,
    )
  )
    errors.push(`${relative(root, file)} contains analytics or tracking code`);
  const buildMeta = source.match(
    /<meta name="tobari-docs-build" content="([0-9a-f]{40})"\s*\/?>/,
  );
  if (
    !new RegExp(
      `<meta name="tobari-product-snapshot" content="${productSnapshot}"\\s*\\/?>`,
    ).test(source)
  ) {
    errors.push(
      `${relative(root, file)} does not identify the pinned product snapshot`,
    );
  }
  if (!buildMeta) {
    errors.push(
      `${relative(root, file)} does not identify the documentation build commit`,
    );
  }
  for (const match of source.matchAll(
    /https:\/\/github\.com\/tasuku43\/tobari\/(?:blob|tree)\/([0-9a-f]{40})\/([^"<]*)/g,
  )) {
    const [, linkedCommit, linkedPath] = match;
    if (linkedCommit === productSnapshot) continue;
    const buildOnlyPath =
      linkedPath.startsWith("docs/architecture-site/") ||
      linkedPath === "CONTRIBUTING.md" ||
      linkedPath === "SECURITY.md" ||
      linkedPath === "tools/sitegen/main.go";
    if (linkedCommit !== buildMeta?.[1] || !buildOnlyPath) {
      errors.push(
        `${relative(root, file)} links product evidence to unpinned commit ${linkedCommit}: ${linkedPath}`,
      );
    }
  }
}

function localePath(route, locale) {
  const localePrefix = locale === "ja" ? "ja/" : "";
  return `${base}${localePrefix}${route ? `${route}/` : ""}`;
}

function alternateLinks(document) {
  const links = new Map();
  walk(document, (node) => {
    if (node.tagName !== "link" || attr(node, "rel") !== "alternate") return;
    const language = attr(node, "hreflang");
    const href = attr(node, "href");
    if (language && href) links.set(language, href);
  });
  return links;
}

for (const route of localeRoutes.canonical.keys()) {
  const pair = [
    outputPathFor(route),
    outputPathFor(route ? `ja/${route}` : "ja"),
  ];
  const expected = new Map([
    ["en", localePath(route, "en")],
    ["ja", localePath(route, "ja")],
    ["x-default", localePath(route, "en")],
  ]);
  for (const file of pair) {
    const document = htmlByPath.get(file)?.document;
    if (!document) continue;
    const alternates = alternateLinks(document);
    for (const [language, pathname] of expected) {
      const href = alternates.get(language);
      let actualPath;
      try {
        actualPath = href ? new URL(href).pathname : undefined;
      } catch {
        actualPath = undefined;
      }
      if (actualPath !== pathname) {
        errors.push(
          `${relative(root, file)} has invalid ${language} alternate: ${href || "missing"}`,
        );
      }
    }
  }
}

const resourceAttrs = new Map([
  ["script", ["src"]],
  ["img", ["src", "srcset"]],
  ["source", ["src", "srcset"]],
  ["video", ["src", "poster"]],
  ["audio", ["src"]],
  ["iframe", ["src"]],
]);

for (const [file, { source, document }] of htmlByPath) {
  const label = relative(root, file);
  const tags = new Map();
  const ids = new Set();
  const links = [];
  let htmlRoot;
  walk(document, (node) => {
    if (node.tagName) tags.set(node.tagName, (tags.get(node.tagName) || 0) + 1);
    if (node.tagName === "html") htmlRoot = node;
    const id = attr(node, "id");
    if (id) ids.add(id);
    if (node.tagName === "a") {
      const href = attr(node, "href");
      if (href) links.push(href);
    }
    if (node.tagName === "link") {
      const rel = attr(node, "rel") || "";
      const href = attr(node, "href");
      if (
        href &&
        /(?:^|\s)(?:stylesheet|icon|manifest|preload|modulepreload)(?:\s|$)/.test(
          rel,
        )
      )
        links.push(href);
    }
    for (const attribute of resourceAttrs.get(node.tagName) || []) {
      const raw = attr(node, attribute);
      if (!raw) continue;
      const values =
        attribute === "srcset"
          ? raw.split(",").map((part) => part.trim().split(/\s+/)[0])
          : [raw];
      for (const value of values) {
        if (/^(?:https?:)?\/\//.test(value))
          errors.push(`${label} loads an external runtime resource: ${value}`);
        else if (!value.startsWith("data:")) links.push(value);
      }
    }
  });
  const expectedLanguage =
    label === join("ja", "index.html") || label.startsWith(`ja${sep}`)
      ? "ja"
      : "en";
  if (
    (tags.get("html") || 0) !== 1 ||
    attr(htmlRoot, "lang") !== expectedLanguage
  ) {
    errors.push(`${label} must have one ${expectedLanguage} html root`);
  }
  if ((tags.get("main") || 0) !== 1)
    errors.push(`${label} must have one main landmark`);
  if ((tags.get("h1") || 0) !== 1)
    errors.push(`${label} must have exactly one h1`);
  if ((tags.get("title") || 0) !== 1)
    errors.push(`${label} must have one document title`);
  if (/<p(?:\s[^>]*)?>\s*<p(?:\s|>)/i.test(source))
    errors.push(`${label} contains a nested paragraph`);

  for (const value of links) {
    if (/^(?:https?:|mailto:|tel:|data:|javascript:)/.test(value)) continue;
    const target = localTarget(file, value);
    if (target.error) {
      errors.push(`${label}: ${target.error}`);
      continue;
    }
    try {
      const targetStat = await stat(target.path);
      if (targetStat.isDirectory())
        throw new Error("directory URL did not resolve to index.html");
    } catch {
      errors.push(`${label} has a broken local link/resource: ${value}`);
      continue;
    }
    if (target.fragment && target.path.endsWith(".html")) {
      const targetDocument =
        htmlByPath.get(target.path)?.document ||
        parse(await readFile(target.path, "utf8"));
      const targetIds = new Set();
      walk(targetDocument, (node) => {
        const id = attr(node, "id");
        if (id) targetIds.add(id);
      });
      if (!targetIds.has(target.fragment))
        errors.push(`${label} links to missing fragment ${value}`);
    }
  }
}

const home = htmlByPath.get(outputPathFor(""))?.source || "";
for (const phrase of [
  "Run a coding agent inside one selected project.",
  "Stop it, review it, then allow only what is needed",
  "Protected in the supported topology",
  "Not protected by Tobari",
  "files inside the selected project are not protected from the agent",
]) {
  if (!home.includes(phrase))
    errors.push(`home page is missing required content: ${phrase}`);
}
if (
  !home.includes("data-tobari-theme-select") ||
  !home.includes('value="system"') ||
  !home.includes('value="light"') ||
  !home.includes('value="dark"')
) {
  errors.push(
    "home page does not expose System, Light, and Dark theme choices",
  );
}
if (!home.includes("tobari-theme"))
  errors.push("theme preference is not stored under the documented local key");

const requestJourney =
  htmlByPath.get(outputPathFor("how-it-works/request-journey"))?.source || "";
for (const phrase of [
  "Static sequence descriptions",
  "Information not sent",
  "Previous",
  "Pause",
  "Restart",
]) {
  if (!requestJourney.includes(phrase))
    errors.push(
      `request journey is missing sequence fallback/control: ${phrase}`,
    );
}
const cssFiles = files.filter((file) => file.endsWith(".css"));
const css = (
  await Promise.all(cssFiles.map((file) => readFile(file, "utf8")))
).join("\n");
if (
  !css.includes("prefers-reduced-motion:reduce") &&
  !css.includes("prefers-reduced-motion: reduce")
)
  errors.push("built CSS does not contain reduced-motion handling");
if (!css.includes("@media print"))
  errors.push("built CSS does not contain print rules");

if (errors.length) {
  console.error(
    `Static site verification failed with ${errors.length} problem(s):`,
  );
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}

console.log(
  `Verified ${htmlFiles.length} HTML pages and ${files.length - htmlFiles.length} static assets under ${base}.`,
);
