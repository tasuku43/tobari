import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { projectBase, testOrigin } from "../site.config.mjs";

const representativePages = [
  "",
  "start/quickstart/",
  "start/runtime-setup/",
  "how-it-works/system-overview/",
  "how-it-works/request-journey/",
  "how-it-works/policy-learning/",
  "how-it-works/credentials/",
  "security/guarantees-and-limitations/",
  "reference/cli/",
  "ja/",
  "ja/start/runtime-setup/",
  "ja/how-it-works/credentials/",
  "ja/security/guarantees-and-limitations/",
  "ja/reference/cli/",
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
    const primaryLabel = route.startsWith("ja/")
      ? "主要ナビゲーション"
      : "Primary";
    await expect(page.locator(`nav[aria-label="${primaryLabel}"]`)).toHaveCount(
      1,
    );
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
  await page.goto("ja/");
  await expect(page.locator("html")).toHaveAttribute(
    "data-theme-selection",
    "system",
  );
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("[data-tobari-theme-select]").first()).toHaveValue(
    "system",
  );
});

test("language switch preserves the current topic and Pages base", async ({
  page,
}) => {
  const topic = "how-it-works/system-overview/";
  const englishPath = `${projectBase}${topic}`;
  const japanesePath = `${projectBase}ja/${topic}`;

  await page.goto(topic);
  const englishSelect = page.locator("starlight-lang-select select").first();
  await expect(englishSelect.locator("option")).toHaveText([
    "English",
    "日本語",
  ]);
  expect(
    await englishSelect
      .locator("option")
      .evaluateAll((options) =>
        options.map((option) => option.getAttribute("value")),
      ),
  ).toEqual([englishPath, japanesePath]);
  await englishSelect.selectOption(japanesePath);
  await page.waitForURL(`**${japanesePath}`);

  await expect(page.locator("html")).toHaveAttribute("lang", "ja");
  await expect(
    page.locator('nav[aria-label="主要ナビゲーション"] > a'),
  ).toHaveText([
    "はじめに",
    "仕組み",
    "セキュリティ",
    "ガイド",
    "リファレンス",
  ]);
  await expect(page.locator("[data-tobari-theme-select]").first()).toHaveText(
    /システム\s*ライト\s*ダーク/,
  );
  await expect(
    page.getByRole("heading", { name: "ドキュメントの生成元" }),
  ).toBeVisible();

  const japaneseSelect = page.locator("starlight-lang-select select").first();
  await expect(japaneseSelect).toHaveValue(japanesePath);
  await japaneseSelect.selectOption(englishPath);
  await page.waitForURL(`**${englishPath}`);
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
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
  await expect(explorer.locator(".sequence-step-index > li")).toHaveCount(7);
  await explorer.locator('[data-sequence-step="3"]').click();
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "Step 4 of 7",
  );
  await explorer.locator('[data-control="restart"]').click();
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
      return ["gateway", "upstream"].map((id) => {
        const box = element
          .querySelector(`[data-node="${id}"]`)
          ?.getBoundingClientRect();
        return box ? { x: box.x - stage.x, y: box.y - stage.y } : null;
      });
    });
  const positionsBefore = await componentPositions();
  await map.locator('[data-conversation="egress"]').click();
  await expect(map.locator('[data-conversation="egress"]')).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(
    map.locator('[data-conversation-detail="egress"]'),
  ).toBeVisible();
  await expect(map.locator('[data-node="gateway"]')).toHaveClass(/node-active/);
  await expect(map.locator('[data-node="upstream"]')).toHaveClass(
    /node-active/,
  );
  await expect(map.locator('[data-node="broker"]')).toHaveCount(0);
  const positionsAfter = await componentPositions();
  expect(positionsAfter).toEqual(positionsBefore);
});

test("credential guide explains release-surface native Workspace ownership", async ({
  page,
}) => {
  await page.goto("how-it-works/credentials/");
  await expect(
    page.getByRole("heading", { name: "Standard ownership" }),
  ).toBeVisible();
  await expect(page.locator("main")).toContainText(
    "the release surface has no Tobari-owned authentication command",
  );
  await expect(page.locator("main")).toContainText(
    "forwards the original values only after allow",
  );
  await expect(page.locator("main")).not.toContainText("research surface");
  await expect(page.locator("main")).not.toContainText("task build:dev");
  await expect(page.locator("main")).not.toContainText("tobari auth login");
  await expect(page.locator("tobari-credential-map")).toHaveCount(0);
});

test("home and request sequence publish Workspace-owned login without an auth-service actor", async ({
  page,
}) => {
  await page.goto("");
  await expect(page.locator("main")).toContainText(
    "Complete the supported tool's login inside the persistent Context Home shared by sibling Workspaces",
  );
  await expect(page.locator("main")).toContainText(
    "Host credentials are never inherited",
  );
  await expect(page.locator("main")).not.toContainText("managed by Broker");
  await expect(page.locator("main")).not.toContainText(
    "primary value in the Workspace",
  );

  await page.goto("how-it-works/request-journey/");
  await expect(page.locator("main")).not.toContainText(
    "native Workspace authentication",
  );
  await expect(page.locator("main")).not.toContainText("Auth Broker");
  await expect(
    page.locator(".actor-lane", { hasText: "native Workspace authentication" }),
  ).toHaveCount(0);
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
  await page.goto("ja/how-it-works/request-journey/");
  const japaneseExplorer = page.locator("tobari-sequence").first();
  await expect(
    japaneseExplorer.locator('[data-field="explanation"]'),
  ).not.toBeEmpty();
  await expect(japaneseExplorer.locator('[data-control="next"]')).toBeEnabled();
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

  const japaneseResponse = await page.goto(
    `${testOrigin}${projectBase}ja/how-it-works/request-journey/`,
  );
  expect(japaneseResponse?.ok()).toBeTruthy();
  await expect(
    page.getByRole("heading", { name: "静的なシーケンス説明" }).first(),
  ).toBeVisible();
  const japaneseTranscript = page
    .locator(".static-sequence-disclosure")
    .first();
  await expect(
    japaneseTranscript.getByText("送る情報", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    japaneseTranscript.getByText("送らない情報", { exact: true }).first(),
  ).toBeVisible();
  await context.close();
});

test("native credential contract remains readable with JavaScript disabled", async ({
  browser,
}) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  const response = await page.goto(
    `${testOrigin}${projectBase}how-it-works/credentials/`,
  );
  expect(response?.ok()).toBeTruthy();
  await expect(
    page.getByRole("heading", { name: "Standard ownership" }),
  ).toBeVisible();
  await expect(page.locator("main")).toContainText(
    "Every process in every Workspace of the same Context can read that state",
  );
  await expect(page.locator("main")).toContainText(
    "Deny performs no DNS or upstream connection",
  );
  await context.close();
});

test("authentication guide publishes native login and hides research-only details", async ({
  browser,
}) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();

  for (const locale of ["", "ja/"]) {
    const response = await page.goto(
      `${testOrigin}${projectBase}${locale}guides/authentication/`,
    );
    expect(response?.ok()).toBeTruthy();

    const main = page.locator("main");
    await expect(main).toContainText("codex login");
    await expect(main).toContainText("agent-ready");
    await expect(main).toContainText("subscriptionType");
    await expect(main).toContainText("rateLimitTier");
    await expect(main).not.toContainText("research surface");
    await expect(main).not.toContainText("task build:dev");
    await expect(main).not.toContainText("tobari auth login");
    await expect(page.locator(".provider-tool-map")).toHaveCount(0);
  }

  await context.close();
});

test("representative English and Japanese release pages hide research recovery journeys", async ({
  page,
}) => {
  const pages = [
    {
      route: "start/install/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|credential revision|root key|optional credential brokering|tobari auth login|task build:dev/i,
    },
    {
      route: "reference/configuration-and-state/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|credential revision|root key|optional credential brokering|tobari auth login|task build:dev/i,
    },
    {
      route: "reference/json-schemas/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|synthetic-provider-v1\.json|credential revision|root key|optional credential brokering|tobari auth login|task build:dev/i,
    },
    {
      route: "start/understanding-check/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|credential revision|root key|optional credential brokering|tobari auth login|task build:dev/i,
    },
    {
      route: "security/guarantees-and-limitations/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|credential revision|root key|optional credential brokering|tobari auth login|task build:dev/i,
    },
    {
      route: "ja/start/install/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|ルート鍵|プロバイダーマニフェスト|認証情報のリビジョン|認証サービス|task build:dev/i,
    },
    {
      route: "ja/reference/configuration-and-state/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|ルート鍵|プロバイダーマニフェスト|認証情報のリビジョン|認証サービス|task build:dev/i,
    },
    {
      route: "ja/reference/json-schemas/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|synthetic-provider-v1\.json|ルート鍵|プロバイダーマニフェスト|認証情報のリビジョン|認証サービス|task build:dev/i,
    },
    {
      route: "ja/start/understanding-check/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|ルート鍵|プロバイダーマニフェスト|認証情報のリビジョン|認証サービス|task build:dev/i,
    },
    {
      route: "ja/security/guarantees-and-limitations/",
      forbidden:
        /Auth Broker|authbroker|provider-manifest|ルート鍵|プロバイダーマニフェスト|認証情報のリビジョン|認証サービス|task build:dev/i,
    },
  ];

  for (const { route, forbidden } of pages) {
    const response = await page.goto(route);
    expect(response?.ok(), route).toBeTruthy();
    const body = await page.locator("main").innerText();
    expect(body, `${route} must remain release-only`).not.toMatch(forbidden);
  }

  for (const route of [
    "reference/faults-and-recovery/",
    "ja/reference/faults-and-recovery/",
  ]) {
    await page.goto(route);
    const filter = page.getByPlaceholder(/policy.*unavailable.*runtime/i);
    await expect(filter).toBeVisible();
    await expect(filter).not.toHaveAttribute("placeholder", /auth login/i);
  }

  const ownershipClaims = [
    {
      route: "reference/configuration-and-state/",
      required: [
        "tool-native authentication state and credentials",
        "does not copy",
        "agent",
      ],
    },
    {
      route: "ja/reference/configuration-and-state/",
      required: [
        "agent CLI",
        "Context Home",
        "ホストのログイン情報を継承しません",
      ],
    },
    {
      route: "security/guarantees-and-limitations/",
      required: [
        "Context Home",
        "agent CLI",
        "host credentials are never inherited",
      ],
    },
    {
      route: "start/learning-path/",
      required: [
        "agent’s own login inside the persistent Context Home shared by sibling Workspaces",
        "does not add a host-side credential service",
      ],
    },
    {
      route: "ja/security/guarantees-and-limitations/",
      required: ["Context Home", "agent CLI", "host credential は継承しません"],
    },
    {
      route: "ja/start/learning-path/",
      required: [
        "同じ Context の sibling Workspace が共有する永続 Context Home の中で agent CLI 自身のログイン",
        "ホスト側の認証サービスを追加しません",
      ],
    },
  ];
  for (const { route, required } of ownershipClaims) {
    await page.goto(route);
    const body = await page.locator("main").innerText();
    for (const phrase of required) {
      expect(body, `${route} must state ${phrase}`).toContain(phrase);
    }
  }
});

test("Japanese sequence controls and static transcript are localized", async ({
  page,
}) => {
  await page.goto("ja/how-it-works/request-journey/");
  const explorer = page.locator("tobari-sequence").first();
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "7 段階中 1 段階目",
  );
  await expect(explorer.locator('[data-control="next"]')).toHaveText("次へ");
  await explorer.focus();
  await page.keyboard.press("ArrowRight");
  await expect(explorer.locator('[data-field="count"]')).toHaveText(
    "7 段階中 2 段階目",
  );
  const disclosure = page.locator(".static-sequence-disclosure").first();
  await disclosure.locator(":scope > summary").click();
  await expect(
    disclosure.getByRole("heading", { name: "静的なシーケンス説明" }),
  ).toBeVisible();
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
  await expect(
    menu.locator("starlight-lang-select select").first(),
  ).toBeVisible();
});

test("four-layer diagram exposes dependency direction as a numbered flow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("how-it-works/code-architecture/");
  const diagram = page.locator('[data-diagram="code-layers"]');
  await expect(diagram.locator(".flow-node")).toHaveCount(4);
  await expect(diagram.locator(".flow-step-list > li")).toHaveCount(5);

  const positions = await diagram.evaluate((element) => {
    const stage = element
      .querySelector(".flow-map-stage")
      ?.getBoundingClientRect();
    if (!stage) return null;
    return Object.fromEntries(
      ["cli", "app", "infra", "domain"].map((id) => {
        const box = element
          .querySelector(`[data-node="${id}"]`)
          ?.getBoundingClientRect();
        return [id, box ? { x: box.x - stage.x, y: box.y - stage.y } : null];
      }),
    );
  });
  expect(positions).not.toBeNull();
  expect(positions!.cli!.y).toBeLessThan(positions!.app!.y);
  expect(positions!.cli!.y).toBeLessThan(positions!.infra!.y);
  expect(positions!.app!.x).toBeLessThan(positions!.infra!.x);
  expect(positions!.domain!.y).toBeGreaterThan(positions!.app!.y);
  expect(positions!.domain!.y).toBeGreaterThan(positions!.infra!.y);
});

test("HTTPS diagram follows one request through both TLS connections", async ({
  page,
}) => {
  await page.goto("ja/how-it-works/https-and-tls/");
  const diagram = page.locator('[data-diagram="tls-split"]');
  await expect(diagram.locator(".flow-node")).toHaveCount(4);
  await expect(diagram.locator(".flow-step-list > li")).toHaveCount(7);
  await expect(diagram.locator('[data-node="workspace"]')).toContainText(
    "Workspace",
  );
  await expect(diagram.locator('[data-node="gateway"]')).toContainText(
    "Gateway",
  );
  await expect(diagram.locator('[data-node="opa"]')).toContainText("OPA");
  await expect(diagram).toContainText("透過入口");
  await expect(diagram).toContainText("TLS");
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
    "ja/",
    "ja/how-it-works/credentials/",
    "ja/reference/cli/",
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

  await page.goto("how-it-works/state-and-recovery/");
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

test("the current Catalog snapshot renders declared fault classifications", async ({
  page,
}) => {
  await page.goto("reference/faults-and-recovery/");
  await page.locator(".fault-reference details summary").first().click();
  const state = page.locator(".fault-reference details ul > li > div").first();
  await expect(state).toContainText("Phase and change state:");
  await expect(state.locator("code")).toHaveText(["precondition", "none"]);
  await expect(page.locator(".fault-reference code:empty")).toHaveCount(0);

  await page.goto("reference/cli/");
  const commandDetailsSummary = page
    .getByText(
      "Output, prerequisites, failures, references, and mutation contract",
      { exact: true },
    )
    .first();
  const commandDetails = commandDetailsSummary.locator("xpath=..");
  await commandDetailsSummary.click();
  await expect(
    commandDetails
      .getByText(/observation; not_applicable; do not repeat unchanged/)
      .first(),
  ).toBeVisible();
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
