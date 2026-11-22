-- +goose Up
-- 有価証券報告書（日本基準）の、貸借対照表・損益計算書・キャッシュフロー計算書の主要科目を、決算期ごとに1行にしたもの（円）。
-- 分析レポートの、要約財務諸表と、経年の推移に使う。科目は、edinet_factsから取り出す（collector edinetが、当期の値を保存する）。
-- 科目の名前の違い（受取手形及び売掛金と契約資産、棚卸資産の内訳など）は、ここで吸収する。開示のない科目は、NULL。
CREATE VIEW financial_statements AS
WITH docs AS (
    SELECT DISTINCT ON (code, period_end) doc_id, code, period_end, accounting_std, scope
    FROM edinet_documents
    WHERE doc_type_code = '120' AND accounting_std = 'Japan GAAP' AND period_end IS NOT NULL
    ORDER BY code, period_end, submit_date DESC, doc_id DESC
),
pivot AS (
    SELECT
        d.doc_id, d.code, d.period_end, d.accounting_std, d.scope,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CashAndDeposits')                                   AS cash_and_deposits,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CashAndCashEquivalents')                            AS cash_and_equivalents,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermInvestmentSecurities')                     AS st_securities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:InvestmentSecurities')                              AS inv_securities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NotesAndAccountsReceivableTradeAndContractAssets')  AS recv_a,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NotesAndAccountsReceivableTrade')                   AS recv_b,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:AccountsReceivableTrade'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NotesReceivableTrade'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ElectronicallyRecordedMonetaryClaimsOperatingCA'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ContractAssets'), 0)                   AS recv_sum,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:Inventories')                                       AS inv_total,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:MerchandiseAndFinishedGoods'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:Merchandise'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:FinishedGoods'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:WorkInProcess'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:RawMaterialsAndSupplies'), 0)         AS inv_sum,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentAssets')                                     AS current_assets,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NoncurrentAssets')                                  AS noncurrent_assets,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PropertyPlantAndEquipment')                         AS ppe,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:IntangibleAssets')                                  AS intangibles,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:InvestmentsAndOtherAssets')                         AS investments_and_other,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:Assets')                                            AS total_assets,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NotesAndAccountsPayableTrade')                      AS pay_a,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:AccountsPayableTrade'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NotesPayableTrade'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ElectronicallyRecordedObligationsOperatingCL'), 0) AS pay_sum,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermLoansPayable')                             AS st_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentPortionOfLongTermLoansPayable')              AS cp_lt_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CommercialPapersLiabilities')                       AS cp,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentPortionOfBonds')                             AS cp_bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ShortTermBondsPayable')                             AS st_bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:BondsPayable')                                      AS bonds,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:LongTermLoansPayable')                              AS lt_loans,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ConvertibleBondTypeBondsWithSubscriptionRightsToShares') AS cb,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CurrentLiabilities')                                AS current_liabilities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NoncurrentLiabilities')                             AS noncurrent_liabilities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:Liabilities')                                       AS total_liabilities,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetAssets')                                         AS net_assets,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetSales'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:OperatingRevenue1'))                       AS sales,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CostOfSales')                                       AS cost_of_sales,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:GrossProfit')                                       AS gross_profit,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:SellingGeneralAndAdministrativeExpenses')           AS sga,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:OperatingIncome')                                   AS operating_income,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:OrdinaryIncome')                                    AS ordinary_income,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ProfitLossAttributableToOwnersOfParent'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:ProfitLoss'))                              AS net_income,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetCashProvidedByUsedInOperatingActivities')       AS cfo,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetCashProvidedByUsedInInvestingActivities')       AS cfi,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetCashProvidedByUsedInFinancingActivities')       AS cff,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfPropertyPlantAndEquipmentInvCF')          AS capex_ppe,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfIntangibleAssetsInvCF')                   AS capex_intangible,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:DepreciationAndAmortizationOpeCF')                  AS depreciation,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:DividendsPaidFinCF'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CashDividendsPaidFinCF'))                  AS dividends_paid_raw,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfTreasuryStockFinCF')                      AS treasury_purchase_raw
    FROM docs d
    LEFT JOIN edinet_facts f ON f.doc_id = d.doc_id
    GROUP BY d.doc_id, d.code, d.period_end, d.accounting_std, d.scope
),
calc AS (
    SELECT
        code, doc_id, period_end AS fy_end, scope,
        COALESCE(cash_and_deposits, cash_and_equivalents)                                   AS cash,
        COALESCE(st_securities, 0)                                                          AS st_securities,
        COALESCE(inv_securities, 0)                                                         AS inv_securities,
        COALESCE(recv_a, recv_b, NULLIF(recv_sum, 0))                                       AS receivables,
        COALESCE(inv_total, NULLIF(inv_sum, 0))                                             AS inventories,
        current_assets, noncurrent_assets, ppe, intangibles, investments_and_other, total_assets,
        COALESCE(pay_a, NULLIF(pay_sum, 0))                                                 AS payables,
        COALESCE(st_loans, 0) + COALESCE(cp_lt_loans, 0)                                    AS short_term_loans,
        lt_loans                                                                            AS long_term_loans,
        current_liabilities, noncurrent_liabilities, total_liabilities, net_assets,
        COALESCE(st_loans, 0) + COALESCE(cp, 0) + COALESCE(cp_lt_loans, 0) + COALESCE(cp_bonds, 0)
          + COALESCE(st_bonds, 0) + COALESCE(bonds, 0) + COALESCE(lt_loans, 0) + COALESCE(cb, 0) AS interest_bearing_debt,
        sales, cost_of_sales, gross_profit, sga, operating_income, ordinary_income, net_income,
        cfo, cfi, cff,
        -- 設備投資は、支出（負の値）で開示される。正の金額にする。
        CASE WHEN capex_ppe IS NOT NULL OR capex_intangible IS NOT NULL
             THEN -(COALESCE(capex_ppe, 0) + COALESCE(capex_intangible, 0)) END             AS capex,
        depreciation,
        -dividends_paid_raw                                                                 AS dividends_paid,
        -treasury_purchase_raw                                                              AS treasury_purchase
    FROM pivot
)
SELECT
    code, doc_id, fy_end, scope,
    cash, st_securities, inv_securities, receivables, inventories, current_assets, noncurrent_assets, ppe, intangibles,
    investments_and_other, total_assets, payables, short_term_loans, long_term_loans, current_liabilities, noncurrent_liabilities,
    total_liabilities, net_assets, interest_bearing_debt,
    sales, cost_of_sales, gross_profit, sga, operating_income, ordinary_income, net_income,
    cost_of_sales / NULLIF(sales, 0)                                                        AS cost_ratio,
    sga / NULLIF(sales, 0)                                                                  AS sga_ratio,
    cfo, cfi, cff, capex, depreciation, dividends_paid, treasury_purchase,
    cfo - capex                                                                             AS fcf,
    current_assets / NULLIF(current_liabilities, 0)                                         AS current_ratio,
    net_assets / NULLIF(total_assets, 0)                                                    AS equity_ratio,
    -- ROIC = 営業利益 x (1 - 30%) / (純資産 + 有利子負債)
    operating_income * 0.7 / NULLIF(net_assets + interest_bearing_debt, 0)                  AS roic,
    -- アクルーアル / 総資産 = (純利益 - 営業CF) / 総資産。大きいほど、利益が、現金の裏付けに乏しい
    (net_income - cfo) / NULLIF(total_assets, 0)                                            AS accrual_ratio,
    operating_income + depreciation                                                         AS ebitda,
    -- net_cash = 現金 + 有価証券 + 投資有価証券 x 70% - 有利子負債（edinet_balanceと同じ。金融業は、別途、除外する）
    CASE WHEN cash IS NOT NULL THEN cash + st_securities + inv_securities * 0.7 - interest_bearing_debt END AS net_cash,
    (dividends_paid + COALESCE(treasury_purchase, 0)) / NULLIF(net_income, 0)               AS total_return_ratio,
    dividends_paid / NULLIF(net_income, 0)                                                  AS payout_ratio
FROM calc;

-- 各決算期末の株価と、その決算期の決算から計算した、株価指標の推移。「今の株価が、過去に比べて、割高か割安か」を見るための表。
-- 時価総額 = 決算期末（以前の最後の取引日）の終値 x 発行済株式数（自己株式を除く）。当時の株価と、当時の株式数なので、分割の調整は、いらない。
CREATE VIEW valuation_history AS
WITH fy AS (
    SELECT DISTINCT ON (code, cur_fy_en)
        code, cur_fy_en AS fy_end, sales, np, eq, sh_out_fy - COALESCE(tr_sh_fy, 0) AS shares
    FROM financials
    WHERE cur_per_type = 'FY' AND cur_fy_en IS NOT NULL
    ORDER BY code, cur_fy_en, disc_date DESC, disc_time DESC NULLS LAST, disc_no DESC
),
priced AS (
    SELECT fy.*, p.close AS price, p.date AS price_date
    FROM fy
    LEFT JOIN LATERAL (
        SELECT close, date FROM prices
        WHERE code = fy.code AND date <= fy.fy_end AND date > fy.fy_end - 10 AND close IS NOT NULL
        ORDER BY date DESC LIMIT 1
    ) p ON true
)
SELECT
    pr.code, pr.fy_end, pr.price_date, pr.price, pr.shares,
    pr.price * pr.shares                                                                  AS market_cap,
    CASE WHEN pr.np > 0 THEN pr.price * pr.shares / pr.np END                             AS per,
    CASE WHEN pr.eq > 0 THEN pr.price * pr.shares / pr.eq END                             AS pbr,
    CASE WHEN pr.sales > 0 THEN pr.price * pr.shares / pr.sales END                       AS psr,
    CASE WHEN s.cfo > 0 THEN pr.price * pr.shares / s.cfo END                             AS pcfr,
    pr.price * pr.shares - s.net_cash                                                     AS ev,
    CASE WHEN s.ebitda > 0 THEN (pr.price * pr.shares - s.net_cash) / s.ebitda END        AS ev_ebitda
FROM priced pr
LEFT JOIN financial_statements s ON s.code = pr.code AND s.fy_end = pr.fy_end;

-- +goose Down
DROP VIEW valuation_history;
DROP VIEW financial_statements;
