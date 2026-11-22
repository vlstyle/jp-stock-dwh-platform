package edinet

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode/utf16"
)

const (
	ScopeConsolidated    = "consolidated"
	ScopeNonConsolidated = "non_consolidated"
)

// Report は、有価証券報告書の当期末の貸借対照表と、当期の損益計算書・キャッシュフロー計算書から取り出した項目。
type Report struct {
	AccountingStd string            // "Japan GAAP", "IFRS" など
	Scope         string            // 連結があれば連結、無ければ個別
	Facts         map[string]string // 要素ID -> 金額（円）
}

// 現金・有価証券・有利子負債に関係する要素を、日本基準(jppfs_cor)とIFRS(jpigp_cor)から取り出す。
// 必要な科目の選別はビュー側で行うので、ここでは関連する名前をやや広めに保存する。
var elementRE = regexp.MustCompile(`^(jppfs_cor|jpigp_cor):\w*(Loans|Bonds|Borrowings|CommercialPaper|Securities|Cash)`)

// 当期の損益計算書・キャッシュフロー計算書・販管費注記の要素（日本基準のみ）。
// 固定費・変動費の近似（cost_structure）と、分析レポートの要約財務諸表・株主還元に使う。
var durationElementRE = regexp.MustCompile(`^jppfs_cor:(` + strings.Join([]string{
	// 損益計算書
	"NetSales", "OperatingRevenue1", "OperatingRevenue2", "CostOfSales", "GrossProfit", "SellingGeneralAndAdministrativeExpenses",
	"OperatingIncome", "NonOperatingIncome", "NonOperatingExpenses", "OrdinaryIncome", "ExtraordinaryIncome", "ExtraordinaryLoss",
	"IncomeBeforeIncomeTaxes", "IncomeTaxes", "ProfitLoss", "ProfitLossAttributableToOwnersOfParent", "InterestExpensesNOE",
	// 販管費の注記
	"DepreciationSGA", "DepreciationAndAmortizationSGA",
	// キャッシュフロー計算書（設備投資、株主還元を含む）
	"DepreciationAndAmortizationOpeCF", "NetCashProvidedByUsedInOperatingActivities", "NetCashProvidedByUsedInInvestingActivities",
	"NetCashProvidedByUsedInFinancingActivities", "PurchaseOfPropertyPlantAndEquipmentInvCF", "PurchaseOfIntangibleAssetsInvCF",
	"DividendsPaidFinCF", "CashDividendsPaidFinCF", "PurchaseOfTreasuryStockFinCF", "ProceedsFromIssuanceOfSharesFinCF",
}, "|") + `)$`)

// 当期末の貸借対照表の要素（日本基準のみ）。分析レポートの要約財務諸表と、資産バリュー（清算価値）に使う。
// 現金・有価証券・有利子負債は、上のelementREで取る。
var instantStatementRE = regexp.MustCompile(`^jppfs_cor:(` + strings.Join([]string{
	"CurrentAssets", "NotesAndAccountsReceivableTrade", "NotesAndAccountsReceivableTradeAndContractAssets", "AccountsReceivableTrade",
	"NotesReceivableTrade", "ElectronicallyRecordedMonetaryClaimsOperatingCA", "ContractAssets",
	"Inventories", "Merchandise", "FinishedGoods", "MerchandiseAndFinishedGoods", "WorkInProcess", "RawMaterialsAndSupplies",
	"NoncurrentAssets", "PropertyPlantAndEquipment", "IntangibleAssets", "InvestmentsAndOtherAssets", "Assets",
	"CurrentLiabilities", "NotesAndAccountsPayableTrade", "AccountsPayableTrade", "NotesPayableTrade",
	"ElectronicallyRecordedObligationsOperatingCL", "NoncurrentLiabilities", "Liabilities",
	"ShareholdersEquity", "NetAssets", "LiabilitiesAndNetAssets",
}, "|") + `)$`)

const (
	ctxConsolidated            = "CurrentYearInstant"
	ctxNonConsolidated         = "CurrentYearInstant_NonConsolidatedMember"
	ctxDurationConsolidated    = "CurrentYearDuration"
	ctxDurationNonConsolidated = "CurrentYearDuration_NonConsolidatedMember"
)

var integerRE = regexp.MustCompile(`^-?\d+$`)

func ParseReport(zipData []byte) (Report, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return Report{}, fmt.Errorf("zip: %w", err)
	}
	var main *zip.File
	for _, f := range zr.File {
		base := path.Base(f.Name)
		if strings.HasPrefix(base, "jpcrp") && strings.HasSuffix(base, ".csv") {
			main = f
			break
		}
	}
	if main == nil {
		return Report{}, errors.New("zip: 本文のCSV(jpcrp*.csv)がありません")
	}
	rc, err := main.Open()
	if err != nil {
		return Report{}, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return Report{}, err
	}
	text, err := decodeUTF16LE(raw)
	if err != nil {
		return Report{}, err
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	var rep Report
	consolidated := map[string]string{}
	nonConsolidated := map[string]string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Report{}, fmt.Errorf("csv: %w", err)
		}
		if len(rec) < 9 {
			continue
		}
		elem, context, value := rec[0], rec[2], strings.TrimSpace(rec[8])
		if elem == "jpdei_cor:AccountingStandardsDEI" {
			rep.AccountingStd = value
			continue
		}
		if !integerRE.MatchString(value) {
			continue
		}
		switch context {
		case ctxConsolidated:
			if elementRE.MatchString(elem) || instantStatementRE.MatchString(elem) {
				consolidated[elem] = value
			}
		case ctxNonConsolidated:
			if elementRE.MatchString(elem) || instantStatementRE.MatchString(elem) {
				nonConsolidated[elem] = value
			}
		case ctxDurationConsolidated:
			if durationElementRE.MatchString(elem) {
				consolidated[elem] = value
			}
		case ctxDurationNonConsolidated:
			if durationElementRE.MatchString(elem) {
				nonConsolidated[elem] = value
			}
		}
	}

	if len(consolidated) > 0 {
		rep.Scope, rep.Facts = ScopeConsolidated, consolidated
	} else {
		rep.Scope, rep.Facts = ScopeNonConsolidated, nonConsolidated
	}
	return rep, nil
}

func decodeUTF16LE(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", errors.New("csv: UTF-16のバイト数が奇数です")
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u)), nil
}
