-- +goose Up
-- 現金及び現金同等物、時価総額、現金/時価総額を追加する（ビューの末尾に列を追加）。
-- 時価総額は、開示時点の発行済株式数（自己株式を除く）を株式分割・併合で現在の株数に換算し、最新の株価を掛ける。
CREATE OR REPLACE VIEW valuation AS
WITH latest_price AS (
    SELECT DISTINCT ON (code) code, date, close
    FROM prices
    WHERE close IS NOT NULL
      AND date >= (SELECT max(date) FROM prices) - 7
    ORDER BY code, date DESC
),
latest_info AS (
    SELECT DISTINCT ON (code) code, co_name, mkt_nm, s33_nm
    FROM listed_info
    ORDER BY code, date DESC
)
SELECT
    p.code,
    i.co_name,
    i.mkt_nm,
    i.s33_nm,
    p.date                                              AS price_date,
    p.close,
    b.disc_date                                         AS bps_disc_date,
    b.bps * split_factor(p.code, b.disc_date)           AS bps,
    CASE WHEN b.bps > 0 THEN p.close / (b.bps * split_factor(p.code, b.disc_date)) END
                                                        AS pbr,
    a.disc_date                                         AS eps_disc_date,
    a.eps * split_factor(p.code, a.disc_date)           AS eps,
    CASE WHEN a.eps > 0 THEN p.close / (a.eps * split_factor(p.code, a.disc_date)) END
                                                        AS per,
    f.disc_date                                         AS f_eps_disc_date,
    f.f_eps * split_factor(p.code, f.disc_date)         AS f_eps,
    CASE WHEN f.f_eps > 0 THEN p.close / (f.f_eps * split_factor(p.code, f.disc_date)) END
                                                        AS f_per,
    d.f_div_ann * split_factor(p.code, d.disc_date)     AS f_div_ann,
    CASE WHEN d.f_div_ann > 0 THEN d.f_div_ann * split_factor(p.code, d.disc_date) / p.close END
                                                        AS f_dividend_yield,
    e.disc_date                                         AS eq_ar_disc_date,
    e.eq_ar,
    r.roe,
    r.cfo,
    r.disc_date                                         AS fy_disc_date,
    c.disc_date                                         AS cash_disc_date,
    c.cash_eq,
    p.close * sh.shares / split_factor(p.code, sh.disc_date)
                                                        AS market_cap,
    CASE WHEN sh.shares > 0 THEN c.cash_eq / (p.close * sh.shares / split_factor(p.code, sh.disc_date)) END
                                                        AS cash_to_mcap
FROM latest_price p
LEFT JOIN latest_info i USING (code)
LEFT JOIN LATERAL (
    SELECT disc_date, bps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND bps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) b ON true
LEFT JOIN LATERAL (
    SELECT disc_date, eps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND cur_per_type = 'FY' AND eps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) a ON true
LEFT JOIN LATERAL (
    SELECT disc_date, f_eps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND f_eps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) f ON true
LEFT JOIN LATERAL (
    SELECT disc_date, f_div_ann FROM financials
    WHERE code = p.code AND disc_date <= p.date AND f_div_ann IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) d ON true
LEFT JOIN LATERAL (
    SELECT disc_date, eq_ar FROM financials
    WHERE code = p.code AND disc_date <= p.date AND eq_ar IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) e ON true
LEFT JOIN LATERAL (
    SELECT disc_date, roe, cfo FROM financials
    WHERE code = p.code AND disc_date <= p.date AND cur_per_type = 'FY'
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
    SELECT disc_date, cash_eq FROM financials
    WHERE code = p.code AND disc_date <= p.date AND cash_eq IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) c ON true
LEFT JOIN LATERAL (
    SELECT disc_date, sh_out_fy - COALESCE(tr_sh_fy, 0) AS shares FROM financials
    WHERE code = p.code AND disc_date <= p.date AND sh_out_fy IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) sh ON true;

-- +goose Down
DROP VIEW valuation;
CREATE VIEW valuation AS
WITH latest_price AS (
    SELECT DISTINCT ON (code) code, date, close
    FROM prices
    WHERE close IS NOT NULL
      AND date >= (SELECT max(date) FROM prices) - 7
    ORDER BY code, date DESC
),
latest_info AS (
    SELECT DISTINCT ON (code) code, co_name, mkt_nm, s33_nm
    FROM listed_info
    ORDER BY code, date DESC
)
SELECT
    p.code,
    i.co_name,
    i.mkt_nm,
    i.s33_nm,
    p.date                                              AS price_date,
    p.close,
    b.disc_date                                         AS bps_disc_date,
    b.bps * split_factor(p.code, b.disc_date)           AS bps,
    CASE WHEN b.bps > 0 THEN p.close / (b.bps * split_factor(p.code, b.disc_date)) END
                                                        AS pbr,
    a.disc_date                                         AS eps_disc_date,
    a.eps * split_factor(p.code, a.disc_date)           AS eps,
    CASE WHEN a.eps > 0 THEN p.close / (a.eps * split_factor(p.code, a.disc_date)) END
                                                        AS per,
    f.disc_date                                         AS f_eps_disc_date,
    f.f_eps * split_factor(p.code, f.disc_date)         AS f_eps,
    CASE WHEN f.f_eps > 0 THEN p.close / (f.f_eps * split_factor(p.code, f.disc_date)) END
                                                        AS f_per,
    d.f_div_ann * split_factor(p.code, d.disc_date)     AS f_div_ann,
    CASE WHEN d.f_div_ann > 0 THEN d.f_div_ann * split_factor(p.code, d.disc_date) / p.close END
                                                        AS f_dividend_yield,
    e.disc_date                                         AS eq_ar_disc_date,
    e.eq_ar,
    r.roe,
    r.cfo,
    r.disc_date                                         AS fy_disc_date
FROM latest_price p
LEFT JOIN latest_info i USING (code)
LEFT JOIN LATERAL (
    SELECT disc_date, bps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND bps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) b ON true
LEFT JOIN LATERAL (
    SELECT disc_date, eps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND cur_per_type = 'FY' AND eps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) a ON true
LEFT JOIN LATERAL (
    SELECT disc_date, f_eps FROM financials
    WHERE code = p.code AND disc_date <= p.date AND f_eps IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) f ON true
LEFT JOIN LATERAL (
    SELECT disc_date, f_div_ann FROM financials
    WHERE code = p.code AND disc_date <= p.date AND f_div_ann IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) d ON true
LEFT JOIN LATERAL (
    SELECT disc_date, eq_ar FROM financials
    WHERE code = p.code AND disc_date <= p.date AND eq_ar IS NOT NULL
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) e ON true
LEFT JOIN LATERAL (
    SELECT disc_date, roe, cfo FROM financials
    WHERE code = p.code AND disc_date <= p.date AND cur_per_type = 'FY'
    ORDER BY disc_date DESC, disc_time DESC, disc_no DESC LIMIT 1
) r ON true;
