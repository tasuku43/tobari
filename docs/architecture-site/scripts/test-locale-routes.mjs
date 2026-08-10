import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import {
  collectLocaleRoutes,
  localeContentProblems,
  localeRouteProblems,
} from "./locale-routes.mjs";

const root = await mkdtemp(join(tmpdir(), "tobari-locale-routes-"));
try {
  for (const directory of ["guide", "ja/guide", "ja/orphan"]) {
    await mkdir(join(root, directory), { recursive: true });
  }
  await Promise.all([
    writeFile(join(root, "index.mdx"), "# English\n"),
    writeFile(join(root, "ja/index.md"), "# 日本語\n"),
    writeFile(join(root, "guide/paired.mdx"), "# Paired\n"),
    writeFile(join(root, "ja/guide/paired.md"), "# 対応\n"),
    writeFile(join(root, "guide/missing.mdx"), "# Missing\n"),
    writeFile(join(root, "guide/duplicate.md"), "# First\n"),
    writeFile(join(root, "guide/duplicate.mdx"), "# Second\n"),
    writeFile(join(root, "ja/orphan/page.mdx"), "# 孤立\n"),
  ]);

  const inventory = await collectLocaleRoutes(root);
  assert.deepEqual([...inventory.canonical.keys()].sort(), [
    "",
    "guide/duplicate",
    "guide/missing",
    "guide/paired",
  ]);
  assert.deepEqual([...inventory.japanese.keys()].sort(), [
    "",
    "guide/paired",
    "orphan/page",
  ]);
  assert.deepEqual(
    localeRouteProblems(inventory).map(({ kind }) => kind),
    ["duplicate", "missing-japanese", "missing-japanese", "japanese-orphan"],
  );

  const evidence =
    "https://github.com/tasuku43/tobari/blob/0123456789abcdef0123456789abcdef01234567/docs/example.md";
  const canonical = `# Example\n\n## Contract\n\n${"Canonical explanation. ".repeat(20)}\n\n\`\`\`sh\ntobari doctor\n\`\`\`\n\n${evidence}\n`;
  const japanese = `# 例\n\n## 契約\n\n${"日本語で境界と失敗時の動作を具体的に説明します。".repeat(20)}\n\n\`\`\`sh\ntobari doctor\n\`\`\`\n\n${evidence}\n`;
  assert.deepEqual(
    localeContentProblems({ route: "guide/paired", canonical, japanese }),
    [],
  );
  const driftKinds = localeContentProblems({
    route: "guide/paired",
    canonical,
    japanese: `# 例\n\n${"短い日本語。".repeat(20)}\n\n\`\`\`sh\ntobari cluster down\n\`\`\`\n`,
  }).map(({ kind }) => kind);
  assert.ok(driftKinds.includes("insubstantial-japanese"));
  assert.ok(driftKinds.includes("heading-drift"));
  assert.ok(driftKinds.includes("evidence-drift"));
  assert.ok(driftKinds.includes("machine-example-drift"));
} finally {
  await rm(root, { recursive: true, force: true });
}

console.log(
  "Verified locale route and substantive-content parity failure modes.",
);
