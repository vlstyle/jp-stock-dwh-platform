-- +goose Up
-- スクリーニング用の、銘柄ごとの指標を、1行にまとめたもの（valuation・annual_results・cost_structureの結合）。
-- valuationを全件計算するため、数十秒かかる。通常は、日次のスナップショット（screening）を使う。
-- valuationは、先にMATERIALIZEDで確定させる。そうしないと、結合の形によって、実行計画が悪くなり、数分以上かかることがある。
CREATE VIEW screening_live AS
WITH v AS MATERIALIZED (SELECT * FROM valuation),
latest_fy AS (
    SELECT DISTINCT ON (code) code, fy_end, sales, op_margin, np_margin, roa, sales_yoy, op_yoy, eps_yoy, sales_cagr_3y
    FROM annual_results
    ORDER BY code, fy_end DESC
)
SELECT
    v.code, v.co_name AS name, v.mkt_nm AS market, v.s33_nm AS sector, v.price_date, v.close, v.market_cap,
    v.per, v.pbr, v.f_per, v.f_dividend_yield AS dividend_yield, v.roe, v.eq_ar, v.net_cash_to_mcap,
    a.fy_end, (v.price_date - a.fy_end) AS fy_age_days,
    a.op_margin, a.np_margin, a.roa, a.sales_yoy, a.op_yoy, a.eps_yoy, a.sales_cagr_3y,
    CASE WHEN a.sales > 0 THEN v.market_cap / a.sales END AS psr,
    c.bep_ratio
FROM v
LEFT JOIN latest_fy a USING (code)
LEFT JOIN cost_structure c USING (code);

-- screening_liveの日次のスナップショット（collector snapshot）。snapshot_dateは、最新の株価日。
-- スクリーニングの結果と、前日との差分は、この表に条件を当てて、日ごとに再現できる。
CREATE TABLE screening_daily (
    snapshot_date    date    NOT NULL,
    code             text    NOT NULL,
    name             text,
    market           text,
    sector           text,
    price_date       date,
    close            numeric,
    market_cap       numeric,
    per              numeric,
    pbr              numeric,
    f_per            numeric,
    dividend_yield   numeric,
    roe              numeric,
    eq_ar            numeric,
    net_cash_to_mcap numeric,
    fy_end           date,
    fy_age_days      integer,
    op_margin        numeric,
    np_margin        numeric,
    roa              numeric,
    sales_yoy        numeric,
    op_yoy           numeric,
    eps_yoy          numeric,
    sales_cagr_3y    numeric,
    psr              numeric,
    bep_ratio        numeric,
    PRIMARY KEY (snapshot_date, code)
);

-- 最新のスナップショット。
CREATE VIEW screening AS
SELECT * FROM screening_daily
WHERE snapshot_date = (SELECT max(snapshot_date) FROM screening_daily);

-- +goose Down
DROP VIEW screening;
DROP TABLE screening_daily;
DROP VIEW screening_live;
