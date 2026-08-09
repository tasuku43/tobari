import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { defineConfig } from "astro/config";
import { unified } from "@astrojs/markdown-remark";
import starlight from "@astrojs/starlight";
import { localBase, productSnapshot } from "./site.config.mjs";

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
          label: "Tobari on GitHub",
          href: "https://github.com/tasuku43/tobari",
        },
      ],
      editLink: {
        baseUrl: `https://github.com/tasuku43/tobari/edit/${buildCommit}/docs/architecture-site/`,
      },
      sidebar: [
        {
          label: "Start",
          items: [
            { label: "Overview", link: "/start/overview/" },
            { label: "Install", link: "/start/install/" },
            { label: "Quickstart", link: "/start/quickstart/" },
            { label: "First denial", link: "/start/first-denial/" },
            { label: "Learning path", link: "/start/learning-path/" },
            {
              label: "Understanding check",
              link: "/start/understanding-check/",
            },
          ],
        },
        {
          label: "How it works",
          items: [
            { label: "Mental model", link: "/how-it-works/mental-model/" },
            {
              label: "System overview",
              link: "/how-it-works/system-overview/",
            },
            {
              label: "Workspace, Context, cluster",
              link: "/how-it-works/workspace-context-cluster/",
            },
            {
              label: "Workspace lifecycle",
              link: "/how-it-works/workspace-lifecycle/",
            },
            {
              label: "Request journey",
              link: "/how-it-works/request-journey/",
            },
            { label: "HTTPS and TLS", link: "/how-it-works/https-and-tls/" },
            {
              label: "Project identity",
              link: "/how-it-works/project-identity/",
            },
            {
              label: "Policy learning",
              link: "/how-it-works/policy-learning/",
            },
            { label: "Credentials", link: "/how-it-works/credentials/" },
            {
              label: "State and recovery",
              link: "/how-it-works/state-and-recovery/",
            },
            {
              label: "Implementation stack",
              link: "/how-it-works/implementation-stack/",
            },
            {
              label: "Code architecture",
              link: "/how-it-works/code-architecture/",
            },
          ],
        },
        {
          label: "Security",
          items: [
            { label: "Overview", link: "/security/overview/" },
            {
              label: "Guarantees and limitations",
              link: "/security/guarantees-and-limitations/",
            },
            { label: "Trust boundaries", link: "/security/trust-boundaries/" },
            { label: "Threat model", link: "/security/threat-model/" },
            {
              label: "Credential security",
              link: "/security/credential-security/",
            },
            {
              label: "Audit and privacy",
              link: "/security/audit-and-privacy/",
            },
            { label: "Supply chain", link: "/security/supply-chain/" },
            {
              label: "Verification evidence",
              link: "/security/verification-evidence/",
            },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "Contexts", link: "/guides/contexts/" },
            {
              label: "Runtime customization",
              link: "/guides/runtime-customization/",
            },
            { label: "Authentication", link: "/guides/authentication/" },
            { label: "Policy review", link: "/guides/policy-review/" },
            { label: "Advanced policy", link: "/guides/advanced-policy/" },
            { label: "Colima and Lima", link: "/guides/colima-and-lima/" },
            { label: "Troubleshooting", link: "/guides/troubleshooting/" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "CLI", link: "/reference/cli/" },
            {
              label: "Configuration and state",
              link: "/reference/configuration-and-state/",
            },
            {
              label: "Provider manifest",
              link: "/reference/provider-manifest/",
            },
            { label: "JSON schemas", link: "/reference/json-schemas/" },
            {
              label: "Faults and recovery",
              link: "/reference/faults-and-recovery/",
            },
            {
              label: "Runtime image contract",
              link: "/reference/runtime-image-contract/",
            },
            {
              label: "Component versions",
              link: "/reference/component-versions/",
            },
            { label: "Glossary", link: "/reference/glossary/" },
          ],
        },
      ],
    }),
  ],
});
