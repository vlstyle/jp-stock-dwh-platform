package store

import (
	"context"
	"os"
	"testing"
	"time"

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
			"EPS": "50", "BPS": "500", "EqAR": "0.4", "FEPS": "60", "FDivAnn": "20", "ROE": "0.1", "CFO": "1000", "OdP": ""},
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
	var price, bps, pbr, per, fPer, yield, eqAr float64
	err = s.conn.QueryRow(ctx, `
		SELECT co_name, close::float8, bps::float8, pbr::float8, per::float8, f_per::float8,
		       f_dividend_yield::float8, eq_ar::float8
		FROM valuation WHERE code = '86970'`).Scan(&coName, &price, &bps, &pbr, &per, &fPer, &yield, &eqAr)
	if err != nil {
		t.Fatal(err)
	}
	// 分割(0.5)後の株数ベース: BPS=500*0.5=250, EPS=50*0.5=25, 予想EPS=70*0.5=35, 予想配当=20*0.5=10
	checks := map[string][2]float64{
		"close": {price, 500}, "bps": {bps, 250}, "pbr": {pbr, 2}, "per": {per, 20},
		"f_per": {fPer, 500.0 / 35}, "f_dividend_yield": {yield, 0.02}, "eq_ar": {eqAr, 0.4},
	}
	for name, c := range checks {
		if diff := c[0] - c[1]; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	if coName != "JPX" {
		t.Errorf("co_name = %q", coName)
	}

	var delisted int
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM valuation WHERE code = '99990'`).Scan(&delisted); err != nil || delisted != 0 {
		t.Errorf("stale-price code in valuation: count = %d, err = %v", delisted, err)
	}
}
