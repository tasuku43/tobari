import { expect, test } from "@playwright/test";

test("mental model locates components and follows one request in time order", async ({
  page,
}) => {
  await page.goto("ja/how-it-works/mental-model/");
  const map = page.locator("tobari-system-map").first();

  await expect(map.locator(".map-zone")).toHaveCount(3);
  await expect(map.locator(".lifeline")).toHaveCount(5);
  await expect(map.locator("[data-message-route]")).toHaveCount(9);
  await expect(map.locator("[data-message-label]")).toHaveCount(9);
  await expect(map).toContainText("存在場所");
  await expect(map).toContainText("Workspace から接続先への直接経路はない");
  await expect(map).toContainText("許可または拒否");

  await map.locator('[data-conversation="credential"]').click();
  await expect(map.locator('[data-node="gateway"]')).toHaveClass(/node-active/);
  await expect(map.locator('[data-node="broker"]')).toHaveClass(/node-active/);
});

test("concept diagrams put locations and route labels inside the map", async ({
  page,
}) => {
  await page.goto("ja/how-it-works/https-and-tls/");
  const tls = page.locator('[data-diagram="tls-split"]');

  await expect(tls.locator(".flow-region")).toHaveCount(3);
  await expect(tls.locator(".route-label")).toHaveCount(7);
  await expect(tls.locator(".node-region")).toHaveCount(4);
  await expect(tls).toContainText("外部の HTTPS 接続先");
  await expect(tls).toContainText("CONNECT");

  await page.goto("ja/how-it-works/system-overview/");
  const trust = page.locator('[data-diagram="trust-boundaries"]');
  await expect(trust.locator(".flow-region")).toHaveCount(3);
  await expect(trust.locator(".route-label")).toHaveCount(5);
});

test("abstract dependency diagrams keep numbered routes without fake locations", async ({
  page,
}) => {
  await page.goto("how-it-works/code-architecture/");
  const diagram = page.locator('[data-diagram="code-layers"]');

  await expect(diagram.locator(".flow-region")).toHaveCount(0);
  await expect(diagram.locator(".route-label")).toHaveCount(5);
  await expect(diagram.locator(".flow-step-list > li")).toHaveCount(5);
});
