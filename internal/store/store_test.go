package store

import (
	"context"
	"os"
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
