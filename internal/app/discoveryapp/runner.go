// Package discoveryapp 提供规律发现的应用服务：冻结快照 → 模式发现 →
// 样本外验证 → 持久化研究轨迹。CLI 与 HTTP 共用同一入口，不允许在
// 传输层重新实现该流程。
package discoveryapp

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// PoolResolver 把股票池 ID 解析为真实股票代码列表。
type PoolResolver interface {
	Resolve(ctx context.Context, poolID string) ([]string, error)
}

// KlineSyncer 把缺失的日 K 数据从上游同步到本地库。
type KlineSyncer interface {
	SyncDailyKlines(codes []string, mode tdx.SyncMode, concurrency int) tdx.KlineBatchSyncResult
}

// Runner 是规律发现的统一应用服务入口。
type Runner struct {
	store        *storage.Storage
	poolResolver PoolResolver // 可为 nil，此时 PoolID 不可用
	sync         KlineSyncer  // 可为 nil，此时缺数据直接报错而非自动同步
}

// NewRunner 创建 Runner；poolResolver 与 sync 均可为 nil。
func NewRunner(store *storage.Storage, poolResolver PoolResolver, syncs ...KlineSyncer) *Runner {
	r := &Runner{store: store, poolResolver: poolResolver}
	if len(syncs) > 0 {
		r.sync = syncs[0]
	}
	return r
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

	// 本地缺数据的代码自动从上游同步，避免直接报错（fail-closed 由 resolveRealRange 兜底）。
	if r.sync != nil {
		missing, err := r.missingCodes(ctx, codes)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			r.sync.SyncDailyKlines(missing, tdx.SyncModeAuto, 3)
		}
	}

	snapshotID := strings.TrimSpace(req.SnapshotID)
	researchCodes := codes
	if snapshotID == "" {
		start, end, used, err := resolveRealRange(ctx, r.store, codes)
		if err != nil {
			return nil, err
		}
		researchCodes = used
		snapshotID = fmt.Sprintf("discovery-%d", time.Now().UTC().UnixNano())
		snapshot := &paradigm.DatasetSnapshot{
			ID: snapshotID, Version: "discovery-v1", Universe: used,
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
		SnapshotID: snapshotID, StockCodes: researchCodes, Question: req.Question,
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

// missingCodes 返回本地缺少有效真实日 K 数据的代码（ktype=9 且 OHLCV 均有效）。
func (r *Runner) missingCodes(ctx context.Context, codes []string) ([]string, error) {
	var missing []string
	for _, code := range codes {
		var n int
		err := r.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM kline
			WHERE code = ? AND ktype = 9 AND open > 0 AND high > 0 AND low > 0 AND close > 0
			AND volume > 0`, code).Scan(&n)
		if err != nil {
			return nil, fmt.Errorf("check local klines for %s: %w", code, err)
		}
		if n == 0 {
			missing = append(missing, code)
		}
	}
	return missing, nil
}

// codeSpan 描述一只股票的有效日 K 日期跨度。
type codeSpan struct {
	code string
	min  string // 20060102
	max  string
}

// resolveRealRange 计算多只股票共享的真实日 K 公共区间，并限制在最近 4 年内。
// 无有效数据的股票被跳过；若共享区间不足（存在次新股/退市股拖后腿），
// 则按历史跨度从短到长迭代剔除，直到剩余股票拥有足够长的公共区间。
// 返回区间与最终参与研究的代码列表。
func resolveRealRange(ctx context.Context, store *storage.Storage, codes []string) (string, string, []string, error) {
	type span struct {
		code string
		min  string // 20060102
		max  string
	}
	spans := make([]codeSpan, 0, len(codes))
	for _, code := range codes {
		var minDate, maxDate string
		err := store.DB().QueryRowContext(ctx, `SELECT
			MIN(REPLACE(date, '-', '')), MAX(REPLACE(date, '-', '')) FROM kline
			WHERE code = ? AND ktype = 9 AND open > 0 AND high > 0 AND low > 0 AND close > 0
			AND volume > 0 AND length(REPLACE(date, '-', '')) = 8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'`, code).Scan(&minDate, &maxDate)
		if err != nil || minDate == "" || maxDate == "" {
			log.Printf("discover: 跳过无有效真实日K的股票 %s", code)
			continue
		}
		spans = append(spans, codeSpan{code: code, min: minDate, max: maxDate})
	}
	if len(spans) == 0 {
		return "", "", nil, fmt.Errorf("no stock has valid real daily K-lines")
	}

	// 最小共享区间：约 300 个自然日，保证研究至少有一定样本（单股票例外，允许短历史）。
	const minSharedDays = 300
	for {
		commonStart, commonEnd := "", "99999999"
		for _, sp := range spans {
			if sp.min > commonStart {
				commonStart = sp.min
			}
			if sp.max < commonEnd {
				commonEnd = sp.max
			}
		}
		endTime, err := time.Parse("20060102", commonEnd)
		if err != nil {
			return "", "", nil, err
		}
		defaultStart := endTime.AddDate(-4, 0, 0).Format("20060102")
		if defaultStart > commonStart {
			commonStart = defaultStart
		}
		if commonStart < commonEnd && (len(spans) == 1 || daysBetween(commonStart, commonEnd) >= minSharedDays) {
			used := make([]string, 0, len(spans))
			for _, sp := range spans {
				used = append(used, sp.code)
			}
			slices.Sort(used)
			return formatRangeDate(commonStart), formatRangeDate(commonEnd), used, nil
		}
		if len(spans) <= 1 {
			return "", "", nil, fmt.Errorf("stocks do not share a sufficient real-data date range")
		}
		// 剔除历史跨度最短的股票（次新/退市/停牌最可能拖后腿）。
		slices.SortFunc(spans, func(a, b codeSpan) int { return cmpSpanDays(a, b) })
		log.Printf("discover: 排除历史不足的股票 %s (最早 %s)", spans[0].code, spans[0].min)
		spans = spans[1:]
	}
}

func daysBetween(a, b string) int {
	ta, errA := time.Parse("20060102", a)
	tb, errB := time.Parse("20060102", b)
	if errA != nil || errB != nil {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

func cmpSpanDays(a, b codeSpan) int {
	da := daysBetween(a.min, a.max)
	db := daysBetween(b.min, b.max)
	if da < db {
		return -1
	}
	if da > db {
		return 1
	}
	return 0
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
