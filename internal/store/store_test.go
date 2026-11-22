package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/edinet"
	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
)

func TestUpsertIsIdempotent(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)

	if _, err := s.conn.Exec(ctx, `TRUNCATE prices, listed_info`); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := s.LatestPriceDate(ctx); err != nil || ok {
		t.Fatalf("empty table: ok=%v err=%v", ok, err)
	}

	c := 2045.0
	rows := []jquants.DailyBar{
		{Date: "2024-01-04", Code: "86970", C: &c, UL: "1"},
		{Date: "2024-01-05", Code: "86970"},
	}
	for range 2 {
		if err := s.UpsertPrices(ctx, rows); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertListedInfo(ctx, []jquants.ListedInfo{{Date: "2024-01-04", Code: "86970", CoName: "JPX"}}); err != nil {
			t.Fatal(err)
		}
	}

	var n int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM prices`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("prices count = %d, err = %v", n, err)
	}
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM listed_info`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("listed_info count = %d, err = %v", n, err)
	}

	var upper bool
	var closeP *float64
	if err := s.conn.QueryRow(ctx, `SELECT upper_limit, close::float8 FROM prices WHERE date = '2024-01-04'`).Scan(&upper, &closeP); err != nil {
		t.Fatal(err)
	}
	if !upper || closeP == nil || *closeP != 2045.0 {
		t.Errorf("upper=%v close=%v", upper, closeP)
	}

	d, ok, err := s.LatestPriceDate(ctx)
	if err != nil || !ok || !d.Equal(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("latest = %v ok=%v err=%v", d, ok, err)
	}
}

func TestFinancialsAndValuation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()

	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)

	if _, err := s.conn.Exec(ctx, `TRUNCATE prices, listed_info, financials`); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := s.LatestFinancialDate(ctx); err != nil || ok {
		t.Fatalf("empty table: ok=%v err=%v", ok, err)
	}

	f := func(v float64) *float64 { return &v }
	if err := s.UpsertListedInfo(ctx, []jquants.ListedInfo{{Date: "2024-01-04", Code: "86970", CoName: "JPX", MktNm: "プライム"}}); err != nil {
		t.Fatal(err)
	}
	// 2024-07-01に1:2の株式分割（調整係数0.5）。
	if err := s.UpsertPrices(ctx, []jquants.DailyBar{
		{Date: "2024-05-01", Code: "86970", C: f(1000), AdjFactor: f(1)},
		{Date: "2024-06-01", Code: "86970", C: f(1000), AdjFactor: f(1)},
		{Date: "2024-07-01", Code: "86970", C: f(500), AdjFactor: f(0.5)},
		// 上場廃止などで、直近の株価が無い銘柄。
		{Date: "2024-05-01", Code: "99990", C: f(1), AdjFactor: f(1)},
	}); err != nil {
		t.Fatal(err)
	}

	fins := []jquants.Financial{
		{"Code": "86970", "DiscNo": "1", "DiscDate": "2024-05-01", "DiscTime": "15:00:00", "CurPerType": "FY",
			"EPS": "50", "BPS": "500", "EqAR": "0.4", "FEPS": "60", "FDivAnn": "20", "ROE": "0.1", "CFO": "1000", "OdP": "",
			"CashEq": "450000", "ShOutFY": "1000", "TrShFY": "100"},
		// 1Qの開示。BPSとFY扱いのEPSは使われず、予想EPSだけが更新される。
		{"Code": "86970", "DiscNo": "2", "DiscDate": "2024-06-15", "DiscTime": "15:00:00", "CurPerType": "1Q",
			"EPS": "10", "BPS": "", "FEPS": "70"},
		// 株価日(2024-07-01)より後の開示は、その株価日の指標に使わない。
		{"Code": "86970", "DiscNo": "3", "DiscDate": "2024-08-01", "DiscTime": "15:00:00", "CurPerType": "FY",
			"EPS": "999", "BPS": "999"},
	}
	for range 2 {
		if err := s.UpsertFinancials(ctx, fins); err != nil {
			t.Fatal(err)
		}
	}

	var n int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM financials`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("financials count = %d, err = %v", n, err)
	}
	var odp *float64
	var rawEPS string
	if err := s.conn.QueryRow(ctx, `SELECT odp::float8, raw->>'EPS' FROM financials WHERE disc_no = '1'`).Scan(&odp, &rawEPS); err != nil {
		t.Fatal(err)
	}
	if odp != nil || rawEPS != "50" {
		t.Errorf("odp = %v (want NULL), raw EPS = %q", odp, rawEPS)
	}
	d, ok, err := s.LatestFinancialDate(ctx)
	if err != nil || !ok || !d.Equal(time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("latest = %v ok=%v err=%v", d, ok, err)
	}

	var coName string
	var price, bps, pbr, per, fPer, yield, eqAr, cash, mcap, cashToMcap float64
	err = s.conn.QueryRow(ctx, `
		SELECT co_name, close::float8, bps::float8, pbr::float8, per::float8, f_per::float8,
		       f_dividend_yield::float8, eq_ar::float8, cash_eq::float8, market_cap::float8, cash_to_mcap::float8
		FROM valuation WHERE code = '86970'`).Scan(&coName, &price, &bps, &pbr, &per, &fPer, &yield, &eqAr, &cash, &mcap, &cashToMcap)
	if err != nil {
		t.Fatal(err)
	}
	// 分割(0.5)後の株数ベース: BPS=500*0.5=250, EPS=50*0.5=25, 予想EPS=70*0.5=35, 予想配当=20*0.5=10
	checks := map[string][2]float64{
		"close": {price, 500}, "bps": {bps, 250}, "pbr": {pbr, 2}, "per": {per, 20},
		"f_per": {fPer, 500.0 / 35}, "f_dividend_yield": {yield, 0.02}, "eq_ar": {eqAr, 0.4},
		// 発行済1000-自己株式100=900株が、分割(0.5)後は1800株。時価総額=500*1800。
		"cash_eq": {cash, 450000}, "market_cap": {mcap, 900000}, "cash_to_mcap": {cashToMcap, 0.5},
	}
	for name, c := range checks {
		if diff := c[0] - c[1]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	if coName != "JPX" {
		t.Errorf("co_name = %q", coName)
	}

	// 業種ごとの分布のスナップショット。同じ日付で再実行しても、行が増えない。
	if _, err := s.conn.Exec(ctx, `TRUNCATE sector_stats_daily`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		date, rows, err := s.SnapshotSectorStats(ctx)
		if err != nil || rows == 0 || !date.Equal(time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("snapshot: date=%v rows=%d err=%v", date, rows, err)
		}
	}
	var statN int
	var statMedian float64
	if err := s.conn.QueryRow(ctx, `SELECT n, median::float8 FROM sector_stats WHERE sector = '全体' AND metric = 'per'`).Scan(&statN, &statMedian); err != nil {
		t.Fatal(err)
	}
	if statN != 1 || statMedian != 20 {
		t.Errorf("sector_stats per: n=%d median=%v, want 1, 20", statN, statMedian)
	}
	var total int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM sector_stats_daily`).Scan(&total); err != nil || total != 13 {
		t.Errorf("sector_stats_daily rows = %d (want 13, one sector × 13 metrics), err = %v", total, err)
	}

	// スクリーニング用のスナップショット。再実行しても、行が増えない。上場廃止の銘柄（古い株価だけ）は含まれない。
	if _, err := s.conn.Exec(ctx, `TRUNCATE screening_daily`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		sd, rows, err := s.SnapshotScreening(ctx)
		if err != nil || rows != 1 || !sd.Equal(time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("screening snapshot: date=%v rows=%d err=%v", sd, rows, err)
		}
	}
	var scCode, scName string
	var scPer, scPBR float64
	if err := s.conn.QueryRow(ctx, `SELECT code, name, per::float8, pbr::float8 FROM screening`).Scan(&scCode, &scName, &scPer, &scPBR); err != nil {
		t.Fatal(err)
	}
	if scCode != "86970" || scName != "JPX" || scPer != 20 || scPBR != 2 {
		t.Errorf("screening = %s %s per=%v pbr=%v", scCode, scName, scPer, scPBR)
	}

	var delisted int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM valuation WHERE code = '99990'`).Scan(&delisted); err != nil || delisted != 0 {
		t.Errorf("stale-price code in valuation: count = %d, err = %v", delisted, err)
	}
}

func TestEdinetDocumentUpsert(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	if _, err := s.conn.Exec(ctx, `TRUNCATE edinet_documents CASCADE`); err != nil {
		t.Fatal(err)
	}

	doc := edinet.Document{DocID: "S100AAAA", SecCode: "86970", FilerName: "JPX", DocTypeCode: "120", PeriodEnd: "2026-03-31"}
	day := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	rep := edinet.Report{AccountingStd: "IFRS", Scope: edinet.ScopeConsolidated, Facts: map[string]string{
		"jppfs_cor:CashAndDeposits": "1000", "jppfs_cor:BondsPayable": "-5",
	}}
	for range 2 {
		if err := s.UpsertEdinetDocument(ctx, doc, day, rep); err != nil {
			t.Fatal(err)
		}
	}
	// 再保存で、項目は置き換わる。
	rep.Facts = map[string]string{"jppfs_cor:CashAndDeposits": "2000"}
	if err := s.UpsertEdinetDocument(ctx, doc, day, rep); err != nil {
		t.Fatal(err)
	}

	var n int
	var cash float64
	if err := s.conn.QueryRow(ctx, `SELECT count(*), max(value)::float8 FROM edinet_facts WHERE doc_id = 'S100AAAA'`).Scan(&n, &cash); err != nil || n != 1 || cash != 2000 {
		t.Errorf("facts: n=%d cash=%v err=%v", n, cash, err)
	}
	var std, scope string
	var period time.Time
	if err := s.conn.QueryRow(ctx, `SELECT accounting_std, scope, period_end FROM edinet_documents WHERE doc_id = 'S100AAAA'`).Scan(&std, &scope, &period); err != nil ||
		std != "IFRS" || scope != "consolidated" || !period.Equal(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("doc: %q %q %v err=%v", std, scope, period, err)
	}

	got, err := s.ExistingEdinetDocs(ctx, []string{"S100AAAA", "S100BBBB"})
	if err != nil || len(got) != 1 || !got["S100AAAA"] {
		t.Errorf("existing = %v, err = %v", got, err)
	}
}

func TestNetCashInValuation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	if _, err := s.conn.Exec(ctx, `TRUNCATE prices, listed_info, financials, edinet_documents CASCADE`); err != nil {
		t.Fatal(err)
	}

	f := func(v float64) *float64 { return &v }
	codes := []string{"86970", "86980", "86990", "87000"}
	for _, code := range codes {
		s33 := "電気機器"
		if code == "87000" {
			s33 = "証券･商品先物取引業"
		}
		if err := s.UpsertListedInfo(ctx, []jquants.ListedInfo{{Date: "2026-06-01", Code: code, CoName: code, S33Nm: s33}}); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertPrices(ctx, []jquants.DailyBar{{Date: "2026-06-01", Code: code, C: f(10), AdjFactor: f(1)}}); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertFinancials(ctx, []jquants.Financial{
			{"Code": code, "DiscNo": "1", "DiscDate": "2026-05-01", "CurPerType": "FY", "ShOutFY": "100", "TrShFY": "0"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	day := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	save := func(id, code, period string, facts map[string]string) {
		t.Helper()
		doc := edinet.Document{DocID: id, SecCode: code, DocTypeCode: "120", PeriodEnd: period}
		if err := s.UpsertEdinetDocument(ctx, doc, day, edinet.Report{Scope: edinet.ScopeConsolidated, Facts: facts}); err != nil {
			t.Fatal(err)
		}
	}
	// 86970: 最新の期の書類だけを使う（前期の書類は無視）。
	save("S1", "86970", "2025-03-31", map[string]string{"jppfs_cor:CashAndDeposits": "9999"})
	save("S2", "86970", "2026-03-31", map[string]string{
		"jppfs_cor:CashAndDeposits": "1000", "jppfs_cor:ShortTermInvestmentSecurities": "100",
		"jppfs_cor:InvestmentSecurities": "200", "jppfs_cor:ShortTermLoansPayable": "300",
		"jppfs_cor:BondsPayable": "50", "jppfs_cor:LongTermLoansPayable": "150",
	})
	// 86980: 金融業の科目を持つ会社は、net_cash を出さない。
	save("S3", "86980", "2026-03-31", map[string]string{
		"jppfs_cor:CashAndDeposits": "1000", "jppfs_cor:CashAndDueFromBanksAssetsBNK": "5000",
	})
	// 86990: IFRSで、現金のみ、負債なし。
	save("S4", "86990", "2026-03-31", map[string]string{"jpigp_cor:CashAndCashEquivalentsIFRS": "400"})
	// 87000: 金融業の科目は無いが、業種が証券業の会社も、net_cash を出さない。
	save("S5", "87000", "2026-03-31", map[string]string{"jpigp_cor:CashAndCashEquivalentsIFRS": "400"})

	type row struct {
		cash, debt, netCash, exInv, toMcap *float64
	}
	get := func(code string) row {
		var r row
		if err := s.conn.QueryRow(ctx, `
			SELECT bs_cash::float8, interest_bearing_debt::float8, net_cash::float8, net_cash_ex_inv::float8, net_cash_to_mcap::float8
			FROM valuation WHERE code = $1`, code).Scan(&r.cash, &r.debt, &r.netCash, &r.exInv, &r.toMcap); err != nil {
			t.Fatal(err)
		}
		return r
	}
	eq := func(name string, got *float64, want float64) {
		t.Helper()
		if got == nil || *got-want > 1e-9 || want-*got > 1e-9 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	r := get("86970")
	eq("cash", r.cash, 1000)
	eq("debt", r.debt, 500)
	eq("net_cash", r.netCash, 740) // 1000 + 100 + 200*0.7 - 500
	eq("net_cash_ex_inv", r.exInv, 600)
	eq("net_cash_to_mcap", r.toMcap, 0.74) // 時価総額 = 10 * 100

	if r := get("86980"); r.netCash != nil {
		t.Errorf("financial company net_cash = %v, want NULL", *r.netCash)
	}
	if r := get("87000"); r.netCash != nil || r.exInv != nil || r.toMcap != nil {
		t.Errorf("financial sector net_cash = %v / %v / %v, want NULL", r.netCash, r.exInv, r.toMcap)
	}
	r = get("86990")
	eq("ifrs cash", r.cash, 400)
	eq("ifrs net_cash", r.netCash, 400)
}

// split_factorは、調整のある行だけの部分インデックスを使い、調整の無い行を無視しても、結果が変わらない。
func TestSplitFactorUsesPartialIndex(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	if _, err := s.conn.Exec(ctx, `TRUNCATE prices`); err != nil {
		t.Fatal(err)
	}

	f := func(v float64) *float64 { return &v }
	bars := []jquants.DailyBar{
		{Date: "2024-01-04", Code: "11110", C: f(100), AdjFactor: f(1)},
		{Date: "2024-02-01", Code: "11110", C: f(100), AdjFactor: f(0.5)}, // 1:2の分割
		{Date: "2024-03-01", Code: "11110", C: f(100), AdjFactor: f(1)},
		{Date: "2024-04-01", Code: "11110", C: f(100), AdjFactor: f(0.2)}, // 1:5の分割
		{Date: "2024-05-01", Code: "11110", C: f(100), AdjFactor: f(1)},
		{Date: "2024-05-01", Code: "22220", C: f(100), AdjFactor: f(1)}, // 調整の無い銘柄
		{Date: "2024-05-02", Code: "22220", C: f(100)},                  // adj_factorがNULL
	}
	if err := s.UpsertPrices(ctx, bars); err != nil {
		t.Fatal(err)
	}

	want := map[string]map[string]float64{
		"11110": {"2024-01-01": 0.1, "2024-02-01": 0.2, "2024-04-01": 1, "2024-06-01": 1}, // 基準日より後の調整だけを掛ける
		"22220": {"2024-01-01": 1},
	}
	for code, byDate := range want {
		for after, factor := range byDate {
			var got float64
			if err := s.conn.QueryRow(ctx, `SELECT split_factor($1, $2::date)::float8`, code, after).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if diff := got - factor; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("split_factor(%s, %s) = %v, want %v", code, after, got, factor)
			}
		}
	}

	// 関数の中のクエリが、部分インデックスを使えること（小さい表でも、使うように強制して確認する）。
	if _, err := s.conn.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := s.conn.Query(ctx, `EXPLAIN SELECT COALESCE(exp(sum(ln(adj_factor))), 1) FROM prices
		WHERE code = '11110' AND date > '2024-01-01' AND adj_factor <> 1 AND adj_factor > 0`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	used := false
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "prices_adj_events_idx") {
			used = true
		}
	}
	if !used {
		t.Error("split_factorのクエリが、prices_adj_events_idxを使っていません")
	}
}

// 要約財務諸表（financial_statements）と、年度末の株価指標の推移（valuation_history）。
func TestFinancialStatementsAndValuationHistory(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	if _, err := s.conn.Exec(ctx, `TRUNCATE edinet_documents CASCADE; TRUNCATE prices, financials`); err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	doc := edinet.Document{DocID: "S100FS01", SecCode: "11110", FilerName: "テスト", DocTypeCode: "120", PeriodEnd: "2026-03-31"}
	rep := edinet.Report{AccountingStd: "Japan GAAP", Scope: edinet.ScopeConsolidated, Facts: map[string]string{
		"jppfs_cor:CashAndDeposits": "1000", "jppfs_cor:ShortTermInvestmentSecurities": "100", "jppfs_cor:InvestmentSecurities": "200",
		"jppfs_cor:NotesAndAccountsReceivableTradeAndContractAssets": "800",
		"jppfs_cor:MerchandiseAndFinishedGoods":                      "300", "jppfs_cor:WorkInProcess": "50", // 棚卸資産の内訳だけが開示されている
		"jppfs_cor:CurrentAssets": "2500", "jppfs_cor:PropertyPlantAndEquipment": "3000", "jppfs_cor:IntangibleAssets": "200",
		"jppfs_cor:InvestmentsAndOtherAssets": "300", "jppfs_cor:Assets": "6000",
		"jppfs_cor:AccountsPayableTrade": "400", "jppfs_cor:NotesPayableTrade": "100",
		"jppfs_cor:ShortTermLoansPayable": "150", "jppfs_cor:CurrentPortionOfLongTermLoansPayable": "50", "jppfs_cor:LongTermLoansPayable": "700",
		"jppfs_cor:CurrentLiabilities": "1250", "jppfs_cor:NoncurrentLiabilities": "1000", "jppfs_cor:Liabilities": "2250", "jppfs_cor:NetAssets": "3750",
		"jppfs_cor:NetSales": "10000", "jppfs_cor:CostOfSales": "6000", "jppfs_cor:SellingGeneralAndAdministrativeExpenses": "3000",
		"jppfs_cor:OperatingIncome": "1000", "jppfs_cor:OrdinaryIncome": "1050", "jppfs_cor:ProfitLossAttributableToOwnersOfParent": "700",
		"jppfs_cor:NetCashProvidedByUsedInOperatingActivities": "1200", "jppfs_cor:NetCashProvidedByUsedInInvestingActivities": "-500",
		"jppfs_cor:PurchaseOfPropertyPlantAndEquipmentInvCF": "-400", "jppfs_cor:PurchaseOfIntangibleAssetsInvCF": "-50",
		"jppfs_cor:DepreciationAndAmortizationOpeCF": "300", "jppfs_cor:DividendsPaidFinCF": "-210", "jppfs_cor:PurchaseOfTreasuryStockFinCF": "-70",
	}}
	if err := s.UpsertEdinetDocument(ctx, doc, day, rep); err != nil {
		t.Fatal(err)
	}
	// IFRSの書類は、含まれない。
	if err := s.UpsertEdinetDocument(ctx, edinet.Document{DocID: "S100FS02", SecCode: "22220", DocTypeCode: "120", PeriodEnd: "2026-03-31"}, day,
		edinet.Report{AccountingStd: "IFRS", Scope: edinet.ScopeConsolidated, Facts: map[string]string{"jppfs_cor:Assets": "1"}}); err != nil {
		t.Fatal(err)
	}

	type fs struct {
		Receivables, Inventories, ShortTermLoans, Payables, Capex, Fcf, Debt, NetCash    float64
		CostRatio, CurrentRatio, EquityRatio, Roic, Accrual, Payout, TotalReturn, Ebitda float64
	}
	var v fs
	var n int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM financial_statements`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("financial_statements rows = %d (IFRSは含めない), err = %v", n, err)
	}
	err = s.conn.QueryRow(ctx, `
		SELECT receivables::float8, inventories::float8, short_term_loans::float8, payables::float8, capex::float8, fcf::float8,
		       interest_bearing_debt::float8, net_cash::float8, cost_ratio::float8, current_ratio::float8, equity_ratio::float8,
		       roic::float8, accrual_ratio::float8, payout_ratio::float8, total_return_ratio::float8, ebitda::float8
		FROM financial_statements WHERE code = '11110' AND fy_end = '2026-03-31'`).
		Scan(&v.Receivables, &v.Inventories, &v.ShortTermLoans, &v.Payables, &v.Capex, &v.Fcf, &v.Debt, &v.NetCash,
			&v.CostRatio, &v.CurrentRatio, &v.EquityRatio, &v.Roic, &v.Accrual, &v.Payout, &v.TotalReturn, &v.Ebitda)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]float64{
		"receivables": {v.Receivables, 800}, "inventories": {v.Inventories, 350}, "short_term_loans": {v.ShortTermLoans, 200},
		"payables": {v.Payables, 500}, "capex": {v.Capex, 450}, "fcf": {v.Fcf, 750},
		"interest_bearing_debt": {v.Debt, 900},                           // 150+50+700
		"net_cash":              {v.NetCash, 1000 + 100 + 200*0.7 - 900}, // 現金+有価証券+投資有価証券x70%-有利子負債
		"cost_ratio":            {v.CostRatio, 0.6}, "current_ratio": {v.CurrentRatio, 2},
		"equity_ratio": {v.EquityRatio, 0.625}, "roic": {v.Roic, 700.0 / 4650}, // 営業利益1000x0.7/(純資産3750+有利子負債900)
		"accrual_ratio": {v.Accrual, -500.0 / 6000}, "payout_ratio": {v.Payout, 0.3}, "total_return_ratio": {v.TotalReturn, 0.4},
		"ebitda": {v.Ebitda, 1300},
	}
	for name, c := range checks {
		if diff := c[0] - c[1]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}

	// 年度末の株価指標: 2026-03-31（火）の終値。休日の場合は、直前の取引日。
	f := func(x float64) *float64 { return &x }
	if err := s.UpsertPrices(ctx, []jquants.DailyBar{
		{Date: "2026-03-30", Code: "11110", C: f(1000), AdjFactor: f(1)},
		{Date: "2026-03-31", Code: "11110", C: f(1100), AdjFactor: f(1)},
		{Date: "2026-04-01", Code: "11110", C: f(1500), AdjFactor: f(1)}, // 決算期末より後は、使わない
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFinancials(ctx, []jquants.Financial{{"Code": "11110", "DiscNo": "1", "DiscDate": "2026-05-10", "DiscTime": "15:00:00",
		"CurPerType": "FY", "CurFYEn": "2026-03-31", "Sales": "10000", "NP": "700", "Eq": "3750", "ShOutFY": "1100", "TrShFY": "100"}}); err != nil {
		t.Fatal(err)
	}
	var price, mcap, per, pbr, psr, pcfr, ev, evEbitda float64
	err = s.conn.QueryRow(ctx, `SELECT price::float8, market_cap::float8, per::float8, pbr::float8, psr::float8, pcfr::float8, ev::float8, ev_ebitda::float8
		FROM valuation_history WHERE code = '11110' AND fy_end = '2026-03-31'`).Scan(&price, &mcap, &per, &pbr, &psr, &pcfr, &ev, &evEbitda)
	if err != nil {
		t.Fatal(err)
	}
	// 時価総額 = 1100円 x (1100-100)株 = 1,100,000
	hist := map[string][2]float64{
		"price": {price, 1100}, "market_cap": {mcap, 1100000}, "per": {per, 1100000.0 / 700}, "pbr": {pbr, 1100000.0 / 3750},
		"psr": {psr, 110}, "pcfr": {pcfr, 1100000.0 / 1200}, "ev": {ev, 1100000 - 340}, "ev_ebitda": {evEbitda, (1100000 - 340) / 1300.0},
	}
	for name, c := range hist {
		if diff := c[0] - c[1]; diff > 1e-6 || diff < -1e-6 {
			t.Errorf("valuation_history %s = %v, want %v", name, c[0], c[1])
		}
	}
}
