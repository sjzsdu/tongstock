package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
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
	discoverSnapshot string
	discoverQuestion string
	discoverHoldDays int
	discoverBudget   int
)

var discoverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "只输入股票代码启动真实数据规律发现",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		codes := normalizeDiscoveryCodes(discoverCodes)
		if len(codes) == 0 {
			return fmt.Errorf("--code is required")
		}
		store, err := openValidationStorage()
		if err != nil {
			return err
		}
		defer store.Close()
		snapshotID := strings.TrimSpace(discoverSnapshot)
		if snapshotID == "" {
			start, end, err := resolveRealDiscoveryRange(cmd, store, codes)
			if err != nil {
				return err
			}
			snapshotID = fmt.Sprintf("discovery-%d", time.Now().UTC().UnixNano())
			snapshot := &paradigm.DatasetSnapshot{
				ID: snapshotID, Version: "discovery-v1", Universe: codes,
				DateRange: paradigm.DateRange{Start: start, End: end}, Market: "A",
				PriceAdjustment: paradigm.PriceRaw,
				Description:     "AI discovery auto-frozen real daily K-lines",
				CreatedAt:       time.Now().UTC(),
			}
			if err := paradigm.NewDatasetSnapshotStore(store).CreateKlineSnapshot(snapshot, 9); err != nil {
				return fmt.Errorf("freeze discovery snapshot: %w", err)
			}
		} else if err := paradigm.NewDatasetSnapshotStore(store).VerifyContent(snapshotID); err != nil {
			return fmt.Errorf("verify discovery snapshot: %w", err)
		}
		researcher, err := discovery.NewResearcher(discoveryrepo.New(store))
		if err != nil {
			return err
		}
		result, err := researcher.Run(cmd.Context(), discovery.Request{
			SnapshotID: snapshotID, StockCodes: codes, Question: discoverQuestion,
			HoldDays: discoverHoldDays, SearchBudget: discoverBudget,
		})
		if err != nil {
			return fmt.Errorf("discover patterns: %w", err)
		}
		if err := validateDiscoveredCandidates(cmd, store, result); err != nil {
			return err
		}
		result.ResultHash = result.ComputeHash()
		repo, err := discoveryrepo.NewTraceRepository(store)
		if err != nil {
			return err
		}
		if err := repo.Save(cmd.Context(), result); err != nil {
			return fmt.Errorf("persist research trace: %w", err)
		}
		return printValidationJSON(result)
	},
}

func validateDiscoveredCandidates(cmd *cobra.Command, store *storage.Storage, result *discovery.Result) error {
	evidenceRepo, err := validationrepo.NewEvidenceRepository(store)
	if err != nil {
		return err
	}
	for i := range result.Candidates {
		candidate := &result.Candidates[i]
		for _, handoff := range candidate.ValidationJobs {
			factory, err := validation.NewFactory(validation.FactoryDeps{
				Method: candidate.Method, Bars: validationrepo.New(store),
				Benchmark: validationrepo.NewBenchmark(store),
			})
			if err != nil {
				return err
			}
			bundle, runErr := factory.Run(cmd.Context(), validation.ValidationJob{
				MethodHash: handoff.MethodHash, MethodName: candidate.Method.Name,
				SnapshotID: handoff.SnapshotID, StockCode: handoff.StockCode,
				DateStart: handoff.DateStart, DateEnd: handoff.DateEnd,
				DiscoveryTrials: handoff.DiscoveryTrials,
			})
			if runErr != nil {
				candidate.ValidationEvidence = append(candidate.ValidationEvidence, discovery.ValidationEvidenceRef{
					StockCode: handoff.StockCode, Status: "failed", Error: runErr.Error(),
				})
				continue
			}
			if err := evidenceRepo.Save(cmd.Context(), bundle); err != nil {
				return fmt.Errorf("persist validation evidence for %s/%s: %w", candidate.TemplateID, handoff.StockCode, err)
			}
			candidate.ValidationEvidence = append(candidate.ValidationEvidence, discovery.ValidationEvidenceRef{
				StockCode: handoff.StockCode, Status: "completed", ResultHash: bundle.ResultHash,
				Confidence: string(bundle.Confidence), Passable: bundle.Passable,
			})
		}
	}
	return nil
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

func resolveRealDiscoveryRange(cmd *cobra.Command, store *storage.Storage, codes []string) (string, string, error) {
	commonStart, commonEnd := "", "99999999"
	for _, code := range codes {
		var minDate, maxDate string
		err := store.DB().QueryRowContext(cmd.Context(), `SELECT
			MIN(REPLACE(date, '-', '')), MAX(REPLACE(date, '-', '')) FROM kline
			WHERE code = ? AND ktype = 9 AND open > 0 AND high > 0 AND low > 0 AND close > 0
			AND volume > 0 AND length(REPLACE(date, '-', '')) = 8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'`, code).Scan(&minDate, &maxDate)
		if err != nil || minDate == "" || maxDate == "" {
			return "", "", fmt.Errorf("no valid real daily K-lines for %s", code)
		}
		if minDate > commonStart {
			commonStart = minDate
		}
		if maxDate < commonEnd {
			commonEnd = maxDate
		}
	}
	endTime, err := time.Parse("20060102", commonEnd)
	if err != nil {
		return "", "", err
	}
	defaultStart := endTime.AddDate(-4, 0, 0).Format("20060102")
	if defaultStart > commonStart {
		commonStart = defaultStart
	}
	if commonStart >= commonEnd {
		return "", "", fmt.Errorf("stocks do not share a sufficient real-data date range")
	}
	return formatCLIValidationDate(commonStart), formatCLIValidationDate(commonEnd), nil
}

func normalizeDiscoveryCodes(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				seen[part] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

func init() {
	discoverRunCmd.Flags().StringSliceVar(&discoverCodes, "code", nil, "股票代码，可重复或逗号分隔")
	discoverRunCmd.Flags().StringVar(&discoverSnapshot, "snapshot", "", "已冻结数据快照 ID (空=自动创建)")
	discoverRunCmd.Flags().StringVar(&discoverQuestion, "question", "", "可选研究问题")
	discoverRunCmd.Flags().IntVar(&discoverHoldDays, "hold-days", 0, "候选持有交易日 (0=自动 5)")
	discoverRunCmd.Flags().IntVar(&discoverBudget, "search-budget", 0, "搜索预算 (0=自动 24)")
	discoverCmd.AddCommand(discoverRunCmd, discoverShowCmd)
	rootCmd.AddCommand(discoverCmd)
}
