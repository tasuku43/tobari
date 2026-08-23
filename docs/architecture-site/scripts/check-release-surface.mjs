import { readFile, readdir, stat } from "node:fs/promises";
import { extname, join, relative, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const target = process.argv[2] ? resolve(root, process.argv[2]) : root;
const textExtensions = new Set([
  ".astro",
  ".css",
  ".html",
  ".js",
  ".json",
  ".md",
  ".mdx",
  ".mjs",
  ".svg",
  ".ts",
]);
const forbidden = [
  {
    label: "research surface vocabulary",
    pattern:
      /\b(?:research\s+surface|research-surface|repository\s+research|repository-only\s+research|experimental\s+(?:surface|profile|build|authentication))\b/i,
  },
  {
    label: "research command path",
    pattern:
      /(?:\b(?:tobari\s+auth\s+(?:login|import|status|logout)|tobari\s+serve)\b|`auth\s+(?:login|import|status|logout)(?:\s+[^`\n]+)?`|(?:^|\n)\s*auth\s+(?:login|import|status|logout)\b|task\s+build:dev|provider-manifest)/i,
  },
  {
    label: "bare research auth command",
    pattern: /(?<!\bgh\s)\bauth\s+(?:login|import|status|logout)\b/i,
  },
  {
    label: "Broker actor or route",
    pattern:
      /\b(?:auth\s+broker|brokered|broker-handle|broker handle|auth-broker|auth_broker)\b/i,
  },
  {
    label: "research authority store",
    pattern:
      /(?:\bauth\/(?:providers|projection|contexts|keys|projects)\b|\b(?:vaults?|root[- ]key|credential\s+companion|provider\s+(?:manifest|binding|acquisition|projection))\b|\bnative\s+authentication\s+configurations?\b)/i,
  },
  {
    label: "research fault or handle journey",
    pattern:
      /\b(?:broker_auth_required|credential_(?:handle_invalid|broker_unavailable|refresh_outcome_unknown)|opaque\s+(?:credential\s+)?handle(?:s|\s+projection|\s+state|\s+revocation)?|handle\s+(?:projection|state|revocation)|credential\s+handle(?:s?)|credential[- ]binding(?:\s+(?:403|failure|error))?)\b|認証情報の結び付け(?:に関する)?障害/i,
  },
  {
    label: "research fault or source path",
    pattern:
      /\b(?:auth_mutation_outcome_unknown|auth_catalog\.go|broker_credentials\.py|credentialhost|auth_host_login(?:_test)?\.go|authbroker_image_test\.go|check-authbroker-source\.sh)\b|\bauthbroker\//i,
  },
  {
    label: "research provider fixture",
    pattern: /\bsynthetic-provider-v1\.json\b/i,
  },
  {
    label: "research credential placement",
    pattern:
      /(?:primary\s+(?:credential|secret|value)[^\n]{0,100}\b(?:in|into)\s+the\s+Workspace|認証情報の本体[^\n]{0,100}Workspace|managed\s+by\s+Broker)/i,
  },
  {
    label: "research credential state or recovery",
    pattern:
      /(?:encrypted\s+(?:credential|Workspace\s+Manifest\s+Workspace)\s+state|credential\s+Workspace\s+state|installation\s+(?:auth|host-owned\s+)?key|installation\s+host-owned\s+state|credential\s+revision|optional\s+credential\s+brokering|credential\s+brokering|authentication-managed\s+secret|revocable\s+handle|host-owned\s+state[^\n]{0,140}(?:Keychain|encrypted\s+(?:credential|Workspace)|original\s+key)|(?:Keychain|original\s+key)[^\n]{0,140}host-owned\s+state)/i,
  },
  {
    label: "research authentication service role",
    pattern:
      /(?:\b(?:locked|shared)\s+native\s+Workspace\s+authentication\b|\bnative\s+Workspace\s+authentication\s+(?:service|image|source|digest|socket|daemon|component)\b|\bGateway\s*\/\s*native\s+Workspace\s+authentication\b|\bpost-policy\s+authentication\s+resolution\b)/i,
  },
  {
    label: "research authentication contextual role",
    pattern:
      /(?:\bnative\s+Workspace\s+authentication\b[^\n]{0,140}\b(?:identifier|socket|image|service|start(?:up)?|locked|query|resolve|root[- ]key|managed[- ]secret|source)\b|\b(?:identifier|socket|image|service|start(?:up)?|locked|query|resolve|root[- ]key|managed[- ]secret|source)\b[^\n]{0,140}\bnative\s+Workspace\s+authentication\b)/i,
  },
  {
    label: "research authentication contextual role (Japanese)",
    pattern:
      /(?:\bnative\s+Workspace\s+authentication\b[^\n]{0,160}(?:識別子|ソケット|イメージ|サービス|起動|ロック|照会|解決|ルート鍵|管理対象の秘密|ソース|管理|参加|構成要素)|(?:識別子|ソケット|イメージ|サービス|起動|ロック|照会|解決|ルート鍵|管理対象の秘密|ソース|管理|参加|構成要素)[^\n]{0,160}\bnative\s+Workspace\s+authentication\b)/i,
  },
  {
    label: "conflicting principal and credential projection",
    pattern:
      /(?:Workspace\s+identitys|Workspace\s+identity\s+values|configured\s+credential\s+headers|認証情報を含まない[^\n]{0,100}(?:Workspace Manifest|プロジェクト)[^\n]{0,80}識別情報)/i,
  },
  {
    label: "research Japanese credential authority",
    pattern:
      /(?:認証情報(?:の)?リビジョン|ルート鍵|ハンドル(?:記録|投影|状態|発行|解決)?|プロバイダーマニフェスト|認証(?:情報)?(?:の)?(?:照会|解決)|認証(?:ソケット|イメージ|デーモン|識別子)|認証サービス(?!を追加しません|を持ちません|ではありません|として管理しません))/i,
  },
  {
    label: "research compile-time profile",
    pattern:
      /\b(?:compile-time\s+profile|research\s+profile|standard\s+profile)\b/i,
  },
];

async function filesBelow(directory) {
  const result = [];
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) result.push(...(await filesBelow(path)));
    else result.push(path);
  }
  return result;
}

const roots = process.argv[2]
  ? [target]
  : [
      join(root, "src/content/docs"),
      join(root, "src/components"),
      join(root, "src/data"),
      join(root, "src/generated"),
      join(root, "src/navigation.mjs"),
    ];
const files = [];
for (const directory of roots) {
  const info = await stat(directory);
  if (info.isDirectory()) files.push(...(await filesBelow(directory)));
  else files.push(directory);
}

const errors = [];
for (const file of files) {
  if (!textExtensions.has(extname(file))) continue;
  const label = relative(root, file);
  const source = await readFile(file, "utf8");
  for (const rule of forbidden) {
    const match = source.match(rule.pattern);
    if (match) errors.push(`${label}: ${rule.label} (${match[0]})`);
  }
}

if (errors.length) {
  console.error(
    `Release-surface absence check failed with ${errors.length} issue(s):`,
  );
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}
console.log(
  `Release-surface absence check passed for ${files.length} source files at ${relative(root, target) || "."}.`,
);
