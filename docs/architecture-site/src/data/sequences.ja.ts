import type { SequenceScenario, SequenceStep } from "./sequences";

type StepTranslation = Pick<
  SequenceStep,
  "title" | "sent" | "withheld" | "owner" | "failure" | "explanation"
>;

interface ScenarioTranslation {
  label: string;
  summary: string;
  steps: StepTranslation[];
}

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
  Gateway: "Gateway（通信の執行）",
  "Workspace source endpoint": "Workspace の送信元エンドポイント",
  OPA: "OPA（ポリシー判断）",
  "OPA runtime": "OPA ランタイム",
  "OPA / Upstream": "OPA／接続先",
  "Auth Broker": "Auth Broker（認証情報処理）",
  "Host credential companion": "ホスト認証情報コンパニオン",
  "Datadog token endpoint": "Datadog トークンエンドポイント",
  "DNS / resolver": "DNS／名前解決",
  Upstream: "接続先",
  "Review store": "確認記録ストア",
  "Host diagnostics": "ホスト診断",
  "Host operator": "信頼するホストの操作者",
  "Tobari CLI": "Tobari CLI（ホスト側）",
  "Policy validator": "ポリシー検証器",
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
  "Brokered credential path": "Auth Broker 管理認証情報の経路",
  "Brokered credential paths": "Auth Broker 管理認証情報の経路",
  "Network path and closed failure": "通信経路と、障害時に閉じる動作",
  "Denial evidence and trusted-host activation":
    "拒否の証拠と信頼するホストでの有効化",
  "Allowed, denied, and unavailable request paths":
    "許可・拒否・利用不能時のリクエスト経路",
  "Trace a request": "リクエストを追跡する",
  "Allowed and invalid broker-handle paths":
    "許可された Broker ハンドルと不正なハンドルの経路",
  "The quickstart denial and review": "クイックスタートでの拒否と確認",
  "From denied effect to deliberate retry":
    "拒否された通信から意図的な再実行まで",
  "Credential decision sequence": "認証情報を伴う通信の判断順序",
  "Static, AWS, Datadog, uncertain, and invalid broker paths":
    "静的認証情報、AWS、Datadog、結果不明、不正なハンドルの各経路",
  "Static、AWS、Datadog、uncertain、invalid broker path":
    "静的認証情報、AWS、Datadog、結果不明、不正なハンドルの各経路",
  "static、AWS、Datadog、結果不明、不正 handle の各経路":
    "静的認証情報、AWS、Datadog、結果不明、不正なハンドルの各経路",
  "Auth Broker を使う認証情報の経路": "Auth Broker を使う認証情報の経路",
  ホスト側でのポリシーレビューと有効化: "ホスト側でのポリシー確認と有効化",
  "ネットワーク経路と fail-closed の動作": "通信経路と、障害時に閉じる動作",
  "拒否の根拠と、信頼するホストでの有効化":
    "拒否の証拠と、信頼するホストでの有効化",
  "許可、拒否、障害時のリクエスト経路":
    "許可・拒否・利用不能時のリクエスト経路",
  リクエストを追う: "リクエストを追う",
  "Host-side policy review と activation": "ホスト側でのポリシー確認と有効化",
  "Denial evidence と trusted-host activation":
    "拒否の証拠と信頼するホストでの有効化",
  "Quickstart の拒否とレビュー": "クイックスタートでの拒否と確認",
  "拒否された effect から意図的な retry まで":
    "拒否された通信から意図的な再実行まで",
};

const sequenceScenariosJa: Record<string, ScenarioTranslation> = {
  "allowed-passthrough": {
    label: "通常のリクエストが許可される場合",
    summary:
      "通常の HTTP 通信を特定し、OPA に一度だけ許可判断を求めます。本文を接続先へ送るのは、許可された後です。",
    steps: [
      {
        title: "保護済みの HTTP リクエストを受け取る",
        sent: "HTTP メソッド、URL、ヘッダー、ストリーミングされる本文",
        withheld: "リクエストが申告するプロジェクト ID は信頼しない",
        owner: "Gateway（通信の執行）",
        failure: "ポリシー判断や接続先の処理へ進む前に停止します。",
        explanation:
          "明示的な互換プロキシと、合成 DNS を使う透過経路は Gateway で合流します。",
      },
      {
        title: "プロジェクトプリンシパルを確立する",
        sent: "ホストが管理する登録情報内の Workspace Manifest ID とプロジェクト ID",
        withheld: "Workspace が送った識別情報ヘッダー",
        owner: "Gateway（通信の執行）",
        failure:
          "送信元エンドポイントが不明または曖昧なら、通信を許可せず終了します。",
        explanation:
          "リクエストの自己申告ではなく、カーネルが観測した Workspace の送信元エンドポイントによってプリンシパルが決まります。",
      },
      {
        title: "判断入力を正規化する",
        sent: "プリンシパル、スキーム、ホスト、ポート、HTTP メソッド、正規化したパス、秘密を含まないヘッダー",
        withheld: "リクエスト本文と認証情報ヘッダー",
        owner: "Gateway（通信の執行）",
        failure: "不正な入力を拒否し、接続先へは何も送りません。",
        explanation:
          "リクエスト本文はポリシールールを識別する次元ではありません。",
      },
      {
        title: "OPA へ一度だけ問い合わせる",
        sent: "構造化された許可判断",
        withheld: "認証情報とリクエスト本文",
        owner: "OPA（ポリシー判断）",
        failure:
          "拒否、不正な出力、タイムアウト、利用不能のいずれでも経路を閉じます。",
        explanation:
          "OPA は通常の HTTP 通信を許可するか判断し、Gateway がその判断どおりに経路を制御します。",
      },
      {
        title: "接続先を名前解決する",
        sent: "許可された接続先のホスト名",
        withheld: "Workspace のネットワークアクセスと認証情報",
        owner: "Gateway（通信の執行）",
        failure: "名前解決または接続先検証の失敗をエラーとして返します。",
        explanation: "名前解決は許可後にだけ行い、Gateway が所有します。",
      },
      {
        title: "接続先へ別の接続を作る",
        sent: "許可されたリクエスト。本文はストリーミングする",
        withheld: "ポリシーの内部情報と、信頼済みのプリンシパル登録情報",
        owner: "Gateway（通信の執行）",
        failure: "接続失敗を返しますが、ポリシーは変更しません。",
        explanation:
          "外向きの接続を開くのは Gateway です。Workspace が直接の外向き通信を得ることはありません。",
      },
      {
        title: "秘密を含まない監査記録を残す",
        sent: "判断次元、結果、上限付きの診断メタデータ",
        withheld: "本文と秘密情報を含むヘッダー",
        owner: "Gateway（通信の執行）",
        failure: "診断処理の失敗が、拒否を許可へ変えることはありません。",
        explanation:
          "監査記録にはペイロードや認証情報を複製せず、どの通信を判断したかだけを残します。",
      },
    ],
  },
  "learnable-denial": {
    label: "学習候補として記録されるポリシー拒否",
    summary:
      "有効な完全一致ルールに含まれないリクエストは、接続先へ接続する前に拒否されます。",
    steps: [
      {
        title: "まだ許可されていない通信を受け取る",
        sent: "要求されたホスト、ポート、HTTP メソッド、パス",
        withheld: "リクエスト自身を承認する権限",
        owner: "Gateway（通信の執行）",
        failure: "不正なプロキシ通信を拒否します。",
        explanation:
          "リクエスト自体は信頼しないデータであり、自動的な許可申請ではありません。",
      },
      {
        title: "本文を含まないポリシー入力を作る",
        sent: "信頼済みのプリンシパルと、正規化した許可判断の項目",
        withheld: "本文と認証情報ヘッダー",
        owner: "Gateway（通信の執行）",
        failure: "正規化に失敗すると処理を停止します。",
        explanation:
          "本文だけが異なる二つのリクエストは、同じポリシー候補です。",
      },
      {
        title: "既定の拒否を適用する",
        sent: "拒否と、学習可能であることを示す分類",
        withheld: "ポリシーを編集したり、推測でワイルドカードを作ったりしない",
        owner: "OPA（ポリシー判断）",
        failure: "予期しない出力は許可ではなく、利用不能として扱います。",
        explanation:
          "一致する有効なルールがないため、既定の拒否（default deny）を維持します。",
      },
      {
        title: "証拠を保持する",
        sent: "長さを制限した拒否記録に含める、秘密を含まない完全一致の通信情報",
        withheld:
          "リクエスト本文、秘密情報を含むヘッダー、候補 ID、表示順による権限",
        owner: "Gateway（通信の執行）",
        failure: "証拠を保持できなくても、リクエストは拒否したままです。",
        explanation:
          "信頼するホストの CLI が後からこの証拠を検証し、操作対象を指す不透明な参照を生成します。",
      },
      {
        title: "接続先へ接続しない",
        sent: "403 とホスト側の確認場所",
        withheld: "自動再実行も自動承認も行わない",
        owner: "Gateway（通信の執行）",
        failure: "その通信は拒否されたままです。",
        explanation:
          "Workspace に伝えるのは、操作者が確認する場所だけです。Workspace 自身には通信を許可できません。",
      },
    ],
  },
  "opa-unavailable": {
    label: "OPA を利用できない場合",
    summary:
      "OPA を利用できなければ、通信は許可されず、外部への経路も開きません。",
    steps: [
      {
        title: "リクエストが Gateway へ届く",
        sent: "HTTP 通信の情報",
        withheld: "直接の外向き通信",
        owner: "Gateway（通信の執行）",
        failure: "処理を続けられない場合、Gateway で終了します。",
        explanation: "ネットワーク構成は引き続き直接経路を許しません。",
      },
      {
        title: "ポリシー問い合わせに失敗する",
        sent: "秘密を含まない正規化済み入力",
        withheld: "本文と認証情報",
        owner: "OPA（ポリシー判断）",
        failure:
          "タイムアウト、接続失敗、不正な結果は policy_unavailable になります。",
        explanation: "Gateway は有効な構造化判断を一つ必要とします。",
      },
      {
        title: "Gateway が許可せず失敗する",
        sent: "503 policy_unavailable",
        withheld: "接続先の DNS 問い合わせも接続も行わない",
        owner: "Gateway（通信の執行）",
        failure:
          "呼び出し側は OPA を診断できますが、その通信は許可されません。",
        explanation:
          "許可判断の基盤を利用できない状態は、通常の既定拒否とは別の障害です。",
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
  "broker-allowed": {
    label: "Broker 管理の静的ヘッダーが許可される場合",
    summary:
      "schema 1 の静的な認証情報を取り出すのは、通常の HTTP 通信が許可された後だけです。",
    steps: [
      {
        title: "ハンドルを提示する",
        sent: "宣言済みの送信元ヘッダーに入った、プロジェクトに結び付く不透明なハンドル",
        withheld: "実物の認証情報",
        owner: "Gateway（通信の執行）",
        failure: "識別子が曖昧または不正なら、通信を許可せず失敗します。",
        explanation:
          "ハンドルは制約された Auth Broker のレコードを指します。認証情報そのものではありません。",
      },
      {
        title: "ハンドルを取り除く",
        sent: "ハンドルは Auth Broker の内部処理へだけ渡す",
        withheld: "ハンドルを接続先にも OPA にも送らない",
        owner: "Gateway（通信の執行）",
        failure:
          "Tobari 形式に見える不正なハンドルを、通常のパススルー認証として扱うことはありません。",
        explanation:
          "Gateway はポリシー判断の前に、認識した送信元ヘッダーからハンドルを取り除きます。",
      },
      {
        title: "秘密を含まない検査を行う",
        sent: "ハンドルと、信頼済みの Workspace Manifest／プロジェクト識別情報",
        withheld: "実物の認証情報",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "Workspace Manifest、プロジェクト、プロバイダー、リビジョン、宛先、ヘッダーの結び付きのどれかが異なれば拒否します。",
        explanation:
          "この検査では秘密を開示せずに、レコードの結び付きを確認します。",
      },
      {
        title: "通常の HTTP 通信を判断する",
        sent: "HTTP の判断次元と、秘密を含まない認可メタデータ",
        withheld: "ハンドル、本文、実物の認証情報",
        owner: "OPA（ポリシー判断）",
        failure: "拒否の場合、秘密情報を解決しません。",
        explanation:
          "ログインしても通信の許可ルールは増えません。OPA は引き続きホスト、ポート、HTTP メソッド、パスを判断します。",
      },
      {
        title: "一度だけ解決する",
        sent: "許可済みレコードと、完全一致する HTTP／ヘッダーの結び付き",
        withheld: "秘密は Workspace へ返さない",
        owner: "Auth Broker（認証情報処理）",
        failure: "ロック中、古い状態、不整合のいずれでも許可せず失敗します。",
        explanation:
          "解決はポリシー許可後、このリクエストのために一度だけ行います。",
      },
      {
        title: "宣言済みヘッダーを置き換える",
        sent: "マニフェストが宣言した宛先ヘッダーにだけ実物の認証情報を設定",
        withheld: "ハンドルと Auth Broker の内部情報",
        owner: "Gateway（通信の執行）",
        failure: "別のヘッダーや宛先は試しません。",
        explanation:
          "Gateway は完全一致する HTTPS 宛先へ接続し、許可済みリクエストをストリーミングします。",
      },
    ],
  },
  "aws-brokered-allowed": {
    label: "AWS SigV4 リクエストが許可される場合",
    summary:
      "OPA が通常の AWS 向け HTTP 通信を許可した後、Gateway がサイズ上限の範囲で本文を取り込みます。続いて Auth Broker が、非公開のホストコンパニオンへ認証情報のエクスポートを一度だけ求めます。",
    steps: [
      {
        title: "AWS のプレースホルダーを受け取る",
        sent: "確認済みの AWS 認証情報プレースホルダーに設定された、プロジェクトに結び付く一つのハンドル",
        withheld:
          "アクセスキー、シークレットキー、セッショントークン、信頼済みのプロジェクト識別情報",
        owner: "Gateway（通信の執行）",
        failure:
          "プレースホルダーが不正、混在、曖昧なら、通信を許可せず失敗します。",
        explanation:
          "三つの値は同じ不透明なハンドルであり、利用可能な AWS 認証情報ではありません。",
      },
      {
        title: "署名プランを秘密なしで検査する",
        sent: "ハンドル、ホスト由来のプリンシパル、リビジョン、AWS の接続先、署名プランの結び付き",
        withheld: "不透明なホスト CLI の状態と一時的な AWS ロール認証情報",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "Workspace Manifest、プロジェクト、リビジョン、宛先、署名プランのどれかが異なれば 403 を返します。",
        explanation:
          "Auth Broker はポリシー判断前には秘密を含まないメタデータだけを返します。",
      },
      {
        title: "通常の HTTP 通信を許可するか判断する",
        sent: "Workspace Manifest、プロジェクト、HTTPS の接続先、HTTP メソッド、正規化したパス",
        withheld: "本文、本文のハッシュ、ハンドル、不透明な AWS 状態、認証情報",
        owner: "OPA（ポリシー判断）",
        failure:
          "拒否の場合、本文の取込、コンパニオン呼び出し、署名、接続先処理はすべて行いません。",
        explanation:
          "AWS 認証は完全一致する HTTP ポリシールールの代わりにはなりません。",
      },
      {
        title: "許可済みリクエストを上限内で取り込む",
        sent: "8 MiB の上限内にある許可済みリクエスト全体と、そのハッシュ",
        withheld: "本文とそのハッシュを OPA、監査、保管庫の状態へ渡さない",
        owner: "Gateway（通信の執行）",
        failure:
          "大きすぎる、または曖昧な署名形式は接続先へアクセスせず拒否します。",
        explanation:
          "AWS SigV4 は、許可後に通常のストリーミングを行わない、確認済みの例外です。",
      },
      {
        title: "認証情報を一度だけエクスポートする",
        sent: "認証済みの固定処理、完全一致するリビジョン、暗号化された不透明なドライバー状態",
        withheld:
          "Workspace のデータで argv、実行ファイル、プロファイル、ホストソケットを選ばせない",
        owner: "ホスト認証情報コンパニオン",
        failure:
          "外部処理の開始前に失敗したと確認できれば 503、開始後または明示的に結果不明なら再実行不可の 409 です。",
        explanation:
          "同じバイナリとして常駐するコンパニオンは、コンパイル済みの AWS 認証情報エクスポート処理だけを実行します。",
      },
      {
        title: "Auth Broker 内で署名する",
        sent: "上限付きのプロセス認証情報結果と、更新された不透明な状態",
        withheld:
          "一時的な AWS 認証情報を Workspace、OPA、ポリシー、永続化する投影へ渡さない",
        owner: "Auth Broker（認証情報処理）",
        failure: "古いリビジョンまたは不整合な状態は転送前に拒否します。",
        explanation:
          "Auth Broker は同じリビジョンを再確認して保存し、標準的なヘッダー方式の SigV4 を計算します。",
      },
      {
        title: "署名済みリクエストを接続先へ送る",
        sent: "Auth Broker が生成した SigV4 ヘッダーを持つ許可済みリクエストを、別の TLS 接続で送る",
        withheld:
          "不透明なハンドル、コンパニオンの通信手順、ルートキー、ホスト CLI のキャッシュ",
        owner: "Gateway（通信の執行）",
        failure: "別の宛先、再送、別の署名形式は試しません。",
        explanation:
          "Gateway はリクエストのスナップショットを検証し、返されたヘッダーだけを適用して、一度だけ接続先へ送ります。",
      },
    ],
  },
  "datadog-refresh-allowed": {
    label: "Datadog OAuth 更新が許可される場合",
    summary:
      "OPA の許可後、Auth Broker は十分な有効期間が残るトークンを選ぶか、固定された Datadog US1 エンドポイントで一度だけ更新してから、Gateway が接続先へ接続します。",
    steps: [
      {
        title: "Bearer ハンドルを受け取る",
        sent: "確認済みの Bearer 構文に入った、プロジェクトに結び付くハンドル",
        withheld:
          "OAuth アクセストークン、リフレッシュトークン、クライアントシークレット",
        owner: "Gateway（通信の執行）",
        failure:
          "宛先や構文が違う場合、または識別子が重複する場合は、別の認証方式へ切り替えず 403 を返します。",
        explanation:
          "ハンドルは制約されたレコードを選びますが、ネットワーク権限は与えません。",
      },
      {
        title: "セッションの結び付きを秘密なしで検査する",
        sent: "ハンドル、ホスト由来のプリンシパル、同じリビジョン、US1 の宛先、Bearer ヘッダーの結び付き",
        withheld: "暗号化された OAuth セッションと、すべてのトークン値",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "コピー、古い状態、不一致のいずれでも OPA より前に失敗します。",
        explanation:
          "正規化した、秘密を含まないプロバイダーメタデータだけを Gateway へ返します。",
      },
      {
        title: "通常の HTTP 通信を許可するか判断する",
        sent: "Workspace Manifest、プロジェクト、プロバイダー ID、HTTPS の接続先、HTTP メソッド、正規化したパス",
        withheld: "本文、ハンドル、リビジョン、OAuth クライアント、トークン",
        owner: "OPA（ポリシー判断）",
        failure:
          "拒否の場合、トークン選択、更新、接続先処理はすべて行いません。",
        explanation:
          "Datadog へのログインに成功しても、HTTP 通信の許可ルールは作られません。",
      },
      {
        title: "許可後に更新の要否を判断する",
        sent: "許可判断に使ったものと同じリビジョンと、対象の Bearer ヘッダー／宛先の結び付き",
        withheld:
          "リクエスト本文は認証情報の次元にもポリシーの次元にもならない",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "ロック中、古い状態、または永続バリアがある場合は通信を許可せず失敗します。",
        explanation:
          "Auth Broker は有効期限まで 5 分を超えるトークンだけを再利用し、それ以外では同じレコードの更新を一度開始します。",
      },
      {
        title: "固定されたトークンエンドポイントで交換する",
        sent: "検証済み TLS で https://api.datadoghq.com/oauth2/v1/token へ送る、上限付きの OAuth 更新フォーム一件",
        withheld:
          "環境由来のプロキシ、リダイレクト、別ホスト、Workspace 入力、pup プロセス",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "送信前の失敗と確認できれば 503、明示的または送信後に結果を確定できなければ再実行不可の 409 となり、永続バリアを残します。",
        explanation:
          "Datadog の更新は Auth Broker が所有します。信頼するホストの pup ドライバーはログイン時だけ使います。",
      },
      {
        title: "更新した状態を確定する",
        sent: "同じ認証情報リビジョンに対する、厳密かつ上限付きのトークン応答",
        withheld: "トークンを Workspace、OPA、監査、CLI 出力へ渡さない",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "不正または古い応答では実行バリアを解除できず、接続先にも届きません。",
        explanation:
          "Auth Broker は更新した OAuth セッションを不可分に保存してから、そのリクエストだけで使う Bearer 値を返します。",
      },
      {
        title: "Bearer 認証付きリクエストを接続先へ送る",
        sent: "そのリクエストだけで使うアクセストークンを付け、別の TLS 接続で送る",
        withheld:
          "不透明なハンドル、リフレッシュトークン、OAuth クライアントシークレット、保管庫の状態",
        owner: "Gateway（通信の執行）",
        failure:
          "接続先のエラーでポリシーを変えたり、更新を再実行したりしません。",
        explanation:
          "Gateway は宣言済みの Authorization ヘッダーだけを置き換え、一度だけ接続先へ送ります。",
      },
    ],
  },
  "credential-outcome-unknown": {
    label: "認証情報更新の結果が不明な場合",
    summary:
      "Datadog のトークン交換を始めた後に、結果を確認できなくなった場合を追います。AWS コンパニオン処理にも、同じ永続バリアと再実行不可の 409 が適用されます。",
    steps: [
      {
        title: "永続バリアを書き込む",
        sent: "暗号化された認証情報レコード内に保存する、同じリビジョンの操作ダイジェスト",
        withheld: "秘密の値と本文",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "バリアを不可分に書き込めなければ、外部の認証情報処理を開始しません。",
        explanation:
          "AWS コンパニオン処理または Datadog の更新を始める前に、バリアを永続化します。",
      },
      {
        title: "Datadog 更新の結果が曖昧になる",
        sent: "固定された Datadog 更新リクエスト 1 件。AWS の場合は、代わりに確認済みのコンパニオン処理を 1 件開始する",
        withheld: "接続先へのアプリケーションリクエストはまだ送っていない",
        owner: "Auth Broker（認証情報処理）",
        failure:
          "外部処理の開始後に切断した場合、安全に再実行できるとは判断できません。",
        explanation:
          "結果を受け取れなくても、トークンエンドポイントが交換を完了した可能性があります。AWS コンパニオンも、処理開始後は同じ扱いです。",
      },
      {
        title: "Gateway が再実行不可の 409 を返す",
        sent: "credential_refresh_outcome_unknown",
        withheld: "認証情報、署名ヘッダー、自動再実行、接続先への送信",
        owner: "Gateway（通信の執行）",
        failure: "呼び出し側の自動再実行は停止したままにする必要があります。",
        explanation:
          "これは、実行前と分かっている 503 の可用性障害とは異なります。",
      },
      {
        title: "操作者が状態を照合する",
        sent: "元のリクエストが終了した後、意図的に実行する auth status の確認",
        withheld:
          "状態を確認しないまま再実行したり、プロバイダーの状態を推測したりしない",
        owner: "信頼するホストの操作者",
        failure: "Auth Broker がロック中または利用不能なら、先に復旧します。",
        explanation:
          "ready かつ configured なら明示的に再実行できます。not_configured なら、再ログインするか logout を実行し、Workspace へ入り直す必要があります。",
      },
      {
        title: "接続先には何も届かない",
        sent: "何も送らない",
        withheld: "アプリケーションリクエスト全体",
        owner: "Gateway（通信の執行）",
        failure: "外部のアプリケーション通信を開始しません。",
        explanation:
          "認証情報の更新結果が不明でも、元のアプリケーションリクエストを送ることはありません。",
      },
    ],
  },
  "invalid-handle": {
    label: "不正または古い Broker ハンドル",
    summary:
      "コピーされた、古い、不正な、または結び付きが異なる Tobari ハンドルを、通常のパススルー認証として扱うことはできません。",
    steps: [
      {
        title: "Tobari ハンドルの識別子を検出する",
        sent: "Auth Broker ハンドルの識別子を持つ値",
        withheld: "実物の認証情報は Workspace に存在しない",
        owner: "Gateway（通信の執行）",
        failure: "識別子が曖昧なら、直ちに不正と判断します。",
        explanation:
          "識別子を認識した時点で、Auth Broker が管理する認証情報として処理します。",
      },
      {
        title: "結び付きの検査に失敗する",
        sent: "ハンドルと信頼済みのプリンシパル",
        withheld: "秘密情報の解決",
        owner: "Auth Broker（認証情報処理）",
        failure: "不正、古い、コピーされた、不一致のハンドルは拒否します。",
        explanation:
          "レコードは Workspace Manifest、プロジェクト、プロバイダー、認証情報のリビジョン、宛先、ヘッダーの結び付きのすべてに一致する必要があります。",
      },
      {
        title: "フォールバックしない",
        sent: "403 credential_handle_invalid",
        withheld:
          "パススルー方式、managed アダプター、ポリシー問い合わせへ切り替えない",
        owner: "Gateway（通信の執行）",
        failure:
          "認証情報を修復した後、Workspace へ入り直して投影を更新する必要があります。",
        explanation:
          "Tobari 形式の値を通常の認可データへ格下げすることはありません。",
      },
      {
        title: "ポリシーにも接続先にも問い合わせない",
        sent: "何も送らない",
        withheld: "リクエスト、認証情報、外向き通信",
        owner: "Gateway（通信の執行）",
        failure: "外部への通信は発生しません。",
        explanation:
          "認可できる有効な通常リクエストがないため、OPA より前に不正な結び付きを拒否します。",
      },
    ],
  },
  "policy-review": {
    label: "ポリシーの確認と有効化",
    summary:
      "信頼するホストの操作者が保持された証拠を確認し、完全一致ルールを明示的に有効化します。",
    steps: [
      {
        title: "保持された証拠を読む",
        sent: "取得件数を制限した末尾範囲から読む、厳密で秘密を含まない拒否記録",
        withheld: "本文、秘密情報、既存の候補 ID",
        owner: "Tobari CLI（ホスト側）",
        failure: "不正な記録は確認候補として扱いません。",
        explanation:
          "Gateway が証拠を記録し、信頼するホストの CLI が確認候補として表示する前に検証します。",
      },
      {
        title: "確認候補を見つける",
        sent: "検証済みの通信情報と、CLI が生成した候補を指す不透明な参照",
        withheld: "本文、秘密情報、一覧の順番による権限",
        owner: "Tobari CLI（ホスト側）",
        failure: "確認候補がなければ、許可または拒否する対象もありません。",
        explanation:
          "確認候補の一覧を表示するだけでは、ポリシーは変わりません。",
      },
      {
        title: "完全一致する結果を選ぶ",
        sent: "変更していない不透明な参照と、明示的な許可または拒否の意図",
        withheld: "再構築した ID やワイルドカードは使わない",
        owner: "信頼するホストの操作者",
        failure: "参照が欠落している、古い、または一致しない場合は拒否します。",
        explanation:
          "信頼するホストで行う操作は、拒否された Workspace のリクエストから分離されています。",
      },
      {
        title: "完全一致ルールを作る",
        sent: "Workspace Manifest、プロジェクト、ホスト、ポート、HTTP メソッド、パス",
        withheld: "本文と認証情報",
        owner: "Tobari CLI（ホスト側）",
        failure: "現在の有効なポリシーは変更しません。",
        explanation:
          "ルールが別の Workspace に広がったり、ワイルドカードへ自動的に一般化されたりすることはありません。",
      },
      {
        title: "ポリシー全体を検証する",
        sent: "構文と意味の完全な検証結果",
        withheld: "部分的に有効化しない",
        owner: "OPA ツール（ポリシー検証）",
        failure: "ポリシーソースが一つでも不正なら、有効化を中止します。",
        explanation: "集約ポリシーを一つの単位として検査します。",
      },
      {
        title: "不可分に有効化する",
        sent: "完全に検証され、内容から識別できる形にした集約済みポリシー",
        withheld: "一時的な半書き状態のルールセットを作らない",
        owner: "Tobari CLI（ホスト側）",
        failure:
          "確認候補を部分的に有効化することはありません。保持していたポリシーソースと投影の状態を復元し、OPA が利用不能なら Gateway は通信を許可せず失敗します。",
        explanation:
          "OPA は検証済みのポリシーバンドル全体を読み込み、そのリビジョンを返します。reset は学習済みの判断を削除して既定の拒否へ戻す操作であり、通信を許可する操作ではありません。",
      },
      {
        title: "意図して再実行する",
        sent: "タスクを再実行するという明示的な指示",
        withheld: "Gateway は自動で再送しない",
        owner: "利用者／エージェントの作業手順",
        failure: "再実行しなければリクエストは送信されません。",
        explanation:
          "ポリシー変更後も、以前の拒否を自動的に再送することはありません。",
      },
    ],
  },
};

function actorLabel(actor: string): string {
  const label = sequenceActorLabelsJa[actor];
  if (!label) {
    throw new Error(`missing Japanese SequenceExplorer actor label: ${actor}`);
  }
  return label;
}

export function localizeSequenceScenarioJa(
  scenario: SequenceScenario,
): LocalizedSequenceScenario {
  const translation = sequenceScenariosJa[scenario.id];
  if (!translation) {
    throw new Error(
      `missing Japanese SequenceExplorer scenario translation: ${scenario.id}`,
    );
  }
  if (translation.steps.length !== scenario.steps.length) {
    throw new Error(
      `Japanese SequenceExplorer step count differs for ${scenario.id}: ${translation.steps.length} != ${scenario.steps.length}`,
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
      from: step.from,
      to: step.to,
      fromLabel: actorLabel(step.from),
      toLabel: actorLabel(step.to),
    })),
  };
}
