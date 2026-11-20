-- +goose Up
-- 最新の有価証券報告書から、ネットキャッシュを算出して、valuationに追加する（列は末尾に追加）。
CREATE VIEW edinet_balance AS
WITH latest AS (
    SELECT DISTINCT ON (code) doc_id, code, period_end, submit_date, accounting_std, scope
    FROM edinet_documents
    WHERE doc_type_code = '120'
    ORDER BY code, period_end DESC NULLS LAST, submit_date DESC, doc_id DESC
),
pivot AS (
    SELECT
        l.doc_id, l.code, l.period_end, l.submit_date, l.accounting_std, l.scope,
        COALESCE(bool_or(f.element_id ~ '(BNK|CASEC|CLSEC)$'), false)                                      AS is_financial,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CashAndDeposits')                             AS cash_and_deposits,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CashAndCashEquivalents')                      AS cash_and_equivalents,
        max(f.value) FILTER (WHERE f.element_id = 'jpigp_cor:CashAndCashEquivalentsIFRS')                  AS cash_ifrs,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermInvestmentSecurities')               AS st_securities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:InvestmentSecurities')                        AS inv_securities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermLoansPayable')                       AS st_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CommercialPapersLiabilities')                 AS cp,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentPortionOfLongTermLoansPayable')        AS cp_lt_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentPortionOfBonds')                       AS cp_bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermBondsPayable')                       AS st_bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:BondsPayable')                                AS bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:LongTermLoansPayable')                        AS lt_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ConvertibleBondTypeBondsWithSubscriptionRightsToShares') AS cb,
        max(f.value) FILTER (WHERE f.element_id = 'jpigp_cor:BondsAndBorrowingsCLIFRS')                    AS ifrs_bb_cl,
        max(f.value) FILTER (WHERE f.element_id = 'jpigp_cor:BorrowingsCLIFRS')                            AS ifrs_b_cl,
        max(f.value) FILTER (WHERE f.element_id = 'jpigp_cor:BondsAndBorrowingsNCLIFRS')                   AS ifrs_bb_ncl,
        max(f.value) FILTER (WHERE f.element_id = 'jpigp_cor:BorrowingsNCLIFRS')                           AS ifrs_b_ncl
    FROM latest l
    LEFT JOIN edinet_facts f ON f.doc_id = l.doc_id
    GROUP BY l.doc_id, l.code, l.period_end, l.submit_date, l.accounting_std, l.scope
),
calc AS (
    SELECT
        doc_id, code, period_end, submit_date, accounting_std, scope, is_financial,
        COALESCE(cash_and_deposits, cash_and_equivalents, cash_ifrs)                  AS cash,
        COALESCE(st_securities, 0)                                                    AS st_securities,
        COALESCE(inv_securities, 0)                                                   AS inv_securities,
        COALESCE(st_loans, 0) + COALESCE(cp, 0) + COALESCE(cp_lt_loans, 0) + COALESCE(cp_bonds, 0)
          + COALESCE(st_bonds, 0) + COALESCE(bonds, 0) + COALESCE(lt_loans, 0) + COALESCE(cb, 0)
          + COALESCE(ifrs_bb_cl, ifrs_b_cl, 0) + COALESCE(ifrs_bb_ncl, ifrs_b_ncl, 0) AS interest_bearing_debt
    FROM pivot
)
-- 銘柄ごとの、最新の有価証券報告書の、貸借対照表から算出した指標（円）。
-- net_cash = 現金 + 有価証券 + 投資有価証券 x 70% - 有利子負債（借入金・社債・CP・新株予約権付社債）。リース債務は含めない。
-- 銀行・証券など、金融業の科目を持つ会社は、現金や負債の意味が違うので、net_cash を NULL にする。
SELECT
    code, doc_id, period_end, submit_date, accounting_std, scope, is_financial,
    cash, st_securities, inv_securities, interest_bearing_debt,
    CASE WHEN NOT is_financial AND cash IS NOT NULL
         THEN cash + st_securities + inv_securities * 0.7 - interest_bearing_debt END AS net_cash,
    CASE WHEN NOT is_financial AND cash IS NOT NULL
         THEN cash + st_securities - interest_bearing_debt END                         AS net_cash_ex_inv
FROM calc;

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
                                                        AS cash_to_mcap,
    eb.period_end                                       AS balance_period_end,
    eb.cash                                             AS bs_cash,
    eb.interest_bearing_debt,
    eb.net_cash,
    eb.net_cash_ex_inv,
    CASE WHEN sh.shares > 0 THEN eb.net_cash / (p.close * sh.shares / split_factor(p.code, sh.disc_date)) END
                                                        AS net_cash_to_mcap
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
) sh ON true
LEFT JOIN edinet_balance eb ON eb.code = p.code;

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

DROP VIEW edinet_balance;
