import type { LocalizedText } from "./credentialArchitecture";

export interface ProviderToolPair {
  providerId: string;
  providerName: LocalizedText;
  toolCommand: string;
  toolName: LocalizedText;
  acquisition: LocalizedText;
  mode: "login" | "import";
}

// This is a presentation projection of the reviewed built-in manifests and
// fixed CLI drivers. Runtime authority remains in those contracts; this data
// exists only so the bilingual site renders one consistent support map.
export const providerToolPairs: ProviderToolPair[] = [
  {
    providerId: "openai",
    providerName: { en: "OpenAI account", ja: "OpenAI アカウント" },
    toolCommand: "codex",
    toolName: { en: "Codex CLI", ja: "Codex CLI" },
    acquisition: {
      en: "Reviewed host-side ChatGPT OAuth flow through exact Codex 0.146.0",
      ja: "信頼するホスト上の Codex 0.146.0 を使うレビュー済み ChatGPT OAuth フロー",
    },
    mode: "login",
  },
  {
    providerId: "anthropic",
    providerName: { en: "Anthropic account", ja: "Anthropic アカウント" },
    toolCommand: "claude",
    toolName: { en: "Claude Code", ja: "Claude Code" },
    acquisition: {
      en: "Reviewed host-side setup-token flow through exact Claude Code 2.1.220",
      ja: "信頼するホスト上の Claude Code 2.1.220 を使うレビュー済み setup-token フロー",
    },
    mode: "login",
  },
  {
    providerId: "github",
    providerName: { en: "GitHub", ja: "GitHub" },
    toolCommand: "gh",
    toolName: { en: "GitHub CLI", ja: "GitHub CLI" },
    acquisition: {
      en: "Reviewed GitHub device flow",
      ja: "レビュー済みの GitHub デバイスフロー",
    },
    mode: "login",
  },
  {
    providerId: "aws",
    providerName: { en: "AWS", ja: "AWS" },
    toolCommand: "aws",
    toolName: { en: "AWS CLI", ja: "AWS CLI" },
    acquisition: {
      en: "Identity Center or Console login method",
      ja: "Identity Center またはコンソールログイン",
    },
    mode: "login",
  },
  {
    providerId: "datadog",
    providerName: { en: "Datadog", ja: "Datadog" },
    toolCommand: "pup",
    toolName: { en: "pup", ja: "pup" },
    acquisition: {
      en: "Reviewed fixed-US1 OAuth flow",
      ja: "レビュー済みの US1 固定 OAuth フロー",
    },
    mode: "login",
  },
  {
    providerId: "chatwork",
    providerName: { en: "Chatwork", ja: "Chatwork" },
    toolCommand: "cwk",
    toolName: { en: "cwk", ja: "cwk" },
    acquisition: {
      en: "Protected non-terminal stdin import",
      ja: "保護された非ターミナル標準入力からのインポート",
    },
    mode: "import",
  },
];
