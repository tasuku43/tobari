import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
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

for (const [route, canonicalFile] of canonical) {
  const japaneseFile = japanese.get(route);
  if (!japaneseFile) continue;
  const [canonicalSource, japaneseSource] = await Promise.all([
    readFile(resolve(contentRoot, canonicalFile), "utf8"),
    readFile(resolve(contentRoot, japaneseFile), "utf8"),
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
