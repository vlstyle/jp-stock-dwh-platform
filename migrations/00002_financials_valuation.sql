-- +goose Up
CREATE TABLE financials (
    code          text        NOT NULL,
    disc_no       text        NOT NULL,
    disc_date     date        NOT NULL,
    disc_time     time,
    doc_type      text,
    cur_per_type  text,
    cur_per_st    date,
    cur_per_en    date,
    cur_fy_st     date,
    cur_fy_en     date,
    sales         numeric,
    op            numeric,
    odp           numeric,
    np            numeric,
    eps           numeric,
    deps          numeric,
    ta            numeric,
    eq            numeric,
    eq_ar         numeric,
    bps           numeric,
    cfo           numeric,
    cfi           numeric,
    cff           numeric,
    cash_eq       numeric,
    sh_eq         numeric,
    roe           numeric,
    div_ann       numeric,
    f_sales       numeric,
    f_op          numeric,
    f_odp         numeric,
    f_np          numeric,
    f_eps         numeric,
    f_div_ann     numeric,
    sh_out_fy     numeric,
    tr_sh_fy      numeric,
    avg_sh        numeric,
    raw           jsonb       NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (code, disc_no)
);

CREATE INDEX financials_code_disc_idx ON financials (code, disc_date DESC, disc_time DESC);
CREATE INDEX financials_disc_date_idx ON financials (disc_date);

-- 株式分割・併合による、after_date より後の調整係数の累積。
-- 過去時点のEPS・BPSに掛けると、現在の株数ベースに換算できる。
-- +goose StatementBegin
CREATE FUNCTION split_factor(p_code text, p_after date) RETURNS numeric
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(exp(sum(ln(adj_factor))), 1)
    FROM prices
    WHERE code = p_code AND date > p_after AND adj_factor > 0
$$;
-- +goose StatementEnd

-- 銘柄ごとの最新の株価日を基準にした、割安度指標。
-- 財務は株価日以前に開示された最新のものを使う。EPS・BPS・配当は株式分割・併合で現在の株数に換算する。
CREATE VIEW valuation AS
WITH latest_price AS (
    SELECT DISTINCT ON (code) code, date, close
    FROM prices
    WHERE close IS NOT NULL
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

-- +goose Down
DROP VIEW valuation;
DROP FUNCTION split_factor(text, date);
DROP TABLE financials;
