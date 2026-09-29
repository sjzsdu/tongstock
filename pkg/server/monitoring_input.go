package server

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/internal/monitoring"
	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// 监控输入构造：把真实观测转换成监控引擎需要的收益序列与持仓。
//
// 数据源按可信度排序：
//  1. forward_ledger      前向账本 (Paper Trading 真实权益曲线 + 持仓)
//  2. trading_positions   真实交易持仓标的 (自首笔建仓起的真实行情)
//  3. watchlist_proxy     自选股等权组合 (真实行情代理, 页面需标注)
//
// 任何来源的观测不足都会被记录到 status.Notes, 并降级到下一个来源,
// 全部不足时返回错误, 绝不凭空生成报告。
const (
	monitoringSeriesLookbackDays = 400 // 收益序列回看的日历天数
	monitoringBaselineMaxPoints  = 180 // 基准窗口上限 (观测点)
	monitoringMinForwardPoints   = 20  // 前向窗口最少观测
	monitoringMinBaselinePoints  = 20  // 基准窗口最少观测
	monitoringUniverseCap        = 30  // 组合最多纳入的标的数
	monitoringFetchConcurrency   = 4   // 行情并发拉取数
	monitoringBuildTimeout       = 45 * time.Second
	monitoringReportTTL          = 15 * time.Minute
	monitoringDateLayout         = "2006-01-02"
)

// MonitoringInputStatus 描述报告所使用的真实观测输入及其覆盖情况。
type MonitoringInputStatus struct {
	Source        string    `json:"source"`
	SourceLabel   string    `json:"source_label"`
	Universe      []string  `json:"universe"`
	PositionCount int       `json:"position_count"`
	ObsCount      int       `json:"obs_count"`
	ForwardCount  int       `json:"forward_count"`
	BaselineCount int       `json:"baseline_count"`
	ForwardStart  time.Time `json:"forward_start"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	ForwardRuns   int       `json:"forward_runs"`
	Signals       int       `json:"signals"`
	Trades        int       `json:"trades"`
	Notes         []string  `json:"notes"`
	CheckedAt     time.Time `json:"checked_at"`
}

// monitoringPoint 单日观测收益。
type monitoringPoint struct {
	Date time.Time
	Ret  float64
}

// monitoringSource 一个候选数据源解析出的输入。
type monitoringSource struct {
	code      string
	label     string
	series    []monitoringPoint
	positions []monitoring.PositionItem
	universe  []string
	note      string
}

// monitoringSourceError 表示该数据源不可用, error 文本即诊断说明。
type monitoringSourceError struct {
	note string
}

func (e *monitoringSourceError) Error() string { return e.note }

func sourceUnavailable(note string) error { return &monitoringSourceError{note: note} }

// buildMonitoringInput 依优先级选择第一个观测充足的真实数据源。
func (s *Server) buildMonitoringInput(ctx context.Context) (monitoring.MonitoringInput, MonitoringInputStatus, error) {
	status := MonitoringInputStatus{
		Source:    "none",
		Universe:  []string{},
		Notes:     []string{},
		CheckedAt: time.Now(),
	}
	s.fillLedgerCounts(&status)

	providers := []func(context.Context) (monitoringSource, error){
		s.forwardLedgerSource,
		s.tradingPositionsSource,
		s.watchlistSource,
	}

	var forwardDays int
	if s.monitoringEngine != nil {
		forwardDays = s.monitoringEngine.Config.AnalysisWindowDays
	}

	for _, provide := range providers {
		source, err := provide(ctx)
		if err != nil {
			status.Notes = append(status.Notes, err.Error())
			continue
		}
		split := splitMonitoringSeries(source.series, forwardDays)
		if len(split.Forward) < monitoringMinForwardPoints || len(split.Baseline) < monitoringMinBaselinePoints {
			status.Notes = append(status.Notes, fmt.Sprintf(
				"%s 观测不足: 前向 %d/%d, 基准 %d/%d, 已降级到下一数据源",
				source.label, len(split.Forward), monitoringMinForwardPoints,
				len(split.Baseline), monitoringMinBaselinePoints))
			continue
		}

		status.Source = source.code
		status.SourceLabel = source.label
		status.Universe = source.universe
		status.PositionCount = len(source.positions)
		status.ObsCount = len(source.series)
		status.ForwardCount = len(split.Forward)
		status.BaselineCount = len(split.Baseline)
		status.ForwardStart = split.ForwardDates[0]
		status.Start = source.series[0].Date
		status.End = source.series[len(source.series)-1].Date
		status.Notes = append(status.Notes, source.note)

		return monitoring.MonitoringInput{
			BaselineReturns: split.Baseline,
			ForwardReturns:  split.Forward,
			ForwardDates:    split.ForwardDates,
			Positions:       source.positions,
		}, status, nil
	}

	return monitoring.MonitoringInput{}, status, fmt.Errorf(
		"尚无基于真实观测输入的监控报告: %s", joinNotes(status.Notes))
}

func (s *Server) fillLedgerCounts(status *MonitoringInputStatus) {
	if s.ledger == nil {
		return
	}
	runs := s.ledger.ListRuns(50)
	status.ForwardRuns = len(runs)
	for _, run := range runs {
		status.Signals += run.SignalCount
	}
	if s.tradingDB != nil {
		if trades, err := s.tradingDB.GetAll(); err == nil {
			status.Trades = len(trades)
		}
	}
}

func joinNotes(notes []string) string {
	if len(notes) == 0 {
		return "没有任何可用数据源"
	}
	out := ""
	for i, note := range notes {
		if i > 0 {
			out += "; "
		}
		out += note
	}
	return out
}

// monitoringSplit 收益序列的前向/基准切分结果。
type monitoringSplit struct {
	Forward      []float64
	ForwardDates []time.Time
	Baseline     []float64
}

// splitMonitoringSeries 切分前向窗口与基准窗口。
// points 必须按日期升序, 每个元素本身即一日收益。
// 前向窗口取最近 forwardDays 个观测, 基准窗口取其之前最多
// monitoringBaselineMaxPoints 个观测 (即"近期 vs 自身历史")。
func splitMonitoringSeries(points []monitoringPoint, forwardDays int) monitoringSplit {
	if forwardDays <= 0 {
		forwardDays = 60
	}
	n := len(points)
	if n <= forwardDays+monitoringMinBaselinePoints {
		return monitoringSplit{}
	}
	forwardStart := n - forwardDays
	baselineStart := forwardStart - monitoringBaselineMaxPoints
	if baselineStart < 0 {
		baselineStart = 0
	}
	if forwardStart-baselineStart < monitoringMinBaselinePoints {
		return monitoringSplit{}
	}

	split := monitoringSplit{
		Forward:      make([]float64, 0, n-forwardStart),
		ForwardDates: make([]time.Time, 0, n-forwardStart),
		Baseline:     make([]float64, 0, forwardStart-baselineStart),
	}
	for _, point := range points[forwardStart:] {
		split.Forward = append(split.Forward, point.Ret)
		split.ForwardDates = append(split.ForwardDates, point.Date)
	}
	for _, point := range points[baselineStart:forwardStart] {
		split.Baseline = append(split.Baseline, point.Ret)
	}
	return split
}

// ============================================================================
// 数据源 1: 前向账本
// ============================================================================

func (s *Server) forwardLedgerSource(context.Context) (monitoringSource, error) {
	if s.ledger == nil {
		return monitoringSource{}, sourceUnavailable("前向账本未初始化")
	}
	runs := s.ledger.ListRuns(10)
	if len(runs) == 0 {
		return monitoringSource{}, sourceUnavailable(
			"尚无前向运行 (范式前向观察未启动, 可通过 POST /api/forward/runs 创建)")
	}

	// 选权益曲线点数最多的运行
	best := runs[0]
	for _, run := range runs[1:] {
		if len(run.EquityCurve) > len(best.EquityCurve) {
			best = run
		}
	}
	if len(best.EquityCurve) < 2 {
		return monitoringSource{}, sourceUnavailable(
			fmt.Sprintf("前向运行 %s 权益曲线尚无观测 (点数 %d)", best.ID, len(best.EquityCurve)))
	}

	series := make([]monitoringPoint, 0, len(best.EquityCurve)-1)
	for i := 1; i < len(best.EquityCurve); i++ {
		prev, cur := best.EquityCurve[i-1], best.EquityCurve[i]
		if prev.Total <= 0 || cur.Total <= 0 || cur.Date.Equal(prev.Date) {
			continue
		}
		series = append(series, monitoringPoint{Date: cur.Date, Ret: cur.Total/prev.Total - 1})
	}

	positions := make([]monitoring.PositionItem, 0, len(best.Positions))
	universe := make([]string, 0, len(best.Positions))
	for code := range best.Positions {
		universe = append(universe, code)
	}
	sort.Strings(universe)

	var totalValue float64
	values := make(map[string]float64, len(universe))
	for _, code := range universe {
		pos := best.Positions[code]
		price := pos.LastPrice
		if price <= 0 {
			price = pos.AveragePrice
		}
		value := float64(pos.Quantity) * price
		values[code] = value
		totalValue += value
	}
	for _, code := range universe {
		weight := 0.0
		if totalValue > 0 {
			weight = values[code] / totalValue
		}
		positions = append(positions, monitoring.PositionItem{
			Code:   code,
			Name:   s.positionName(code, ""),
			Weight: weight,
			Value:  values[code],
		})
	}

	return monitoringSource{
		code:      "forward_ledger",
		label:     "前向账本 (" + best.ID + ")",
		series:    series,
		positions: positions,
		universe:  universe,
		note: fmt.Sprintf("前向账本: 运行 %d 个, 使用 %s 的 %d 个权益观测",
			len(runs), best.ID, len(series)),
	}, nil
}

// ============================================================================
// 数据源 2: 真实交易持仓
// ============================================================================

func (s *Server) tradingPositionsSource(ctx context.Context) (monitoringSource, error) {
	if s.tradingDB == nil {
		return monitoringSource{}, sourceUnavailable("交易记录未初始化")
	}
	openPositions, err := s.tradingDB.GetAllPositions()
	if err != nil {
		return monitoringSource{}, sourceUnavailable(fmt.Sprintf("读取交易记录失败: %v", err))
	}
	if len(openPositions) == 0 {
		return monitoringSource{}, sourceUnavailable("尚无真实交易持仓 (没有未平仓的买入记录)")
	}

	codes := make([]string, 0, len(openPositions))
	earliest := time.Time{}
	for _, pos := range openPositions {
		codes = append(codes, pos.Code)
		if earliest.IsZero() || pos.CreatedAt.Before(earliest) {
			earliest = pos.CreatedAt
		}
	}
	sort.Strings(codes)
	if len(codes) > monitoringUniverseCap {
		codes = codes[:monitoringUniverseCap]
	}

	series := equalWeightSeries(ctx, s, codes)
	// 只统计真实建仓之后的行情, 不把建仓前的涨跌算进持仓观测。
	if !earliest.IsZero() {
		from := time.Date(earliest.Year(), earliest.Month(), earliest.Day(), 0, 0, 0, 0, time.Local)
		trimmed := make([]monitoringPoint, 0, len(series))
		for _, point := range series {
			if !point.Date.Before(from) {
				trimmed = append(trimmed, point)
			}
		}
		series = trimmed
	}

	weight := 1.0 / float64(len(codes))
	positions := make([]monitoring.PositionItem, 0, len(codes))
	for _, pos := range openPositions {
		positions = append(positions, monitoring.PositionItem{
			Code:   pos.Code,
			Name:   pos.Name,
			Weight: weight,
		})
	}

	return monitoringSource{
		code:      "trading_positions",
		label:     "真实持仓标的等权组合",
		series:    series,
		positions: positions,
		universe:  codes,
		note: fmt.Sprintf("真实交易: %d 笔成交, 持仓标的 %d 只, 建仓起真实行情; 交易记录不含数量, 权重按等权计",
			len(openPositions), len(codes)),
	}, nil
}

// ============================================================================
// 数据源 3: 自选股等权组合 (真实行情代理)
// ============================================================================

func (s *Server) watchlistSource(ctx context.Context) (monitoringSource, error) {
	if s.watchlistDB == nil {
		return monitoringSource{}, sourceUnavailable("自选股列表未初始化")
	}
	stocks, err := s.watchlistDB.GetAll()
	if err != nil {
		return monitoringSource{}, sourceUnavailable(fmt.Sprintf("读取自选股失败: %v", err))
	}
	if len(stocks) == 0 {
		return monitoringSource{}, sourceUnavailable("自选股为空, 无法构造代理组合")
	}

	codes := make([]string, 0, len(stocks))
	nameByCode := make(map[string]string, len(stocks))
	for _, stock := range stocks {
		if _, exists := nameByCode[stock.Code]; exists {
			continue
		}
		nameByCode[stock.Code] = stock.Name
		codes = append(codes, stock.Code)
	}
	sort.Strings(codes)
	if len(codes) > monitoringUniverseCap {
		codes = codes[:monitoringUniverseCap]
	}

	series := equalWeightSeries(ctx, s, codes)
	weight := 1.0 / float64(len(codes))
	positions := make([]monitoring.PositionItem, 0, len(codes))
	for _, code := range codes {
		positions = append(positions, monitoring.PositionItem{
			Code:   code,
			Name:   s.positionName(code, nameByCode[code]),
			Weight: weight,
		})
	}

	return monitoringSource{
		code:      "watchlist_proxy",
		label:     "自选股等权组合 (真实行情代理)",
		series:    series,
		positions: positions,
		universe:  codes,
		note: fmt.Sprintf("自选组合代理: %d 只自选股等权, 取最近 %d 个日历日的真实行情; 该组合为关注池, 非实际持仓",
			len(codes), monitoringSeriesLookbackDays),
	}, nil
}

// ============================================================================
// 行情与辅助
// ============================================================================

// equalWeightSeries 计算一组标的的等权日收益序列 (按可用标的的日均值)。
func equalWeightSeries(ctx context.Context, s *Server, codes []string) []monitoringPoint {
	if len(codes) == 0 {
		return nil
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		perCode []map[string]float64
	)
	semaphore := make(chan struct{}, monitoringFetchConcurrency)

	for _, code := range codes {
		wg.Add(1)
		go func(code string) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			closes, err := s.dailyCloses(ctx, code)
			if err != nil {
				return
			}
			returnByDay := make(map[string]float64, len(closes))
			for i := 1; i < len(closes); i++ {
				prev, cur := closes[i-1], closes[i]
				if prev.Close <= 0 || !cur.Date.After(prev.Date) {
					continue
				}
				returnByDay[cur.Date.Format(monitoringDateLayout)] = cur.Close/prev.Close - 1
			}
			if len(returnByDay) == 0 {
				return
			}
			mu.Lock()
			perCode = append(perCode, returnByDay)
			mu.Unlock()
		}(code)
	}
	wg.Wait()

	if len(perCode) == 0 {
		return nil
	}

	daySet := make(map[string]struct{})
	for _, returns := range perCode {
		for day := range returns {
			daySet[day] = struct{}{}
		}
	}
	days := make([]string, 0, len(daySet))
	for day := range daySet {
		days = append(days, day)
	}
	sort.Strings(days)

	series := make([]monitoringPoint, 0, len(days))
	for _, day := range days {
		var sum float64
		var count int
		for _, returns := range perCode {
			if value, ok := returns[day]; ok {
				sum += value
				count++
			}
		}
		if count == 0 {
			continue
		}
		date, err := time.ParseInLocation(monitoringDateLayout, day, time.Local)
		if err != nil {
			continue
		}
		series = append(series, monitoringPoint{Date: date, Ret: sum / float64(count)})
	}
	return series
}

type dailyClose struct {
	Date  time.Time
	Close float64
}

// dailyCloses 读取单只股票最近的日线收盘价。优先读本地缓存, 缓存缺失才回源同步。
func (s *Server) dailyCloses(ctx context.Context, code string) ([]dailyClose, error) {
	if s.stockData == nil {
		return nil, fmt.Errorf("股票数据服务未初始化")
	}
	spec := stockdata.DataSpec{
		Type:        stockdata.DataKline,
		Market:      marketForCode(code),
		Code:        code,
		Granularity: "day",
		KType:       tdx.ParseKlineType("day"),
	}

	result, err := s.stockData.Query(ctx, stockdata.DataRequest{Spec: spec, Mode: stockdata.CacheOnly})
	if err != nil {
		result, err = s.stockData.Query(ctx, stockdata.DataRequest{Spec: spec, Mode: stockdata.AllowStale})
	}
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -monitoringSeriesLookbackDays)
	byDay := make(map[string]dailyClose, len(result.Klines))
	for _, kline := range result.Klines {
		if kline == nil || kline.Close <= 0 {
			continue
		}
		date := time.Date(kline.Time.Year(), kline.Time.Month(), kline.Time.Day(), 0, 0, 0, 0, time.Local)
		if date.Before(cutoff) {
			continue
		}
		byDay[date.Format(monitoringDateLayout)] = dailyClose{Date: date, Close: kline.Close}
	}

	closes := make([]dailyClose, 0, len(byDay))
	for _, close := range byDay {
		closes = append(closes, close)
	}
	sort.Slice(closes, func(i, j int) bool { return closes[i].Date.Before(closes[j].Date) })
	if len(closes) < 2 {
		return nil, fmt.Errorf("%s 日线不足 (%d 根)", code, len(closes))
	}
	return closes, nil
}

// positionName 补齐持仓名称, 优先股票信息库, 其次调用方已知名称。
func (s *Server) positionName(code, fallback string) string {
	if s.stockinfoDB != nil {
		if info, err := s.stockinfoDB.GetByCode(code); err == nil && info != nil && info.Name != "" {
			return info.Name
		}
	}
	return fallback
}
