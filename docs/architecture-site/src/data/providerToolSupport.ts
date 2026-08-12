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
];
