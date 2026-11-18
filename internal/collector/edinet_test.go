package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/edinet"
)

type fakeEdinetSource struct {
	docs    map[string][]edinet.Document
	failIDs map[string]bool
	listed  []string
	fetched []string
}

func (f *fakeEdinetSource) Documents(_ context.Context, date string) ([]edinet.Document, error) {
	f.listed = append(f.listed, date)
	return f.docs[date], nil
}

func (f *fakeEdinetSource) Report(_ context.Context, id string) (edinet.Report, error) {
	f.fetched = append(f.fetched, id)
	if f.failIDs[id] {
		return edinet.Report{}, errors.New("boom")
	}
	return edinet.Report{Scope: edinet.ScopeConsolidated, Facts: map[string]string{"jppfs_cor:CashAndDeposits": "1"}}, nil
}

type fakeEdinetSink struct {
	existing map[string]bool
	saved    []string
}

func (f *fakeEdinetSink) ExistingEdinetDocs(context.Context, []string) (map[string]bool, error) {
	return f.existing, nil
}

func (f *fakeEdinetSink) UpsertEdinetDocument(_ context.Context, d edinet.Document, _ time.Time, _ edinet.Report) error {
	f.saved = append(f.saved, d.DocID)
	return nil
}

func doc(id, secCode, typ string) edinet.Document {
	return edinet.Document{DocID: id, SecCode: secCode, DocTypeCode: typ, CSVFlag: "1", WithdrawalStatus: "0"}
}

func newEdinetCollector(src *fakeEdinetSource, sink *fakeEdinetSink) *EdinetCollector {
	return &EdinetCollector{Source: src, Sink: sink, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestCollectAnnualReportsFiltersAndSkipsExisting(t *testing.T) {
	withdrawn := doc("W", "10000", "120")
	withdrawn.WithdrawalStatus = "1"
	noCSV := doc("N", "10000", "120")
	noCSV.CSVFlag = "0"
	src := &fakeEdinetSource{docs: map[string][]edinet.Document{
		"2026-06-26": {doc("A", "62480", "120"), doc("B", "", "120"), doc("C", "62480", "140"), doc("D", "72010", "120"), withdrawn, noCSV},
	}}
	sink := &fakeEdinetSink{existing: map[string]bool{"D": true}}

	// 2026-06-26(金) 〜 2026-06-29(月)。土日は一覧を取得しない。
	if err := newEdinetCollector(src, sink).CollectAnnualReports(context.Background(), day("2026-06-26"), day("2026-06-29")); err != nil {
		t.Fatal(err)
	}
	if len(src.listed) != 2 || src.listed[0] != "2026-06-26" || src.listed[1] != "2026-06-29" {
		t.Errorf("listed = %v", src.listed)
	}
	if len(sink.saved) != 1 || sink.saved[0] != "A" {
		t.Errorf("saved = %v", sink.saved)
	}
	if len(src.fetched) != 1 || src.fetched[0] != "A" {
		t.Errorf("fetched = %v", src.fetched)
	}
}

func TestCollectAnnualReportsContinuesAfterFailure(t *testing.T) {
	src := &fakeEdinetSource{
		docs:    map[string][]edinet.Document{"2026-06-26": {doc("A", "62480", "120"), doc("B", "72010", "120")}},
		failIDs: map[string]bool{"A": true},
	}
	sink := &fakeEdinetSink{}
	err := newEdinetCollector(src, sink).CollectAnnualReports(context.Background(), day("2026-06-26"), day("2026-06-26"))
	if err == nil {
		t.Fatal("expected error")
	}
	if len(sink.saved) != 1 || sink.saved[0] != "B" {
		t.Errorf("saved = %v", sink.saved)
	}
}
