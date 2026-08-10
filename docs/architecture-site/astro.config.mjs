import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { defineConfig } from "astro/config";
import { unified } from "@astrojs/markdown-remark";
import starlight from "@astrojs/starlight";
import { localBase, productSnapshot } from "./site.config.mjs";
import { navigationGroups } from "./src/navigation.mjs";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const requestedBase = process.env.SITE_BASE || localBase;
const base = `/${requestedBase.split("/").filter(Boolean).join("/")}${requestedBase === "/" ? "" : "/"}`;
const siteOrigin = process.env.SITE_ORIGIN || "https://tasuku43.github.io";
const socialImage = `${siteOrigin}${base}assets/tobari-boundary-og.png`;
const buildCommit =
  process.env.GITHUB_SHA ||
  execFileSync("git", ["rev-parse", "HEAD"], {
    cwd: repositoryRoot,
    encoding: "utf8",
  }).trim();
const generatedAt = process.env.SOURCE_DATE_EPOCH
  ? new Date(Number(process.env.SOURCE_DATE_EPOCH) * 1000).toISOString()
  : execFileSync("git", ["show", "-s", "--format=%cI", productSnapshot], {
      cwd: repositoryRoot,
      encoding: "utf8",
    }).trim();

function prefixProjectBase(options) {
  return (tree) => {
    const elements = (node, tagName) =>
      (node?.children || []).filter((child) => child.tagName === tagName);
    const textContent = (node) => {
      if (node?.type === "text") return node.value || "";
      return (node?.children || []).map(textContent).join("");
    };
    const annotateResponsiveTable = (table) => {
      const head = elements(table, "thead")[0];
      const headerRow = elements(head, "tr")[0];
      const labels = (headerRow?.children || [])
        .filter((child) => child.tagName === "th" || child.tagName === "td")
        .map((cell) => textContent(cell).replace(/\s+/g, " ").trim());
      if (!labels.length || labels.some((label) => !label)) return;

      const bodyRows = elements(table, "tbody").flatMap((body) =>
        elements(body, "tr"),
      );
      if (!bodyRows.length) return;
      table.properties ||= {};
      table.properties.dataTobariResponsiveTable = "true";
      for (const row of bodyRows) {
        const cells = (row.children || []).filter(
          (child) => child.tagName === "td",
        );
        cells.forEach((cell, index) => {
          cell.properties ||= {};
          cell.properties.dataLabel = labels[index] || `Column ${index + 1}`;
        });
      }
    };
    const visit = (node) => {
      const href = node?.properties?.href;
      if (typeof href === "string") {
        const evidence = href.match(
          /^https:\/\/github\.com\/tasuku43\/tobari\/(blob|tree)\/[0-9a-f]{40}(\/.*)$/,
        );
        if (evidence) {
          node.properties.href = `https://github.com/tasuku43/tobari/${evidence[1]}/${options.snapshot}${evidence[2]}`;
        }
      }
      const normalizedHref = node?.properties?.href;
      if (
        typeof normalizedHref === "string" &&
        normalizedHref.startsWith("/") &&
        !normalizedHref.startsWith("//") &&
        options.base !== "/" &&
        !normalizedHref.startsWith(options.base)
      ) {
        node.properties.href = `${options.base.slice(0, -1)}${normalizedHref}`;
      }
      if (node?.tagName === "table" || node?.tagName === "pre") {
        node.properties ||= {};
        node.properties.tabIndex = 0;
      }
      if (node?.tagName === "table") annotateResponsiveTable(node);
      if (node?.tagName === "aside") {
        node.properties ||= {};
        node.properties.role ||= "note";
      }
      if (
        node?.type === "mdxJsxFlowElement" ||
        node?.type === "mdxJsxTextElement"
      ) {
        if (node.name === "table" || node.name === "pre") {
          node.attributes ||= [];
          if (
            !node.attributes.some((attribute) => attribute.name === "tabIndex")
          ) {
            node.attributes.push({
              type: "mdxJsxAttribute",
              name: "tabIndex",
              value: 0,
            });
          }
        }
        if (node.name === "aside") {
          node.attributes ||= [];
          if (!node.attributes.some((attribute) => attribute.name === "role")) {
            node.attributes.push({
              type: "mdxJsxAttribute",
              name: "role",
              value: "note",
            });
          }
        }
      }
      for (const child of node?.children || []) visit(child);
    };
    visit(tree);
  };
}

export default defineConfig({
  site: siteOrigin,
  base,
  output: "static",
  markdown: {
    processor: unified({
      rehypePlugins: [[prefixProjectBase, { base, snapshot: productSnapshot }]],
    }),
  },
  vite: {
    define: {
      __TOBARI_DOCS_COMMIT__: JSON.stringify(productSnapshot),
      __TOBARI_DOCS_BUILD_COMMIT__: JSON.stringify(buildCommit),
      __TOBARI_DOCS_GENERATED_AT__: JSON.stringify(generatedAt),
      __TOBARI_DOCS_REPOSITORY__: JSON.stringify(
        "https://github.com/tasuku43/tobari",
      ),
    },
  },
  integrations: [
    starlight({
      title: "Tobari",
      description:
        "How Tobari runs project workspaces, mediates outbound HTTP and HTTPS, and keeps authorization and credentials separate.",
      head: [
        {
          tag: "meta",
          attrs: { name: "tobari-product-snapshot", content: productSnapshot },
        },
        {
          tag: "meta",
          attrs: { name: "tobari-docs-build", content: buildCommit },
        },
        { tag: "meta", attrs: { property: "og:image", content: socialImage } },
        { tag: "meta", attrs: { property: "og:image:width", content: "1200" } },
        { tag: "meta", attrs: { property: "og:image:height", content: "630" } },
        {
          tag: "meta",
          attrs: {
            property: "og:image:alt",
            content: "An abstract path crossing layered Tobari boundaries.",
          },
        },
        {
          tag: "meta",
          attrs: { name: "twitter:card", content: "summary_large_image" },
        },
        { tag: "meta", attrs: { name: "twitter:image", content: socialImage } },
      ],
      customCss: ["./src/styles/custom.css"],
      locales: {
        root: { label: "English", lang: "en" },
        ja: { label: "日本語", lang: "ja" },
      },
      defaultLocale: "root",
      components: {
        Header: "./src/components/GlobalHeader.astro",
        Footer: "./src/components/PageFooter.astro",
        ThemeProvider: "./src/components/ThemeProvider.astro",
        ThemeSelect: "./src/components/ThemeSelect.astro",
      },
      credits: false,
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/tasuku43/tobari",
        },
      ],
      editLink: {
        baseUrl: `https://github.com/tasuku43/tobari/edit/${buildCommit}/docs/architecture-site/`,
      },
      sidebar: navigationGroups,
    }),
  ],
});
