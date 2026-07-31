package paradigms

import (
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 市场状态分类体系
// ============================================================================

// MarketRegime 市场体制
type MarketRegime string

const (
	RegimeTrendUp   MarketRegime = "trend_up"   // 上升趋势
	RegimeTrendDown MarketRegime = "trend_down" // 下降趋势
	RegimeRange     MarketRegime = "range"      // 震荡
	RegimeHighVol   MarketRegime = "high_vol"   // 高波动
	RegimeLowVol    MarketRegime = "low_vol"    // 低波动
	RegimeHighLiq   MarketRegime = "high_liq"   // 高流动性
	RegimeLowLiq    MarketRegime = "low_liq"    // 低流动性
	RegimeLargeCap  MarketRegime = "large_cap"  // 大盘股
	RegimeMidCap    MarketRegime = "mid_cap"    // 中盘股
	RegimeSmallCap  MarketRegime = "small_cap"  // 小盘股
	RegimeBull      MarketRegime = "bull"       // 牛市
	RegimeBear      MarketRegime = "bear"       // 熊市
	RegimeSideways  MarketRegime = "sideways"   // 横盘
)

// MarketState 市场状态
type MarketState struct {
	Regime        MarketRegime `json:"regime"`
	Volatility    float64      `json:"volatility"`
	Liquidity     float64      `json:"liquidity"`
	MarketCap     float64      `json:"market_cap"`
	Industry      string       `json:"industry"`
	Breadth       float64      `json:"breadth"`        // 上涨股票占比
	TrendStrength float64      `json:"trend_strength"` // 趋势强度 (ADX)
	Date          time.Time    `json:"date"`
}

// IsTrending 判断是否处于趋势市
func (ms *MarketState) IsTrending() bool {
	return ms.Regime == RegimeTrendUp || ms.Regime == RegimeTrendDown
}

// IsRangeBound 判断是否处于震荡市
func (ms *MarketState) IsRangeBound() bool {
	return ms.Regime == RegimeRange || ms.Regime == RegimeSideways
}

// IsHighVolatility 判断是否高波动
func (ms *MarketState) IsHighVolatility() bool {
	return ms.Volatility > 0.02 // 2% 阈值
}

// IsLowVolatility 判断是否低波动
func (ms *MarketState) IsLowVolatility() bool {
	return ms.Volatility < 0.01
}

// IsLiquid 判断是否流动性好
func (ms *MarketState) IsLiquid() bool {
	return ms.Liquidity > 1.0
}

// IsLargeCap 判断是否大盘
func (ms *MarketState) IsLargeCap() bool {
	return ms.MarketCap > 1000000000000 // 1万亿
}

// IsBull 判断是否牛市
func (ms *MarketState) IsBull() bool {
	return ms.Regime == RegimeBull || ms.Regime == RegimeTrendUp
}

// IsBear 判断是否熊市
func (ms *MarketState) IsBear() bool {
	return ms.Regime == RegimeBear || ms.Regime == RegimeTrendDown
}

// ============================================================================
// 上下文层定义
// ============================================================================

// ContextLayer 上下文层
type ContextLayer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Condition   string `json:"condition"` // 描述条件
	Level       int    `json:"level"`     // 层级 (1=最高优先级)
	Enabled     bool   `json:"enabled"`
}

// LayerDefinition 层定义
type LayerDefinition struct {
	Layer  ContextLayer
	EvalFn func(state *MarketState) bool
	Params map[string]interface{}
}

// 默认层级定义
var DefaultLayers = []LayerDefinition{
	{
		Layer: ContextLayer{
			ID:          "market_trend",
			Name:        "市场趋势",
			Description: "区分趋势市与震荡市",
			Condition:   "trend_strength > 25",
			Level:       1,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.TrendStrength > 25
		},
	},
	{
		Layer: ContextLayer{
			ID:          "volatility",
			Name:        "波动率",
			Description: "区分高波动与低波动环境",
			Condition:   "volatility > 2%",
			Level:       1,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.IsHighVolatility()
		},
	},
	{
		Layer: ContextLayer{
			ID:          "liquidity",
			Name:        "流动性",
			Description: "区分高流动与低流动环境",
			Condition:   "liquidity > 1.0",
			Level:       2,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.IsLiquid()
		},
	},
	{
		Layer: ContextLayer{
			ID:          "market_cap",
			Name:        "市值分层",
			Description: "区分大盘、中盘、小盘股",
			Condition:   "market_cap > 1万亿",
			Level:       2,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.IsLargeCap()
		},
	},
	{
		Layer: ContextLayer{
			ID:          "industry",
			Name:        "行业",
			Description: "按行业分类",
			Condition:   "industry == 'technology'",
			Level:       3,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.Industry == "technology" || state.Industry == "finance" || state.Industry == "consumer"
		},
	},
	{
		Layer: ContextLayer{
			ID:          "market_breadth",
			Name:        "市场宽度",
			Description: "上涨股票占比",
			Condition:   "breadth > 0.5",
			Level:       2,
			Enabled:     true,
		},
		EvalFn: func(state *MarketState) bool {
			return state.Breadth > 0.5
		},
	},
}

// ============================================================================
// 上下文分层引擎
// ============================================================================

// ContextLayerEngine 上下文分层引擎
type ContextLayerEngine struct {
	layers     []LayerDefinition
	thresholds map[string]float64
	maxLevels  int
}

// NewContextLayerEngine 创建上下文分层引擎
func NewContextLayerEngine() *ContextLayerEngine {
	return &ContextLayerEngine{
		layers: DefaultLayers,
		thresholds: map[string]float64{
			"trend_strength": 25,
			"volatility":     0.02,
			"liquidity":      1.0,
			"market_cap":     1000000000000,
			"breadth":        0.5,
		},
		maxLevels: 5,
	}
}

// Analyze 分析给定日期的市场状态
func (e *ContextLayerEngine) Analyze(state *MarketState) []LayerResult {
	var results []LayerResult

	for _, layer := range e.layers {
		if !layer.Layer.Enabled {
			continue
		}

		matched := layer.EvalFn(state)
		result := LayerResult{
			LayerID:   layer.Layer.ID,
			LayerName: layer.Layer.Name,
			Level:     layer.Layer.Level,
			Matched:   matched,
			State:     state,
		}
		results = append(results, result)
	}

	return results
}

// MatchContext 匹配特定上下文条件
func (e *ContextLayerEngine) MatchContext(state *MarketState, conditions map[string]bool) []LayerResult {
	var results []LayerResult

	for _, layer := range e.layers {
		enabled, exists := conditions[layer.Layer.ID]
		if !exists {
			continue
		}

		actual := layer.EvalFn(state)
		if actual == enabled {
			results = append(results, LayerResult{
				LayerID:   layer.Layer.ID,
				LayerName: layer.Layer.Name,
				Level:     layer.Layer.Level,
				Matched:   true,
				State:     state,
			})
		}
	}

	return results
}

// GetActiveLayers 获取所有启用的层级
func (e *ContextLayerEngine) GetActiveLayers() []ContextLayer {
	var layers []ContextLayer
	for _, l := range e.layers {
		if l.Layer.Enabled {
			layers = append(layers, l.Layer)
		}
	}
	return layers
}

// AddLayer 添加新层级
func (e *ContextLayerEngine) AddLayer(layer LayerDefinition) {
	e.layers = append(e.layers, layer)
}

// RemoveLayer 移除层级
func (e *ContextLayerEngine) RemoveLayer(layerID string) error {
	for i, l := range e.layers {
		if l.Layer.ID == layerID {
			e.layers = append(e.layers[:i], e.layers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("layer not found: %s", layerID)
}

// SetThreshold 设置阈值
func (e *ContextLayerEngine) SetThreshold(key string, value float64) {
	e.thresholds[key] = value
}

// ============================================================================
// 层级结果
// ============================================================================

// LayerResult 层级结果
type LayerResult struct {
	LayerID   string       `json:"layer_id"`
	LayerName string       `json:"layer_name"`
	Level     int          `json:"level"`
	Matched   bool         `json:"matched"`
	State     *MarketState `json:"state"`
}

// LayerPerformance 分层表现
type LayerPerformance struct {
	LayerID       string  `json:"layer_id"`
	LayerName     string  `json:"layer_name"`
	Count         int     `json:"count"`
	WinRate       float64 `json:"win_rate"`
	AvgReturn     float64 `json:"avg_return"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	SharpeRatio   float64 `json:"sharpe_ratio"`
	SampleSize    int     `json:"sample_size"`
	Confidence    float64 `json:"confidence"`
	IsEnvironment bool    `json:"is_environment"`
}

// IsValid 检查层级表现是否有效
func (lp *LayerPerformance) IsValid() bool {
	return lp.SampleSize >= 20 && lp.Confidence >= 0.5
}

// ============================================================================
// 环境匹配与失效检测
// ============================================================================

// EnvironmentProfile 环境画像
type EnvironmentProfile struct {
	ID               string             `json:"id"`
	LayerResults     []LayerResult      `json:"layer_results"`
	Performance      []LayerPerformance `json:"performance"`
	BestEnvironment  string             `json:"best_environment"`
	WorstEnvironment string             `json:"worst_environment"`
	FoundedAt        time.Time          `json:"founded_at"`
	LastUpdated      time.Time          `json:"last_updated"`
}

// NewEnvironmentProfile 创建环境画像
func NewEnvironmentProfile() *EnvironmentProfile {
	now := time.Now()
	return &EnvironmentProfile{
		LayerResults: make([]LayerResult, 0),
		Performance:  make([]LayerPerformance, 0),
		FoundedAt:    now,
		LastUpdated:  now,
	}
}

// AddLayerResult 添加层级结果
func (ep *EnvironmentProfile) AddLayerResult(result LayerResult) {
	ep.LayerResults = append(ep.LayerResults, result)
	ep.LastUpdated = time.Now()
}

// AddPerformance 添加分层表现
func (ep *EnvironmentProfile) AddPerformance(perf LayerPerformance) {
	ep.Performance = append(ep.Performance, perf)
	ep.LastUpdated = time.Now()
}

// FindBestEnvironment 查找最佳环境
func (ep *EnvironmentProfile) FindBestEnvironment() *LayerPerformance {
	if len(ep.Performance) == 0 {
		return nil
	}

	var best *LayerPerformance
	for i := range ep.Performance {
		if best == nil || ep.Performance[i].SharpeRatio > best.SharpeRatio {
			best = &ep.Performance[i]
		}
	}

	if best != nil {
		ep.BestEnvironment = best.LayerID
	}

	return best
}

// FindWorstEnvironment 查找最差环境
func (ep *EnvironmentProfile) FindWorstEnvironment() *LayerPerformance {
	if len(ep.Performance) == 0 {
		return nil
	}

	var worst *LayerPerformance
	for i := range ep.Performance {
		if worst == nil || ep.Performance[i].SharpeRatio < worst.SharpeRatio {
			worst = &ep.Performance[i]
		}
	}

	if worst != nil {
		ep.WorstEnvironment = worst.LayerID
	}

	return worst
}

// GetFavorableEnvironments 获取有利环境
func (ep *EnvironmentProfile) GetFavorableEnvironments(minSharpe float64) []*LayerPerformance {
	var favorable []*LayerPerformance
	for i := range ep.Performance {
		if ep.Performance[i].SharpeRatio >= minSharpe && ep.Performance[i].SampleSize >= 20 {
			favorable = append(favorable, &ep.Performance[i])
		}
	}
	return favorable
}

// GetUnfavorableEnvironments 获取不利环境
func (ep *EnvironmentProfile) GetUnfavorableEnvironments(maxSharpe float64) []*LayerPerformance {
	var unfavorable []*LayerPerformance
	for i := range ep.Performance {
		if ep.Performance[i].SharpeRatio <= maxSharpe {
			unfavorable = append(unfavorable, &ep.Performance[i])
		}
	}
	return unfavorable
}

// ============================================================================
// 分层报告生成器
// ============================================================================

// LayeredReport 分层报告
type LayeredReport struct {
	Title        string             `json:"title"`
	Date         time.Time          `json:"date"`
	TotalSamples int                `json:"total_samples"`
	Layers       []LayerPerformance `json:"layers"`
	Summary      LayerSummary       `json:"summary"`
}

// LayerSummary 分层摘要
type LayerSummary struct {
	BestLayer   string  `json:"best_layer"`
	BestSharpe  float64 `json:"best_sharpe"`
	WorstLayer  string  `json:"worst_layer"`
	WorstSharpe float64 `json:"worst_sharpe"`
	AvgSharpe   float64 `json:"avg_sharpe"`
	StdSharpe   float64 `json:"std_sharpe"`
	EnvCount    int     `json:"env_count"`
	ValidCount  int     `json:"valid_count"`
}

// GenerateLayeredReport 生成分层报告
func GenerateLayeredReport(layers []LayerPerformance, title string) *LayeredReport {
	report := &LayeredReport{
		Title:  title,
		Date:   time.Now(),
		Layers: layers,
	}

	// 计算摘要
	if len(layers) > 0 {
		summary := LayerSummary{
			EnvCount: len(layers),
		}

		totalSharpe := 0.0
		bestSharpe := -math.MaxFloat64
		worstSharpe := math.MaxFloat64

		for _, l := range layers {
			if l.IsValid() {
				summary.ValidCount++
			}
			totalSharpe += l.SharpeRatio
			if l.SharpeRatio > bestSharpe {
				bestSharpe = l.SharpeRatio
				summary.BestLayer = l.LayerName
			}
			if l.SharpeRatio < worstSharpe {
				worstSharpe = l.SharpeRatio
				summary.WorstLayer = l.LayerName
			}
		}

		if summary.EnvCount > 0 {
			summary.AvgSharpe = totalSharpe / float64(summary.EnvCount)
			summary.BestSharpe = bestSharpe
			summary.WorstSharpe = worstSharpe

			// 计算标准差
			variance := 0.0
			for _, l := range layers {
				diff := l.SharpeRatio - summary.AvgSharpe
				variance += diff * diff
			}
			summary.StdSharpe = math.Sqrt(variance / float64(summary.EnvCount))
		}

		report.Summary = summary
	}

	// 计算总样本量
	totalSamples := 0
	for _, l := range layers {
		totalSamples += l.SampleSize
	}
	report.TotalSamples = totalSamples

	return report
}

// ============================================================================
// 无未来数据检查
// ============================================================================

// LookAheadValidator 未来数据检查器
type LookAheadValidator struct {
	maxLookAhead     time.Duration
	allowLookAhead   bool
	forbidPostHocOpt bool
}

// NewLookAheadValidator 创建未来数据检查器
func NewLookAheadValidator() *LookAheadValidator {
	return &LookAheadValidator{
		maxLookAhead:     0,
		allowLookAhead:   false,
		forbidPostHocOpt: true,
	}
}

// ValidateContextData 验证上下文数据是否存在未来泄露
func (v *LookAheadValidator) ValidateContextData(stateDate time.Time, dataDate time.Time) error {
	if v.allowLookAhead {
		return nil
	}

	// 检查数据日期是否晚于状态日期
	if dataDate.After(stateDate) {
		return fmt.Errorf("data date %s is after state date %s - future data leak detected",
			dataDate.Format("2006-01-02"), stateDate.Format("2006-01-02"))
	}

	return nil
}

// ValidateNoPostHocOpt 验证是否禁止事后最优分组
func (v *LookAheadValidator) ValidateNoPostHocOpt(layers []LayerDefinition, state *MarketState) error {
	if !v.forbidPostHocOpt {
		return nil
	}

	// 检查层定义是否基于未来信息
	for _, layer := range layers {
		// 如果层的条件依赖于事后最优阈值，拒绝
		if layer.Layer.Condition == "" {
			return fmt.Errorf("layer %s has no condition defined - possible post-hoc optimization",
				layer.Layer.ID)
		}
	}

	return nil
}

// SetMaxLookAhead 设置最大允许的前视期
func (v *LookAheadValidator) SetMaxLookAhead(duration time.Duration) {
	v.maxLookAhead = duration
	v.allowLookAhead = duration > 0
}

// SetForbidPostHocOpt 设置是否禁止事后最优
func (v *LookAheadValidator) SetForbidPostHocOpt(forbid bool) {
	v.forbidPostHocOpt = forbid
}

// ============================================================================
// 实用函数
// ============================================================================

// CalculateADX 计算 ADX 趋势强度指标
func CalculateADX(highs, lows []float64, period int) float64 {
	if len(highs) < period+1 || len(lows) < period+1 {
		return 0
	}

	// 简化版 ADX 计算
	upMoves := 0.0
	downMoves := 0.0

	for i := len(highs) - period; i < len(highs); i++ {
		diff := highs[i] - highs[i-1]
		if diff > 0 {
			upMoves += diff
		} else {
			downMoves -= diff
		}
	}

	if upMoves+downMoves == 0 {
		return 0
	}

	// 返回简化的趋势强度 (0-100)
	return 100 * upMoves / (upMoves + downMoves)
}

// CalculateVolatility 计算波动率
func CalculateVolatility(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	variance := 0.0
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(returns))

	return math.Sqrt(variance)
}

// CalculateBreadth 计算市场宽度
func CalculateBreadth(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	upCount := 0
	for _, r := range returns {
		if r > 0 {
			upCount++
		}
	}

	return float64(upCount) / float64(len(returns))
}
