import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  collectLocaleRoutes,
  localeContentProblems,
  localeRouteProblems,
} from "./locale-routes.mjs";

const contentRoot = resolve(
  process.argv[2] ||
    fileURLToPath(new URL("../src/content/docs", import.meta.url)),
);
const { canonical, japanese, duplicates } =
  await collectLocaleRoutes(contentRoot);
const errors = localeRouteProblems({ canonical, japanese, duplicates }).map(
  ({ message }) => message,
);
const localAstroImport = /\bfrom\s+["']([^"']+\.astro)["']/g;

async function pageSource(file) {
  const pagePath = resolve(contentRoot, file);
  const source = await readFile(pagePath, "utf8");
  const componentPaths = [
    ...new Set(
      [...source.matchAll(localAstroImport)].map((match) =>
        resolve(dirname(pagePath), match[1]),
      ),
    ),
  ];
  const componentSources = await Promise.all(
    componentPaths.map((path) => readFile(path, "utf8")),
  );
  return [source, ...componentSources].join("\n");
}

for (const [route, canonicalFile] of canonical) {
  const japaneseFile = japanese.get(route);
  if (!japaneseFile) continue;
  const [canonicalSource, japaneseSource] = await Promise.all([
    pageSource(canonicalFile),
    pageSource(japaneseFile),
  ]);
  errors.push(
    ...localeContentProblems({
      route,
      canonical: canonicalSource,
      japanese: japaneseSource,
    }).map(({ message }) => message),
  );
}

if (errors.length) {
  console.error(
    `Locale route verification failed with ${errors.length} problem(s):`,
  );
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}

console.log(
  `Verified ${canonical.size} canonical English routes and ${japanese.size} same-topic Japanese routes.`,
);
