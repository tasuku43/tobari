import { expect, test } from "@playwright/test";

test.describe("task-first documentation flow", () => {
  test("the English home page explains the product before component detail", async ({
    page,
  }) => {
    await page.goto("./");

    await expect(
      page.getByRole("heading", {
        name: "Run a coding agent inside one selected project.",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Try it in five minutes" }),
    ).toHaveAttribute("href", /\/start\/quickstart\/$/);
    await expect(page.locator(".home-flow > li")).toHaveCount(4);
    await expect(
      page.getByText(
        "files inside the selected project are not protected from the agent",
        { exact: false },
      ),
    ).toBeVisible();
  });

  test("the Japanese home page exposes runtime setup as a first-use task", async ({
    page,
  }) => {
    await page.goto("ja/");

    await expect(
      page.getByRole("heading", {
        name: "コーディングエージェントを隔離環境で実行し、外部通信を制御する。",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "通信が拒否された" }),
    ).toHaveAttribute("href", /\/ja\/start\/first-denial\/$/);
    await expect(
      page.getByRole("link", { name: "カスタムランタイムを作る" }),
    ).toHaveAttribute("href", /\/ja\/guides\/runtime-customization\/$/);
    await expect(page.locator(".home-tasks > a")).toHaveCount(5);
  });

  test("quickstart stays focused and points to the usable runtime setup", async ({
    page,
  }) => {
    await page.goto("start/quickstart/");

    await expect(page.locator(".quickstart-flow > li")).toHaveCount(6);
    await expect(page.locator(".quickstart-step")).toHaveCount(5);
    await expect(
      page.getByRole("heading", { name: "What you have now verified" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Set up the coding-agent runtime" }),
    ).toHaveAttribute("href", /\/guides\/runtime-customization\/$/);
    await expect(page.locator("tobari-sequence-explorer")).toHaveCount(0);
  });

  test("runtime setup explains the mandatory first-use path", async ({ page }) => {
    await page.goto("guides/runtime-customization/");

    await expect(
      page.getByRole("heading", { name: "Set up a custom runtime", level: 1 }),
    ).toBeVisible();
    await expect(page.locator("[data-runtime-setup-flow] li")).toHaveCount(5);
    await expect(
      page.getByText("It does not include a coding agent", { exact: false }),
    ).toBeVisible();
    await expect(
      page.getByText("npm install --global @openai/codex"),
    ).toBeVisible();
    await expect(page.locator(".learning-badge-step")).toHaveText("3 / 10");
  });

  test("learning pages start with installation and the practical setup", async ({
    page,
  }) => {
    await page.goto("start/overview/");

    await expect(page.locator(".learning-badge-step")).toHaveText("1 / 10");
    await expect(page.locator("nav.learning-progress")).toBeVisible();
    await expect(
      page.locator("nav.learning-progress .learning-next strong"),
    ).toHaveText("Install");
  });

  test("CLI reference starts from the task instead of a flat command list", async ({
    page,
  }) => {
    await page.goto("reference/cli/");

    await expect(
      page.getByRole("heading", { name: "Choose what you need to do" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "tobari policy review" }),
    ).toHaveAttribute("href", "#command-policy-review");
  });
});
