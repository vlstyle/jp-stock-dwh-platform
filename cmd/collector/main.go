package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/vlstyle/jp-stock-dwh-platform/internal/collector"
	"github.com/vlstyle/jp-stock-dwh-platform/internal/edinet"
	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
	"github.com/vlstyle/jp-stock-dwh-platform/internal/store"
)

const usage = `usage: collector <command> [flags]

commands:
  migrate   DBスキーマを最新にする
  master    上場銘柄一覧を取得して保存する
  prices    株価四本値を取得して保存する (--from, --to: YYYY-MM-DD)
  financials  財務情報を開示日ごとに取得して保存する (--from, --to: YYYY-MM-DD)
  edinet    EDINETの有価証券報告書から、現金・有価証券・有利子負債を取得して保存する (--from, --to: YYYY-MM-DD, --days)

environment:
  DATABASE_URL           PostgreSQL接続文字列 (必須)
  JQUANTS_API_KEY        J-Quants APIキー (master, prices で必須)
  JQUANTS_RATE_PER_MIN   1分あたりのリクエスト数 (既定: 5 = Freeプラン)
  EDINET_API_KEY         EDINET APIキー (edinet で必須)
  EDINET_INTERVAL_MS     EDINETへのリクエスト間隔 (ミリ秒。既定: 1000)
`

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log, os.Args[1:]); err != nil {
		log.Error("failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("command is required")
	}
	cmd, args := args[0], args[1:]

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	if cmd == "migrate" {
		return store.Migrate(ctx, dbURL)
	}

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fromStr := fs.String("from", "", "取得開始日 YYYY-MM-DD (prices・financials のみ。既定: prices はDBの最新日の翌日、financials は最新開示日)")
	toStr := fs.String("to", "", "取得終了日 YYYY-MM-DD (prices・financials のみ。既定: 今日(JST)から--lag-days日前)")
	days := fs.Int("days", 7, "edinet で --from 未指定時に、--to から遡る日数")
	lagDays := fs.Int("lag-days", 0, "--to 未指定時に今日から遅らせる日数 (Freeプランのデータ遅延に合わせる。例: 84)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cmd == "edinet" {
		return runEdinet(ctx, log, dbURL, *fromStr, *toStr, *days)
	}

	apiKey := os.Getenv("JQUANTS_API_KEY")
	if apiKey == "" {
		return errors.New("JQUANTS_API_KEY is required")
	}
	ratePerMin := 5
	if v := os.Getenv("JQUANTS_RATE_PER_MIN"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return fmt.Errorf("invalid JQUANTS_RATE_PER_MIN: %q", v)
		}
		ratePerMin = n
	}

	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close(context.WithoutCancel(ctx))

	c := &collector.Collector{
		Source: jquants.New(apiKey, ratePerMin),
		Sink:   st,
		Log:    log,
	}

	switch cmd {
	case "master":
		return c.CollectListedInfo(ctx)
	case "prices", "financials":
		from, err := parseDate(*fromStr)
		if err != nil {
			return err
		}
		to, err := parseDate(*toStr)
		if err != nil {
			return err
		}
		if *lagDays < 0 {
			return errors.New("--lag-days must be >= 0")
		}
		if to.IsZero() {
			to = todayJST().AddDate(0, 0, -*lagDays)
		}
		if cmd == "financials" {
			return c.CollectFinancials(ctx, from, to)
		}
		return c.CollectPrices(ctx, from, to)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func runEdinet(ctx context.Context, log *slog.Logger, dbURL, fromStr, toStr string, days int) error {
	apiKey := os.Getenv("EDINET_API_KEY")
	if apiKey == "" {
		return errors.New("EDINET_API_KEY is required")
	}
	interval := time.Second
	if v := os.Getenv("EDINET_INTERVAL_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return fmt.Errorf("invalid EDINET_INTERVAL_MS: %q", v)
		}
		interval = time.Duration(n) * time.Millisecond
	}
	from, err := parseDate(fromStr)
	if err != nil {
		return err
	}
	to, err := parseDate(toStr)
	if err != nil {
		return err
	}
	if to.IsZero() {
		to = todayJST()
	}
	if from.IsZero() {
		if days < 0 {
			return errors.New("--days must be >= 0")
		}
		from = to.AddDate(0, 0, -days)
	}

	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close(context.WithoutCancel(ctx))

	c := &collector.EdinetCollector{Source: edinet.New(apiKey, interval), Sink: st, Log: log}
	return c.CollectAnnualReports(ctx, from, to)
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	d, err := time.Parse(collector.DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD)", s)
	}
	return d, nil
}

func todayJST() time.Time {
	jst, _ := time.LoadLocation("Asia/Tokyo")
	n := time.Now().In(jst)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}
