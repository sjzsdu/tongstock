package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/statesnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func init() {
	parent := &cobra.Command{Use: "backfill", Short: "回填本地派生数据"}
	var from, mode, codes string
	var verify bool
	cmd := &cobra.Command{Use: "states", Short: "按股单遍回填四维每日状态（只读本地行情）", RunE: func(cmd *cobra.Command, args []string) error {
		if err := param.AutoInit(); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: stateBackfillDSN(cfg.Database.DSN)})
		if err != nil {
			return err
		}
		defer store.Close()
		// Date indexes touch thousands of pages per stock. Keep this dedicated
		// backfill connection cache bounded at 128 MiB to avoid page-cache thrashing.
		if _, err := store.DB().ExecContext(cmd.Context(), `PRAGMA cache_size=-131072`); err != nil {
			return err
		}
		start := ""
		if from != "" {
			for _, layout := range []string{"20060102", "2006-01-02"} {
				if d, e := time.Parse(layout, from); e == nil {
					start = d.Format("20060102")
					break
				}
			}
			if start == "" {
				return fmt.Errorf("invalid --from")
			}
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return runStatesBackfill(ctx, store, statesnapshot.Options{From: start, Mode: mode, Codes: splitCodes(codes), Verify: verify, Progress: func(code string, n int64) { fmt.Fprintf(cmd.ErrOrStderr(), "states %s changed=%d\n", code, n) }}, cmd)
	}}
	cmd.Flags().StringVar(&from, "from", "", "起始交易日 YYYY-MM-DD；指标种子始终读完整历史")
	cmd.Flags().StringVar(&mode, "mode", "full", "full/incremental")
	cmd.Flags().StringVar(&codes, "codes", "", "可选股票池，缺省为本地全市场（无数量上限）")
	cmd.Flags().BoolVar(&verify, "verify", false, "逐行校验持久化状态和参考内核 hash/覆盖率")
	parent.AddCommand(cmd)
	rootCmd.AddCommand(parent)
}

func runStatesBackfill(ctx context.Context, store *storage.Storage, opt statesnapshot.Options, cmd *cobra.Command) error {
	opt.Mode = strings.ToLower(opt.Mode)
	result, err := (statesnapshot.Store{DB: store.DB()}).Run(ctx, opt)
	if e := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err == nil {
		err = e
	}
	return err
}

// Acquire the SQLite write reservation before reading each stock, so another
// sync cannot invalidate the read snapshot before the first state write.
func stateBackfillDSN(dsn string) string {
	if dsn == "" {
		dsn = config.DBPath()
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_txlock=immediate"
}
