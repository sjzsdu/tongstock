package main

import (
	"fmt"
	"sync"

	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/stockpoolrepo"
	"github.com/sjzsdu/tongstock/internal/app/discoveryapp"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "从冻结真实历史中自动挖掘可证伪候选规律",
	Long: "用户只需提供股票代码。系统自动冻结真实数据，在发现区间搜索候选，" +
		"预留未触碰样本，输出可执行方法和验证交接信息，不直接生成买入推荐。",
}

var (
	discoverCodes    []string
	discoverPool     string
	discoverSnapshot string
	discoverQuestion string
	discoverHoldDays int
	discoverBudget   int
)

// lazyKlineSyncer 延迟建立 TDX 连接：仅当 discover 发现本地缺数据时才联网同步。
type lazyKlineSyncer struct {
	once sync.Once
	svc  *tdx.Service
	err  error
}

func (l *lazyKlineSyncer) SyncDailyKlines(codes []string, mode tdx.SyncMode, concurrency int) tdx.KlineBatchSyncResult {
	l.once.Do(func() { l.svc, l.err = dialService() })
	if l.err != nil {
		return tdx.KlineBatchSyncResult{Total: len(codes), Failed: len(codes)}
	}
	return l.svc.SyncDailyKlines(codes, mode, concurrency)
}

var discoverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "只输入股票代码启动真实数据规律发现",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openValidationStorage()
		if err != nil {
			return err
		}
		defer store.Close()
		var resolver discoveryapp.PoolResolver
		if discoverPool != "" {
			resolver, err = stockpoolrepo.NewResolver(store)
			if err != nil {
				return err
			}
		}
		result, err := discoveryapp.NewRunner(store, resolver, &lazyKlineSyncer{}).Run(cmd.Context(), discoveryapp.RunRequest{
			Codes: discoverCodes, PoolID: discoverPool,
			SnapshotID: discoverSnapshot, Question: discoverQuestion,
			HoldDays: discoverHoldDays, SearchBudget: discoverBudget,
		})
		if err != nil {
			return err
		}
		return printValidationJSON(result)
	},
}

var discoverShowCmd = &cobra.Command{
	Use:   "show <research-id>",
	Short: "查询并重新校验持久化的发现轨迹",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openValidationStorage()
		if err != nil {
			return err
		}
		defer store.Close()
		repo, err := discoveryrepo.NewTraceRepository(store)
		if err != nil {
			return err
		}
		result, err := repo.Get(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("get research trace: %w", err)
		}
		return printValidationJSON(result)
	},
}

func init() {
	discoverRunCmd.Flags().StringSliceVar(&discoverCodes, "code", nil, "股票代码，可重复或逗号分隔")
	discoverRunCmd.Flags().StringVar(&discoverPool, "pool", "", "股票池 ID：解析其过滤条件得到股票代码")
	discoverRunCmd.Flags().StringVar(&discoverSnapshot, "snapshot", "", "已冻结数据快照 ID (空=自动创建)")
	discoverRunCmd.Flags().StringVar(&discoverQuestion, "question", "", "可选研究问题")
	discoverRunCmd.Flags().IntVar(&discoverHoldDays, "hold-days", 0, "候选持有交易日 (0=自动 5)")
	discoverRunCmd.Flags().IntVar(&discoverBudget, "search-budget", 0, "搜索预算 (0=自动 24)")
	discoverCmd.AddCommand(discoverRunCmd, discoverShowCmd)
	rootCmd.AddCommand(discoverCmd)
}
