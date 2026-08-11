import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

async function expectContextAfterLaterHeading(page: import("@playwright/test").Page) {
  const text = (await page.locator("main").innerText()).replace(/\s+/g, " ");
  const later = text.indexOf("あとで:");
  const context = text.indexOf("Context");
  expect(later).toBeGreaterThan(-1);
  expect(context).toBeGreaterThan(later);
}

test.describe("progressive first-use onboarding", () => {
  test("runtime setup postpones Context until the optional later section", async ({
    page,
  }) => {
    await page.goto("ja/start/runtime-setup/");

    await expect(
      page.getByRole("heading", { name: "1. ランタイム用の Dockerfile を作る" }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "4. 実際のプロジェクトへ入り、エージェントを確認する",
      }),
    ).toBeVisible();
    await expect(page.locator("main")).not.toContainText("--context");
    await expectContextAfterLaterHeading(page);
    await expect(
      page.getByRole("link", { name: "ツールの認証を設定する" }),
    ).toHaveAttribute("href", /\/ja\/start\/authentication-setup\/$/);
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  });

  test("authentication setup uses the default path before introducing Contexts", async ({
    page,
  }) => {
    await page.goto("ja/start/authentication-setup/");

    await expect(
      page.getByRole("heading", { name: "ツールに合う認証方法を選ぶ" }),
    ).toBeVisible();
    await expect(page.locator("main")).not.toContainText("--context");
    await expectContextAfterLaterHeading(page);
    await expect(
      page.getByText("tobari auth login --provider github"),
    ).toBeVisible();
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
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
