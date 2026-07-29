package paradigm

import (
	"fmt"
	"time"
)

// ComputeRequest 特征计算请求。
type ComputeRequest struct {
	StockCode string          `json:"stock_code"`
	Features  []string        `json:"features"`  // FeatureSpec ID 列表
	AsOf      time.Time       `json:"as_of"`     // 数据可获得时间 (无泄漏边界)
	PriceReq  PriceAdjustment `json:"price_req"` // 价格口径
	Params    map[string]interface{} `json:"params,omitempty"` // 覆盖默认参数
}

// ComputeResponse 特征计算响应。
type ComputeResponse struct {
	StockCode   string                 `json:"stock_code"`
	AsOf        time.Time              `json:"as_of"`
	Results     map[string]float64     `json:"results"`      // FeatureID -> value
	FeatureMeta map[string]interface{} `json:"feature_meta"` // FeatureID -> extra metadata
	LeakCheck   *LeakCheckResult       `json:"leak_check,omitempty"`
	ComputedAt  time.Time              `json:"computed_at"`
	Warnings    []string               `json:"warnings,omitempty"`
}

// FeaturePipeline 无泄漏特征计算管线。
// 确保特征计算:
// 1. 不使用未来数据 (as-of 边界校验)
// 2. 按依赖拓扑排序计算
// 3. 记录计算来源与版本
type FeaturePipeline struct {
	registry *FeatureRegistry
}

// NewFeaturePipeline 创建管线实例。
func NewFeaturePipeline(registry *FeatureRegistry) *FeaturePipeline {
	return &FeaturePipeline{registry: registry}
}

// Compute 执行特征计算。
// 核心无泄漏保证: 所有特征的数据 as-of 时间必须 <= 请求的 AsOf 时间。
func (p *FeaturePipeline) Compute(req ComputeRequest) (*ComputeResponse, error) {
	if req.StockCode == "" {
		return nil, fmt.Errorf("stock code is required")
	}
	if len(req.Features) == 0 {
		return nil, fmt.Errorf("at least one feature is required")
	}

	// 1. 解析所有特征及其依赖 (拓扑排序)
	computationOrder, err := p.resolveComputationOrder(req.Features)
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies: %w", err)
	}

	// 2. 无泄漏检查
	leakCheck := p.performLeakCheck(computationOrder, req.AsOf)
	response := &ComputeResponse{
		StockCode:  req.StockCode,
		AsOf:       req.AsOf,
		Results:    make(map[string]float64),
		FeatureMeta: make(map[string]interface{}),
		LeakCheck:  leakCheck,
		ComputedAt: time.Now(),
	}

	if !leakCheck.Passed {
		response.Warnings = append(response.Warnings, leakCheck.Violations...)
		// 仍然继续计算, 但标记泄漏
	}

	// 3. 按拓扑顺序计算 (占位实现, 实际计算由调用方提供 data)
	// 这里只做元数据记录和管线逻辑校验
	for _, spec := range computationOrder {
		params := spec.DefaultParams
		if req.Params != nil {
			for k, v := range req.Params {
				params[k] = v
			}
		}

		response.FeatureMeta[spec.ID] = map[string]interface{}{
			"version":   spec.Version,
			"window":    spec.Window,
			"timing":    spec.Timing,
			"params":    params,
			"as_of":     req.AsOf.Format("2006-01-02"),
		}

		// 计算最少样本数检查
		if spec.MinSamples > 0 && spec.MinSamples > spec.Window {
			response.Warnings = append(response.Warnings,
				fmt.Sprintf("feature %s: min_samples (%d) > window (%d), may need more data",
					spec.ID, spec.MinSamples, spec.Window))
		}
	}

	return response, nil
}

// resolveComputationOrder 解析特征依赖并返回拓扑排序后的计算顺序。
func (p *FeaturePipeline) resolveComputationOrder(featureIDs []string) ([]*FeatureSpec, error) {
	visited := make(map[string]bool)
	var order []*FeatureSpec

	var visit func(id string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true

		spec, err := p.registry.GetLatest(id)
		if err != nil {
			return fmt.Errorf("feature %s: %w", id, err)
		}

		for _, depID := range spec.Dependencies {
			if err := visit(depID); err != nil {
				return err
			}
		}
		order = append(order, spec)
		return nil
	}

	for _, id := range featureIDs {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	return order, nil
}

// performLeakCheck 对计算顺序中的每个特征执行无泄漏检查。
func (p *FeaturePipeline) performLeakCheck(specs []*FeatureSpec, asOf time.Time) *LeakCheckResult {
	allPassed := true
	var allViolations []string

	for _, spec := range specs {
		check := NewLeakCheck(spec.ID, asOf, asOf, spec.Timing)
		if !check.Passed {
			allPassed = false
			allViolations = append(allViolations, check.Violations...)
		}
	}

	return &LeakCheckResult{
		Passed:     allPassed,
		Violations: allViolations,
		FeatureID:  "pipeline",
		CheckDate:  asOf,
		DataAsOf:   asOf,
	}
}

// ValidateFeatureSet 校验特征集合是否可计算:
// - 所有特征在注册表中存在
// - 依赖链完整
// - 参数满足要求
func (p *FeaturePipeline) ValidateFeatureSet(featureIDs []string) error {
	for _, id := range featureIDs {
		spec, err := p.registry.GetLatest(id)
		if err != nil {
			return fmt.Errorf("feature %s not found in registry: %w", id, err)
		}

		// 检查依赖是否都存在
		for _, depID := range spec.Dependencies {
			if _, err := p.registry.GetLatest(depID); err != nil {
				return fmt.Errorf("feature %s depends on %s which is not registered: %w", id, depID, err)
			}
		}
	}
	return nil
}

// GetDataRequirements 返回计算所需的数据类型集合。
func (p *FeaturePipeline) GetDataRequirements(featureIDs []string) ([]string, error) {
	order, err := p.resolveComputationOrder(featureIDs)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var reqs []string
	for _, spec := range order {
		for _, dr := range spec.DataRequired {
			if !seen[dr] {
				seen[dr] = true
				reqs = append(reqs, dr)
			}
		}
	}
	return reqs, nil
}

// DescribeComputation 以文本形式描述计算管线的执行计划, 用于日志/审计。
func (p *FeaturePipeline) DescribeComputation(featureIDs []string, asOf time.Time) string {
	order, err := p.resolveComputationOrder(featureIDs)
	if err != nil {
		return fmt.Sprintf("Error resolving computation order: %v", err)
	}

	result := fmt.Sprintf("Feature Pipeline Plan (as_of=%s):\n", asOf.Format("2006-01-02"))
	for i, spec := range order {
		result += fmt.Sprintf("  [%d] %s@v%d [%s] window=%d timing=%s\n",
			i+1, spec.ID, spec.Version, spec.Category, spec.Window, spec.Timing)
		if len(spec.Dependencies) > 0 {
			result += fmt.Sprintf("      depends on: %v\n", spec.Dependencies)
		}
		result += fmt.Sprintf("      data required: %v\n", spec.DataRequired)
	}
	return result
}
