// Package monitoring - 风险集中监控模块
package monitoring

import (
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 风险集中监控
// ============================================================================

// ConcentrationType 集中度类型
type ConcentrationType string

const (
	ConcentrationPosition    ConcentrationType = "position"    // 持仓集中度
	ConcentrationIndustry    ConcentrationType = "industry"    // 行业集中度
	ConcentrationStock       ConcentrationType = "stock"       // 个股集中度
	ConcentrationSector      ConcentrationType = "sector"      // 板块集中度
	ConcentrationCorrelation ConcentrationType = "correlation" // 相关性集中度
)

// ConcentrationResult 集中度检测结果
type ConcentrationResult struct {
	Type           ConcentrationType  `json:"type"`
	HHI            float64            `json:"hhi"`             // 赫芬达尔指数 (0-1)
	EffectiveCount float64            `json:"effective_count"` // 有效标的数 (1/HHI)
	IsConcentrated bool               `json:"is_concentrated"`
	Severity       string             `json:"severity"`
	TopContributor string             `json:"top_contributor"` // 最大贡献者
	TopWeight      float64            `json:"top_weight"`      // 最大贡献者权重
	Threshold      float64            `json:"threshold"`
	Breakdown      map[string]float64 `json:"breakdown"` // 各成分权重
	Description    string             `json:"description"`
	DetectedAt     time.Time          `json:"detected_at"`
}

// ConcentrationConfig 集中度配置
type ConcentrationConfig struct {
	// 赫芬达尔指数阈值
	HHIWarningThreshold float64 `json:"hhi_warning_threshold"` // > 0.25 警告
	HHIDangerThreshold  float64 `json:"hhi_danger_threshold"`  // > 0.50 危险

	// 单标的权重阈值
	MaxSingleWeightWarning float64 `json:"max_single_weight_warning"` // > 0.20 警告
	MaxSingleWeightDanger  float64 `json:"max_single_weight_danger"`  // > 0.40 危险

	// 有效标的数下限
	MinEffectiveCount int `json:"min_effective_count"` // < 5 个标的触发警告

	// 行业集中度阈值
	MaxIndustryWeight float64 `json:"max_industry_weight"` // 单行业 > 0.40 触发
}

// NewConcentrationConfig 创建默认集中度配置
func NewConcentrationConfig() ConcentrationConfig {
	return ConcentrationConfig{
		HHIWarningThreshold:    0.25,
		HHIDangerThreshold:     0.50,
		MaxSingleWeightWarning: 0.20,
		MaxSingleWeightDanger:  0.40,
		MinEffectiveCount:      5,
		MaxIndustryWeight:      0.40,
	}
}

// ConcentrationMonitor 集中度监控器
type ConcentrationMonitor struct {
	Config ConcentrationConfig
}

// NewConcentrationMonitor 创建集中度监控器
func NewConcentrationMonitor(config ConcentrationConfig) *ConcentrationMonitor {
	return &ConcentrationMonitor{Config: config}
}

// PositionItem 持仓项
type PositionItem struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Industry string  `json:"industry"`
	Weight   float64 `json:"weight"` // 占总资产比例
	Value    float64 `json:"value"`
}

// IndustryItem 行业聚合
type IndustryItem struct {
	Industry string  `json:"industry"`
	Weight   float64 `json:"weight"`
	Count    int     `json:"count"`
}

// MonitorAll 执行全套集中度监控
func (m *ConcentrationMonitor) MonitorAll(positions []PositionItem) []ConcentrationResult {
	var results []ConcentrationResult

	// 1. 个股集中度
	results = append(results, m.MonitorStockConcentration(positions)...)

	// 2. 行业集中度
	results = append(results, m.MonitorIndustryConcentration(positions))

	// 3. 持仓集中度 (HHI)
	results = append(results, m.MonitorPositionConcentration(positions))

	return results
}

// MonitorStockConcentration 监控个股集中度
func (m *ConcentrationMonitor) MonitorStockConcentration(positions []PositionItem) []ConcentrationResult {
	if len(positions) == 0 {
		return nil
	}

	weights := make(map[string]float64)
	for _, p := range positions {
		weights[p.Code] = p.Weight
	}

	hhi := herfindahlIndex(weights)
	effectiveCount := 0.0
	if hhi > 0 {
		effectiveCount = 1.0 / hhi
	}

	// 找最大贡献者
	var topCode string
	var topWeight float64
	for code, w := range weights {
		if w > topWeight {
			topWeight = w
			topCode = code
		}
	}

	isConcentrated := topWeight > m.Config.MaxSingleWeightWarning
	severity := "normal"
	if topWeight > m.Config.MaxSingleWeightDanger {
		severity = "critical"
	} else if isConcentrated {
		severity = "warning"
	}

	return []ConcentrationResult{{
		Type:           ConcentrationStock,
		HHI:            hhi,
		EffectiveCount: effectiveCount,
		IsConcentrated: isConcentrated,
		Severity:       severity,
		TopContributor: topCode,
		TopWeight:      topWeight,
		Threshold:      m.Config.MaxSingleWeightWarning,
		Breakdown:      weights,
		Description: fmt.Sprintf("个股集中度: HHI=%.3f, 有效标的=%.1f, 最大持仓 %s (%.1f%%) [%s]",
			hhi, effectiveCount, topCode, topWeight*100, severity),
		DetectedAt: time.Now(),
	}}
}

// MonitorIndustryConcentration 监控行业集中度
func (m *ConcentrationMonitor) MonitorIndustryConcentration(positions []PositionItem) ConcentrationResult {
	if len(positions) == 0 {
		return ConcentrationResult{
			Type:        ConcentrationIndustry,
			Description: "无持仓数据",
			DetectedAt:  time.Now(),
		}
	}

	// 按行业聚合
	industries := make(map[string]IndustryItem)
	for _, p := range positions {
		item := industries[p.Industry]
		item.Industry = p.Industry
		item.Weight += p.Weight
		item.Count++
		industries[p.Industry] = item
	}

	// 转换为权重 map
	weights := make(map[string]float64)
	var topIndustry string
	var topWeight float64
	for code, item := range industries {
		weights[code] = item.Weight
		if item.Weight > topWeight {
			topWeight = item.Weight
			topIndustry = code
		}
	}

	hhi := herfindahlIndex(weights)
	effectiveCount := 0.0
	if hhi > 0 {
		effectiveCount = 1.0 / hhi
	}

	isConcentrated := topWeight > m.Config.MaxIndustryWeight
	severity := "normal"
	if topWeight > 0.60 {
		severity = "critical"
	} else if isConcentrated {
		severity = "warning"
	}

	return ConcentrationResult{
		Type:           ConcentrationIndustry,
		HHI:            hhi,
		EffectiveCount: effectiveCount,
		IsConcentrated: isConcentrated,
		Severity:       severity,
		TopContributor: topIndustry,
		TopWeight:      topWeight,
		Threshold:      m.Config.MaxIndustryWeight,
		Breakdown:      weights,
		Description: fmt.Sprintf("行业集中度: HHI=%.3f, 有效行业=%.1f, 最大行业 %s (%.1f%%) [%s]",
			hhi, effectiveCount, topIndustry, topWeight*100, severity),
		DetectedAt: time.Now(),
	}
}

// MonitorPositionConcentration 监控整体持仓集中度 (HHI)
func (m *ConcentrationMonitor) MonitorPositionConcentration(positions []PositionItem) ConcentrationResult {
	if len(positions) == 0 {
		return ConcentrationResult{
			Type:        ConcentrationPosition,
			Description: "无持仓数据",
			DetectedAt:  time.Now(),
		}
	}

	weights := make(map[string]float64)
	for _, p := range positions {
		weights[p.Code] = p.Weight
	}

	hhi := herfindahlIndex(weights)
	effectiveCount := 0.0
	if hhi > 0 {
		effectiveCount = 1.0 / hhi
	}

	// 低有效标的数也触发警告
	isConcentrated := hhi > m.Config.HHIWarningThreshold || effectiveCount < float64(m.Config.MinEffectiveCount)
	severity := "normal"
	if hhi > m.Config.HHIDangerThreshold || effectiveCount < 3 {
		severity = "critical"
	} else if isConcentrated {
		severity = "warning"
	}

	// 找最大贡献者
	var topCode string
	var topWeight float64
	for code, w := range weights {
		if w > topWeight {
			topWeight = w
			topCode = code
		}
	}

	return ConcentrationResult{
		Type:           ConcentrationPosition,
		HHI:            hhi,
		EffectiveCount: effectiveCount,
		IsConcentrated: isConcentrated,
		Severity:       severity,
		TopContributor: topCode,
		TopWeight:      topWeight,
		Threshold:      m.Config.HHIWarningThreshold,
		Breakdown:      weights,
		Description: fmt.Sprintf("整体集中度: HHI=%.3f, 有效标的=%.1f (最少 %d) [%s]",
			hhi, effectiveCount, m.Config.MinEffectiveCount, severity),
		DetectedAt: time.Now(),
	}
}

// CalculateCorrelationClustering 计算相关性聚类 (简化版)
// 基于收益率相关性矩阵检测持仓是否过度集中在相关资产
func (m *ConcentrationMonitor) CalculateCorrelationClustering(returns [][]float64) CorrelationResult {
	result := CorrelationResult{
		Clusters:       make([]CorrelationCluster, 0),
		HighCorrPairs:  make([]CorrelatedPair, 0),
		AvgCorrelation: 0,
		IsClustered:    false,
		Severity:       "normal",
	}

	if len(returns) < 3 || len(returns[0]) < 5 {
		result.Description = "数据不足, 无法计算相关性聚类"
		return result
	}

	n := len(returns)
	var totalCorr float64
	var pairCount int
	var maxCorr float64
	var maxI, maxJ int

	// 计算相关矩阵
	corrMatrix := make([][]float64, n)
	for i := 0; i < n; i++ {
		corrMatrix[i] = make([]float64, n)
		corrMatrix[i][i] = 1.0
		for j := i + 1; j < n; j++ {
			corr := pearsonCorrelation(returns[i], returns[j])
			corrMatrix[i][j] = corr
			corrMatrix[j][i] = corr

			totalCorr += math.Abs(corr)
			pairCount++

			if math.Abs(corr) > maxCorr {
				maxCorr = math.Abs(corr)
				maxI = i
				maxJ = j
			}

			if math.Abs(corr) > 0.7 {
				result.HighCorrPairs = append(result.HighCorrPairs, CorrelatedPair{
					I: i, J: j, Correlation: corr,
				})
			}
		}
	}

	if pairCount > 0 {
		result.AvgCorrelation = totalCorr / float64(pairCount)
	}

	// 简化聚类: 基于高相关性分组
	if len(result.HighCorrPairs) > 0 {
		result.IsClustered = true
		result.Severity = "warning"

		// 简单聚类: 传递闭包
		visited := make([]bool, n)
		for _, pair := range result.HighCorrPairs {
			if !visited[pair.I] || !visited[pair.J] {
				cluster := CorrelationCluster{}
				cluster.Members = append(cluster.Members, pair.I, pair.J)
				visited[pair.I] = true
				visited[pair.J] = true
				result.Clusters = append(result.Clusters, cluster)
			}
		}

		if len(result.HighCorrPairs) > n/2 {
			result.Severity = "critical"
		}
	}

	result.Description = fmt.Sprintf("相关性监控: 平均相关性 %.3f, 最大相关性 %.3f (assets %d-%d), 高相关对 %d 个",
		result.AvgCorrelation, maxCorr, maxI, maxJ, len(result.HighCorrPairs))

	return result
}

// CorrelationResult 相关性聚类结果
type CorrelationResult struct {
	Clusters       []CorrelationCluster `json:"clusters"`
	HighCorrPairs  []CorrelatedPair     `json:"high_corr_pairs"`
	AvgCorrelation float64              `json:"avg_correlation"`
	MaxCorrelation float64              `json:"max_correlation"`
	IsClustered    bool                 `json:"is_clustered"`
	Severity       string               `json:"severity"`
	Description    string               `json:"description"`
}

// CorrelationCluster 相关聚类
type CorrelationCluster struct {
	Members []int   `json:"members"`
	AvgCorr float64 `json:"avg_correlation"`
}

// CorrelatedPair 高相关对
type CorrelatedPair struct {
	I           int     `json:"i"`
	J           int     `json:"j"`
	Correlation float64 `json:"correlation"`
}

// ============================================================================
// 统计辅助函数
// ============================================================================

// herfindahlIndex 计算赫芬达尔指数 (0-1)
// weights: {key: weight}
// HHI = sum(w_i^2)
func herfindahlIndex(weights map[string]float64) float64 {
	var hhi float64
	for _, w := range weights {
		hhi += w * w
	}
	// 归一化: 如果权重和不等于 1, 进行调整
	var sum float64
	for _, w := range weights {
		sum += w
	}
	if sum > 0 && math.Abs(sum-1.0) > 0.001 {
		// 权重未归一化, 调整 HHI
		// HHI_normalized = HHI / sum^2
		hhi = hhi / (sum * sum)
	}
	return hhi
}

// pearsonCorrelation 计算 Pearson 相关系数
func pearsonCorrelation(x, y []float64) float64 {
	n := len(x)
	if n < 2 || len(y) != n {
		return 0
	}

	mx := mean(x)
	my := mean(y)

	var num, dx, dy float64
	for i := 0; i < n; i++ {
		dx = x[i] - mx
		dy = y[i] - my
		num += dx * dy
	}

	var dxSum, dySum float64
	for i := 0; i < n; i++ {
		dxSum += (x[i] - mx) * (x[i] - mx)
		dySum += (y[i] - my) * (y[i] - my)
	}

	den := math.Sqrt(dxSum * dySum)
	if den == 0 {
		return 0
	}
	return num / den
}
