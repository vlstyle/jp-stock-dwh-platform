-- +goose Up
-- 通期(FY)の決算を、決算期ごとに1行にした推移。訂正開示があれば、最新の開示を採る。
-- 成長率は、直前の決算期が約1年前で、かつ分母が正の場合だけ計算する（決算期の変更・赤字からの回復で、意味を持たないため）。
CREATE VIEW annual_results AS
WITH fy AS (
    SELECT DISTINCT ON (code, cur_fy_en)
        code, cur_fy_en AS fy_end, disc_date,
        sales, op, odp, np, eps * split_factor(code, disc_date) AS eps, ta, eq, eq_ar,
        bps * split_factor(code, disc_date) AS bps, cfo, cfi, cff, cash_eq, roe, div_ann
    FROM financials
    WHERE cur_per_type = 'FY' AND cur_fy_en IS NOT NULL
    ORDER BY code, cur_fy_en, disc_date DESC, disc_time DESC NULLS LAST, disc_no DESC
),
lagged AS (
    SELECT fy.*,
        lag(fy_end) OVER w AS prev_fy_end, lag(sales) OVER w AS prev_sales, lag(op) OVER w AS prev_op,
        lag(np) OVER w AS prev_np, lag(eps) OVER w AS prev_eps,
        lag(fy_end, 3) OVER w AS prev3_fy_end, lag(sales, 3) OVER w AS prev3_sales
    FROM fy
    WINDOW w AS (PARTITION BY code ORDER BY fy_end)
)
SELECT
    code, fy_end, disc_date,
    sales, op, odp, np, eps, ta, eq, eq_ar, bps, cfo, cfi, cff, cash_eq, roe, div_ann,
    op / NULLIF(sales, 0)                  AS op_margin,
    np / NULLIF(sales, 0)                  AS np_margin,
    np / NULLIF(ta, 0)                     AS roa,
    cfo + cfi                              AS fcf,
    CASE WHEN fy_end - prev_fy_end BETWEEN 350 AND 380 AND prev_sales > 0 THEN sales / prev_sales - 1 END AS sales_yoy,
    CASE WHEN fy_end - prev_fy_end BETWEEN 350 AND 380 AND prev_op > 0 THEN op / prev_op - 1 END          AS op_yoy,
    CASE WHEN fy_end - prev_fy_end BETWEEN 350 AND 380 AND prev_np > 0 THEN np / prev_np - 1 END          AS np_yoy,
    CASE WHEN fy_end - prev_fy_end BETWEEN 350 AND 380 AND prev_eps > 0 THEN eps / prev_eps - 1 END       AS eps_yoy,
    CASE WHEN fy_end - prev3_fy_end BETWEEN 1050 AND 1130 AND prev3_sales > 0 AND sales > 0
         THEN power(sales / prev3_sales, 1.0 / 3) - 1 END                                                  AS sales_cagr_3y
FROM lagged;

-- 業種(東証33業種)ごとの、指標の分布（全体も、'全体'として含む）。外れ値に強い、中央値と四分位を使う。
-- 対象は、valuationに含まれる銘柄（最新の株価がある銘柄）。指標がNULLの銘柄は、nに含まれない。
CREATE VIEW sector_stats AS
WITH latest_fy AS (
    SELECT DISTINCT ON (code) code, op_margin, np_margin, roa, sales_yoy, op_yoy, sales_cagr_3y
    FROM annual_results
    ORDER BY code, fy_end DESC
),
base AS (
    SELECT v.s33_nm, v.per, v.pbr, v.f_per, v.f_dividend_yield, v.roe, v.eq_ar, v.market_cap,
           a.op_margin, a.np_margin, a.roa, a.sales_yoy, a.op_yoy, a.sales_cagr_3y
    FROM valuation v
    LEFT JOIN latest_fy a USING (code)
),
scoped AS (
    SELECT s33_nm AS sector, * FROM base WHERE s33_nm IS NOT NULL
    UNION ALL
    SELECT '全体' AS sector, * FROM base
)
SELECT
    s.sector, m.metric,
    count(m.value)                                         AS n,
    percentile_cont(0.25) WITHIN GROUP (ORDER BY m.value)  AS p25,
    percentile_cont(0.5)  WITHIN GROUP (ORDER BY m.value)  AS median,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY m.value)  AS p75
FROM scoped s
CROSS JOIN LATERAL (VALUES
    ('per', s.per), ('pbr', s.pbr), ('f_per', s.f_per), ('dividend_yield', s.f_dividend_yield),
    ('roe', s.roe), ('eq_ar', s.eq_ar), ('market_cap', s.market_cap),
    ('op_margin', s.op_margin), ('np_margin', s.np_margin), ('roa', s.roa),
    ('sales_yoy', s.sales_yoy), ('op_yoy', s.op_yoy), ('sales_cagr_3y', s.sales_cagr_3y)
) AS m(metric, value)
GROUP BY s.sector, m.metric;

-- +goose Down
DROP VIEW sector_stats;
DROP VIEW annual_results;
