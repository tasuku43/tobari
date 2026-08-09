import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { projectBase, testOrigin } from "../site.config.mjs";

const representativePages = [
  "",
  "start/quickstart/",
  "how-it-works/system-overview/",
  "how-it-works/request-journey/",
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
});
