-- +goose Up
-- financial_statementsの、科目名の修正（本番の有価証券報告書で確認）。
--  - 投資活動によるCFは、NetCashProvidedByUsedInInvestmentActivities（InvestingではなくInvestment）
--  - 不動産業の棚卸資産は、販売用不動産（RealEstateForSale）・仕掛販売用不動産
--  - 有形・無形固定資産の取得を、合算で開示する会社の設備投資
-- 既存の列の並び・型は、変えない（CREATE OR REPLACE VIEW。valuation_historyは、このビューを参照する）。
CREATE OR REPLACE VIEW financial_statements AS
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
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:RawMaterialsAndSupplies'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:RealEstateForSale'), 0)
          + COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:RealEstateForSaleInProcess'), 0)       AS inv_sum,
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
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetCashProvidedByUsedInInvestmentActivities')      AS cfi,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetCashProvidedByUsedInFinancingActivities')       AS cff,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfPropertyPlantAndEquipmentInvCF')          AS capex_ppe,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfIntangibleAssetsInvCF')                   AS capex_intangible,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfPropertyPlantAndEquipmentAndIntangibleAssetsInvCF'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:PurchaseOfNoncurrentAssetsInvCF'))      AS capex_combined,
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
             THEN -(COALESCE(capex_ppe, 0) + COALESCE(capex_intangible, 0))
             WHEN capex_combined IS NOT NULL THEN -capex_combined END                        AS capex,
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

-- +goose Down
-- 修正前の定義（00012）に戻すには、00012のビュー定義を、CREATE OR REPLACE VIEWで流し直す。ここでは、何もしない（修正は、科目名の追加だけ）。
SELECT 1;
