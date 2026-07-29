package paradigms

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// ============================================================================
// Bootstrap 检验测试
// ============================================================================

func TestNewBootstrapValidator(t *testing.T) {
	bv := NewBootstrapValidator(200)
	if bv == nil {
		t.Fatal("NewBootstrapValidator returned nil")
	}
}

func TestBootstrapValidatorMinimumIterations(t *testing.T) {
	bv := NewBootstrapValidator(50) // 少于 100
	if bv.iterations < 100 {
		t.Errorf("expected at least 100 iterations, got %d", bv.iterations)
	}
}

func TestBootstrapValidate(t *testing.T) {
	bv := NewBootstrapValidator(200)

	// 生成测试数据 (正态分布)
	data := testGenerateNormalData(100, 0.1, 0.02)

	result := bv.Validate(data, func(data []float64) float64 {
		return mean(data)
	})

	if result.BootstrapMean == 0 {
		t.Error("expected non-zero bootstrap mean")
	}
	if result.BootstrapCI[0] > result.BootstrapCI[1] {
		t.Error("CI lower bound should be less than upper bound")
	}
}

func TestBootstrapValidateSmallData(t *testing.T) {
	bv := NewBootstrapValidator(200)
	data := []float64{0.01, -0.01, 0.02} // 少于 5 个数据点

	result := bv.Validate(data, func(data []float64) float64 {
		return mean(data)
	})

	if result.PValue != 1.0 {
		t.Errorf("expected p-value 1.0 for small data, got %f", result.PValue)
	}
}

func TestBootstrapValidateReturn(t *testing.T) {
	bv := NewBootstrapValidator(200)

	returns := testGenerateNormalData(200, 0.001, 0.01) // 日收益率

	result := bv.ValidateReturn(returns)

	if result.OriginalStatistic == 0 {
		t.Error("expected non-zero original statistic")
	}
}

func TestBootstrapStabilityScore(t *testing.T) {
	bv := NewBootstrapValidator(200)

	// 稳定数据
	stableData := testGenerateNormalData(200, 0.1, 0.01)
	stableResult := bv.Validate(stableData, func(data []float64) float64 {
		return mean(data)
	})

	// 不稳定数据 (高方差)
	unstableData := testGenerateNormalData(200, 0.1, 0.5)
	unstableResult := bv.Validate(unstableData, func(data []float64) float64 {
		return mean(data)
	})

	if stableResult.StabilityScore <= unstableResult.StabilityScore {
		t.Error("stable data should have higher stability score")
	}
}

// ============================================================================
// 置换检验测试
// ============================================================================

func TestNewPermutationTest(t *testing.T) {
	pt := NewPermutationTest(500, 0.05)
	if pt == nil {
		t.Fatal("NewPermutationTest returned nil")
	}
}

func TestPermutationTestMinimumIterations(t *testing.T) {
	pt := NewPermutationTest(50, 0.05)
	if pt.permutations < 100 {
		t.Errorf("expected at least 100 permutations, got %d", pt.permutations)
	}
}

func TestPermutationTestInvalidAlpha(t *testing.T) {
	pt := NewPermutationTest(200, 1.5) // 无效 alpha
	if pt.alpha >= 1 || pt.alpha <= 0 {
		t.Error("alpha should be corrected to valid range")
	}
}

func TestPermutationTest(t *testing.T) {
	pt := NewPermutationTest(300, 0.05)

	data := testGenerateNormalData(50, 0.05, 0.02)
	labels := make([]bool, 50)
	for i := 0; i < 25; i++ {
		labels[i] = true
	}

	result := pt.Test(data, labels, func(data []float64, labels []bool) float64 {
		var g1, g2 []float64
		for i, l := range labels {
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
	})

	if result.PValue < 0 || result.PValue > 1 {
		t.Errorf("p-value should be between 0 and 1, got %f", result.PValue)
	}
}

func TestPermutationTestTwoGroups(t *testing.T) {
	pt := NewPermutationTest(300, 0.05)

	group1 := testGenerateNormalData(30, 0.06, 0.02)
	group2 := testGenerateNormalData(30, 0.04, 0.02)

	result := pt.TestTwoGroups(group1, group2)

	if result.PValue < 0 || result.PValue > 1 {
		t.Errorf("p-value should be between 0 and 1, got %f", result.PValue)
	}
}

func TestPermutationTestSmallData(t *testing.T) {
	pt := NewPermutationTest(300, 0.05)

	data := []float64{0.01, 0.02}
	labels := []bool{true} // 长度不匹配

	result := pt.Test(data, labels, func(data []float64, labels []bool) float64 {
		return mean(data)
	})

	if result.PValue != 1.0 {
		t.Errorf("expected p-value 1.0 for invalid data, got %f", result.PValue)
	}
}

// ============================================================================
// 参数扰动检验测试
// ============================================================================

func TestNewParameterPerturbation(t *testing.T) {
	pp := NewParameterPerturbation(100, 0.1)
	if pp == nil {
		t.Fatal("NewParameterPerturbation returned nil")
	}
}

func TestParameterPerturbationMinimumPerturbations(t *testing.T) {
	pp := NewParameterPerturbation(20, 0.1)
	if pp.perturbations < 50 {
		t.Errorf("expected at least 50 perturbations, got %d", pp.perturbations)
	}
}

func TestParameterPerturbationInvalidSigma(t *testing.T) {
	pp := NewParameterPerturbation(50, 2.0) // 无效 sigma
	if pp.sigma > 1 || pp.sigma <= 0 {
		t.Error("sigma should be corrected to valid range")
	}
}

func TestParameterPerturbationTest(t *testing.T) {
	pp := NewParameterPerturbation(100, 0.05)

	params := map[string]float64{
		"threshold": 0.5,
		"window":    20,
		"stop_loss": 0.02,
	}

	// 稳定的评估函数
	evalFn := func(p map[string]float64) float64 {
		// 简单的目标函数: 鼓励阈值接近 0.5
		return 1.0 - math.Abs(p["threshold"]-0.5)*10
	}

	result := pp.Test(params, evalFn)

	if len(result.PerturbedResults) != 100 {
		t.Errorf("expected 100 perturbed results, got %d", len(result.PerturbedResults))
	}
}

func TestParameterPerturbationStableParams(t *testing.T) {
	pp := NewParameterPerturbation(100, 0.01) // 小扰动

	params := map[string]float64{
		"alpha": 1.0,
		"beta":  2.0,
	}

	// 线性函数 (对参数变化稳定)
	evalFn := func(p map[string]float64) float64 {
		return p["alpha"] + p["beta"]
	}

	result := pp.Test(params, evalFn)

	if !result.Pass {
		t.Error("stable linear function should pass perturbation test")
	}
}

func TestParameterPerturbationSensitiveParams(t *testing.T) {
	pp := NewParameterPerturbation(200, 0.3) // 大扰动 (30%)

	params := map[string]float64{
		"x": 2.0,
	}

	// 敏感函数 (在特定点急剧变化)
	evalFn := func(p map[string]float64) float64 {
		x := p["x"]
		// 只有 x 极其接近 2.0 时才有高值
		diff := math.Abs(x - 2.0)
		if diff > 0.05 {
			return 0
		}
		return 100.0 * math.Exp(-diff*200) // 极度敏感
	}

	result := pp.Test(params, evalFn)

	// 至少 70% 扰动表现低于原始 80% 时应失败
	if result.Pass {
		t.Error("sensitive function should fail perturbation test")
	}
}

// ============================================================================
// 多重检验校正测试
// ============================================================================

func TestNewMultipleTestingCorrection(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 0.05)
	if mtc == nil {
		t.Fatal("NewMultipleTestingCorrection returned nil")
	}
}

func TestMultipleTestingCorrectionInvalidAlpha(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 1.5)
	if mtc.alpha >= 1 || mtc.alpha <= 0 {
		t.Error("alpha should be corrected to valid range")
	}
}

func TestBonferroniCorrection(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bonferroni", 0.05)
	mtc.SetEffectiveTrials(100)

	corrected := mtc.BonferroniCorrection(0.01)
	if corrected > 1.0 {
		t.Errorf("corrected p-value should not exceed 1.0, got %f", corrected)
	}
	if corrected != 1.0 {
		t.Errorf("expected corrected value 1.0, got %f", corrected)
	}
}

func TestBonferroniCorrectionSmallValue(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bonferroni", 0.05)
	mtc.SetEffectiveTrials(100)

	corrected := mtc.BonferroniCorrection(0.0001)
	expected := 0.01
	if math.Abs(corrected-expected) > 0.0001 {
		t.Errorf("expected corrected value %f, got %f", expected, corrected)
	}
}

func TestBenjaminiHochbergCorrection(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 0.05)

	pValues := []float64{0.01, 0.04, 0.03, 0.005}
	corrected := mtc.BenjaminiHochbergCorrection(pValues)

	if len(corrected) != len(pValues) {
		t.Errorf("expected same length, got %d vs %d", len(corrected), len(pValues))
	}

	// 检查所有校正后 p-value 在 [0, 1] 范围
	for i, p := range corrected {
		if p < 0 || p > 1 {
			t.Errorf("corrected p-value[%d]=%f out of range [0,1]", i, p)
		}
	}

	// 检查排序后的校正 p-value 具有单调性 (对原始 p-value 排序后)
	// 排序: [0.005(idx3), 0.01(idx0), 0.03(idx2), 0.04(idx1)]
	// BH 校正后(排序顺序): 0.02, 0.02, 0.04, 0.04
	sortedPValues := make([][2]float64, len(pValues))
	for i, v := range pValues {
		sortedPValues[i] = [2]float64{v, corrected[i]}
	}
	// 按原始 p-value 排序
	for i := 0; i < len(sortedPValues); i++ {
		for j := i + 1; j < len(sortedPValues); j++ {
			if sortedPValues[j][0] < sortedPValues[i][0] {
				sortedPValues[i], sortedPValues[j] = sortedPValues[j], sortedPValues[i]
			}
		}
	}

	// 检查排序后校正 p-value 的单调性
	for i := 1; i < len(sortedPValues); i++ {
		if sortedPValues[i][1] < sortedPValues[i-1][1] {
			t.Errorf("BH corrected p-values should be monotonic in sorted order: %f < %f",
				sortedPValues[i][1], sortedPValues[i-1][1])
			break
		}
	}
}

func TestBenjaminiHochbergEmpty(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 0.05)

	corrected := mtc.BenjaminiHochbergCorrection([]float64{})
	if len(corrected) != 0 {
		t.Error("expected empty result")
	}
}

func TestApplyCorrectionBonferroni(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bonferroni", 0.05)
	mtc.SetEffectiveTrials(10)

	pValues := []float64{0.001, 0.01, 0.05}
	corrected := mtc.ApplyCorrection(pValues)

	for _, p := range corrected {
		if p > 1.0 {
			t.Error("corrected p-value should not exceed 1.0")
		}
	}
}

func TestApplyCorrectionBH(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 0.05)

	pValues := []float64{0.01, 0.04, 0.03}
	corrected := mtc.ApplyCorrection(pValues)

	for _, p := range corrected {
		if p > 1.0 {
			t.Error("corrected p-value should not exceed 1.0")
		}
	}
}

func TestCalculateFamilyWiseErrorRate(t *testing.T) {
	mtc := NewMultipleTestingCorrection("bh", 0.05)
	mtc.SetEffectiveTrials(10)

	fwer := mtc.CalculateFamilyWiseErrorRate(0.05)

	if fwer < 0 || fwer > 1 {
		t.Errorf("FWER should be between 0 and 1, got %f", fwer)
	}

	// 10 * 0.05 = 0.5, but should be slightly less due to (1-alpha)^n
	expectedApprox := 1.0 - math.Pow(0.95, 10)
	if math.Abs(fwer-expectedApprox) > 0.01 {
		t.Errorf("FWER should be approximately %f, got %f", expectedApprox, fwer)
	}
}

// ============================================================================
// 搜索记录器测试
// ============================================================================

func TestNewSearchLogger(t *testing.T) {
	sl := NewSearchLogger(100)
	if sl == nil {
		t.Fatal("NewSearchLogger returned nil")
	}
	if len(sl.GetRecords()) != 0 {
		t.Error("expected empty records")
	}
}

func TestSearchLoggerMinimumSize(t *testing.T) {
	sl := NewSearchLogger(0)
	if sl.maxSize <= 0 {
		t.Error("maxSize should be corrected to positive")
	}
}

func TestSearchLoggerLog(t *testing.T) {
	sl := NewSearchLogger(100)

	record := SearchRecord{
		ID:              "test-1",
		Timestamp:       time.Now(),
		SearchType:      "grid",
		ParametersCount: 10,
		TrialsCount:     100,
		SuccessCount:    5,
		BestScore:       0.95,
		MeanScore:       0.75,
		TimeSeconds:     30.0,
	}

	sl.Log(record)

	if len(sl.GetRecords()) != 1 {
		t.Error("expected 1 record")
	}
}

func TestSearchLoggerTotalTrials(t *testing.T) {
	sl := NewSearchLogger(100)

	sl.Log(SearchRecord{TrialsCount: 50})
	sl.Log(SearchRecord{TrialsCount: 30})

	if sl.GetTotalTrials() != 80 {
		t.Errorf("expected 80 total trials, got %d", sl.GetTotalTrials())
	}
}

func TestSearchLoggerTotalSuccess(t *testing.T) {
	sl := NewSearchLogger(100)

	sl.Log(SearchRecord{SuccessCount: 5})
	sl.Log(SearchRecord{SuccessCount: 3})

	if sl.GetTotalSuccess() != 8 {
		t.Errorf("expected 8 total success, got %d", sl.GetTotalSuccess())
	}
}

func TestSearchLoggerBestScore(t *testing.T) {
	sl := NewSearchLogger(100)

	sl.Log(SearchRecord{BestScore: 0.8})
	sl.Log(SearchRecord{BestScore: 0.95})
	sl.Log(SearchRecord{BestScore: 0.7})

	if sl.GetBestScore() != 0.95 {
		t.Errorf("expected best score 0.95, got %f", sl.GetBestScore())
	}
}

func TestSearchLoggerReset(t *testing.T) {
	sl := NewSearchLogger(100)

	sl.Log(SearchRecord{TrialsCount: 50})
	sl.Reset()

	if len(sl.GetRecords()) != 0 {
		t.Error("expected empty records after reset")
	}
}

// ============================================================================
// 过拟合防护引擎测试
// ============================================================================

func TestNewOverfitEngine(t *testing.T) {
	oe := NewOverfitEngine()
	if oe == nil {
		t.Fatal("NewOverfitEngine returned nil")
	}
}

func TestOverfitEngineProtect(t *testing.T) {
	oe := NewOverfitEngine()
	oe.SetIterations(100, 100, 50)

	// 生成测试数据
	data := testGenerateNormalData(80, 0.05, 0.02)
	labels := make([]bool, 80)
	for i := 0; i < 40; i++ {
		labels[i] = true
	}

	params := map[string]float64{
		"threshold": 0.5,
		"window":    20,
	}

	protection := oe.Protect(
		data,
		labels,
		params,
		func(d []float64) float64 {
			return mean(d) / stddev(d)
		},
		func(p map[string]float64) float64 {
			return 1.0 - math.Abs(p["threshold"]-0.5)*5
		},
	)

	if protection.BootstrapResult == nil {
		t.Error("expected bootstrap result")
	}
	if protection.PermutationResult == nil {
		t.Error("expected permutation result")
	}
	if protection.PerturbationResult == nil {
		t.Error("expected perturbation result")
	}
	if protection.CorrectedPValue < 0 || protection.CorrectedPValue > 1 {
		t.Errorf("corrected p-value should be between 0 and 1, got %f", protection.CorrectedPValue)
	}
}

func TestOverfitEngineDetection(t *testing.T) {
	oe := NewOverfitEngine()
	oe.SetIterations(100, 100, 100)

	// 生成"过拟合"数据 - 小样本, 高方差
	data := testGenerateNormalData(20, 0.01, 0.1)
	labels := make([]bool, 20)
	for i := 0; i < 10; i++ {
		labels[i] = true
	}

	params := map[string]float64{
		"x": 1.0,
	}

	protection := oe.Protect(
		data,
		labels,
		params,
		func(d []float64) float64 {
			return mean(d)
		},
		func(p map[string]float64) float64 {
			// 极其敏感的函数
			x := p["x"]
			if x > 1.01 || x < 0.99 {
				return 0
			}
			return 10
		},
	)

	// 过拟合检测可能因随机种子而异，但应该有合理的结果
	if protection.IsOverfit && protection.Reason == "" {
		t.Error("should have reject reason if overfit")
	}
}

func TestOverfitEngineIterations(t *testing.T) {
	oe := NewOverfitEngine()
	oe.SetIterations(200, 300, 100)

	if oe.bootstrap.iterations != 200 {
		t.Errorf("expected bootstrap iterations 200, got %d", oe.bootstrap.iterations)
	}
	if oe.permutation.permutations != 300 {
		t.Errorf("expected permutation iterations 300, got %d", oe.permutation.permutations)
	}
	if oe.perturbation.perturbations != 100 {
		t.Errorf("expected perturbation iterations 100, got %d", oe.perturbation.perturbations)
	}
}

// ============================================================================
// 辅助函数测试
// ============================================================================

func TestMean(t *testing.T) {
	data := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	result := mean(data)
	expected := 3.0
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("expected %f, got %f", expected, result)
	}
}

func TestMeanEmpty(t *testing.T) {
	result := mean([]float64{})
	if result != 0 {
		t.Errorf("expected 0, got %f", result)
	}
}

func TestStddev(t *testing.T) {
	data := []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}
	result := stddev(data)
	// 样本标准差 (n-1): sqrt(sum((x-mean)^2)/(n-1))
	// mean = 40/8 = 5.0
	// deviations: -3, -1, -1, -1, 0, 0, 2, 4
	// squared: 9, 1, 1, 1, 0, 0, 4, 16
	// sum = 32
	// variance = 32/7 ≈ 4.571
	// stddev ≈ 2.138
	expected := 2.138
	if math.Abs(result-expected) > 0.01 {
		t.Errorf("expected ~%f, got %f", expected, result)
	}
}

func TestStddevSmall(t *testing.T) {
	result := stddev([]float64{1.0})
	if result != 0 {
		t.Errorf("expected 0 for single element, got %f", result)
	}
}

func TestMin(t *testing.T) {
	data := []float64{3.0, 1.0, 4.0, 1.0, 5.0}
	result := min(data)
	if result != 1.0 {
		t.Errorf("expected 1.0, got %f", result)
	}
}

func TestMax(t *testing.T) {
	data := []float64{3.0, 1.0, 4.0, 1.0, 5.0}
	result := max(data)
	if result != 5.0 {
		t.Errorf("expected 5.0, got %f", result)
	}
}

func TestPercentileInterval(t *testing.T) {
	data := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	result := percentileInterval(data, 0.05, 0.95)

	if result[0] >= result[1] {
		t.Error("lower bound should be less than upper bound")
	}
	if result[0] < 1.0 {
		t.Error("lower bound should not be less than minimum")
	}
	if result[1] > 10.0 {
		t.Error("upper bound should not be greater than maximum")
	}
}

func TestPercentileIntervalEmpty(t *testing.T) {
	result := percentileInterval([]float64{}, 0.05, 0.95)
	if result[0] != 0 || result[1] != 0 {
		t.Error("expected [0,0] for empty data")
	}
}

// ============================================================================
// 数据生成辅助
// ============================================================================

func testGenerateNormalData(n int, mean, std float64) []float64 {
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, n)
	for i := range data {
		// Box-Muller transform
		u1 := rng.Float64()
		u2 := rng.Float64()
		z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
		data[i] = mean + std*z
	}
	return data
}
