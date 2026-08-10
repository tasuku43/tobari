import { readdir } from "node:fs/promises";
import { extname, join, relative, sep } from "node:path";

const contentExtensions = new Set([".md", ".mdx"]);

async function contentFiles(directory) {
  const files = [];
  const entries = await readdir(directory, { withFileTypes: true });
  entries.sort((left, right) => left.name.localeCompare(right.name, "en"));
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...(await contentFiles(path)));
    else if (entry.isFile() && contentExtensions.has(extname(entry.name)))
      files.push(path);
  }
  return files;
}

function routeFromFile(file) {
  const extension = extname(file);
  let route = file.slice(0, -extension.length).split(sep).join("/");
  if (route === "index") return "";
  if (route.endsWith("/index")) route = route.slice(0, -"/index".length);
  return route;
}

function addRoute(routes, duplicates, route, file) {
  const existing = routes.get(route);
  if (existing) duplicates.push({ route, files: [existing, file] });
  else routes.set(route, file);
}

export async function collectLocaleRoutes(contentRoot) {
  const canonical = new Map();
  const japanese = new Map();
  const duplicates = [];

  for (const path of await contentFiles(contentRoot)) {
    const file = relative(contentRoot, path);
    const normalized = file.split(sep).join("/");
    if (normalized.startsWith("ja/")) {
      addRoute(
        japanese,
        duplicates,
        routeFromFile(normalized.slice("ja/".length)),
        normalized,
      );
    } else {
      addRoute(canonical, duplicates, routeFromFile(normalized), normalized);
    }
  }

  return { canonical, japanese, duplicates };
}

export function displayRoute(route, locale = "") {
  const prefix = locale ? `${locale}/` : "";
  return `/${prefix}${route ? `${route}/` : ""}`;
}

export function localeRouteProblems({ canonical, japanese, duplicates }) {
  const problems = [];
  for (const duplicate of duplicates) {
    problems.push({
      kind: "duplicate",
      message: `duplicate source route ${displayRoute(duplicate.route)}: ${duplicate.files.join(", ")}`,
    });
  }
  for (const [route, source] of canonical) {
    if (!japanese.has(route)) {
      problems.push({
        kind: "missing-japanese",
        message: `missing Japanese counterpart ${displayRoute(route, "ja")} for ${source}`,
      });
    }
  }
  for (const [route, source] of japanese) {
    if (!canonical.has(route)) {
      problems.push({
        kind: "japanese-orphan",
        message: `Japanese-only orphan ${source} has no canonical ${displayRoute(route)} route`,
      });
    }
  }
  return problems;
}

const japaneseScript = /[\u3040-\u30ff\u3400-\u9fff]/g;
const repositoryEvidence =
  /https:\/\/github\.com\/tasuku43\/tobari\/(?:blob|tree)\/[0-9a-f]{40}\/[A-Za-z0-9._~%+@/-]+/g;

function sortedMatches(source, pattern) {
  return [...new Set(source.match(pattern) || [])].sort();
}

function machineExamplePayload(source) {
  return [...source.matchAll(/^(```|~~~)[^\n]*\n([\s\S]*?)^\1\s*$/gm)]
    .map((match) => match[2])
    .join("\n")
    .replace(/\s+/g, "");
}

export function localeContentProblems({ route, canonical, japanese }) {
  const label = displayRoute(route, "ja");
  const problems = [];
  const japaneseCharacters = (japanese.match(japaneseScript) || []).length;
  if (japaneseCharacters < 100) {
    problems.push({
      kind: "insubstantial-japanese",
      message: `${label} has only ${japaneseCharacters} Japanese-script characters`,
    });
  }
  if (japanese.length < canonical.length * 0.5) {
    problems.push({
      kind: "insubstantial-japanese",
      message: `${label} is less than half the canonical source length`,
    });
  }

  const canonicalHeadings = canonical.match(/^#{1,6}\s+/gm)?.length || 0;
  const japaneseHeadings = japanese.match(/^#{1,6}\s+/gm)?.length || 0;
  if (canonicalHeadings !== japaneseHeadings) {
    problems.push({
      kind: "heading-drift",
      message: `${label} has ${japaneseHeadings} Markdown headings; canonical has ${canonicalHeadings}`,
    });
  }

  const canonicalEvidence = sortedMatches(canonical, repositoryEvidence);
  const japaneseEvidence = sortedMatches(japanese, repositoryEvidence);
  if (JSON.stringify(canonicalEvidence) !== JSON.stringify(japaneseEvidence)) {
    problems.push({
      kind: "evidence-drift",
      message: `${label} does not preserve the canonical commit-fixed evidence targets`,
    });
  }

  if (machineExamplePayload(canonical) !== machineExamplePayload(japanese)) {
    problems.push({
      kind: "machine-example-drift",
      message: `${label} changes the non-whitespace content of fenced machine examples`,
    });
  }
  return problems;
}
