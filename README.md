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
```

| 環境変数 | 内容 |
|---|---|
| `DATABASE_URL` | PostgreSQL接続文字列（必須） |
| `JQUANTS_API_KEY` | J-Quants APIキー（`master`・`prices`で必須） |
| `JQUANTS_RATE_PER_MIN` | 1分あたりのリクエスト数。既定は5（Freeプラン）。プランに合わせて上げる |

`prices`は平日を1日ずつ全銘柄分取得する。レート制限（429）を受けた場合は待機して再試行する。

## テーブル

| テーブル | 内容 | 主キー |
|---|---|---|
| `listed_info` | 上場銘柄一覧（`/equities/master`） | `(code, date)` |
| `prices` | 株価四本値（`/equities/bars/daily`） | `(code, date)` |

## テスト

`TEST_DATABASE_URL`を設定すると、PostgreSQLを使う統合テストも実行する（未設定ならスキップ）。

```bash
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/stock_test go test ./...
```

## イメージ

`main`へのpushで、`ghcr.io/vlstyle/jp-stock-dwh-platform`に`<commit sha>`と`latest`のタグでpushする。
