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
	"github.com/vlstyle/jp-stock-dwh-platform/internal/jquants"
	"github.com/vlstyle/jp-stock-dwh-platform/internal/store"
)

const usage = `usage: collector <command> [flags]

commands:
  migrate   DBスキーマを最新にする
  master    上場銘柄一覧を取得して保存する
  prices    株価四本値を取得して保存する (--from, --to: YYYY-MM-DD)

environment:
  DATABASE_URL           PostgreSQL接続文字列 (必須)
  JQUANTS_API_KEY        J-Quants APIキー (master, prices で必須)
  JQUANTS_RATE_PER_MIN   1分あたりのリクエスト数 (既定: 5 = Freeプラン)
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
	fromStr := fs.String("from", "", "取得開始日 YYYY-MM-DD (prices のみ。既定: DBの最新日の翌日)")
	toStr := fs.String("to", "", "取得終了日 YYYY-MM-DD (prices のみ。既定: 今日(JST))")
	if err := fs.Parse(args); err != nil {
		return err
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
	case "prices":
		from, err := parseDate(*fromStr)
		if err != nil {
			return err
		}
		to, err := parseDate(*toStr)
		if err != nil {
			return err
		}
		if to.IsZero() {
			to = todayJST()
		}
		return c.CollectPrices(ctx, from, to)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command: %s", cmd)
	}
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
