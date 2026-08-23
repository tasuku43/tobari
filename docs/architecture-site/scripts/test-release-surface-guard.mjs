import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const scanner = new URL("./check-release-surface.mjs", import.meta.url);
const fixtureDirectory = await mkdtemp(
  join(tmpdir(), "tobari-release-surface-"),
);
const fixture = join(fixtureDirectory, "mixed-absence.md");

try {
  await writeFile(
    fixture,
    "The release surface has no Tobari-owned authentication command or credential service.\n" +
      "This forbidden regression mentions auth/providers.\n" +
      "The old fixture is synthetic-provider-v1.json.\n" +
      "The fault filter suggests auth login.\n" +
      "Gateway/native Workspace authentication boundary.\n",
  );
  const result = spawnSync(process.execPath, [scanner.pathname, fixture], {
    encoding: "utf8",
  });
  if (
    result.status === 0 ||
    !result.stderr.includes("research authority store") ||
    !result.stderr.includes("research authentication service role") ||
    !result.stderr.includes("research provider fixture") ||
    !result.stderr.includes("bare research auth command")
  ) {
    throw new Error(
      `mixed-content guard canary did not fail closed (status=${result.status})`,
    );
  }
  console.log("Release-surface mixed-content guard canary passed.");
} finally {
  await rm(fixtureDirectory, { recursive: true, force: true });
}
