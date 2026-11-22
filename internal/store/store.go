package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/edinet"
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

type finCol struct {
	col string
	key string
	typ string
}

var finCols = []finCol{
	{"code", "Code", "text"},
	{"disc_no", "DiscNo", "text"},
	{"disc_date", "DiscDate", "date"},
	{"disc_time", "DiscTime", "time"},
	{"doc_type", "DocType", "text"},
	{"cur_per_type", "CurPerType", "text"},
	{"cur_per_st", "CurPerSt", "date"},
	{"cur_per_en", "CurPerEn", "date"},
	{"cur_fy_st", "CurFYSt", "date"},
	{"cur_fy_en", "CurFYEn", "date"},
	{"sales", "Sales", "numeric"},
	{"op", "OP", "numeric"},
	{"odp", "OdP", "numeric"},
	{"np", "NP", "numeric"},
	{"eps", "EPS", "numeric"},
	{"deps", "DEPS", "numeric"},
	{"ta", "TA", "numeric"},
	{"eq", "Eq", "numeric"},
	{"eq_ar", "EqAR", "numeric"},
	{"bps", "BPS", "numeric"},
	{"cfo", "CFO", "numeric"},
	{"cfi", "CFI", "numeric"},
	{"cff", "CFF", "numeric"},
	{"cash_eq", "CashEq", "numeric"},
	{"sh_eq", "ShEq", "numeric"},
	{"roe", "ROE", "numeric"},
	{"div_ann", "DivAnn", "numeric"},
	{"f_sales", "FSales", "numeric"},
	{"f_op", "FOP", "numeric"},
	{"f_odp", "FOdP", "numeric"},
	{"f_np", "FNP", "numeric"},
	{"f_eps", "FEPS", "numeric"},
	{"f_div_ann", "FDivAnn", "numeric"},
	{"sh_out_fy", "ShOutFY", "numeric"},
	{"tr_sh_fy", "TrShFY", "numeric"},
	{"avg_sh", "AvgSh", "numeric"},
}

// APIの値は文字列で、値が無い場合は空文字列なので、SQL側でNULLIFしてから型変換する。
var upsertFinancial = buildUpsertFinancial()

func buildUpsertFinancial() string {
	var cols, vals, sets []string
	for i, c := range finCols {
		cols = append(cols, c.col)
		if c.typ == "text" {
			vals = append(vals, fmt.Sprintf("NULLIF($%d::text, '')", i+1))
		} else {
			vals = append(vals, fmt.Sprintf("NULLIF($%d::text, '')::%s", i+1, c.typ))
		}
		if c.col != "code" && c.col != "disc_no" {
			sets = append(sets, fmt.Sprintf("%s = EXCLUDED.%s", c.col, c.col))
		}
	}
	raw := len(finCols) + 1
	cols = append(cols, "raw")
	vals = append(vals, fmt.Sprintf("$%d::jsonb", raw))
	sets = append(sets, "raw = EXCLUDED.raw", "updated_at = now()")
	return "INSERT INTO financials (" + strings.Join(cols, ", ") + ")\nVALUES (" + strings.Join(vals, ", ") +
		")\nON CONFLICT (code, disc_no) DO UPDATE SET " + strings.Join(sets, ", ")
}

func (s *Store) UpsertFinancials(ctx context.Context, rows []jquants.Financial) error {
	return s.sendBatches(ctx, len(rows), func(b *pgx.Batch, i int) error {
		r := rows[i]
		raw, err := json.Marshal(r)
		if err != nil {
			return err
		}
		args := make([]any, 0, len(finCols)+1)
		for _, c := range finCols {
			args = append(args, r[c.key])
		}
		args = append(args, string(raw))
		b.Queue(upsertFinancial, args...)
		return nil
	})
}

func (s *Store) LatestFinancialDate(ctx context.Context) (time.Time, bool, error) {
	var d *time.Time
	if err := s.conn.QueryRow(ctx, `SELECT max(disc_date) FROM financials`).Scan(&d); err != nil {
		return time.Time{}, false, err
	}
	if d == nil {
		return time.Time{}, false, nil
	}
	return *d, true, nil
}

// ExistingEdinetDocs は、ids のうち、保存済みの書類IDの集合を返す。
func (s *Store) ExistingEdinetDocs(ctx context.Context, ids []string) (map[string]bool, error) {
	rows, err := s.conn.Query(ctx, `SELECT doc_id FROM edinet_documents WHERE doc_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found[id] = true
	}
	return found, rows.Err()
}

const upsertEdinetDocument = `
INSERT INTO edinet_documents (doc_id, code, filer_name, doc_type_code, period_end, submit_date, accounting_std, scope)
VALUES ($1, $2, $3, $4, NULLIF($5::text, '')::date, $6, NULLIF($7, ''), $8)
ON CONFLICT (doc_id) DO UPDATE SET
    code = EXCLUDED.code, filer_name = EXCLUDED.filer_name, doc_type_code = EXCLUDED.doc_type_code,
    period_end = EXCLUDED.period_end, submit_date = EXCLUDED.submit_date,
    accounting_std = EXCLUDED.accounting_std, scope = EXCLUDED.scope, updated_at = now()`

// UpsertEdinetDocument は書類と、その項目を、1トランザクションで保存する。
func (s *Store) UpsertEdinetDocument(ctx context.Context, d edinet.Document, submitDate time.Time, r edinet.Report) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// EDINETの証券コードは5桁（J-Quantsの銘柄コードと同じ）。
	if _, err := tx.Exec(ctx, upsertEdinetDocument, d.DocID, d.SecCode, d.FilerName, d.DocTypeCode,
		d.PeriodEnd, submitDate, r.AccountingStd, r.Scope); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM edinet_facts WHERE doc_id = $1`, d.DocID); err != nil {
		return err
	}
	b := &pgx.Batch{}
	for elem, v := range r.Facts {
		b.Queue(`INSERT INTO edinet_facts (doc_id, element_id, value) VALUES ($1, $2, $3::numeric)`, d.DocID, elem, v)
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const snapshotSectorStats = `
INSERT INTO sector_stats_daily (snapshot_date, sector, metric, n, p25, median, p75)
SELECT $1::date, sector, metric, n, p25, median, p75 FROM sector_stats_live
ON CONFLICT (snapshot_date, sector, metric) DO UPDATE SET
    n = EXCLUDED.n, p25 = EXCLUDED.p25, median = EXCLUDED.median, p75 = EXCLUDED.p75`

// SnapshotSectorStats は、現在の業種ごとの指標の分布を、最新の株価日の日付で保存する。同じ日付で再実行すると、置き換える。
func (s *Store) SnapshotSectorStats(ctx context.Context) (time.Time, int64, error) {
	date, ok, err := s.LatestPriceDate(ctx)
	if err != nil {
		return time.Time{}, 0, err
	}
	if !ok {
		return time.Time{}, 0, errors.New("株価がありません")
	}
	tag, err := s.conn.Exec(ctx, snapshotSectorStats, date)
	if err != nil {
		return time.Time{}, 0, err
	}
	return date, tag.RowsAffected(), nil
}

// screeningColumns は、screening_liveとscreening_dailyに共通の、指標の列（snapshot_dateを除く）。
const screeningColumns = `code, name, market, sector, price_date, close, market_cap, per, pbr, f_per, dividend_yield, roe, eq_ar,
    net_cash_to_mcap, fy_end, fy_age_days, op_margin, np_margin, roa, sales_yoy, op_yoy, eps_yoy, sales_cagr_3y, psr, bep_ratio`

// SnapshotScreening は、現在の銘柄ごとの指標（screening_live）を、最新の株価日の日付で保存する。
// 同じ日付で再実行すると、その日付の行を、すべて置き換える（上場廃止などで、消えた銘柄が残らない）。
func (s *Store) SnapshotScreening(ctx context.Context) (time.Time, int64, error) {
	date, ok, err := s.LatestPriceDate(ctx)
	if err != nil {
		return time.Time{}, 0, err
	}
	if !ok {
		return time.Time{}, 0, errors.New("株価がありません")
	}
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM screening_daily WHERE snapshot_date = $1`, date); err != nil {
		return time.Time{}, 0, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO screening_daily (snapshot_date, `+screeningColumns+`)
SELECT $1::date, `+screeningColumns+` FROM screening_live`, date)
	if err != nil {
		return time.Time{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, 0, err
	}
	return date, tag.RowsAffected(), nil
}
