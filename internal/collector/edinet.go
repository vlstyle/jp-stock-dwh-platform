package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/edinet"
)

type EdinetSource interface {
	Documents(ctx context.Context, date string) ([]edinet.Document, error)
	Report(ctx context.Context, docID string) (edinet.Report, error)
}

type EdinetSink interface {
	ExistingEdinetDocs(ctx context.Context, ids []string) (map[string]bool, error)
	UpsertEdinetDocument(ctx context.Context, d edinet.Document, submitDate time.Time, r edinet.Report) error
}

type EdinetCollector struct {
	Source EdinetSource
	Sink   EdinetSink
	Log    *slog.Logger
}

// CollectAnnualReports は from から to までの平日に提出された、上場会社の有価証券報告書を取得して保存する。
// 保存済みの書類は取得しない。1件の取得に失敗しても続行し、最後にまとめてエラーを返す。
func (c *EdinetCollector) CollectAnnualReports(ctx context.Context, from, to time.Time) error {
	var errs []error
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		date := d.Format(DateLayout)
		docs, err := c.Source.Documents(ctx, date)
		if err != nil {
			return err
		}
		var targets []edinet.Document
		var ids []string
		for _, doc := range docs {
			if doc.DocTypeCode == edinet.DocTypeAnnualReport && doc.SecCode != "" &&
				doc.WithdrawalStatus == "0" && doc.CSVFlag == "1" {
				targets = append(targets, doc)
				ids = append(ids, doc.DocID)
			}
		}
		if len(targets) == 0 {
			continue
		}
		existing, err := c.Sink.ExistingEdinetDocs(ctx, ids)
		if err != nil {
			return err
		}

		saved := 0
		for _, doc := range targets {
			if existing[doc.DocID] {
				continue
			}
			rep, err := c.Source.Report(ctx, doc.DocID)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				c.Log.Error("edinet report failed", "date", date, "doc_id", doc.DocID, "code", doc.SecCode, "error", err)
				errs = append(errs, fmt.Errorf("%s: %w", doc.DocID, err))
				continue
			}
			if err := c.Sink.UpsertEdinetDocument(ctx, doc, d, rep); err != nil {
				return err
			}
			saved++
		}
		c.Log.Info("edinet reports upserted", "date", date, "targets", len(targets), "saved", saved)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d件の書類の取得に失敗しました: %w", len(errs), errors.Join(errs...))
	}
	return nil
}
