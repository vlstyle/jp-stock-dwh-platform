package edinet

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16le(s string) []byte {
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func makeZip(t *testing.T, csv string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string][]byte{
		"XBRL_TO_CSV/jpaud-aai-cn-001_E00000-000.csv":    utf16le("\"x\"\n"),
		"XBRL_TO_CSV/jpcrp030000-asr-001_E00000-000.csv": utf16le(csv),
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	zw.Close()
	return buf.Bytes()
}

const header = "\"要素ID\"\t\"項目名\"\t\"コンテキストID\"\t\"相対年度\"\t\"連結・個別\"\t\"期間・時点\"\t\"ユニットID\"\t\"単位\"\t\"値\"\r\n"

func row(elem, ctx, val string) string {
	return "\"" + elem + "\"\t\"名\"\t\"" + ctx + "\"\t\"当期末\"\t\"連結\"\t\"時点\"\t\"JPY\"\t\"円\"\t\"" + val + "\"\r\n"
}

func TestParseReportPrefersConsolidated(t *testing.T) {
	csv := header +
		row("jpdei_cor:AccountingStandardsDEI", "FilingDateInstant", "Japan GAAP") +
		row("jppfs_cor:CashAndDeposits", "CurrentYearInstant", "1000") +
		row("jppfs_cor:CashAndDeposits", "CurrentYearInstant_NonConsolidatedMember", "300") +
		row("jppfs_cor:CashAndDeposits", "Prior1YearInstant", "900") +
		row("jppfs_cor:LongTermLoansPayable", "CurrentYearInstant_NonConsolidatedMember", "50") +
		row("jppfs_cor:BondsPayable", "CurrentYearInstant", "－") +
		row("jppfs_cor:InvestmentSecurities", "CurrentYearInstant", "-20") +
		row("jppfs_cor:AccountsPayableTrade", "CurrentYearInstant", "7") +
		row("jpigp_cor:BorrowingsCLIFRS", "CurrentYearInstant", "11")
	rep, err := ParseReport(makeZip(t, csv))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"jppfs_cor:CashAndDeposits":      "1000",
		"jppfs_cor:InvestmentSecurities": "-20",
		"jpigp_cor:BorrowingsCLIFRS":     "11",
	}
	if rep.Scope != ScopeConsolidated || rep.AccountingStd != "Japan GAAP" || len(rep.Facts) != len(want) {
		t.Fatalf("report = %+v", rep)
	}
	for k, v := range want {
		if rep.Facts[k] != v {
			t.Errorf("%s = %q, want %q", k, rep.Facts[k], v)
		}
	}
}

func TestParseReportTakesCurrentYearDuration(t *testing.T) {
	csv := header +
		row("jppfs_cor:NetSales", "CurrentYearDuration", "5000") +
		row("jppfs_cor:NetSales", "Prior1YearDuration", "4000") +
		row("jppfs_cor:NetSales", "CurrentYearDuration_NonConsolidatedMember", "900") +
		row("jppfs_cor:CostOfSales", "CurrentYearDuration", "3000") +
		row("jppfs_cor:DepreciationAndAmortizationOpeCF", "CurrentYearDuration", "200") +
		row("jppfs_cor:IncomeTaxes", "CurrentYearDuration", "10") +
		row("jppfs_cor:CostOfSales", "CurrentYearDuration_SomeSegmentMember", "1") +
		row("jppfs_cor:CashAndDeposits", "CurrentYearInstant", "1000")
	rep, err := ParseReport(makeZip(t, csv))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"jppfs_cor:NetSales":                         "5000",
		"jppfs_cor:CostOfSales":                      "3000",
		"jppfs_cor:DepreciationAndAmortizationOpeCF": "200",
		"jppfs_cor:CashAndDeposits":                  "1000",
	}
	if rep.Scope != ScopeConsolidated || len(rep.Facts) != len(want) {
		t.Fatalf("report = %+v", rep)
	}
	for k, v := range want {
		if rep.Facts[k] != v {
			t.Errorf("%s = %q, want %q", k, rep.Facts[k], v)
		}
	}
}

func TestParseReportFallsBackToNonConsolidated(t *testing.T) {
	csv := header +
		row("jppfs_cor:CashAndDeposits", "CurrentYearInstant_NonConsolidatedMember", "300") +
		row("jppfs_cor:LongTermLoansPayable", "CurrentYearInstant_NonConsolidatedMember", "50")
	rep, err := ParseReport(makeZip(t, csv))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scope != ScopeNonConsolidated || rep.Facts["jppfs_cor:CashAndDeposits"] != "300" || len(rep.Facts) != 2 {
		t.Errorf("report = %+v", rep)
	}
}

func TestParseReportRejectsNonZip(t *testing.T) {
	if _, err := ParseReport([]byte("not a zip")); err == nil {
		t.Fatal("expected error")
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("secret-key", time.Millisecond, WithBaseURL(srv.URL))
}

func TestDocumentsSendsKeyInHeaderOnly(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Ocp-Apim-Subscription-Key") != "secret-key" || r.URL.Query().Get("Subscription-Key") != "" {
			t.Errorf("auth: header=%q query=%q", r.Header.Get("Ocp-Apim-Subscription-Key"), r.URL.RawQuery)
		}
		if r.URL.Query().Get("date") != "2026-06-26" || r.URL.Query().Get("type") != "2" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"metadata":{"status":"200","message":"OK"},"results":[{"docID":"S1","secCode":"62480","docTypeCode":"120","periodEnd":null,"csvFlag":"1","withdrawalStatus":"0"}]}`))
	})
	docs, err := c.Documents(context.Background(), "2026-06-26")
	if err != nil || len(docs) != 1 || docs[0].DocID != "S1" || docs[0].PeriodEnd != "" {
		t.Fatalf("docs = %+v, err = %v", docs, err)
	}
}

func TestDocumentsBodyErrorsWithHTTP200(t *testing.T) {
	for name, body := range map[string]string{
		"invalid key": `{"StatusCode": 401,"message": "Access denied"}`,
		"not found":   `{"metadata":{"status":"404","message":"Not Found"}}`,
	} {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		_, err := c.Documents(context.Background(), "2026-06-26")
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestReportErrorAsJSON(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(`{"metadata":{"status":"404","message":"Not Found"}}`))
	})
	_, err := c.Report(context.Background(), "S1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != "404" {
		t.Errorf("err = %v", err)
	}
}

func TestReportParsesZip(t *testing.T) {
	z := makeZip(t, header+row("jppfs_cor:CashAndDeposits", "CurrentYearInstant", "5"))
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents/S1" || r.URL.Query().Get("type") != "5" {
			t.Errorf("url = %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(z)
	})
	rep, err := c.Report(context.Background(), "S1")
	if err != nil || rep.Facts["jppfs_cor:CashAndDeposits"] != "5" {
		t.Errorf("rep = %+v, err = %v", rep, err)
	}
}
