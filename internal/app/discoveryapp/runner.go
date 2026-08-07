// Package discoveryapp 提供规律发现的应用服务：冻结快照 → 模式发现 →
// 样本外验证 → 持久化研究轨迹。CLI 与 HTTP 共用同一入口，不允许在
// 传输层重新实现该流程。
package discoveryapp

import (
	"context"
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
)

// PoolResolver 把股票池 ID 解析为真实股票代码列表。
type PoolResolver interface {
	Resolve(ctx context.Context, poolID string) ([]string, error)
}

// Runner 是规律发现的统一应用服务入口。
type Runner struct {
	store        *storage.Storage
	poolResolver PoolResolver // 可为 nil，此时 PoolID 不可用
}

// NewRunner 创建 Runner。
func NewRunner(store *storage.Storage, poolResolver PoolResolver) *Runner {
	return &Runner{store: store, poolResolver: poolResolver}
}

// RunRequest 描述一次发现研究请求。
type RunRequest struct {
	Codes        []string // 显式股票代码
	PoolID       string   // 股票池 ID（与 Codes 可同时使用，取并集）
	SnapshotID   string   // 已冻结快照 ID（空=自动冻结）
	Question     string
	HoldDays     int
	SearchBudget int
}

// Run 执行完整发现流程并持久化研究轨迹。
func (r *Runner) Run(ctx context.Context, req RunRequest) (*discovery.Result, error) {
	codes := NormalizeCodes(req.Codes)
	if req.PoolID != "" {
		if r.poolResolver == nil {
			return nil, fmt.Errorf("pool resolver unavailable")
		}
		poolCodes, err := r.poolResolver.Resolve(ctx, req.PoolID)
		if err != nil {
			return nil, fmt.Errorf("resolve pool %s: %w", req.PoolID, err)
		}
		codes = NormalizeCodes(append(codes, poolCodes...))
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("no stock codes provided (use --code or --pool)")
	}

	snapshotID := strings.TrimSpace(req.SnapshotID)
	if snapshotID == "" {
		start, end, err := resolveRealRange(ctx, r.store, codes)
		if err != nil {
			return nil, err
		}
		snapshotID = fmt.Sprintf("discovery-%d", time.Now().UTC().UnixNano())
		snapshot := &paradigm.DatasetSnapshot{
			ID: snapshotID, Version: "discovery-v1", Universe: codes,
			DateRange: paradigm.DateRange{Start: start, End: end}, Market: "A",
			PriceAdjustment: paradigm.PriceRaw,
			Description:     "AI discovery auto-frozen real daily K-lines",
			CreatedAt:       time.Now().UTC(),
		}
		if err := paradigm.NewDatasetSnapshotStore(r.store).CreateKlineSnapshot(snapshot, 9); err != nil {
			return nil, fmt.Errorf("freeze discovery snapshot: %w", err)
		}
	} else if err := paradigm.NewDatasetSnapshotStore(r.store).VerifyContent(snapshotID); err != nil {
		return nil, fmt.Errorf("verify discovery snapshot: %w", err)
	}

	researcher, err := discovery.NewResearcher(discoveryrepo.New(r.store))
	if err != nil {
		return nil, err
	}
	result, err := researcher.Run(ctx, discovery.Request{
		SnapshotID: snapshotID, StockCodes: codes, Question: req.Question,
		HoldDays: req.HoldDays, SearchBudget: req.SearchBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("discover patterns: %w", err)
	}

	if err := validateCandidates(ctx, r.store, result); err != nil {
		return nil, err
	}

	result.ResultHash = result.ComputeHash()
	repo, err := discoveryrepo.NewTraceRepository(r.store)
	if err != nil {
		return nil, err
	}
	if err := repo.Save(ctx, result); err != nil {
		return nil, fmt.Errorf("persist research trace: %w", err)
	}
	return result, nil
}

// NormalizeCodes 规范化代码输入：按逗号切分、去空白、去重、排序。
func NormalizeCodes(values []string) []string {
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

// resolveRealRange 计算多只股票共享的真实日 K 公共区间，并限制在最近 4 年内。
func resolveRealRange(ctx context.Context, store *storage.Storage, codes []string) (string, string, error) {
	commonStart, commonEnd := "", "99999999"
	for _, code := range codes {
		var minDate, maxDate string
		err := store.DB().QueryRowContext(ctx, `SELECT
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
	return formatRangeDate(commonStart), formatRangeDate(commonEnd), nil
}

func formatRangeDate(value string) string {
	if len(value) != 8 {
		return value
	}
	return value[:4] + "-" + value[4:6] + "-" + value[6:]
}

// validateCandidates 在保留样本上验证每个候选方法并持久化证据。
func validateCandidates(ctx context.Context, store *storage.Storage, result *discovery.Result) error {
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
			bundle, runErr := factory.Run(ctx, validation.ValidationJob{
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
			if err := evidenceRepo.Save(ctx, bundle); err != nil {
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
