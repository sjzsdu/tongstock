package factorlab

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// 评估参考线（研究产出参考线，非方法库晋级门槛）。
const (
	minAbsTStat = 2.0
	minIC       = 0.03
	// staleWarningDays 是快照数据截止日距今超过多少自然日时必须告警：
	// 截面排序基于过时行情就没有预测意义（约 10 个交易日）。
	staleWarningDays = 14
)

// Options 是一轮因子研究的参数。
type Options struct {
	SnapshotID string
	// HorizonDays 是前向收益的持有期（交易日）。默认 5。
	HorizonDays int
	// TopK 是最后截面日产出的头部股票数。默认 30。
	TopK int
	// MaxCodes 限制参与研究的股票数（默认 300，与 methodautomation 对齐）。
	MaxCodes int
}

// Service 编排「解析快照 → 逐日截面 → 因子统计 → TopN 排序」。
type Service struct {
	deps methodautomation.Deps
	now  func() time.Time
}

// New 构造因子研究服务，依赖与 methodautomation 完全同形（快照 + 股票池 + K 线）。
func New(deps methodautomation.Deps) (*Service, error) {
	if deps.Snapshots == nil || deps.Universe == nil {
		return nil, fmt.Errorf("factorlab requires snapshot store and universe resolver")
	}
	if deps.Bars == nil {
		return nil, fmt.Errorf("factorlab requires a bar provider")
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{deps: deps, now: now}, nil
}

// Run 执行一轮横截面因子研究。
func (s *Service) Run(ctx context.Context, opts Options) (*RunResult, error) {
	startedAt := s.now()
	horizon := opts.HorizonDays
	if horizon <= 0 {
		horizon = 5
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = 30
	}

	snapshot, err := resolveSnapshot(s.deps.Snapshots, opts.SnapshotID)
	if err != nil {
		return nil, err
	}
	maxCodes := opts.MaxCodes
	if maxCodes <= 0 {
		maxCodes = 300
	}
	codes, _, err := s.deps.Universe.ResolveUniverse(ctx, snapshot.ID, 30, maxCodes)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot universe: %w", err)
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("snapshot %s has no code with enough real bars", snapshot.ID)
	}

	result := &RunResult{
		EngineVersion: EngineVersion,
		SnapshotID:    snapshot.ID,
		StartedAt:     startedAt,
		HorizonDays:   horizon,
		TopK:          topK,
		Codes:         len(codes),
	}

	// 每只股票一次性加载全区间 K 线（快照数据是不可变的，区间即快照日期范围），
	// 再在内存里切逐日截面：把 O(截面数 × 股票数) 次 LoadBars 压成 O(股票数)。
	barData := make([][]validation.BacktestBar, 0, len(codes))
	globalLast := ""
	for _, code := range codes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bars, err := s.deps.Bars.LoadBars(ctx, snapshot.ID, code, "", "")
		if err != nil {
			continue // 单票数据缺失如实跳过，不阻塞全市场研究
		}
		if len(bars) == 0 {
			continue
		}
		if last := bars[len(bars)-1].Date; last > globalLast {
			globalLast = last
		}
		barData = append(barData, bars)
	}
	if len(barData) == 0 {
		return nil, fmt.Errorf("snapshot %s has no loadable bars", snapshot.ID)
	}
	// 截面日期 = 全体代码 K 线日期的并集（快照里各代码日期基本一致，并集防御错位）。
	dateSet := map[string]bool{}
	for _, bars := range barData {
		for _, b := range bars {
			dateSet[b.Date] = true
		}
	}
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		return nil, fmt.Errorf("snapshot %s has no dated bars", snapshot.ID)
	}

	factors := BuiltinFactors()
	// 因子逐截面原始值（含最后截面日，排名用）+ RankIC 序列累计。
	rawByFactor := make([]map[string]map[string]float64, len(factors)) // factor -> date -> code -> value
	icsByFactor := make([][]float64, len(factors))
	pairsByFactor := make([]int, len(factors))
	coverageSum := make([]float64, len(factors))
	sectionsByFactor := make([]int, len(factors))
	for i := range rawByFactor {
		rawByFactor[i] = map[string]map[string]float64{}
		icsByFactor[i] = []float64{}
	}

	// 逐日截面：当日之前（含）的 K 线切片按日期单调推进，避免每截面全量重切。
	pos := make([]int, len(barData)) // 每只股票的「已含当日」切片右界（排他）
	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// 前向收益：该截面日期起第 horizon 个交易日的收益（按各股自己的日历）。
		fwd := map[string]float64{}
		factorValues := make([]map[string]float64, len(factors))
		for fi := range factors {
			factorValues[fi] = map[string]float64{}
		}
		for ci, bars := range barData {
			// 推进到「已含当日」：history 必须包含当日这根 K 线。
			for pos[ci] < len(bars) && bars[pos[ci]].Date <= date {
				pos[ci]++
			}
			history := bars[:pos[ci]]
			if len(history) == 0 {
				continue
			}
			// 前向收益需要「当日收盘 → 其后第 horizon 个交易日收盘」，当日必须在本股票日历里。
			if history[len(history)-1].Date != date {
				continue
			}
			if ret, ok := forwardReturnAt(bars, pos[ci]-1, horizon); ok {
				fwd[bars[pos[ci]-1].Code] = ret
			}
			for fi := range factors {
				if v, ok := factors[fi].Compute(history); ok {
					factorValues[fi][bars[pos[ci]-1].Code] = v
				}
			}
		}
		result.Sections++
		for fi := range factors {
			// 原始截面值无条件保存：最后截面日没有前向窗口，但排名必须用它。
			rawByFactor[fi][date] = factorValues[fi]
			if len(factorValues[fi]) < minCrossSection || len(fwd) < minCrossSection {
				continue
			}
			// 因子与前向收益的交集股票（仅用于 IC 统计）。
			joined := map[string]float64{}
			for code, v := range factorValues[fi] {
				if _, ok := fwd[code]; ok {
					joined[code] = v
				}
			}
			if len(joined) < minCrossSection {
				continue
			}
			ic, ok := rankIC(joined, fwd)
			if !ok {
				continue
			}
			icsByFactor[fi] = append(icsByFactor[fi], ic)
			pairsByFactor[fi] += len(joined)
			coverageSum[fi] += float64(len(joined)) / float64(len(fwd))
			sectionsByFactor[fi]++
		}
	}
	result.LastDate = dates[len(dates)-1]

	// 快照新鲜度：预测必须基于新鲜行情，过时快照的截面排序只是历史陈迹。
	// 这里如实报告数据截止日与距今天数，由前端醒目提示（不静默拒绝）。
	result.SnapshotDateEnd = snapshot.DateRange.End
	if result.SnapshotDateEnd == "" {
		result.SnapshotDateEnd = result.LastDate
	}
	if end, perr := time.Parse("2006-01-02", result.SnapshotDateEnd); perr == nil {
		result.StaleDays = int(s.now().Sub(end).Hours() / 24)
		if result.StaleDays < 0 {
			result.StaleDays = 0
		}
	}

	for fi, f := range factors {
		ics := icsByFactor[fi]
		ev := FactorEval{Key: f.Key, Name: f.Name, Description: f.Description, Prior: f.Prior, Sections: sectionsByFactor[fi], Pairs: pairsByFactor[fi]}
		if coverageSum[fi] > 0 && sectionsByFactor[fi] > 0 {
			ev.Coverage = coverageSum[fi] / float64(sectionsByFactor[fi])
		}
		if len(ics) > 0 {
			ev.MeanIC = mean(ics)
			// 前向窗口重叠 → IC 自相关：用步长 = horizon 的不重叠子序列
			// 计算波动与 t 统计，不把 970 个高度相关截面当成 970 个独立样本。
			independent := deOverlap(ics, horizon)
			ev.EffectiveSections = len(independent)
			ev.ICStd = std(independent)
			if ev.ICStd > 0 {
				ev.ICIR = ev.MeanIC / ev.ICStd
				ev.TStat = ev.ICIR * math.Sqrt(float64(len(independent)))
			}
			// 显著性看预测力大小（|IC|），方向由数据决定（sign(MeanIC)），
			// 与先验相反就如实反向 —— A股短周期普遍是反转而非动量。
			ev.Significant = math.Abs(ev.TStat) >= minAbsTStat && math.Abs(ev.MeanIC) >= minIC
			if ev.Significant {
				if ev.MeanIC > 0 {
					ev.Direction = 1
				} else {
					ev.Direction = -1
				}
			}
		}
		result.Factors = append(result.Factors, ev)
	}
	// 按因子 Key 建评估索引：result.Factors 排序后下标不再与 factors 对齐。
	evalByKey := map[string]FactorEval{}
	for _, ev := range result.Factors {
		evalByKey[ev.Key] = ev
	}
	sort.SliceStable(result.Factors, func(a, b int) bool {
		x, y := result.Factors[a], result.Factors[b]
		if x.Significant != y.Significant {
			return x.Significant
		}
		return math.Abs(x.TStat) > math.Abs(y.TStat)
	})

	// 组合分：显著因子截面 z 分的 IC 加权和（经典 IC-weighting：
	// |MeanIC| 越大权重越高，方向用数据方向），最后截面日 TopK。
	lastDate := dates[len(dates)-1]
	weights := map[string]float64{}
	directions := map[string]float64{}
	totalWeight := 0.0
	for _, f := range factors {
		ev, ok := evalByKey[f.Key]
		if !ok || !ev.Significant {
			continue
		}
		weights[f.Key] = math.Abs(ev.MeanIC)
		directions[f.Key] = ev.Direction
		totalWeight += math.Abs(ev.MeanIC)
	}
	if totalWeight > 0 {
		score := map[string]float64{}
		contrib := map[string]map[string]float64{}
		for fi, f := range factors {
			if _, ok := weights[f.Key]; !ok {
				continue
			}
			vals := rawByFactor[fi][lastDate]
			if len(vals) == 0 {
				continue
			}
			zs, ok := zscores(vals)
			if !ok {
				continue
			}
			for code, z := range zs {
				contribution := directions[f.Key] * weights[f.Key] * z / totalWeight
				score[code] += contribution
				if contrib[code] == nil {
					contrib[code] = map[string]float64{}
				}
				contrib[code][f.Key] = contribution
			}
		}
		for code, sc := range score {
			result.TopPicks = append(result.TopPicks, TopPick{Code: code, Score: sc, Contributions: contrib[code]})
		}
		sort.Slice(result.TopPicks, func(a, b int) bool {
			if result.TopPicks[a].Score == result.TopPicks[b].Score {
				return result.TopPicks[a].Code < result.TopPicks[b].Code
			}
			return result.TopPicks[a].Score > result.TopPicks[b].Score
		})
		if len(result.TopPicks) > topK {
			result.TopPicks = result.TopPicks[:topK]
		}
	}
	result.Note = summarize(result, totalWeight > 0)
	result.FinishedAt = s.now()
	return result, nil
}

// ToPickRun 把一轮研究结果转换成因子通道的可持久化产出。
// topK<=0 表示全量采用 res.TopPicks。
// 与 Run 的语义分工：Run 在无显著因子时返回**空结果 + 诚实文案**（无预测力
// 是如实报告，不是故障）；本函数只在持久化层面拒绝——没有名单就没有可落库
// 的产出，诚实原则不允许空名单占位，调用方据此跳过落库而非报错。
func (r *RunResult) ToPickRun(topK int) (*PickRun, error) {
	if r == nil || strings.TrimSpace(r.LastDate) == "" {
		return nil, fmt.Errorf("factor research result has no cross-section date")
	}
	if len(r.TopPicks) == 0 {
		return nil, fmt.Errorf("no significant factors; %s", r.Note)
	}
	picks := r.TopPicks
	if topK > 0 && topK < len(picks) {
		picks = picks[:topK]
	}
	created := r.StartedAt.UnixMilli()
	return &PickRun{
		RunID:           "pick-" + r.LastDate,
		SnapshotID:      r.SnapshotID,
		SnapshotDateEnd: r.SnapshotDateEnd,
		AsOf:            r.LastDate,
		StaleDays:       r.StaleDays,
		FactorsSnapshot: r.Factors,
		Note:            r.Note,
		Picks:           append([]TopPick{}, picks...),
		CreatedAt:       created,
		UpdatedAt:       r.FinishedAt.UnixMilli(),
	}, nil
}

// deOverlap 按步长 stride 抽取不重叠的独立子序列：前向窗口长 h 时，相邻
// 截面日的前向窗口重叠 h-1 天，IC 序列自相关。个别截面无 IC（稀疏跳过）时
// 索引步长近似等于日期步长，这是可接受的近似（如实记录在字段注释中）。
func deOverlap(ics []float64, stride int) []float64 {
	if stride <= 1 {
		return ics
	}
	out := make([]float64, 0, len(ics)/stride+1)
	for i := 0; i < len(ics); i += stride {
		out = append(out, ics[i])
	}
	return out
}

// forwardReturnAt 返回 bars[todayIdx]（当日）收盘到其后第 horizon 个交易日收盘的收益。
// 前向窗口越出数据末端时返回 false（最近的 horizon 个截面日因此没有标签，自然不计入）。
func forwardReturnAt(bars []validation.BacktestBar, todayIdx, horizon int) (float64, bool) {
	if todayIdx < 0 || todayIdx+horizon >= len(bars) {
		return 0, false
	}
	base, fwd := bars[todayIdx].Close, bars[todayIdx+horizon].Close
	if base <= 0 || fwd <= 0 {
		return 0, false
	}
	return fwd/base - 1, true
}

func summarize(r *RunResult, hasPicks bool) string {
	sig := 0
	flipped := 0
	for _, f := range r.Factors {
		if f.Significant {
			sig++
			if f.Prior != 0 && f.Direction != f.Prior {
				flipped++
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "在 %s 冻结快照 %d 只股票、%d 个截面日上评估了 %d 个因子，持有期 %d 个交易日：", r.SnapshotID, r.Codes, r.Sections, len(r.Factors), r.HorizonDays)
	if sig == 0 {
		b.WriteString("没有因子达到统计显著参考线（|t|≥2 且 |IC|≥0.03，t 用去重叠独立截面计算），说明当前数据截面上的短线预测力有限，需谨慎对待任何排序产出。")
	} else {
		fmt.Fprintf(&b, "%d 个因子达到显著参考线：", sig)
		names := make([]string, 0, sig)
		for _, f := range r.Factors {
			if f.Significant {
				dir := "值大→领涨"
				if f.Direction < 0 {
					dir = "值小→领涨"
				}
				names = append(names, fmt.Sprintf("%s（IC=%.3f, t=%.1f, %s）", f.Name, f.MeanIC, f.TStat, dir))
			}
		}
		b.WriteString(strings.Join(names, "、"))
		b.WriteString("。因子方向由数据决定")
		if flipped > 0 {
			fmt.Fprintf(&b, "，其中 %d 个与先验相反（如 A股短周期反转），如实反向计入组合", flipped)
		}
		b.WriteString("。")
	}
	if hasPicks && len(r.TopPicks) > 0 {
		fmt.Fprintf(&b, "已按显著因子组合分（|IC| 加权）输出 %s 截面日 Top %d 名单（分数构成随结果透出）。", r.LastDate, len(r.TopPicks))
	} else {
		b.WriteString("因没有显著因子，未输出 Top 名单。")
	}
	if r.StaleDays > staleWarningDays {
		fmt.Fprintf(&b, " ⚠ 快照数据止于 %s（距今 %d 天）：截面排序基于过时行情，预测已不可信，请先更新数据并冻结新快照。", r.SnapshotDateEnd, r.StaleDays)
	}
	return b.String()
}

// resolveSnapshot 与 methodautomation 的选择规则一致：显式 ID 优先，
// 否则分页扫描取最新的「股票池足以过门槛」且内容校验通过的快照。
func resolveSnapshot(store SnapshotStore, snapshotID string) (*paradigm.DatasetSnapshot, error) {
	if id := strings.TrimSpace(snapshotID); id != "" {
		snapshot, err := store.GetByID(id)
		if err != nil {
			return nil, fmt.Errorf("load frozen snapshot %s: %w", id, err)
		}
		if err := store.VerifyContent(id); err != nil {
			return nil, fmt.Errorf("verify frozen snapshot %s: %w", id, err)
		}
		return snapshot, nil
	}
	const (
		scanPage = 50
		scanMax  = 500
	)
	for offset := 0; offset < scanMax; offset += scanPage {
		snaps, err := store.List(scanPage, offset)
		if err != nil {
			return nil, fmt.Errorf("list frozen snapshots: %w", err)
		}
		for _, snap := range snaps {
			if snap == nil || len(snap.Universe) < 5 {
				continue
			}
			if err := store.VerifyContent(snap.ID); err != nil {
				continue
			}
			return snap, nil
		}
		if len(snaps) < scanPage {
			break
		}
	}
	return nil, fmt.Errorf("no usable frozen dataset snapshot; run a multi-stock AI research or freeze any multi-code snapshot first")
}
