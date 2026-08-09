import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { projectBase, testOrigin } from "../site.config.mjs";

const representativePages = [
  "",
  "start/quickstart/",
  "how-it-works/system-overview/",
  "how-it-works/request-journey/",
  "how-it-works/policy-learning/",
  "how-it-works/credentials/",
  "security/guarantees-and-limitations/",
  "reference/cli/",
];

async function expectNoRuntimeErrors(page: Page, route: string) {
  const errors: string[] = [];
  const external: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (!["127.0.0.1", "localhost"].includes(url.hostname))
      external.push(request.url());
  });
  const response = await page.goto(route);
  expect(response?.ok(), `route ${route}`).toBeTruthy();
  await page.waitForLoadState("networkidle");
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
}

for (const route of representativePages) {
  test(`${route || "home"} has accessible structure and no runtime errors`, async ({
    page,
  }) => {
    await expectNoRuntimeErrors(page, route);
    await expect(page.locator("main")).toHaveCount(1);
    await expect(page.locator("h1")).toHaveCount(1);
    await expect(page.locator('nav[aria-label="Primary"]')).toHaveCount(1);
    const results = await new AxeBuilder({ page }).analyze();
    expect(results.violations).toEqual([]);
  });
}

test("theme supports System, Light, and Dark and persists the explicit choice", async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("");
  const select = page.locator("[data-tobari-theme-select]").first();
  await expect(select.locator("option")).toHaveText([
    "System",
    "Light",
    "Dark",
  ]);

  for (const theme of ["light", "dark"] as const) {
    await select.selectOption(theme);
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    expect(
      await page.evaluate(() => localStorage.getItem("tobari-theme")),
    ).toBe(theme);
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
  }

  await select.selectOption("system");
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme-selection",
    "system",
  );
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(await page.evaluate(() => localStorage.getItem("tobari-theme"))).toBe(
    "system",
  );
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme-selection",
    "system",
  );
});

test("sequence never autoplays and is keyboard operable", async ({ page }) => {
  await page.goto("how-it-works/request-journey/");
  const explorer = page.locator("tobari-sequence").first();
  const gatewayPosition = () =>
    explorer.evaluate((element) => {
      const stage = element
        .querySelector(".sequence-map-stage")
        ?.getBoundingClientRect();
      const gateway = Array.from(element.querySelectorAll(".actor-lane"))
        .find((lane) => lane.textContent?.includes("Gateway"))
        ?.getBoundingClientRect();
      if (!stage || !gateway) return null;
      return { x: gateway.x - stage.x, y: gateway.y - stage.y };
    });
  const gatewayBefore = await gatewayPosition();
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
  await page.waitForTimeout(800);
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
  await explorer.focus();
  await page.keyboard.press("ArrowRight");
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 2 of 7",
  );
  const gatewayAfter = await gatewayPosition();
  expect(gatewayAfter).toEqual(gatewayBefore);
  await page.keyboard.press("ArrowLeft");
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
  await explorer.locator('[data-control="play"]').click();
  await expect(explorer.locator('[data-control="pause"]')).toBeEnabled();
  await explorer.locator('[data-control="pause"]').click();
  await expect(explorer.locator('[data-control="pause"]')).toBeDisabled();
  await explorer.locator('[data-control="restart"]').click();
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
});

test("system map keeps positions fixed while selecting a conversation", async ({
  page,
}) => {
  await page.goto("how-it-works/mental-model/");
  const map = page.locator("tobari-system-map").first();
  const componentPositions = () =>
    map.evaluate((element) => {
      const stage = element
        .querySelector(".map-stage")
        ?.getBoundingClientRect();
      if (!stage) return null;
      return ["gateway", "broker"].map((id) => {
        const box = element
          .querySelector(`[data-node="${id}"]`)
          ?.getBoundingClientRect();
        return box ? { x: box.x - stage.x, y: box.y - stage.y } : null;
      });
    });
  const positionsBefore = await componentPositions();
  await map.locator('[data-conversation="credential"]').click();
  await expect(map.locator('[data-conversation="credential"]')).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(
    map.locator('[data-conversation-detail="credential"]'),
  ).toBeVisible();
  await expect(map.locator('[data-node="gateway"]')).toHaveClass(/node-active/);
  await expect(map.locator('[data-node="broker"]')).toHaveClass(/node-active/);
  const positionsAfter = await componentPositions();
  expect(positionsAfter).toEqual(positionsBefore);
});

test("policy loop exposes the cycle without automatic playback", async ({
  page,
}) => {
  await page.goto("how-it-works/policy-learning/");
  const loop = page.locator("[data-policy-loop]");
  await expect(loop.locator('[data-loop-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
  await page.waitForTimeout(500);
  await expect(loop.locator('[data-loop-field="count"]')).toHaveText(
    "Step 1 of 7",
  );
  await loop.locator('[data-loop-control="next"]').click();
  await expect(loop.locator('[data-loop-field="count"]')).toHaveText(
    "Step 2 of 7",
  );
  await expect(loop.locator(".loop-node.is-active")).toHaveCount(2);
});

test("reduced motion keeps sequence information and controls", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("how-it-works/request-journey/");
  const explorer = page.locator("tobari-sequence").first();
  await expect(explorer.locator('[data-field="explanation"]')).not.toBeEmpty();
  await expect(explorer.locator('[data-control="next"]')).toBeEnabled();
  expect(
    await page.evaluate(
      () => matchMedia("(prefers-reduced-motion: reduce)").matches,
    ),
  ).toBe(true);
});

test("static sequence remains readable with JavaScript disabled", async ({
  browser,
}) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  const response = await page.goto(
    `${testOrigin}${projectBase}how-it-works/request-journey/`,
  );
  expect(response?.ok()).toBeTruthy();
  await expect(
    page.getByRole("heading", { name: "Static sequence descriptions" }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Information sent", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Information not sent", { exact: true }).first(),
  ).toBeVisible();
  await context.close();
});

test("JavaScript presentation collapses the secondary static transcript", async ({
  page,
}) => {
  await page.goto("how-it-works/request-journey/");
  const disclosure = page.locator(".static-sequence-disclosure").first();
  await expect(disclosure).not.toHaveAttribute("open", "");
  await expect(
    disclosure.getByRole("heading", { name: "Static sequence descriptions" }),
  ).not.toBeVisible();
  await disclosure.locator(":scope > summary").click();
  await expect(
    disclosure.getByRole("heading", { name: "Static sequence descriptions" }),
  ).toBeVisible();
});

test("home exposes global navigation on a narrow viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("");
  const menu = page.locator(".mobile-primary");
  await expect(menu.locator(":scope > summary")).toBeVisible();
  await menu.locator(":scope > summary").click();
  const navigation = page.getByRole("navigation", {
    name: "Primary navigation menu",
  });
  await expect(navigation).toBeVisible();
  await expect(navigation.getByRole("link")).toHaveText([
    "Start",
    "How it works",
    "Security",
    "Guides",
    "Reference",
  ]);
});

test("four-layer diagram uses an aligned two-by-two grid", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("how-it-works/code-architecture/");
  const nodes = page.locator('[data-diagram="code-layers"] .diagram-node');
  await expect(nodes).toHaveCount(4);
  const boxes = await nodes.evaluateAll((elements) =>
    elements.map((element) => {
      const box = element.getBoundingClientRect();
      return { x: box.x, y: box.y, width: box.width, height: box.height };
    }),
  );
  expect(Math.abs(boxes[0].y - boxes[1].y)).toBeLessThanOrEqual(1);
  expect(Math.abs(boxes[0].height - boxes[1].height)).toBeLessThanOrEqual(1);
  expect(Math.abs(boxes[2].y - boxes[3].y)).toBeLessThanOrEqual(1);
  expect(Math.abs(boxes[2].height - boxes[3].height)).toBeLessThanOrEqual(1);
  expect(boxes[2].y).toBeGreaterThan(boxes[0].y + boxes[0].height);
});

test("360px mobile layout has no page-level horizontal overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 });
  for (const route of [
    "",
    "how-it-works/request-journey/",
    "how-it-works/credentials/",
    "how-it-works/state-and-recovery/",
    "reference/cli/",
  ]) {
    await page.goto(route);
    const overflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    );
    expect(
      overflow,
      `${route || "home"} horizontal overflow`,
    ).toBeLessThanOrEqual(1);
    await expect(page.locator("main")).toBeVisible();
  }

  await page.goto("how-it-works/credentials/");
  const responsiveTable = page
    .locator('table[data-tobari-responsive-table="true"]')
    .first();
  await expect(responsiveTable).toBeVisible();
  const firstCell = responsiveTable.locator("tbody td").first();
  expect(await firstCell.getAttribute("data-label")).toBeTruthy();
  expect(
    await firstCell.evaluate(
      (cell) => getComputedStyle(cell, "::before").content !== "none",
    ),
  ).toBe(true);
});

test("project base path owns generated assets and links", async ({ page }) => {
  await page.goto("");
  const localUrls = await page
    .locator("link[href], script[src], a[href]")
    .evaluateAll((elements) =>
      elements
        .map(
          (element) =>
            element.getAttribute("href") || element.getAttribute("src"),
        )
        .filter((value): value is string => Boolean(value))
        .filter((value) => value.startsWith("/")),
    );
  expect(localUrls.every((value) => value.startsWith(projectBase))).toBe(true);
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute(
    "href",
    `${testOrigin.replace("http://127.0.0.1:4322", "https://tasuku43.github.io")}${projectBase}`,
  );
  const stylesheet = page.locator('link[rel="stylesheet"]');
  expect(await stylesheet.count()).toBeGreaterThan(0);
  const stylesheetHrefs = await stylesheet.evaluateAll((elements) =>
    elements.map((element) => element.getAttribute("href")),
  );
  expect(stylesheetHrefs.every((value) => value?.startsWith(projectBase))).toBe(
    true,
  );
});
