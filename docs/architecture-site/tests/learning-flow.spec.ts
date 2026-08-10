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
    await expect(page.locator(".home-flow > li")).toHaveCount(3);
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
        name: "コーディングエージェントを、選んだプロジェクトの中で動かす。",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "通信が拒否された" }),
    ).toHaveAttribute("href", /\/ja\/start\/first-denial\/$/);
    await expect(page.locator(".home-tasks > a")).toHaveCount(5);
  });

  test("quickstart stays focused on five observable steps", async ({
    page,
  }) => {
    await page.goto("start/quickstart/");

    await expect(page.locator(".quickstart-step")).toHaveCount(5);
    await expect(
      page.getByRole("heading", { name: "What you have now verified" }),
    ).toBeVisible();
    await expect(page.locator("tobari-sequence-explorer")).toHaveCount(0);
  });

  test("learning pages show current position and the next learning goal", async ({
    page,
  }) => {
    await page.goto("start/overview/");

    await expect(page.locator(".learning-badge-step")).toHaveText("1 / 10");
    await expect(page.locator("nav.learning-progress")).toBeVisible();
    await expect(
      page.locator("nav.learning-progress .learning-next strong"),
    ).toHaveText("Mental model");
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
