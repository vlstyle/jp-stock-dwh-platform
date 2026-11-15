package collector

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
)

type fakeSource struct {
	fetched []string
	empty   map[string]bool
}

func (f *fakeSource) ListedInfo(context.Context, string) ([]jquants.ListedInfo, error) {
	return []jquants.ListedInfo{{Code: "86970"}}, nil
}

func (f *fakeSource) DailyBars(_ context.Context, date string) ([]jquants.DailyBar, error) {
	f.fetched = append(f.fetched, date)
	if f.empty[date] {
		return nil, nil
	}
	return []jquants.DailyBar{{Date: date, Code: "86970"}}, nil
}

type fakeSink struct {
	latest time.Time
	saved  []string
}

func (f *fakeSink) UpsertListedInfo(context.Context, []jquants.ListedInfo) error { return nil }

func (f *fakeSink) UpsertPrices(_ context.Context, rows []jquants.DailyBar) error {
	f.saved = append(f.saved, rows[0].Date)
	return nil
}

func (f *fakeSink) LatestPriceDate(context.Context) (time.Time, bool, error) {
	return f.latest, !f.latest.IsZero(), nil
}

func newCollector(src *fakeSource, sink *fakeSink) *Collector {
	return &Collector{Source: src, Sink: sink, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func day(s string) time.Time {
	d, err := time.Parse(DateLayout, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestCollectPricesSkipsWeekendsAndHolidays(t *testing.T) {
	src := &fakeSource{empty: map[string]bool{"2024-01-08": true}}
	sink := &fakeSink{}

	// 2024-01-05(金) 〜 2024-01-09(火)。01-06,07は週末、01-08は祝日扱い。
	if err := newCollector(src, sink).CollectPrices(context.Background(), day("2024-01-05"), day("2024-01-09")); err != nil {
		t.Fatal(err)
	}

	wantFetched := []string{"2024-01-05", "2024-01-08", "2024-01-09"}
	if len(src.fetched) != len(wantFetched) {
		t.Fatalf("fetched = %v", src.fetched)
	}
	for i, w := range wantFetched {
		if src.fetched[i] != w {
			t.Errorf("fetched[%d] = %s, want %s", i, src.fetched[i], w)
		}
	}
	if len(sink.saved) != 2 {
		t.Errorf("saved = %v", sink.saved)
	}
}

func TestCollectPricesDefaultsFromLatest(t *testing.T) {
	src := &fakeSource{}
	sink := &fakeSink{latest: day("2024-01-04")}

	if err := newCollector(src, sink).CollectPrices(context.Background(), time.Time{}, day("2024-01-05")); err != nil {
		t.Fatal(err)
	}
	if len(src.fetched) != 1 || src.fetched[0] != "2024-01-05" {
		t.Errorf("fetched = %v", src.fetched)
	}
}

func TestCollectPricesEmptyTableNeedsFrom(t *testing.T) {
	err := newCollector(&fakeSource{}, &fakeSink{}).CollectPrices(context.Background(), time.Time{}, day("2024-01-05"))
	if err == nil {
		t.Fatal("expected error")
	}
}
