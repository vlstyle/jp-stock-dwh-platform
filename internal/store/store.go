package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
	"github.com/vlstyle/jp-stock-dwh-platform/migrations"
)

const dateLayout = "2006-01-02"

const batchSize = 1000

type Store struct {
	conn *pgx.Conn
}

func Open(ctx context.Context, url string) (*Store, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.conn.Close(ctx)
}

func Migrate(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}

func (s *Store) LatestPriceDate(ctx context.Context) (time.Time, bool, error) {
	var d *time.Time
	if err := s.conn.QueryRow(ctx, `SELECT max(date) FROM prices`).Scan(&d); err != nil {
		return time.Time{}, false, err
	}
	if d == nil {
		return time.Time{}, false, nil
	}
	return *d, true, nil
}

const upsertListedInfo = `
INSERT INTO listed_info (code, date, co_name, co_name_en, s17, s17_nm, s33, s33_nm, scale_cat, mkt, mkt_nm, mrgn, mrgn_nm, prod_cat)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (code, date) DO UPDATE SET
    co_name = EXCLUDED.co_name, co_name_en = EXCLUDED.co_name_en,
    s17 = EXCLUDED.s17, s17_nm = EXCLUDED.s17_nm, s33 = EXCLUDED.s33, s33_nm = EXCLUDED.s33_nm,
    scale_cat = EXCLUDED.scale_cat, mkt = EXCLUDED.mkt, mkt_nm = EXCLUDED.mkt_nm,
    mrgn = EXCLUDED.mrgn, mrgn_nm = EXCLUDED.mrgn_nm, prod_cat = EXCLUDED.prod_cat,
    updated_at = now()`

func (s *Store) UpsertListedInfo(ctx context.Context, rows []jquants.ListedInfo) error {
	return s.sendBatches(ctx, len(rows), func(b *pgx.Batch, i int) error {
		r := rows[i]
		d, err := time.Parse(dateLayout, r.Date)
		if err != nil {
			return fmt.Errorf("listed_info %s: date %q: %w", r.Code, r.Date, err)
		}
		b.Queue(upsertListedInfo, r.Code, d, r.CoName, r.CoNameEn, r.S17, r.S17Nm, r.S33, r.S33Nm,
			r.ScaleCat, r.Mkt, r.MktNm, r.Mrgn, r.MrgnNm, r.ProdCat)
		return nil
	})
}

const upsertPrice = `
INSERT INTO prices (code, date, open, high, low, close, volume, turnover, adj_factor,
                    adj_open, adj_high, adj_low, adj_close, adj_volume, upper_limit, lower_limit)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (code, date) DO UPDATE SET
    open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close,
    volume = EXCLUDED.volume, turnover = EXCLUDED.turnover, adj_factor = EXCLUDED.adj_factor,
    adj_open = EXCLUDED.adj_open, adj_high = EXCLUDED.adj_high, adj_low = EXCLUDED.adj_low,
    adj_close = EXCLUDED.adj_close, adj_volume = EXCLUDED.adj_volume,
    upper_limit = EXCLUDED.upper_limit, lower_limit = EXCLUDED.lower_limit,
    updated_at = now()`

func (s *Store) UpsertPrices(ctx context.Context, rows []jquants.DailyBar) error {
	return s.sendBatches(ctx, len(rows), func(b *pgx.Batch, i int) error {
		r := rows[i]
		d, err := time.Parse(dateLayout, r.Date)
		if err != nil {
			return fmt.Errorf("prices %s: date %q: %w", r.Code, r.Date, err)
		}
		b.Queue(upsertPrice, r.Code, d, r.O, r.H, r.L, r.C, r.Vo, r.Va, r.AdjFactor,
			r.AdjO, r.AdjH, r.AdjL, r.AdjC, r.AdjVo, r.UL == "1", r.LL == "1")
		return nil
	})
}

func (s *Store) sendBatches(ctx context.Context, n int, queue func(b *pgx.Batch, i int) error) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for start := 0; start < n; start += batchSize {
		end := min(start+batchSize, n)
		b := &pgx.Batch{}
		for i := start; i < end; i++ {
			if err := queue(b, i); err != nil {
				return err
			}
		}
		if err := tx.SendBatch(ctx, b).Close(); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
