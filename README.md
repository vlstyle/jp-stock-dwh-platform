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
| `edinet_facts` | 上記の書類の、当期末の現金・有価証券・有利子負債と、当期の損益計算書・CFの主要科目（日本基準のみ）に関する項目（円） | `(doc_id, element_id)` |
| `financials` | 財務情報（`/fins/summary`）。主要な項目は列に、全項目は`raw`(jsonb)に保存。訂正開示も別レコード | `(code, disc_no)` |

## ビュー

`valuation`: 銘柄ごとの最新の株価日を基準にした割安度指標（`pbr`、`per`、`f_per`(予想PER)、`f_dividend_yield`、`eq_ar`、`roe`、`cfo`、`cash_eq`(現金及び現金同等物)、`market_cap`(時価総額)、`cash_to_mcap`(現金/時価総額)、`net_cash`(ネットキャッシュ)、`net_cash_to_mcap`(ネットキャッシュ/時価総額)など）。分析側はこのビューを入口にする。

- 対象は、全体の最新株価日から7日以内に株価がある銘柄（上場廃止などで古い株価しかない銘柄は含めない）
- 財務は、株価日以前に開示された最新のものを使う（`bps`・`f_eps`などは項目ごとに最新の値がある開示を選ぶ）
- `per`は直近の通期(`FY`)の実績EPS、`f_per`は最新の開示の通期予想EPSで計算する
- EPS・BPS・配当は、開示後の株式分割・併合を`split_factor()`（株価の調整係数の累積）で現在の株数に換算する
- 値が0以下のEPS・BPSの銘柄は、`per`・`pbr`をNULLにする
- 連結決算の項目を使う（非連結のみ提出の企業は、J-Quantsが個別の値を入れる）

`net_cash`は、銘柄ごとの最新の有価証券報告書（`edinet_balance`ビュー）から、現金（現金及び預金。無ければ現金同等物）＋有価証券＋投資有価証券×70%−有利子負債（短期・長期借入金、社債、CP、新株予約権付社債。IFRSは社債及び借入金）で算出する。リース債務は含めない。銀行・証券の科目を持つ会社と、業種が銀行・証券・保険・その他金融の会社は、`net_cash`がNULL。`net_cash_ex_inv`は投資有価証券を含めない値。決算期末から有報提出までの、最大約1年の遅れがあり、`balance_period_end`で確認できる。

## テスト

`TEST_DATABASE_URL`を設定すると、PostgreSQLを使う統合テストも実行する（未設定ならスキップ）。

```bash
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/stock_test go test ./...
```

## イメージ

`main`へのpushで、`ghcr.io/vlstyle/jp-stock-dwh-platform`に`<commit sha>`と`latest`のタグでpushする。

`cost_structure`: 銘柄ごとの最新の有価証券報告書（日本基準のみ）から、固定費・変動費を近似したビュー。変動費＝売上原価−売上原価に含まれる減価償却費、固定費＝販管費＋売上原価に含まれる減価償却費（減価償却費はCF計算書の値から、販管費注記の分を引く）。`variable_cost_ratio`・`fixed_cost_ratio`（対売上高）、`bep_sales`（損益分岐点売上高）、`bep_ratio`（損益分岐点売上高÷売上高。低いほど、売上が減っても赤字になりにくい）を持つ。必要な科目（売上高・売上原価・販管費・減価償却費）のどれかが無い会社は含まれない。IFRS・米国基準は対象外。
- 変動費は、売上原価の人件費・外注費・賃借料などの固定的な費用を含むため、過大になる。固定費・損益分岐点は、過小になる方向の近似
- 既存の書類は、取り込む科目を増やす前に保存したもので、科目が無い。`collector edinet --refetch --codes <コード> --from … --to …`で、提出日を指定して取得し直す

`annual_results`: 銘柄・決算期ごとの通期(`FY`)の決算の推移。売上・営業利益・純利益・EPS（株式分割を調整）・総資産・CFなどと、`op_margin`・`np_margin`・`roa`・`fcf`、前年比（`sales_yoy`・`op_yoy`・`np_yoy`・`eps_yoy`）、3年の売上CAGR（`sales_cagr_3y`）を持つ。前年比は、直前の決算期が約1年前で、分母が正の場合だけ計算する。

`sector_stats`: 東証33業種（と全体＝`'全体'`）ごとの、指標（`per`・`pbr`・`roe`・`op_margin`・`roa`・`sales_yoy`など）の分布の、**最新のスナップショット**。1行が「業種×指標」で、`n`（件数）・`p25`・`median`・`p75`・`snapshot_date`を持つ。対象は、`valuation`に含まれる銘柄で、値がNULLの銘柄は`n`に含まれない。
- スナップショットは、`collector snapshot`が、`sector_stats_live`（現在の`valuation`から計算するビュー。十数秒かかる）の結果を、最新の株価日の日付で`sector_stats_daily`に保存する。同じ日付での再実行は、置き換える。業種ごとの推移は、`sector_stats_daily`に貯まる（過去分は、さかのぼって作れない）
- 通常は、`sector_stats`を読む。`valuation`の更新後に`snapshot`を実行しないと、古い分布のまま
