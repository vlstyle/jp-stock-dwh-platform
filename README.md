# jp-stock-dwh-platform

日本株の株価・財務データを収集・保管し、スクリーニングするための基盤。

- 収集: [J-Quants API V2](https://jpx-jquants.com/ja/spec)から取得し、PostgreSQLへ冪等に保存する（再実行しても重複しない）
- 保管: PostgreSQL。スキーマは[migrations/](migrations/)（[goose](https://github.com/pressly/goose)）で管理する
- 分析: このリポジトリには含めない。スクリーニング条件・分析ロジックは、DBの読み取り専用ユーザーで接続する別リポジトリで実装する

取得したデータ自体はリポジトリに含めない。

## コマンド

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/stock
export JQUANTS_API_KEY=...

go run ./cmd/collector migrate
go run ./cmd/collector master
go run ./cmd/collector prices --from 2024-01-04 --to 2024-01-31
go run ./cmd/collector prices   # DBの最新日の翌日から今日(JST)まで
go run ./cmd/collector prices --lag-days 84   # 今日から84日前まで(Freeプランのデータ遅延)
go run ./cmd/collector financials --from 2024-08-23 --lag-days 84   # 開示日ごとに財務情報を取得
go run ./cmd/collector edinet --days 7                              # EDINETの有価証券報告書（直近7日）
```

| 環境変数 | 内容 |
|---|---|
| `DATABASE_URL` | PostgreSQL接続文字列（必須） |
| `JQUANTS_API_KEY` | J-Quants APIキー（`master`・`prices`で必須） |
| `EDINET_API_KEY` | EDINETのAPIキー（`edinet`で必須） |
| `EDINET_INTERVAL_MS` | EDINETへのリクエスト間隔(ミリ秒)。既定は1000 |
| `JQUANTS_RATE_PER_MIN` | 1分あたりのリクエスト数。既定は5（Freeプラン）。プランに合わせて上げる |

`prices`は平日を1日ずつ全銘柄分取得する。`financials`は平日を開示日ごとに全銘柄分取得し（`--from`省略時はDBの最新開示日から）、財務情報APIは他のAPIとは別枠の60リクエスト/分で動く。レート制限（429）を受けた場合は待機して再試行する。

`edinet`は、平日を1日ずつ、提出された有価証券報告書（上場会社のみ）の一覧を取得し、未保存の書類のXBRL(CSV形式)から、当期末の貸借対照表の現金・有価証券・借入金・社債などを保存する。`--from`省略時は`--to`（既定は今日）の`--days`日前（既定7）から。連結があれば連結、無ければ個別を使い、日本基準(`jppfs_cor`)とIFRS(`jpigp_cor`)の要素を保存する。EDINETはエラーをHTTP 200の本文で返すので、本文も確認している。

## テーブル

| テーブル | 内容 | 主キー |
|---|---|---|
| `listed_info` | 上場銘柄一覧（`/equities/master`） | `(code, date)` |
| `prices` | 株価四本値（`/equities/bars/daily`） | `(code, date)` |
| `edinet_documents` | EDINETの有価証券報告書のメタデータ（銘柄コード、決算期末、会計基準、連結/個別） | `doc_id` |
| `edinet_facts` | 上記の書類の、当期末の現金・有価証券・有利子負債に関する項目（円） | `(doc_id, element_id)` |
| `financials` | 財務情報（`/fins/summary`）。主要な項目は列に、全項目は`raw`(jsonb)に保存。訂正開示も別レコード | `(code, disc_no)` |

## ビュー

`valuation`: 銘柄ごとの最新の株価日を基準にした割安度指標（`pbr`、`per`、`f_per`(予想PER)、`f_dividend_yield`、`eq_ar`、`roe`、`cfo`、`cash_eq`(現金及び現金同等物)、`market_cap`(時価総額)、`cash_to_mcap`(現金/時価総額)など）。分析側はこのビューを入口にする。

- 対象は、全体の最新株価日から7日以内に株価がある銘柄（上場廃止などで古い株価しかない銘柄は含めない）
- 財務は、株価日以前に開示された最新のものを使う（`bps`・`f_eps`などは項目ごとに最新の値がある開示を選ぶ）
- `per`は直近の通期(`FY`)の実績EPS、`f_per`は最新の開示の通期予想EPSで計算する
- EPS・BPS・配当は、開示後の株式分割・併合を`split_factor()`（株価の調整係数の累積）で現在の株数に換算する
- 値が0以下のEPS・BPSの銘柄は、`per`・`pbr`をNULLにする
- 連結決算の項目を使う（非連結のみ提出の企業は、J-Quantsが個別の値を入れる）

## テスト

`TEST_DATABASE_URL`を設定すると、PostgreSQLを使う統合テストも実行する（未設定ならスキップ）。

```bash
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/stock_test go test ./...
```

## イメージ

`main`へのpushで、`ghcr.io/vlstyle/jp-stock-dwh-platform`に`<commit sha>`と`latest`のタグでpushする。
