import type { SequenceScenario, SequenceStep } from "./sequences";

type StepTranslation = Pick<
  SequenceStep,
  "title" | "sent" | "withheld" | "owner" | "failure" | "explanation"
>;

export interface LocalizedSequenceStep extends SequenceStep {
  fromLabel: string;
  toLabel: string;
}

export interface LocalizedSequenceScenario extends Omit<
  SequenceScenario,
  "steps"
> {
  steps: LocalizedSequenceStep[];
}

export const sequenceActorLabelsJa: Record<string, string> = {
  "Workspace process": "Workspace のプロセス",
  "Workspace source endpoint": "Workspace の送信元 endpoint",
  Gateway: "Gateway（通信の執行）",
  OPA: "OPA（ポリシー判断）",
  "OPA runtime": "OPA ランタイム",
  "OPA / Upstream": "OPA／接続先",
  "DNS / resolver": "DNS／名前解決",
  Upstream: "接続先",
  "Review store": "確認記録ストア",
  "Host diagnostics": "ホスト診断",
  "Host operator": "信頼するホストの操作者",
  "Tobari CLI": "Tobari CLI（ホスト側）",
  "Policy validator": "ポリシー検証器",
  "User / agent workflow": "利用者／エージェントの手順",
};

export const sequenceUiJa = {
  defaultTitle: "リクエストのシーケンスを追う",
  eyebrow: "操作できるシーケンス",
  introduction:
    "自動再生はしません。シナリオを選び、処理と判断を一段階ずつ追ってください。",
  scenario: "シナリオ",
  controlsLabel: "シーケンス操作",
  previous: "前へ",
  next: "次へ",
  play: "再生",
  pause: "一時停止",
  restart: "最初から",
  sharedCluster: "共有クラスター",
  fixedActorMap: "固定されたコンポーネント配置図",
  sent: "送る情報",
  withheld: "送らない情報",
  owner: "担当するコンポーネント",
  failure: "この段階で失敗した場合",
  keyboardHint:
    "キーボード：この図にフォーカスし、左右の矢印キーで移動します。Space キーで再生と一時停止を切り替えます。",
  staticDisclosure: "完全な静的説明を読む",
  staticHeading: "静的なシーケンス説明",
  staticIntroduction:
    "次の説明は上の操作表示と同じ情報を含み、JavaScript やアニメーションがなくても読めます。",
  staticSent: "送る情報",
  staticWithheld: "送らない情報",
  staticOwner: "担当",
  staticFailure: "失敗時",
  sendMarker: "送信",
  receiveMarker: "受信",
  internalMarker: "内部",
  stepCount: (current: number, total: number) =>
    `${total} 段階中 ${current} 段階目`,
  routeLabel: (title: string, from: string, to: string) =>
    `${title}：${from} から ${to} へ送信`,
  explanation: (from: string, to: string, explanation: string) =>
    `${from} → ${to}。${explanation}`,
} as const;

export const sequenceTitleJa: Record<string, string> = {
  "Request sequence explorer": "リクエストのシーケンスを追う",
  "Host-side policy review and activation": "ホスト側でのポリシー確認と有効化",
  "Network path and closed failure": "通信経路と、障害時に閉じる動作",
  "Denial evidence and trusted-host activation":
    "拒否の証拠と信頼するホストでの有効化",
  "Allowed, denied, and unavailable request paths":
    "許可・拒否・利用不能時のリクエスト経路",
  "Trace a request": "リクエストを追跡する",
  "The quickstart denial and review": "クイックスタートでの拒否と確認",
  "From denied effect to deliberate retry":
    "拒否された通信から意図した再実行まで",
};

const translations: Record<
  string,
  { label: string; summary: string; steps: StepTranslation[] }
> = {
  "allowed-passthrough": {
    label: "通常のリクエストが許可される場合",
    summary: "通常の HTTP 通信を特定し、OPA に一度だけ許可判断を求めます。",
    steps: [
      {
        title: "保護された HTTP リクエストを受け取る",
        sent: "HTTP メソッド、URL、ヘッダー、ストリーミングされる本文",
        withheld: "リクエスト本文が申告する識別情報は信頼しない",
        owner: "Gateway（通信の執行）",
        failure: "ポリシー判断や接続先の処理へ進む前に停止します。",
        explanation:
          "明示的 proxy compatibility と transparent route は Gateway で合流します。",
      },
      {
        title: "プリンシパルを確立する",
        sent: "principal registry 内の host-owned Manifest ID と Workspace ID",
        withheld: "Workspace が送った identity header",
        owner: "Gateway（通信の執行）",
        failure: "送信元 endpoint が未知または曖昧なら fail closed です。",
        explanation:
          "request text ではなく、カーネルが観測した source endpoint が authority を選びます。",
      },
      {
        title: "判断入力を正規化する",
        sent: "principal、scheme、host、port、method、path、秘密を含まない header",
        withheld: "リクエスト本文と認証情報の値",
        owner: "Gateway（通信の執行）",
        failure: "不正な入力は upstream 処理より前に拒否します。",
        explanation: "本文はポリシールールを識別する次元ではありません。",
      },
      {
        title: "OPA へ一度だけ問い合わせる",
        sent: "構造化された許可判断",
        withheld: "認証情報の値とリクエスト本文",
        owner: "OPA（ポリシー判断）",
        failure: "deny、不正な出力、timeout、停止はいずれも経路を閉じます。",
        explanation: "OPA が effect を判断し、Gateway が結果を強制します。",
      },
      {
        title: "接続先を名前解決する",
        sent: "許可された接続先 host 名",
        withheld: "Workspace network access と認証情報の値",
        owner: "Gateway（通信の執行）",
        failure: "名前解決または接続先検証の失敗を返します。",
        explanation: "名前解決は allow 後だけ行い、Gateway が所有します。",
      },
      {
        title: "接続先へ別の接続を作る",
        sent: "許可されたリクエスト。本文はストリーミングします。",
        withheld: "ポリシー内部と信頼済み registry state",
        owner: "Gateway（通信の執行）",
        failure: "接続失敗を返しますが、ポリシーは変えません。",
        explanation: "Workspace に直接の外向き経路はありません。",
      },
      {
        title: "秘密を含まない監査記録を残す",
        sent: "判断次元、結果、上限付き診断情報",
        withheld: "本文と認証情報の値",
        owner: "Gateway（通信の執行）",
        failure: "診断の失敗が deny を allow へ変えることはありません。",
        explanation:
          "audit は payload や認証情報の値を複写せず effect を説明します。",
      },
    ],
  },
  "learnable-denial": {
    label: "学習可能なポリシー拒否",
    summary:
      "現在の完全一致ルールの外にある通信は、接続先へ届く前に拒否します。",
    steps: [
      {
        title: "保護された HTTP リクエストを受け取る",
        sent: "HTTP メソッド、URL、ヘッダー、ストリーミングされる本文",
        withheld: "リクエストが申告する識別情報は信頼しない",
        owner: "Gateway（通信の執行）",
        failure: "不正な入力を拒否します。",
        explanation: "Workspace はホストの Gateway を通じてだけ接続します。",
      },
      {
        title: "判断入力を正規化する",
        sent: "Workspace の識別情報と秘密を含まない HTTP の次元",
        withheld: "リクエスト本文と認証情報の値",
        owner: "Gateway（通信の執行）",
        failure: "正規化に失敗したら通信を終了します。",
        explanation: "本文はポリシールールを識別する次元ではありません。",
      },
      {
        title: "既定の拒否",
        sent: "拒否と、範囲を限定した確認証拠",
        withheld: "自動的なポリシー編集やワイルドカード",
        owner: "OPA（ポリシー判断）",
        failure: "通信は拒否されたままです。",
        explanation: "信頼するホストの操作者が完全一致の証拠を確認します。",
      },
      {
        title: "接続先へ接続しない",
        sent: "何も送らない",
        withheld: "リクエスト全体と外向き通信",
        owner: "Gateway（通信の執行）",
        failure: "外部への副作用は発生しません。",
        explanation: "拒否は接続先への経路を開きません。",
      },
    ],
  },
  "opa-unavailable": {
    label: "OPA を利用できない場合",
    summary: "認可基盤の障害は、許可ではなく通信を閉じる結果になります。",
    steps: [
      {
        title: "保護された HTTP リクエストを受け取る",
        sent: "HTTP メソッド、URL、ヘッダー、ストリーミングされる本文",
        withheld: "リクエストが申告する識別情報は信頼しない",
        owner: "Gateway（通信の執行）",
        failure: "処理を続けられない場合、Gateway で終了します。",
        explanation: "Workspace に直接の外向き経路はありません。",
      },
      {
        title: "ポリシー問い合わせに失敗する",
        sent: "秘密を含まない正規化済み入力",
        withheld: "本文と直接の外向き通信",
        owner: "Gateway（通信の執行）",
        failure: "policy_unavailable として通信を終了します。",
        explanation: "有効な判断がなければ allow にはなりません。",
      },
      {
        title: "接続先には何も届かない",
        sent: "何も送らない",
        withheld: "リクエスト全体",
        owner: "Gateway（通信の執行）",
        failure: "副作用を開始しません。",
        explanation: "壊れたポリシー経路が通信を許可することはありません。",
      },
    ],
  },
  "policy-review": {
    label: "ポリシーの確認と有効化",
    summary:
      "信頼するホストの操作者が証拠を確認し、完全一致ルールを明示的に有効化します。",
    steps: [
      {
        title: "保持された証拠を読む",
        sent: "秘密を含まない完全一致の通信情報と不透明な候補参照",
        withheld: "本文、秘密情報、表示順による権限",
        owner: "Tobari CLI（ホスト側）",
        failure: "不正な証拠は候補になりません。",
        explanation: "候補を表示するだけではポリシーは変わりません。",
      },
      {
        title: "不可分に有効化する",
        sent: "完全に検証した集約済みポリシー",
        withheld: "一時的な半書きのルールセット",
        owner: "Tobari CLI（ホスト側）",
        failure: "検証失敗時は現在のポリシーを維持します。",
        explanation: "操作者が一つの完全一致ルールを選んでから有効化します。",
      },
      {
        title: "意図して再実行する",
        sent: "タスクを再実行する明示的な指示",
        withheld: "Gateway による自動再送",
        owner: "利用者／エージェントの手順",
        failure: "再実行しなければ通信は送信されません。",
        explanation: "以前の拒否を自動で再送することはありません。",
      },
    ],
  },
};

function actorLabel(actor: string): string {
  const label = sequenceActorLabelsJa[actor];
  if (!label)
    throw new Error(`missing Japanese SequenceExplorer actor label: ${actor}`);
  return label;
}

export function localizeSequenceScenarioJa(
  scenario: SequenceScenario,
): LocalizedSequenceScenario {
  const translation = translations[scenario.id];
  if (!translation || translation.steps.length !== scenario.steps.length) {
    throw new Error(
      `missing Japanese SequenceExplorer translation: ${scenario.id}`,
    );
  }
  scenario.actors.forEach(actorLabel);
  return {
    ...scenario,
    label: translation.label,
    summary: translation.summary,
    steps: scenario.steps.map((step, index) => ({
      ...step,
      ...translation.steps[index],
      fromLabel: actorLabel(step.from),
      toLabel: actorLabel(step.to),
    })),
  };
}
