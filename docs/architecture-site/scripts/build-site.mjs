import { spawn } from "node:child_process";
import { localBase, projectBase } from "../site.config.mjs";

const mode = process.argv[2] || "root";
const base =
  mode === "root" ? localBase : mode === "pages" ? projectBase : null;
if (!base) {
  console.error("usage: node scripts/build-site.mjs <root|pages>");
  process.exit(2);
}

const npmExecutable = process.env.npm_execpath;
if (!npmExecutable) {
  console.error("build-site must run through an npm script");
  process.exit(2);
}

console.log(`Building documentation with base ${base}`);

const child = spawn(
  process.execPath,
  [npmExecutable, "exec", "--", "astro", "build"],
  {
    cwd: new URL("..", import.meta.url),
    env: { ...process.env, SITE_BASE: base },
    stdio: "inherit",
  },
);
child.once("error", (error) => {
  console.error(`cannot start Astro: ${error.message}`);
  process.exit(1);
});
child.once("exit", (code, signal) => {
  if (signal) {
    console.error(`Astro terminated by ${signal}`);
    process.exit(1);
  }
  process.exit(code ?? 1);
});
