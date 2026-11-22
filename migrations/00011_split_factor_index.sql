-- +goose Up
-- split_factorを、速くする。pricesは約530万行だが、株式分割・併合などの調整(adj_factor <> 1)がある行は、約1,000行（0.02%）しかない。
-- 従来は、残りの99.98%の行でも、numeric型のln()を計算していたため、1回の呼び出しが数十ミリ秒〜数十秒かかり
-- （valuationは、1銘柄につき約11回呼ぶ）、valuationの全銘柄の計算が、数分〜十数分かかっていた。
-- 調整のある行だけの、部分インデックスを使う。ln(1)=0なので、結果は、変わらない（本番のデータで、全銘柄の差が0であることを確認）。
-- インデックスの作成中（数秒）は、pricesへの書き込みが、待たされる。
CREATE INDEX prices_adj_events_idx ON prices (code, date) INCLUDE (adj_factor) WHERE adj_factor <> 1;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION split_factor(p_code text, p_after date) RETURNS numeric
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(exp(sum(ln(adj_factor))), 1)
    FROM prices
    WHERE code = p_code AND date > p_after AND adj_factor <> 1 AND adj_factor > 0
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION split_factor(p_code text, p_after date) RETURNS numeric
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(exp(sum(ln(adj_factor))), 1)
    FROM prices
    WHERE code = p_code AND date > p_after AND adj_factor > 0
$$;
-- +goose StatementEnd
DROP INDEX prices_adj_events_idx;
