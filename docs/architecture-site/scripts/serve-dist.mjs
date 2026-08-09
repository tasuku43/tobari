import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { createServer } from "node:http";
import { extname, join, normalize, resolve, sep } from "node:path";
import { projectBase } from "../site.config.mjs";

const root = resolve(process.argv[2] || "dist");
const port = Number(process.env.PORT || 4322);
const base = projectBase;
const types = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".webp": "image/webp",
  ".xml": "application/xml; charset=utf-8",
};

const server = createServer(async (request, response) => {
  const requestUrl = new URL(
    request.url || "/",
    `http://${request.headers.host || "127.0.0.1"}`,
  );
  if (!requestUrl.pathname.startsWith(base)) {
    response.writeHead(404).end("Not found");
    return;
  }
  let pathname;
  try {
    pathname = decodeURIComponent(requestUrl.pathname.slice(base.length));
  } catch {
    response.writeHead(400).end("Bad URL");
    return;
  }
  let path = normalize(join(root, pathname));
  if (!path.startsWith(root + sep) && path !== root) {
    response.writeHead(403).end("Forbidden");
    return;
  }
  try {
    const info = await stat(path);
    if (info.isDirectory()) path = join(path, "index.html");
  } catch {
    if (!extname(path)) path = join(path, "index.html");
  }
  try {
    const info = await stat(path);
    if (!info.isFile()) throw new Error("not a file");
    response.writeHead(200, {
      "content-type": types[extname(path)] || "application/octet-stream",
      "cache-control": "no-store",
      "x-content-type-options": "nosniff",
    });
    createReadStream(path).pipe(response);
  } catch {
    response
      .writeHead(404, { "content-type": "text/plain; charset=utf-8" })
      .end("Not found");
  }
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Serving ${root} at http://127.0.0.1:${port}${base}`);
});
