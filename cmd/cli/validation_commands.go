package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

// validateCmd 暴露验证工厂能力：把已编译方法在真实数据上自动回测、统计检验、critic 反证，
// 输出统一 EvidenceBundle。
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "验证工厂: 真实数据自动回测 + 统计检验 + critic 反证",
	Long: `验证工厂 (Validation Factory) 接收已编译方法 + 股票代码 + 日期范围，
在真实 K 线上执行确定性回测、walk-forward 切分、多重检验惩罚和 critic 反证，
输出 EvidenceBundle (置信等级 + promotion blockers + 结果哈希)。

流程：method compile > compiled.json && tongstock validate run --method compiled.json --code 600519
系统会根据真实库内最新交易日自动选取最近两年，并在回测前冻结不可变快照。`,
}

var (
	valMethodFile string
	valCode       string
	valStart      string
	valEnd        string
	valSnapshot   string
	valSplit      string
	valTrials     int
	valBenchmark  string
	valCash       float64
	valOutput     string
	valListMethod string
	valListLimit  int
)

var validateRunCmd = &cobra.Command{
	Use:   "run",
	Short: "执行验证流水线并输出 EvidenceBundle JSON",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if valMethodFile == "" {
			return fmt.Errorf("--method is required (compiled method JSON)")
		}
		if valCode == "" {
			return fmt.Errorf("--code is required")
		}
		// 1. 加载已编译方法
		raw, err := os.ReadFile(valMethodFile)
		if err != nil {
			return fmt.Errorf("read method: %w", err)
		}
		m, err := decodeCompiledMethod(raw)
		if err != nil {
			return fmt.Errorf("parse compiled method: %w", err)
		}
		if !m.IsExecutable() {
			return fmt.Errorf("method %q is not executable (has errors or ambiguities)", m.Name)
		}

		// 2. 装配依赖
		cfg, err := config.Load()
		if err != nil {
			cfg = config.DefaultConfig()
		}
		db, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer db.Close()
		if err := db.Migrate(); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}

		// 3. 自动选择真实数据范围并冻结。验证器后续不再读可变 kline 表。
		snapshotStore := paradigm.NewDatasetSnapshotStore(db)
		dateStart, dateEnd := valStart, valEnd
		snapshotID := valSnapshot
		if snapshotID == "" {
			dateStart, dateEnd, err = resolveRealValidationRange(cmd.Context(), db, valCode, dateStart, dateEnd)
			if err != nil {
				return err
			}
			universe := []string{valCode}
			if valBenchmark != "" && valBenchmark != valCode {
				universe = append(universe, valBenchmark)
			}
			snapshotID = fmt.Sprintf("validation-%s-%d", strings.ToLower(valCode), time.Now().UTC().UnixNano())
			snapshot := &paradigm.DatasetSnapshot{
				ID: snapshotID, Version: "validation-v1", Universe: universe,
				DateRange: paradigm.DateRange{Start: dateStart, End: dateEnd},
				Market:    "A", PriceAdjustment: paradigm.PriceRaw,
				Description: "validation factory auto-frozen real daily K-lines",
				CreatedAt:   time.Now().UTC(),
			}
			if err := snapshotStore.CreateKlineSnapshot(snapshot, validationrepo.KlineTypeDaily); err != nil {
				return fmt.Errorf("freeze real validation snapshot: %w", err)
			}
		} else {
			snapshot, err := snapshotStore.GetByID(snapshotID)
			if err != nil {
				return fmt.Errorf("load frozen snapshot %s: %w", snapshotID, err)
			}
			if err := snapshotStore.VerifyContent(snapshotID); err != nil {
				return fmt.Errorf("verify frozen snapshot %s: %w", snapshotID, err)
			}
			if dateStart == "" {
				dateStart = snapshot.DateRange.Start
			}
			if dateEnd == "" {
				dateEnd = snapshot.DateRange.End
			}
		}

		barProvider := validationrepo.New(db)
		benchProvider := validationrepo.NewBenchmark(db)

		factory, err := validation.NewFactory(validation.FactoryDeps{
			Method:    m,
			Bars:      barProvider,
			Benchmark: benchProvider,
		})
		if err != nil {
			return err
		}

		// 4. 构造 ValidationJob
		job := validation.ValidationJob{
			MethodHash:      m.ContentHash,
			MethodName:      m.Name,
			SnapshotID:      snapshotID,
			StockCode:       valCode,
			DateStart:       dateStart,
			DateEnd:         dateEnd,
			SplitType:       valSplit,
			DiscoveryTrials: valTrials,
			BenchmarkCode:   valBenchmark,
			InitialCash:     valCash,
		}

		// 5. 执行
		bundle, err := factory.Run(cmd.Context(), job)
		if err != nil {
			return fmt.Errorf("validation failed (fail closed): %w", err)
		}
		evidenceRepo, err := validationrepo.NewEvidenceRepository(db)
		if err != nil {
			return err
		}
		if err := evidenceRepo.Save(cmd.Context(), bundle); err != nil {
			return fmt.Errorf("persist evidence artifact: %w", err)
		}

		// 6. 输出/制品
		out, _ := json.MarshalIndent(bundle, "", "  ")
		if valOutput != "" {
			if err := os.MkdirAll(filepath.Dir(valOutput), 0o755); err != nil && filepath.Dir(valOutput) != "." {
				return fmt.Errorf("create output directory: %w", err)
			}
			if err := os.WriteFile(valOutput, append(out, '\n'), 0o644); err != nil {
				return fmt.Errorf("write evidence bundle: %w", err)
			}
		}
		fmt.Println(string(out))
		return nil
	},
}

var validateEvidenceCmd = &cobra.Command{
	Use:   "evidence <result-hash>",
	Short: "按结果哈希查询已持久化 EvidenceBundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openValidationStorage()
		if err != nil {
			return err
		}
		defer store.Close()
		repo, err := validationrepo.NewEvidenceRepository(store)
		if err != nil {
			return err
		}
		bundle, err := repo.Get(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("get validation evidence: %w", err)
		}
		return printValidationJSON(bundle)
	},
}

var validateListCmd = &cobra.Command{
	Use:   "list",
	Short: "按方法哈希列出可引用的验证制品",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if valListMethod == "" {
			return fmt.Errorf("--method-hash is required")
		}
		store, err := openValidationStorage()
		if err != nil {
			return err
		}
		defer store.Close()
		repo, err := validationrepo.NewEvidenceRepository(store)
		if err != nil {
			return err
		}
		bundles, err := repo.ListByMethod(cmd.Context(), valListMethod, valListLimit)
		if err != nil {
			return fmt.Errorf("list validation evidence: %w", err)
		}
		return printValidationJSON(bundles)
	},
}

func init() {
	validateRunCmd.Flags().StringVar(&valMethodFile, "method", "", "已编译方法 JSON 文件路径")
	validateRunCmd.Flags().StringVar(&valCode, "code", "", "股票代码 (如 600519)")
	validateRunCmd.Flags().StringVar(&valStart, "start", "", "起始日期 YYYY-MM-DD (空=快照/最近2年)")
	validateRunCmd.Flags().StringVar(&valEnd, "end", "", "截止日期 YYYY-MM-DD")
	validateRunCmd.Flags().StringVar(&valSnapshot, "snapshot", "", "已冻结数据快照 ID (空=自动从真实行情创建)")
	validateRunCmd.Flags().StringVar(&valSplit, "split", "", "切分类型: fixed / walk_forward (空=walk_forward)")
	validateRunCmd.Flags().IntVar(&valTrials, "trials", 1, "发现阶段尝试次数 (用于 Bonferroni 多重检验惩罚)")
	validateRunCmd.Flags().StringVar(&valBenchmark, "benchmark", "", "基准代码 (空=同标的买入持有)")
	validateRunCmd.Flags().Float64Var(&valCash, "cash", 1_000_000, "初始资金")
	validateRunCmd.Flags().StringVar(&valOutput, "output", "", "EvidenceBundle JSON 制品路径 (可选)")
	validateListCmd.Flags().StringVar(&valListMethod, "method-hash", "", "已编译方法内容哈希")
	validateListCmd.Flags().IntVar(&valListLimit, "limit", 20, "最多返回制品数 (1-100)")

	validateCmd.AddCommand(validateRunCmd, validateEvidenceCmd, validateListCmd)
	rootCmd.AddCommand(validateCmd)
}

func openValidationStorage() (*storage.Storage, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return store, nil
}

func printValidationJSON(value any) error {
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func decodeCompiledMethod(raw []byte) (*methods.CompiledMethod, error) {
	var wrapped struct {
		Method *methods.CompiledMethod `json:"method"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Method != nil {
		return wrapped.Method, nil
	}
	method := &methods.CompiledMethod{}
	if err := json.Unmarshal(raw, method); err != nil {
		return nil, err
	}
	return method, nil
}

func resolveRealValidationRange(ctx context.Context, store *storage.Storage, code, requestedStart, requestedEnd string) (string, string, error) {
	if code == "" {
		return "", "", fmt.Errorf("code is required")
	}
	var minDate, maxDate string
	err := store.DB().QueryRowContext(ctx, `SELECT
		MIN(REPLACE(date, '-', '')), MAX(REPLACE(date, '-', ''))
		FROM kline WHERE code = ? AND ktype = ?
		AND open > 0 AND high > 0 AND low > 0 AND close > 0
		AND length(REPLACE(date, '-', '')) = 8
		AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'`,
		code, validationrepo.KlineTypeDaily).Scan(&minDate, &maxDate)
	if err != nil || minDate == "" || maxDate == "" {
		return "", "", fmt.Errorf("no valid real daily K-lines for %s", code)
	}
	end := normalizeCLIValidationDate(requestedEnd)
	if end == "" || end > maxDate {
		end = maxDate
	}
	start := normalizeCLIValidationDate(requestedStart)
	if start == "" {
		endTime, err := time.Parse("20060102", end)
		if err != nil {
			return "", "", fmt.Errorf("invalid latest real date %q: %w", end, err)
		}
		start = endTime.AddDate(-2, 0, 0).Format("20060102")
		if start < minDate {
			start = minDate
		}
	}
	if start > end {
		return "", "", fmt.Errorf("validation start %s is after end %s", start, end)
	}
	return formatCLIValidationDate(start), formatCLIValidationDate(end), nil
}

func normalizeCLIValidationDate(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "")
}

func formatCLIValidationDate(value string) string {
	if len(value) != 8 {
		return value
	}
	return value[:4] + "-" + value[4:6] + "-" + value[6:]
}
