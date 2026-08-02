package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

type Researcher struct {
	bars BarProvider
}

func NewResearcher(bars BarProvider) (*Researcher, error) {
	if bars == nil {
		return nil, fmt.Errorf("bar provider is required")
	}
	return &Researcher{bars: bars}, nil
}

type patternTemplate struct {
	id        string
	name      string
	entry     *methods.Expr
	rationale string
}

type patternStats struct {
	returns []float64
	base    []float64
}

func (r *Researcher) Run(ctx context.Context, request Request) (*Result, error) {
	if err := request.Normalize(); err != nil {
		return nil, err
	}
	request.StockCodes = sortedUnique(request.StockCodes)
	if len(request.StockCodes) == 0 {
		return nil, fmt.Errorf("stock codes are empty after normalization")
	}
	templates := candidateTemplates(request.HoldDays)
	if request.SearchBudget < len(templates) {
		templates = templates[:request.SearchBudget]
	}
	stats := make(map[string]*patternStats, len(templates))
	for _, template := range templates {
		stats[template.id] = &patternStats{}
	}

	result := &Result{
		ResearchID: researchID(request), SnapshotID: request.SnapshotID,
		GeneratorVersion: GeneratorVersion, GeneratedAt: time.Now().UTC(),
		Question: request.Question, HoldDays: request.HoldDays, SearchBudget: request.SearchBudget,
	}
	for _, code := range request.StockCodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bars, err := r.bars.Load(ctx, request.SnapshotID, code)
		if err != nil {
			return nil, fmt.Errorf("load frozen bars for %s: %w", code, err)
		}
		boundary, discoveryBars, err := reserveUntouchedTail(code, bars, request.HoldDays)
		if err != nil {
			return nil, err
		}
		result.Boundaries = append(result.Boundaries, boundary)
		baseline := forwardReturns(discoveryBars, request.HoldDays)
		for _, template := range templates {
			method, _, err := methods.Compile(templateCandidate(template, request.HoldDays))
			if err != nil || !method.IsExecutable() {
				return nil, fmt.Errorf("generator template %s is not executable", template.id)
			}
			matched, err := matchedForwardReturns(method, discoveryBars, request.HoldDays)
			if err != nil {
				return nil, fmt.Errorf("evaluate %s on %s: %w", template.id, code, err)
			}
			stats[template.id].returns = append(stats[template.id].returns, matched...)
			stats[template.id].base = append(stats[template.id].base, baseline...)
		}
	}

	minObservations := maxInt(12, len(request.StockCodes)*5)
	for _, template := range templates {
		result.DiscoveryTrials++
		stat := stats[template.id]
		mean, std := meanStd(stat.returns)
		baselineMean, _ := meanStd(stat.base)
		winRate := positiveRate(stat.returns)
		tStat := 0.0
		if len(stat.returns) > 1 && std > 0 {
			tStat = mean / (std / math.Sqrt(float64(len(stat.returns))))
		}
		if len(stat.returns) < minObservations {
			result.Rejected = append(result.Rejected, RejectedCandidate{
				TemplateID: template.id, Reason: fmt.Sprintf("observations %d < required %d", len(stat.returns), minObservations),
				Observations: len(stat.returns),
			})
			continue
		}
		if mean <= 0 || mean <= baselineMean {
			result.Rejected = append(result.Rejected, RejectedCandidate{
				TemplateID: template.id, Reason: "discovery return is non-positive or does not exceed unconditional baseline",
				Observations: len(stat.returns),
			})
			continue
		}
		method, _, err := methods.Compile(templateCandidate(template, request.HoldDays))
		if err != nil || !method.IsExecutable() {
			return nil, fmt.Errorf("compile accepted template %s: %w", template.id, err)
		}
		result.Candidates = append(result.Candidates, CandidateEvidence{
			TemplateID: template.id, Method: method, Observations: len(stat.returns),
			MeanForwardReturn: mean, WinRate: winRate, BaselineReturn: baselineMean,
			Lift: mean - baselineMean, TStatistic: tStat, Rationale: template.rationale,
			Source: fmt.Sprintf("frozen_snapshot:%s; generator:%s; feature_at=t; label=t+1..t+%d",
				request.SnapshotID, GeneratorVersion, request.HoldDays+1),
		})
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		if result.Candidates[i].TStatistic == result.Candidates[j].TStatistic {
			return result.Candidates[i].Lift > result.Candidates[j].Lift
		}
		return result.Candidates[i].TStatistic > result.Candidates[j].TStatistic
	})
	for i := range result.Candidates {
		result.Candidates[i].Rank = i + 1
		for _, boundary := range result.Boundaries {
			result.Candidates[i].ValidationJobs = append(result.Candidates[i].ValidationJobs, ValidationHandoff{
				MethodHash: result.Candidates[i].Method.ContentHash, SnapshotID: request.SnapshotID,
				StockCode: boundary.Code, DateStart: boundary.ReservedStartDate, DateEnd: boundary.LastDate,
				DiscoveryTrials: result.DiscoveryTrials,
			})
		}
	}
	if len(result.Candidates) == 0 {
		result.Conclusion = "insufficient_evidence"
	} else {
		result.Conclusion = "ranked_hypotheses"
	}
	result.ResultHash = result.ComputeHash()
	return result, nil
}

func reserveUntouchedTail(code string, bars []methods.Bar, holdDays int) (CodeBoundary, []methods.Bar, error) {
	minimum := 180 + holdDays + 1
	if len(bars) < minimum {
		return CodeBoundary{}, nil, fmt.Errorf("insufficient real bars for %s: %d < %d", code, len(bars), minimum)
	}
	reserved := len(bars) * 30 / 100
	if reserved < 60 {
		reserved = 60
	}
	discoveryEnd := len(bars) - reserved
	if discoveryEnd < 120+holdDays+1 {
		return CodeBoundary{}, nil, fmt.Errorf("insufficient discovery window for %s after reserving untouched data", code)
	}
	boundary := CodeBoundary{
		Code: code, FirstDate: bars[0].Date, DiscoveryEndDate: bars[discoveryEnd-1].Date,
		ReservedStartDate: bars[discoveryEnd].Date, LastDate: bars[len(bars)-1].Date,
		DiscoveryBars: discoveryEnd, ReservedBars: reserved,
	}
	return boundary, bars[:discoveryEnd], nil
}

func matchedForwardReturns(method *methods.CompiledMethod, bars []methods.Bar, holdDays int) ([]float64, error) {
	var returns []float64
	for i := 0; i+holdDays+1 < len(bars); i++ {
		matched, err := method.Entry(bars[i], bars[:i+1])
		if err != nil {
			return nil, err
		}
		if !matched.Matched {
			continue
		}
		entry := bars[i+1].Open
		exit := bars[i+holdDays+1].Close
		if entry <= 0 || exit <= 0 {
			continue
		}
		returns = append(returns, (exit-entry)/entry)
		i += holdDays // 避免同一持有窗口重叠扩大样本量。
	}
	return returns, nil
}

func forwardReturns(bars []methods.Bar, holdDays int) []float64 {
	returns := make([]float64, 0, len(bars))
	for i := 0; i+holdDays+1 < len(bars); i++ {
		entry, exit := bars[i+1].Open, bars[i+holdDays+1].Close
		if entry > 0 && exit > 0 {
			returns = append(returns, (exit-entry)/entry)
		}
	}
	return returns
}

func templateCandidate(template patternTemplate, holdDays int) *methods.Candidate {
	position := 0.10
	return &methods.Candidate{
		Name: "AI发现候选: " + template.name, Description: template.rationale,
		SourceKind: "deterministic_discovery", Universe: "researched_stocks",
		Entry: template.entry, HoldingMaxDays: holdDays, PositionMode: "pct_equity", PositionPct: &position,
	}
}

func candidateTemplates(holdDays int) []patternTemplate {
	templates := []patternTemplate{
		{id: "close_breakout_prevhigh20", name: "收盘突破前20日高点",
			entry:     compare(methods.CmpGT, indicator("close"), indicator("prevhigh20")),
			rationale: "检验无前视的20日突破后的趋势延续，高点不包含当日。"},
		{id: "close_breakdown_prevlow20", name: "收盘跌破前20日低点",
			entry:     compare(methods.CmpLT, indicator("close"), indicator("prevlow20")),
			rationale: "检验无前视破位后的延续或反转，不预设方向。"},
		{id: "gap_up_2pct", name: "向上跳空超过2%",
			entry:     compare(methods.CmpGT, indicator("gap_pct"), constant(0.02)),
			rationale: "检验隔夜信息冲击后的条件收益，跳空仅使用当日开盘与前收盘。"},
		{id: "gap_down_2pct", name: "向下跳空超过2%",
			entry:     compare(methods.CmpLT, indicator("gap_pct"), constant(-0.02)),
			rationale: "检验负向隔夜冲击后的反转或延续，不将未来信息混入特征。"},
		{id: "daily_rise_5pct", name: "单日上涨超过5%",
			entry:     compare(methods.CmpGT, indicator("return1"), constant(0.05)),
			rationale: "检验大幅上涨后动量延续与过度反应两种竞争解释。"},
		{id: "daily_fall_5pct", name: "单日下跌超过5%",
			entry:     compare(methods.CmpLT, indicator("return1"), constant(-0.05)),
			rationale: "检验急跌后恐慌反转或负向动量，只标记候选不作买入结论。"},
		{id: "volume_above_ma20", name: "成交量高于20日均量",
			entry:     compare(methods.CmpGT, indicator("volume"), indicator("volma20")),
			rationale: "检验放量所代表的关注度变化是否改变后续收益分布。"},
		{id: "volatility20_above_3pct", name: "20日波动率高于3%",
			entry:     compare(methods.CmpGT, indicator("volatility20"), constant(0.03)),
			rationale: "检验高波动市场状态下规律是否存在，防止把市场阶段当成普遍效应。"},
		{id: "volatility20_below_1_5pct", name: "20日波动率低于1.5%",
			entry:     compare(methods.CmpLT, indicator("volatility20"), constant(0.015)),
			rationale: "检验低波动盘整状态下的后续收益，与高波动候选分开。"},
	}
	for _, n := range []int{10, 20, 60} {
		ma := indicator(fmt.Sprintf("ma%d", n))
		templates = append(templates,
			patternTemplate{id: fmt.Sprintf("close_above_ma%d", n), name: fmt.Sprintf("收盘站上%d日均线", n),
				entry: compare(methods.CmpGT, indicator("close"), ma), rationale: "检验趋势持续是否在后续持有窗口产生正向条件收益。"},
			patternTemplate{id: fmt.Sprintf("close_below_ma%d", n), name: fmt.Sprintf("收盘跌破%d日均线", n),
				entry: compare(methods.CmpLT, indicator("close"), ma), rationale: "检验趋势超跌后的均值回归，不预设跌破必然反弹。"})
	}
	for _, threshold := range []float64{25, 30, 35} {
		templates = append(templates, patternTemplate{
			id: fmt.Sprintf("rsi14_below_%d", int(threshold)), name: fmt.Sprintf("RSI14低于%.0f", threshold),
			entry:     compare(methods.CmpLT, indicator("rsi14"), constant(threshold)),
			rationale: "检验短期超卖后是否存在可重复反弹，并与无条件基线比较。",
		})
	}
	for _, threshold := range []float64{65, 70, 75} {
		templates = append(templates, patternTemplate{
			id: fmt.Sprintf("rsi14_above_%d", int(threshold)), name: fmt.Sprintf("RSI14高于%.0f", threshold),
			entry:     compare(methods.CmpGT, indicator("rsi14"), constant(threshold)),
			rationale: "检验强势动量是继续还是均值回归，结论只来自发现样本。",
		})
	}
	for _, pair := range [][2]int{{5, 20}, {10, 20}, {10, 60}, {20, 60}} {
		fast, slow := indicator(fmt.Sprintf("ma%d", pair[0])), indicator(fmt.Sprintf("ma%d", pair[1]))
		templates = append(templates,
			patternTemplate{id: fmt.Sprintf("ma%d_cross_above_ma%d", pair[0], pair[1]), name: fmt.Sprintf("MA%d上穿MA%d", pair[0], pair[1]),
				entry: cross("above", fast, slow), rationale: "检验均线趋势转强信号在真实历史中的条件收益。"},
			patternTemplate{id: fmt.Sprintf("ma%d_cross_below_ma%d", pair[0], pair[1]), name: fmt.Sprintf("MA%d下穿MA%d", pair[0], pair[1]),
				entry: cross("below", fast, slow), rationale: "检验趋势转弱后是延续还是反转，不使用保留样本。"})
	}
	return templates
}

func indicator(name string) *methods.Expr {
	return &methods.Expr{Type: methods.NodeIndicator, Indicator: name}
}
func constant(value float64) *methods.Expr {
	return &methods.Expr{Type: methods.NodeConstant, Value: &value}
}
func compare(op methods.CmpOp, left, right *methods.Expr) *methods.Expr {
	return &methods.Expr{Type: methods.NodeCompare, Left: left, Right: right, Op: op}
}
func cross(side string, left, right *methods.Expr) *methods.Expr {
	return &methods.Expr{Type: methods.NodeCross, Left: left, Right: right, Cross: side}
}

func researchID(request Request) string {
	payload := struct {
		Snapshot string
		Codes    []string
		Question string
		Hold     int
		Budget   int
		Version  string
	}{request.SnapshotID, request.StockCodes, strings.TrimSpace(request.Question), request.HoldDays, request.SearchBudget, GeneratorVersion}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return "research-" + hex.EncodeToString(sum[:8])
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	if len(values) == 1 {
		return mean, 0
	}
	var squares float64
	for _, value := range values {
		delta := value - mean
		squares += delta * delta
	}
	return mean, math.Sqrt(squares / float64(len(values)-1))
}

func positiveRate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	positive := 0
	for _, value := range values {
		if value > 0 {
			positive++
		}
	}
	return float64(positive) / float64(len(values))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
