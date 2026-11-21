-- +goose Up
-- 最新の有価証券報告書（日本基準）の損益計算書・キャッシュフロー計算書の科目から、固定費・変動費を近似する。
-- 減価償却費（CF）のうち、販管費に含まれる分（開示がある会社のみ）を除いた分を、売上原価に含まれる固定費とみなす。
--   変動費 = 売上原価 − 売上原価に含まれる減価償却費
--   固定費 = 販管費 + 売上原価に含まれる減価償却費
-- 科目の開示が無い会社は、該当する列がNULLになる。
CREATE VIEW cost_structure AS
WITH latest AS (
    SELECT DISTINCT ON (code) doc_id, code, period_end, submit_date, scope
    FROM edinet_documents
    WHERE doc_type_code = '120' AND accounting_std = 'Japan GAAP'
    ORDER BY code, period_end DESC NULLS LAST, submit_date DESC, doc_id DESC
),
pivot AS (
    SELECT
        l.code, l.doc_id, l.period_end, l.scope,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:NetSales'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:OperatingRevenue1'))   AS sales,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:CostOfSales')                    AS cost_of_sales,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:SellingGeneralAndAdministrativeExpenses') AS sga,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:OperatingIncome')                AS operating_income,
        max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:DepreciationAndAmortizationOpeCF') AS depreciation,
        COALESCE(max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:DepreciationSGA'),
                 max(f.value) FILTER (WHERE f.element_id = 'jppfs_cor:DepreciationAndAmortizationSGA'), 0) AS depreciation_sga
    FROM latest l
    LEFT JOIN edinet_facts f ON f.doc_id = l.doc_id
    GROUP BY l.code, l.doc_id, l.period_end, l.scope
),
calc AS (
    SELECT *,
        LEAST(GREATEST(depreciation - depreciation_sga, 0), cost_of_sales) AS depreciation_cogs
    FROM pivot
    WHERE sales > 0 AND cost_of_sales IS NOT NULL AND sga IS NOT NULL AND depreciation IS NOT NULL
),
cost AS (
    SELECT *,
        cost_of_sales - depreciation_cogs AS variable_cost,
        sga + depreciation_cogs           AS fixed_cost
    FROM calc
)
SELECT
    code, doc_id, period_end, scope,
    sales, cost_of_sales, sga, operating_income, depreciation,
    variable_cost,
    fixed_cost,
    variable_cost / sales                                       AS variable_cost_ratio,
    fixed_cost / sales                                          AS fixed_cost_ratio,
    CASE WHEN sales > variable_cost THEN fixed_cost / (1 - variable_cost / sales) END
                                                                AS bep_sales,
    CASE WHEN sales > variable_cost THEN fixed_cost / (sales - variable_cost) END
                                                                AS bep_ratio
FROM cost;

-- +goose Down
DROP VIEW cost_structure;
