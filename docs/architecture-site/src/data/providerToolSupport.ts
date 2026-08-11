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
    providerId: "openai",
    providerName: { en: "OpenAI", ja: "OpenAI" },
    toolCommand: "codex",
    toolName: { en: "Codex CLI 0.146.0", ja: "Codex CLI 0.146.0" },
    acquisition: {
      en: "Reviewed ChatGPT device OAuth through exact trusted-host Codex",
      ja: "信頼するホスト上の正確な Codex を使う、レビュー済み ChatGPT デバイス OAuth",
    },
    mode: "login",
  },
  {
    providerId: "anthropic",
    providerName: { en: "Anthropic", ja: "Anthropic" },
    toolCommand: "claude",
    toolName: { en: "Claude Code 2.1.220", ja: "Claude Code 2.1.220" },
    acquisition: {
      en: "Reviewed setup-token flow through exact trusted-host Claude Code",
      ja: "信頼するホスト上の正確な Claude Code を使う、レビュー済み setup-token フロー",
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
