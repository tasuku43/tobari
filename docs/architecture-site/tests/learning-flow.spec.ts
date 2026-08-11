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

  test("the Japanese home page exposes task-based entry points", async ({
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
      page.getByRole("link", { name: "エージェントと開発ツールを入れる" }),
    ).toHaveAttribute("href", /\/ja\/start\/runtime-setup\/$/);
    await expect(page.locator(".home-tasks > a")).toHaveCount(6);
  });

  test("quickstart stays focused on five observable steps", async ({
    page,
  }) => {
    await page.goto("start/quickstart/");

    await expect(page.locator(".quickstart-flow > li")).toHaveCount(6);
    await expect(page.locator(".quickstart-step")).toHaveCount(5);
    await expect(
      page.getByRole("heading", { name: "What you have now verified" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", {
        name: "Prepare a custom runtime (required before real project work)",
      }),
    ).toHaveAttribute("href", /\/start\/runtime-setup\/$/);
    await expect(page.locator("tobari-sequence-explorer")).toHaveCount(0);
  });

  test("custom runtime setup is an explicit onboarding step", async ({
    page,
  }) => {
    await page.goto("start/runtime-setup/");

    await expect(
      page.getByRole("heading", { name: "1. Create the runtime Dockerfile" }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "4. Enter a real project and verify the agent",
      }),
    ).toBeVisible();
    await expect(page.locator(".learning-badge-step")).toHaveText("3 / 11");
    await expect(
      page.getByRole("link", { name: "Authenticate your tools" }),
    ).toHaveAttribute("href", /\/start\/authentication-setup\/$/);
  });

  test("learning pages show current position and the next learning goal", async ({
    page,
  }) => {
    await page.goto("start/overview/");

    await expect(page.locator(".learning-badge-step")).toHaveText("1 / 11");
    await expect(page.locator("nav.learning-progress")).toBeVisible();
    await expect(
      page.locator("nav.learning-progress .learning-next strong"),
    ).toHaveText("Quickstart");
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
