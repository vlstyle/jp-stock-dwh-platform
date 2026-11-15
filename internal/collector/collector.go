package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
)

const DateLayout = "2006-01-02"

type Source interface {
	ListedInfo(ctx context.Context, date string) ([]jquants.ListedInfo, error)
	DailyBars(ctx context.Context, date string) ([]jquants.DailyBar, error)
	Financials(ctx context.Context, date string) ([]jquants.Financial, error)
}

type Sink interface {
	UpsertListedInfo(ctx context.Context, rows []jquants.ListedInfo) error
	UpsertPrices(ctx context.Context, rows []jquants.DailyBar) error
	LatestPriceDate(ctx context.Context) (time.Time, bool, error)
	UpsertFinancials(ctx context.Context, rows []jquants.Financial) error
	LatestFinancialDate(ctx context.Context) (time.Time, bool, error)
}

type Collector struct {
	Source Source
	Sink   Sink
	Log    *slog.Logger
}

func (c *Collector) CollectListedInfo(ctx context.Context) error {
	rows, err := c.Source.ListedInfo(ctx, "")
	if err != nil {
		return err
	}
	if err := c.Sink.UpsertListedInfo(ctx, rows); err != nil {
		return err
	}
	c.Log.Info("listed_info upserted", "rows", len(rows))
	return nil
}

// CollectPrices は from から to までの平日の株価を1日ずつ取得して保存する。
// from がゼロ値の場合は DB の最新日の翌日から始める。
func (c *Collector) CollectPrices(ctx context.Context, from, to time.Time) error {
	if from.IsZero() {
		latest, ok, err := c.Sink.LatestPriceDate(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("prices テーブルが空のため --from の指定が必要です")
		}
		from = latest.AddDate(0, 0, 1)
	}

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		date := d.Format(DateLayout)
		rows, err := c.Source.DailyBars(ctx, date)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			c.Log.Info("no prices (holiday)", "date", date)
			continue
		}
		if err := c.Sink.UpsertPrices(ctx, rows); err != nil {
			return err
		}
		c.Log.Info("prices upserted", "date", date, "rows", len(rows))
	}
	return nil
}

// CollectFinancials は from から to までの平日に開示された財務情報を1日ずつ取得して保存する。
// from がゼロ値の場合は DB の最新開示日から始める（同日の追加開示を取りこぼさないため当日を含める）。
func (c *Collector) CollectFinancials(ctx context.Context, from, to time.Time) error {
	if from.IsZero() {
		latest, ok, err := c.Sink.LatestFinancialDate(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("financials テーブルが空のため --from の指定が必要です")
		}
		from = latest
	}

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		date := d.Format(DateLayout)
		rows, err := c.Source.Financials(ctx, date)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			c.Log.Info("no financials", "date", date)
			continue
		}
		if err := c.Sink.UpsertFinancials(ctx, rows); err != nil {
			return err
		}
		c.Log.Info("financials upserted", "date", date, "rows", len(rows))
	}
	return nil
}
