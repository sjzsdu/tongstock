package paradigms

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// ============================================================================
// 过拟合防护架构
// ============================================================================

// OverfitProtection 过拟合防护结果
type OverfitProtection struct {
	BootstrapResult     *BootstrapResult    `json:"bootstrap_result"`
	PermutationResult   *PermutationResult  `json:"permutation_result"`
	PerturbationResult  *PerturbationResult `json:"perturbation_result"`
	CorrectedPValue     float64             `json:"corrected_p_value"`      // 校正后 p-value
	SearchSize          int                 `json:"search_size"`            // 搜索规模
	EffectiveTrials     int                 `json:"effective_trials"`       // 有效试验数
	FamilyWiseErrorRate float64             `json:"family_wise_error_rate"` // 族错误率
	BenjaminiHochberg   float64             `json:"benjamini_hochberg"`     // BH 校正后值
	IsOverfit           bool                `json:"is_overfit"`             // 是否过拟合
	Reject              bool                `json:"reject"`                 // 是否拒绝
	Reason              string              `json:"reason"`                 // 拒绝原因
}

// ============================================================================
// Bootstrap 重采样检验
// ============================================================================

// BootstrapResult Bootstrap 检验结果
type BootstrapResult struct {
	OriginalStatistic float64    `json:"original_statistic"` // 原始统计量
	BootstrapMean     float64    `json:"bootstrap_mean"`     // Bootstrap 均值
	BootstrapStd      float64    `json:"bootstrap_std"`      // Bootstrap 标准差
	BootstrapCI       [2]float64 `json:"bootstrap_ci"`       // 置信区间
	SE                float64    `json:"se"`                 // 标准误
	Bias              float64    `json:"bias"`               // 偏差
	PValue            float64    `json:"p_value"`            // Bootstrap p-value
	StabilityScore    float64    `json:"stability_score"`    // 稳定性评分 (0-1)
	Iterations        int        `json:"iterations"`         // Bootstrap 次数
}

// BootstrapValidator Bootstrap 验证器
type BootstrapValidator struct {
	iterations int
	rng        *rand.Rand
}

// NewBootstrapValidator 创建 Bootstrap 验证器
func NewBootstrapValidator(iterations int) *BootstrapValidator {
	if iterations < 100 {
		iterations = 100
	}
	return &BootstrapValidator{
		iterations: iterations,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Validate 执行 Bootstrap 检验
func (bv *BootstrapValidator) Validate(data []float64, statisticFn func([]float64) float64) *BootstrapResult {
	result := &BootstrapResult{
		Iterations: bv.iterations,
	}

	if len(data) < 5 {
		result.PValue = 1.0
		result.StabilityScore = 0.0
		return result
	}

	// 计算原始统计量
	result.OriginalStatistic = statisticFn(data)

	// Bootstrap 重采样
	bootstrapSamples := make([]float64, bv.iterations)
	resampleSize := len(data)

	for i := 0; i < bv.iterations; i++ {
		resampled := make([]float64, resampleSize)
		for j := 0; j < resampleSize; j++ {
			resampled[j] = data[bv.rng.Intn(len(data))]
		}
		bootstrapSamples[i] = statisticFn(resampled)
	}

	// 计算 Bootstrap 统计
	result.BootstrapMean = mean(bootstrapSamples)
	result.BootstrapStd = stddev(bootstrapSamples)

	// 计算置信区间
	result.BootstrapCI = percentileInterval(bootstrapSamples, 0.025, 0.975)

	// 计算标准误和偏差
	result.SE = result.BootstrapStd / math.Sqrt(float64(len(data)))
	result.Bias = result.BootstrapMean - result.OriginalStatistic

	// 计算 p-value (基于 bootstrap 分布)
	countExtreme := 0
	for _, bs := range bootstrapSamples {
		if math.Abs(bs) >= math.Abs(result.OriginalStatistic) {
			countExtreme++
		}
	}
	result.PValue = float64(countExtreme) / float64(bv.iterations)

	// 计算稳定性评分 (基于变异系数和偏差)
	if result.BootstrapMean != 0 {
		cv := result.BootstrapStd / math.Abs(result.BootstrapMean)
		biasRatio := math.Abs(result.Bias) / math.Abs(result.OriginalStatistic)
		result.StabilityScore = 1.0 / (1.0 + cv + biasRatio)
	} else {
		result.StabilityScore = 0.5
	}

	return result
}

// ValidateReturn 针对收益率序列的 Bootstrap 检验
func (bv *BootstrapValidator) ValidateReturn(returns []float64) *BootstrapResult {
	return bv.Validate(returns, func(data []float64) float64 {
		return mean(data) / stddev(data) // Sharpe-like statistic
	})
}

// ============================================================================
// 置换检验
// ============================================================================

// PermutationResult 置换检验结果
type PermutationResult struct {
	OriginalStatistic float64 `json:"original_statistic"` // 原始统计量
	PermutationMean   float64 `json:"permutation_mean"`   // 置换均值
	PermutationStd    float64 `json:"permutation_std"`    // 置换标准差
	PValue            float64 `json:"p_value"`            // 置换 p-value
	Significance      bool    `json:"significance"`       // 是否显著
	Permutations      int     `json:"permutations"`       // 置换次数
	Confidence        float64 `json:"confidence"`         // 置信度 (1-alpha)
}

// PermutationTest 置换检验
type PermutationTest struct {
	permutations int
	alpha        float64
	rng          *rand.Rand
}

// NewPermutationTest 创建置换检验
func NewPermutationTest(permutations int, alpha float64) *PermutationTest {
	if permutations < 100 {
		permutations = 100
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.05
	}
	return &PermutationTest{
		permutations: permutations,
		alpha:        alpha,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Test 执行置换检验
func (pt *PermutationTest) Test(data []float64, labels []bool, statisticFn func([]float64, []bool) float64) *PermutationResult {
	result := &PermutationResult{
		Permutations: pt.permutations,
		Confidence:   1.0 - pt.alpha,
	}

	if len(data) < 5 || len(data) != len(labels) {
		result.PValue = 1.0
		result.Significance = false
		return result
	}

	// 计算原始统计量
	result.OriginalStatistic = statisticFn(data, labels)

	// 置换检验
	permutationStats := make([]float64, pt.permutations)
	labelsCopy := make([]bool, len(labels))
	copy(labelsCopy, labels)

	for i := 0; i < pt.permutations; i++ {
		// 随机打乱标签
		for j := len(labelsCopy) - 1; j > 0; j-- {
			k := pt.rng.Intn(j + 1)
			labelsCopy[j], labelsCopy[k] = labelsCopy[k], labelsCopy[j]
		}
		permutationStats[i] = statisticFn(data, labelsCopy)
	}

	// 计算置换统计
	result.PermutationMean = mean(permutationStats)
	result.PermutationStd = stddev(permutationStats)

	// 计算 p-value
	countExtreme := 0
	for _, ps := range permutationStats {
		if math.Abs(ps) >= math.Abs(result.OriginalStatistic) {
			countExtreme++
		}
	}
	result.PValue = float64(countExtreme) / float64(pt.permutations)
	result.Significance = result.PValue < pt.alpha

	return result
}

// TestTwoGroups 两组比较置换检验
func (pt *PermutationTest) TestTwoGroups(group1, group2 []float64) *PermutationResult {
	// 合并数据
	combined := append(group1, group2...)
	labels := make([]bool, len(combined))
	for i := 0; i < len(group1); i++ {
		labels[i] = true
	}
	for i := len(group1); i < len(combined); i++ {
		labels[i] = false
	}

	statFn := func(data []float64, lbls []bool) float64 {
		var g1, g2 []float64
		for i, l := range lbls {
			if l {
				g1 = append(g1, data[i])
			} else {
				g2 = append(g2, data[i])
			}
		}
		if len(g1) == 0 || len(g2) == 0 {
			return 0
		}
		return mean(g1) - mean(g2)
	}

	return pt.Test(combined, labels, statFn)
}

// ============================================================================
// 参数扰动稳健性检验
// ============================================================================

// PerturbationResult 参数扰动检验结果
type PerturbationResult struct {
	OriginalParams   map[string]float64 `json:"original_params"`   // 原始参数
	PerturbedResults []float64          `json:"perturbed_results"` // 扰动后结果
	MeanPerformance  float64            `json:"mean_performance"`  // 平均表现
	StdPerformance   float64            `json:"std_performance"`   // 表现标准差
	WorstPerformance float64            `json:"worst_performance"` // 最差表现
	BestPerformance  float64            `json:"best_performance"`  // 最佳表现
	RobustnessScore  float64            `json:"robustness_score"`  // 稳健性评分 (0-1)
	Perturbations    int                `json:"perturbations"`     // 扰动次数
	Sigma            float64            `json:"sigma"`             // 扰动标准差
	Pass             bool               `json:"pass"`              // 是否通过
}

// ParameterPerturbation 参数扰动检验
type ParameterPerturbation struct {
	perturbations int
	sigma         float64
	rng           *rand.Rand
}

// NewParameterPerturbation 创建参数扰动检验
func NewParameterPerturbation(perturbations int, sigma float64) *ParameterPerturbation {
	if perturbations < 50 {
		perturbations = 50
	}
	if sigma <= 0 || sigma > 1 {
		sigma = 0.1
	}
	return &ParameterPerturbation{
		perturbations: perturbations,
		sigma:         sigma,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Test 执行参数扰动检验
func (pp *ParameterPerturbation) Test(originalParams map[string]float64, evalFn func(map[string]float64) float64) *PerturbationResult {
	result := &PerturbationResult{
		OriginalParams:   originalParams,
		Perturbations:    pp.perturbations,
		Sigma:            pp.sigma,
		PerturbedResults: make([]float64, pp.perturbations),
	}

	// 计算原始表现
	originalPerformance := evalFn(originalParams)

	// 执行参数扰动
	for i := 0; i < pp.perturbations; i++ {
		perturbed := make(map[string]float64)
		for k, v := range originalParams {
			// 添加高斯噪声
			noise := pp.rng.NormFloat64() * pp.sigma * math.Abs(v)
			perturbed[k] = v + noise
		}
		result.PerturbedResults[i] = evalFn(perturbed)
	}

	// 统计结果
	if len(result.PerturbedResults) > 0 {
		result.MeanPerformance = mean(result.PerturbedResults)
		result.StdPerformance = stddev(result.PerturbedResults)
		result.WorstPerformance = min(result.PerturbedResults)
		result.BestPerformance = max(result.PerturbedResults)
	}

	// 计算稳健性评分
	if originalPerformance != 0 {
		// 表现保持率 (相对于原始表现)
		retention := result.MeanPerformance / originalPerformance
		if retention > 1.0 {
			retention = 1.0
		}
		// 变异系数惩罚
		cv := 0.0
		if result.MeanPerformance != 0 {
			cv = result.StdPerformance / math.Abs(result.MeanPerformance)
		}
		// 综合稳健性
		result.RobustnessScore = retention * math.Exp(-cv)
	}

	// 判断是否通过 (至少 80% 扰动表现优于阈值)
	passThreshold := originalPerformance * 0.8
	passCount := 0
	for _, r := range result.PerturbedResults {
		if r >= passThreshold {
			passCount++
		}
	}
	result.Pass = float64(passCount)/float64(pp.perturbations) >= 0.7

	return result
}

// ============================================================================
// 多重检验校正
// ============================================================================

// MultipleTestingCorrection 多重检验校正
type MultipleTestingCorrection struct {
	method          string // "bonferroni", "bh", "by"
	alpha           float64
	effectiveTrials int
}

// NewMultipleTestingCorrection 创建多重检验校正
func NewMultipleTestingCorrection(method string, alpha float64) *MultipleTestingCorrection {
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.05
	}
	return &MultipleTestingCorrection{
		method:          method,
		alpha:           alpha,
		effectiveTrials: 1,
	}
}

// SetEffectiveTrials 设置有效试验数
func (mtc *MultipleTestingCorrection) SetEffectiveTrials(trials int) {
	if trials < 1 {
		trials = 1
	}
	mtc.effectiveTrials = trials
}

// BonferroniCorrection Bonferroni 校正
func (mtc *MultipleTestingCorrection) BonferroniCorrection(pValue float64) float64 {
	corrected := pValue * float64(mtc.effectiveTrials)
	if corrected > 1.0 {
		return 1.0
	}
	return corrected
}

// BenjaminiHochbergCorrection Benjamini-Hochberg 校正
func (mtc *MultipleTestingCorrection) BenjaminiHochbergCorrection(pValues []float64) []float64 {
	n := len(pValues)
	if n == 0 {
		return pValues
	}

	// 创建索引-值对
	type pValIndex struct {
		val   float64
		index int
	}
	sorted := make([]pValIndex, n)
	for i, v := range pValues {
		sorted[i] = pValIndex{val: v, index: i}
	}

	// 按 p-value 排序 (从小到大)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if sorted[j].val < sorted[i].val {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 计算 BH 校正后 p-value, 先按排序后顺序计算
	correctedSorted := make([]float64, n)
	for i := range sorted {
		correctedSorted[i] = sorted[i].val * float64(n) / float64(i+1)
	}

	// 保证单调性 (从大到小遍历, 确保每个值 >= 后面的值)
	for i := n - 2; i >= 0; i-- {
		if correctedSorted[i] > correctedSorted[i+1] {
			correctedSorted[i] = correctedSorted[i+1]
		}
	}

	// 限制在 [0, 1] 范围
	for i := range correctedSorted {
		if correctedSorted[i] > 1.0 {
			correctedSorted[i] = 1.0
		}
	}

	// 映射回原始顺序
	corrected := make([]float64, n)
	for i := range sorted {
		corrected[sorted[i].index] = correctedSorted[i]
	}

	return corrected
}

// BenjaminiYekutieliCorrection Benjamini-Yekutieli 校正 (考虑负相关)
func (mtc *MultipleTestingCorrection) BenjaminiYekutieliCorrection(pValues []float64) []float64 {
	n := len(pValues)
	if n == 0 {
		return pValues
	}

	// BH 校正
	bhCorrected := mtc.BenjaminiHochbergCorrection(pValues)

	// BY 校正: 增加一个因子 1/(ln(n) + gamma)
	byFactor := 1.0 / (math.Log(float64(n)) + 0.5772) // Euler-Mascheroni constant
	for i := range bhCorrected {
		bhCorrected[i] *= byFactor
		if bhCorrected[i] > 1.0 {
			bhCorrected[i] = 1.0
		}
	}

	return bhCorrected
}

// ApplyCorrection 应用校正
func (mtc *MultipleTestingCorrection) ApplyCorrection(pValues []float64) []float64 {
	switch mtc.method {
	case "bonferroni":
		result := make([]float64, len(pValues))
		for i, p := range pValues {
			result[i] = mtc.BonferroniCorrection(p)
		}
		return result
	case "by":
		return mtc.BenjaminiYekutieliCorrection(pValues)
	default: // "bh"
		return mtc.BenjaminiHochbergCorrection(pValues)
	}
}

// CalculateFamilyWiseErrorRate 计算族错误率
func (mtc *MultipleTestingCorrection) CalculateFamilyWiseErrorRate(perTestAlpha float64) float64 {
	// Bonferroni 不等式
	// FWER <= 1 - (1 - alpha)^n
	fwer := 1.0 - math.Pow(1.0-perTestAlpha, float64(mtc.effectiveTrials))
	return fwer
}

// ============================================================================
// 搜索规模记录器
// ============================================================================

// SearchRecord 搜索记录
type SearchRecord struct {
	ID              string    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	SearchType      string    `json:"search_type"`      // "grid", "random", "evolutionary", "manual"
	ParametersCount int       `json:"parameters_count"` // 搜索的参数数量
	TrialsCount     int       `json:"trials_count"`     // 尝试次数
	SuccessCount    int       `json:"success_count"`    // 成功次数
	BestScore       float64   `json:"best_score"`       // 最佳分数
	MeanScore       float64   `json:"mean_score"`       // 平均分数
	TimeSeconds     float64   `json:"time_seconds"`     // 耗时
}

// SearchLogger 搜索记录器
type SearchLogger struct {
	records []SearchRecord
	maxSize int
}

// NewSearchLogger 创建搜索记录器
func NewSearchLogger(maxSize int) *SearchLogger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &SearchLogger{
		records: make([]SearchRecord, 0),
		maxSize: maxSize,
	}
}

// Log 记录搜索
func (sl *SearchLogger) Log(record SearchRecord) {
	if len(sl.records) >= sl.maxSize {
		sl.records = append(sl.records[1:], record)
	} else {
		sl.records = append(sl.records, record)
	}
}

// GetRecords 获取所有记录
func (sl *SearchLogger) GetRecords() []SearchRecord {
	return sl.records
}

// GetTotalTrials 获取总尝试次数
func (sl *SearchLogger) GetTotalTrials() int {
	total := 0
	for _, r := range sl.records {
		total += r.TrialsCount
	}
	return total
}

// GetTotalSuccess 获取总成功次数
func (sl *SearchLogger) GetTotalSuccess() int {
	total := 0
	for _, r := range sl.records {
		total += r.SuccessCount
	}
	return total
}

// GetBestScore 获取历史最佳分数
func (sl *SearchLogger) GetBestScore() float64 {
	best := -math.MaxFloat64
	for _, r := range sl.records {
		if r.BestScore > best {
			best = r.BestScore
		}
	}
	if best == -math.MaxFloat64 {
		return 0
	}
	return best
}

// GetEffectiveTrials 获取有效试验数 (按独特参数组合估计)
func (sl *SearchLogger) GetEffectiveTrials() int {
	// 简化估计: 使用总尝试次数
	return sl.GetTotalTrials()
}

// Reset 重置记录
func (sl *SearchLogger) Reset() {
	sl.records = make([]SearchRecord, 0)
}

// ============================================================================
// 过拟合防护引擎
// ============================================================================

// OverfitEngine 过拟合防护引擎
type OverfitEngine struct {
	bootstrap       *BootstrapValidator
	permutation     *PermutationTest
	perturbation    *ParameterPerturbation
	correction      *MultipleTestingCorrection
	logger          *SearchLogger
	minBootstrap    int
	minPermutation  int
	minPerturbation int
}

// NewOverfitEngine 创建过拟合防护引擎
func NewOverfitEngine() *OverfitEngine {
	return &OverfitEngine{
		bootstrap:       NewBootstrapValidator(500),
		permutation:     NewPermutationTest(1000, 0.05),
		perturbation:    NewParameterPerturbation(200, 0.1),
		correction:      NewMultipleTestingCorrection("bh", 0.05),
		logger:          NewSearchLogger(1000),
		minBootstrap:    100,
		minPermutation:  200,
		minPerturbation: 50,
	}
}

// SetIterations 设置检验迭代次数
func (oe *OverfitEngine) SetIterations(bootstrap, permutation, perturbation int) {
	oe.bootstrap = NewBootstrapValidator(bootstrap)
	oe.permutation = NewPermutationTest(permutation, 0.05)
	oe.perturbation = NewParameterPerturbation(perturbation, 0.1)
}

// Protect 执行完整过拟合防护检验
func (oe *OverfitEngine) Protect(
	data []float64,
	labels []bool,
	originalParams map[string]float64,
	statisticFn func([]float64) float64,
	evalFn func(map[string]float64) float64,
) *OverfitProtection {
	protection := &OverfitProtection{
		SearchSize:      oe.logger.GetTotalTrials(),
		EffectiveTrials: oe.correction.effectiveTrials,
	}

	// 1. Bootstrap 检验
	protection.BootstrapResult = oe.bootstrap.Validate(data, statisticFn)

	// 2. 置换检验
	protection.PermutationResult = oe.permutation.Test(data, labels, func(data []float64, labels []bool) float64 {
		return statisticFn(data)
	})

	// 3. 参数扰动检验
	protection.PerturbationResult = oe.perturbation.Test(originalParams, evalFn)

	// 4. 多重检验校正
	pValues := []float64{
		protection.BootstrapResult.PValue,
		protection.PermutationResult.PValue,
	}
	correctedPValues := oe.correction.ApplyCorrection(pValues)
	protection.CorrectedPValue = correctedPValues[0] // 使用 Bootstrap 校正后值

	// 5. 计算族错误率
	protection.FamilyWiseErrorRate = oe.correction.CalculateFamilyWiseErrorRate(protection.BootstrapResult.PValue)
	protection.BenjaminiHochberg = correctedPValues[0]

	// 6. 综合判定
	protection.IsOverfit = oe.isOverfit(protection)
	protection.Reject = protection.IsOverfit
	if protection.Reject {
		protection.Reason = oe.getRejectReason(protection)
	} else {
		protection.Reason = "All checks passed"
	}

	return protection
}

// isOverfit 判断是否过拟合
func (oe *OverfitEngine) isOverfit(protection *OverfitProtection) bool {
	// 检查 1: Bootstrap 显著性
	if protection.BootstrapResult != nil && protection.BootstrapResult.PValue >= 0.05 {
		// 检查 2: 参数扰动稳定性
		if protection.PerturbationResult != nil && protection.PerturbationResult.RobustnessScore < 0.5 {
			return true
		}
	}

	// 检查 3: 置换检验不显著
	if protection.PermutationResult != nil && !protection.PermutationResult.Significance {
		// 如果所有检验都不显著，可能过拟合
		if protection.BootstrapResult != nil && protection.BootstrapResult.PValue >= 0.1 {
			return true
		}
	}

	// 检查 4: 高搜索规模下的显著性
	if protection.SearchSize > 1000 && protection.CorrectedPValue >= 0.01 {
		return true
	}

	return false
}

// getRejectReason 获取拒绝原因
func (oe *OverfitEngine) getRejectReason(protection *OverfitProtection) string {
	var reasons []string

	if protection.BootstrapResult != nil && protection.BootstrapResult.PValue >= 0.05 {
		reasons = append(reasons, fmt.Sprintf("Bootstrap p-value=%.3f >= 0.05", protection.BootstrapResult.PValue))
	}

	if protection.PermutationResult != nil && !protection.PermutationResult.Significance {
		reasons = append(reasons, fmt.Sprintf("Permutation p-value=%.3f >= alpha", protection.PermutationResult.PValue))
	}

	if protection.PerturbationResult != nil && protection.PerturbationResult.RobustnessScore < 0.5 {
		reasons = append(reasons, fmt.Sprintf("Robustness score=%.3f < 0.5", protection.PerturbationResult.RobustnessScore))
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Overfit detected by multiple criteria")
	}

	return fmt.Sprintf("Overfit rejected: %s", joinStrings(reasons, "; "))
}

// ============================================================================
// 辅助函数
// ============================================================================

func mean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func stddev(data []float64) float64 {
	if len(data) < 2 {
		return 0
	}
	m := mean(data)
	variance := 0.0
	for _, v := range data {
		diff := v - m
		variance += diff * diff
	}
	return math.Sqrt(variance / float64(len(data)-1))
}

func min(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func percentileInterval(data []float64, lowerPct, upperPct float64) [2]float64 {
	if len(data) == 0 {
		return [2]float64{0, 0}
	}

	// 排序
	sorted := make([]float64, len(data))
	copy(sorted, data)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 计算百分位数
	lowerIdx := int(float64(len(sorted)) * lowerPct)
	upperIdx := int(float64(len(sorted)) * upperPct)
	if lowerIdx >= len(sorted) {
		lowerIdx = len(sorted) - 1
	}
	if upperIdx >= len(sorted) {
		upperIdx = len(sorted) - 1
	}

	return [2]float64{sorted[lowerIdx], sorted[upperIdx]}
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
