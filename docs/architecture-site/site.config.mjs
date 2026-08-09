import { readFileSync } from "node:fs";

export const localBase = "/";
export const projectBase = "/tobari/";
export const testOrigin = "http://127.0.0.1:4322";
export const productSnapshot = readFileSync(
  new URL("./source-snapshot.txt", import.meta.url),
  "utf8",
).trim();

if (!/^[0-9a-f]{40}$/.test(productSnapshot)) {
  throw new Error(
    "source-snapshot.txt must contain one full lowercase commit SHA",
  );
}
