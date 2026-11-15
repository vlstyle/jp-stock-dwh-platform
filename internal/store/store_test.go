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
