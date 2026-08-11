import { expect, test } from "@playwright/test";

test.describe("progressive first-use onboarding", () => {
  test("runtime setup keeps Context out of the core first-use instructions", async ({
    page,
  }) => {
    await page.goto("ja/start/runtime-setup/");

    await expect(
      page.getByRole("heading", {
        name: "最小ランタイムに、使いたいエージェントと開発ツールを追加する",
      }),
    ).toBeVisible();
    await expect(page.locator(".runtime-step")).toHaveCount(4);
    await expect(page.locator(".runtime-core")).not.toContainText("Context");
    await expect(page.locator(".runtime-advanced")).toContainText("Context");
    await expect(
      page.getByRole("link", { name: /ツールの認証を設定する/ }),
    ).toHaveAttribute("href", /\/ja\/start\/authentication-setup\/$/);
  });

  test("authentication setup uses the default path before introducing Contexts", async ({
    page,
  }) => {
    await page.goto("ja/start/authentication-setup/");

    await expect(
      page.getByRole("heading", {
        name: "ログインできることと、外部通信を許可することは別です",
      }),
    ).toBeVisible();
    await expect(page.locator(".auth-core")).not.toContainText("Context");
    await expect(page.locator(".auth-core")).not.toContainText("--context");
    await expect(page.locator(".auth-advanced")).toContainText("Context");
    await expect(page.getByText("tobari auth login --provider github")).toBeVisible();
  });

  test("learning progress introduces runtime, authentication, then Contexts", async ({
    page,
  }) => {
    await page.goto("start/runtime-setup/");
    await expect(page.locator(".learning-badge-step")).toHaveText("3 / 11");
    await expect(
      page.locator("nav.learning-progress .learning-next strong"),
    ).toHaveText("Authenticate your tools");

    await page.goto("start/authentication-setup/");
    await expect(page.locator(".learning-badge-step")).toHaveText("4 / 11");

    await page.goto("guides/contexts/");
    await expect(page.locator(".learning-badge-step")).toHaveText("10 / 11");
  });
});
