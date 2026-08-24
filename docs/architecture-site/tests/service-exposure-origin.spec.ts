import { once } from "node:events";
import { createServer, type Server } from "node:http";
import { expect, test } from "@playwright/test";

const labels = [
  "0123456789abcdef0123456789abcdef",
  "fedcba9876543210fedcba9876543210",
] as const;

function portOf(server: Server): number {
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("Service exposure fixture did not bind TCP");
  }
  return address.port;
}

function exposureRoot(label: string, port: number): string {
  return "http" + "://" + `svc-${label}.localhost:${port}/`;
}

function startOrigin(label: string, cookie: string): Server {
  let server: Server;
  server = createServer((request, response) => {
    const expected = `svc-${label}.localhost:${portOf(server)}`;
    if (request.headers.host !== expected) {
      response.writeHead(421, { "content-type": "text/plain" });
      response.end("wrong authority");
      return;
    }
    if (request.url === "/set") {
      response.setHeader("set-cookie", [
        `${cookie}=owned; Path=/`,
        `domain_localhost_${cookie}=rejected; Domain=localhost; Path=/`,
      ]);
    }
    response.writeHead(200, { "content-type": "application/json" });
    response.end(
      JSON.stringify({
        host: request.headers.host,
        cookie: request.headers.cookie ?? "",
      }),
    );
  });
  return server;
}

let first: Server;
let second: Server;

test.beforeAll(async () => {
  first = startOrigin(labels[0], "first_only");
  second = startOrigin(labels[1], "second_only");
  first.listen(0, "127.0.0.1");
  second.listen(0, "127.0.0.1");
  await Promise.all([once(first, "listening"), once(second, "listening")]);
});

test.afterAll(async () => {
  first.close();
  second.close();
  await Promise.all([once(first, "close"), once(second, "close")]);
});

test("generated Service exposure origins isolate cookies in supported Chromium", async ({
  page,
  context,
}) => {
  const firstRoot = exposureRoot(labels[0], portOf(first));
  const secondRoot = exposureRoot(labels[1], portOf(second));
  await context.clearCookies();

  await page.goto(firstRoot + "set");
  await expect(page.locator("body")).toContainText(
    `svc-${labels[0]}.localhost`,
  );
  await page.goto(secondRoot);
  const secondBefore = JSON.parse(await page.locator("body").innerText()) as {
    cookie: string;
  };
  expect(secondBefore.cookie).not.toContain("first_only=owned");
  expect(secondBefore.cookie).not.toContain("domain_localhost_first_only");

  await page.goto(secondRoot + "set");
  await page.goto(firstRoot);
  const firstAfter = JSON.parse(await page.locator("body").innerText()) as {
    cookie: string;
  };
  expect(firstAfter.cookie).toContain("first_only=owned");
  expect(firstAfter.cookie).not.toContain("second_only=owned");
  expect(firstAfter.cookie).not.toContain("domain_localhost_second_only");

  const cookies = await context.cookies();
  expect(cookies.map((cookie) => cookie.name)).not.toContain(
    "domain_localhost_first_only",
  );
  expect(cookies.map((cookie) => cookie.name)).not.toContain(
    "domain_localhost_second_only",
  );
});
