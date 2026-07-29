package paradigms

import (
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 去重与相似性检测
// ============================================================================

// DeduplicationService 去重服务
type DeduplicationService struct {
	threshold float64 // 相似性阈值 (0-1), 超过此值视为重复
}

func NewDeduplicationService(threshold float64) *DeduplicationService {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.8
	}
	return &DeduplicationService{threshold: threshold}
}

// IsDuplicate 检查两个 Schema 是否重复
func (ds *DeduplicationService) IsDuplicate(schema1, schema2 *ParadigmSchema) bool {
	similarity := ds.CalculateSimilarity(schema1, schema2)
	return similarity >= ds.threshold
}

// CalculateSimilarity 计算两个 Schema 的相似性 (0-1)
func (ds *DeduplicationService) CalculateSimilarity(schema1, schema2 *ParadigmSchema) float64 {
	if schema1 == nil || schema2 == nil {
		return 0.0
	}

	// 规则集合相似度
	ruleSimilarity := ds.ruleSetSimilarity(schema1.Rules, schema2.Rules)

	// 特征集合相似度
	featureSimilarity := ds.featureSetSimilarity(schema1.Features, schema2.Features)

	// 上下文相似度
	contextSimilarity := ds.contextSimilarity(schema1.ContextRules, schema2.ContextRules)

	// 持有期相似度
	hidingPeriodSimilarity := 0.0
	if schema1.HoldingPeriod == schema2.HoldingPeriod {
		hidingPeriodSimilarity = 1.0
	}

	// 加权平均 (规则占主要权重)
	similarity := ruleSimilarity*0.6 + featureSimilarity*0.2 + contextSimilarity*0.1 + hidingPeriodSimilarity*0.1

	return similarity
}

// ruleSetSimilarity 规则集合相似度
func (ds *DeduplicationService) ruleSetSimilarity(rules1, rules2 []Rule) float64 {
	if len(rules1) == 0 && len(rules2) == 0 {
		return 1.0
	}
	if len(rules1) == 0 || len(rules2) == 0 {
		return 0.0
	}

	// 计算规则 1 到规则 2 的最佳匹配得分
	totalScore := 0.0
	matchedRules := make(map[int]bool)

	for _, r1 := range rules1 {
		bestScore := 0.0
		bestIndex := -1

		for j, r2 := range rules2 {
			if matchedRules[j] {
				continue
			}

			score := ds.ruleSimilarity(r1, r2)
			if score > bestScore {
				bestScore = score
				bestIndex = j
			}
		}

		if bestIndex >= 0 {
			matchedRules[bestIndex] = true
			totalScore += bestScore
		}
	}

	// 计算规则 2 到规则 1 的匹配率
	unmatchedCount := 0
	for j := range rules2 {
		if !matchedRules[j] {
			unmatchedCount++
		}
	}

	// 归一化
	maxLen := maxInt(len(rules1), len(rules2))
	if maxLen == 0 {
		return 1.0
	}

	score := totalScore / float64(maxLen)
	// 惩罚未匹配的规则
	penalty := float64(unmatchedCount) / float64(maxLen)
	score -= penalty * 0.3

	return math.Max(0, math.Min(1, score))
}

// ruleSimilarity 单条规则相似度
func (ds *DeduplicationService) ruleSimilarity(r1, r2 Rule) float64 {
	score := 0.0

	// 特征名相同: 0.4
	if r1.FeatureName == r2.FeatureName {
		score += 0.4
	}

	// 运算符相同: 0.3
	if r1.Operator == r2.Operator {
		score += 0.3
	}

	// 类型相同: 0.1
	if r1.Type == r2.Type {
		score += 0.1
	}

	// 阈值接近: 0.2
	if len(r1.Thresholds) > 0 && len(r2.Thresholds) > 0 {
		thresholdSimilarity := ds.thresholdSimilarity(r1.Thresholds, r2.Thresholds)
		score += 0.2 * thresholdSimilarity
	}

	return score
}

// thresholdSimilarity 阈值相似度
func (ds *DeduplicationService) thresholdSimilarity(t1, t2 []float64) float64 {
	if len(t1) == 0 && len(t2) == 0 {
		return 1.0
	}
	if len(t1) != len(t2) {
		return 0.0
	}

	totalSimilarity := 0.0
	for i := range t1 {
		if t1[i] == 0 && t2[i] == 0 {
			totalSimilarity += 1.0
		} else if t1[i] == 0 || t2[i] == 0 {
			totalSimilarity += 0.0
		} else {
			// 使用相对差异
			diff := math.Abs(t1[i] - t2[i]) / math.Max(math.Abs(t1[i]), math.Abs(t2[i]))
			totalSimilarity += 1.0 - math.Min(1.0, diff)
		}
	}

	return totalSimilarity / float64(len(t1))
}

// featureSetSimilarity 特征集合相似度
func (ds *DeduplicationService) featureSetSimilarity(features1, features2 []FeatureDefinition) float64 {
	if len(features1) == 0 && len(features2) == 0 {
		return 1.0
	}
	if len(features1) == 0 || len(features2) == 0 {
		return 0.0
	}

	// 创建特征名集合
	set1 := make(map[string]bool)
	for _, f := range features1 {
		set1[f.Name] = true
	}

	set2 := make(map[string]bool)
	for _, f := range features2 {
		set2[f.Name] = true
	}

	// 计算交集
	intersection := 0
	for name := range set1 {
		if set2[name] {
			intersection++
		}
	}

	// 计算并集
	union := len(set1) + len(set2) - intersection

	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union) // Jaccard 相似度
}

// contextSimilarity 上下文相似度
func (ds *DeduplicationService) contextSimilarity(ctx1, ctx2 []ContextRule) float64 {
	if len(ctx1) == 0 && len(ctx2) == 0 {
		return 1.0
	}
	if len(ctx1) == 0 || len(ctx2) == 0 {
		return 0.5 // 一方为空视为部分相似
	}

	// 创建上下文键值对
	map1 := make(map[ContextKey]map[string]bool)
	for _, c := range ctx1 {
		map1[c.Key] = make(map[string]bool)
		for _, v := range c.Values {
			map1[c.Key][v] = true
		}
	}

	map2 := make(map[ContextKey]map[string]bool)
	for _, c := range ctx2 {
		map2[c.Key] = make(map[string]bool)
		for _, v := range c.Values {
			map2[c.Key][v] = true
		}
	}

	// 比较每个上下文键
	totalSimilarity := 0.0
	keysCompared := 0

	// 检查 ctx1 的所有键
	for key, values1 := range map1 {
		if values2, exists := map2[key]; exists {
			// 计算值的交集/并集
			intersection := 0
			for v := range values1 {
				if values2[v] {
					intersection++
				}
			}
			union := len(values1) + len(values2) - intersection
			if union > 0 {
				totalSimilarity += float64(intersection) / float64(union)
			}
			keysCompared++
		} else {
			// 一方有该键, 一方没有
			totalSimilarity += 0.0
			keysCompared++
		}
	}

	// 检查 ctx2 中独有的键
	for key := range map2 {
		if _, exists := map1[key]; !exists {
			totalSimilarity += 0.0
			keysCompared++
		}
	}

	if keysCompared == 0 {
		return 1.0
	}

	return totalSimilarity / float64(keysCompared)
}

// FindDuplicates 在候选列表中查找重复
func (ds *DeduplicationService) FindDuplicates(candidates []*Candidate) [][]*Candidate {
	var duplicateGroups [][]*Candidate
	used := make(map[string]bool)

	for i := 0; i < len(candidates); i++ {
		if used[candidates[i].ID] {
			continue
		}

		group := []*Candidate{candidates[i]}
		for j := i + 1; j < len(candidates); j++ {
			if used[candidates[j].ID] {
				continue
			}

			if ds.IsDuplicate(candidates[i].Schema, candidates[j].Schema) {
				group = append(group, candidates[j])
				used[candidates[j].ID] = true
			}
		}

		if len(group) > 1 {
			duplicateGroups = append(duplicateGroups, group)
		}

		used[candidates[i].ID] = true
	}

	return duplicateGroups
}

// RemoveDuplicates 从候选列表中移除重复项 (保留第一个)
func (ds *DeduplicationService) RemoveDuplicates(candidates []*Candidate) []*Candidate {
	var result []*Candidate
	seen := make(map[string]bool)

	for _, c := range candidates {
		if seen[c.ID] {
			continue
		}

		isDuplicate := false
		for _, existing := range result {
			if ds.IsDuplicate(existing.Schema, c.Schema) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			result = append(result, c)
		}
		seen[c.ID] = true
	}

	return result
}

// ClusterBySimilarity 按相似性聚类
func (ds *DeduplicationService) ClusterBySimilarity(candidates []*Candidate, threshold float64) [][]*Candidate {
	if threshold <= 0 {
		threshold = ds.threshold
	}

	var clusters [][]*Candidate
	assigned := make(map[string]bool)

	for i := 0; i < len(candidates); i++ {
		if assigned[candidates[i].ID] {
			continue
		}

		cluster := []*Candidate{candidates[i]}
		for j := i + 1; j < len(candidates); j++ {
			if assigned[candidates[j].ID] {
				continue
			}

			similarity := ds.CalculateSimilarity(candidates[i].Schema, candidates[j].Schema)
			if similarity >= threshold {
				cluster = append(cluster, candidates[j])
				assigned[candidates[j].ID] = true
			}
		}

		assigned[candidates[i].ID] = true
		clusters = append(clusters, cluster)
	}

	return clusters
}

// BatchExperiment 批量实验
type BatchExperiment struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	CandidateIDs  []string        `json:"candidate_ids"`
	Status        string          `json:"status"`
	ProcessedCount int            `json:"processed_count"`
	Results       []*TestResult   `json:"results,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	Config        *BatchConfig    `json:"config"`
}

// BatchConfig 批量实验配置
type BatchConfig struct {
	MaxConcurrent   int     `json:"max_concurrent"`   // 最大并发数
	MaxRetries      int     `json:"max_retries"`      // 最大重试次数
	MinScore        float64 `json:"min_score"`        // 最低通过分数
	TimeLimit       int     `json:"time_limit"`       // 时间限制 (秒)
	StopOnFirstFail bool    `json:"stop_on_first_fail"` // 首次失败是否停止
}

// NewBatchExperiment 创建批量实验
func NewBatchExperiment(name string, candidateIDs []string, config *BatchConfig) *BatchExperiment {
	if config == nil {
		config = &BatchConfig{
			MaxConcurrent:   5,
			MaxRetries:      3,
			MinScore:        0.0,
			TimeLimit:       3600,
			StopOnFirstFail: false,
		}
	}

	return &BatchExperiment{
		ID:           fmt.Sprintf("batch-%d", time.Now().UnixNano()),
		Name:         name,
		CandidateIDs: candidateIDs,
		Status:       "pending",
		Results:      make([]*TestResult, 0),
		CreatedAt:    time.Now(),
		Config:       config,
	}
}

// Start 开始实验
func (be *BatchExperiment) Start() {
	be.Status = "running"
}

// Complete 完成实验
func (be *BatchExperiment) Complete() {
	be.Status = "completed"
	now := time.Now()
	be.CompletedAt = &now
}

// Fail 标记实验失败
func (be *BatchExperiment) Fail(reason string) {
	be.Status = "failed"
}

// AddResult 添加结果
func (be *BatchExperiment) AddResult(result *TestResult) {
	be.Results = append(be.Results, result)
	be.ProcessedCount++
}

// Progress 获取进度 (0-1)
func (be *BatchExperiment) Progress() float64 {
	if len(be.CandidateIDs) == 0 {
		return 1.0
	}
	return float64(be.ProcessedCount) / float64(len(be.CandidateIDs))
}

// IsComplete 检查是否完成
func (be *BatchExperiment) IsComplete() bool {
	return be.Status == "completed" || be.Status == "failed"
}

// GetPassedResults 获取通过的结果
func (be *BatchExperiment) GetPassedResults() []*TestResult {
	var passed []*TestResult
	for _, r := range be.Results {
		if r.BacktestResult != nil && r.BacktestResult.Confidence >= be.Config.MinScore {
			passed = append(passed, r)
		}
	}
	return passed
}

// Summary 获取实验摘要
func (be *BatchExperiment) Summary() string {
	return fmt.Sprintf("实验 %s: %d/%d 已处理, %d 通过, 状态: %s",
		be.Name,
		be.ProcessedCount,
		len(be.CandidateIDs),
		len(be.GetPassedResults()),
		be.Status)
}
