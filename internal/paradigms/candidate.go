package paradigms

import (
	"fmt"
	"time"
)

// ============================================================================
// 候选范式生成管线
// ============================================================================

// CandidateStatus 候选状态
type CandidateStatus string

const (
	StatusQuarantine CandidateStatus = "quarantine" // 隔离区: 尚未验证
	StatusTesting    CandidateStatus = "testing"    // 测试中
	StatusValidated  CandidateStatus = "validated"  // 已验证
	StatusRejected   CandidateStatus = "rejected"   // 已拒绝
	StatusPromoted   CandidateStatus = "promoted"   // 已晋级为范式
)

// CandidateSource 候选来源
type CandidateSource string

const (
	SourceManual     CandidateSource = "manual"      // 人工假设
	SourceTemplate   CandidateSource = "template"    // 模板参数搜索
	SourceEventStudy CandidateSource = "event_study" // 事件研究
	SourceAI         CandidateSource = "ai"          // AI 建议
	SourceMutation   CandidateSource = "mutation"    // 变异 (基于已有范式)
)

// Candidate 候选范式
type Candidate struct {
	ID           string          `json:"id"`
	BatchID      string          `json:"batch_id"`        // 生成批次
	Source       CandidateSource `json:"source"`          // 来源
	Schema       *ParadigmSchema `json:"schema"`          // 范式 Schema
	Status       CandidateStatus `json:"status"`          // 状态
	Score        float64         `json:"score,omitempty"` // 综合评分 (0-1)
	Reason       string          `json:"reason"`          // 生成原因/假设描述
	SearchSpace  string          `json:"search_space"`    // 搜索空间描述
	ParentID     string          `json:"parent_id"`       // 父候选 (变异来源)
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ValidatedAt  *time.Time      `json:"validated_at,omitempty"`
	TestResults  *TestResult     `json:"test_results,omitempty"`
	Deduplicated bool            `json:"deduplicated"` // 是否已去重
}

// TestResult 测试结果
type TestResult struct {
	BacktestResult  *BacktestSummary       `json:"backtest_result,omitempty"`
	CrossValidation *CrossValidationResult `json:"cross_validation,omitempty"`
	CheckedAt       time.Time              `json:"checked_at"`
}

// BacktestSummary 回测摘要
type BacktestSummary struct {
	TotalReturn float64 `json:"total_return"`
	SharpeRatio float64 `json:"sharpe_ratio"`
	MaxDrawdown float64 `json:"max_drawdown"`
	WinRate     float64 `json:"win_rate"`
	TradesCount int     `json:"trades_count"`
	SampleSize  int     `json:"sample_size"`
	Confidence  float64 `json:"confidence"` // 结果置信度
}

// CrossValidationResult 交叉验证结果
type CrossValidationResult struct {
	MeanReturn     float64 `json:"mean_return"`
	StdReturn      float64 `json:"std_return"`
	WorstReturn    float64 `json:"worst_return"`
	StabilityScore float64 `json:"stability_score"`
	OverfitRisk    float64 `json:"overfit_risk"` // 过拟合风险 (0-1)
	Folds          int     `json:"folds"`
}

// IsActive 检查候选是否处于活跃状态 (可测试)
func (c *Candidate) IsActive() bool {
	return c.Status == StatusQuarantine || c.Status == StatusTesting
}

// MarkTesting 标记为测试中
func (c *Candidate) MarkTesting() {
	c.Status = StatusTesting
	c.UpdatedAt = time.Now()
}

// MarkValidated 标记为已验证
func (c *Candidate) MarkValidated(result *TestResult) {
	c.Status = StatusValidated
	c.TestResults = result
	now := time.Now()
	c.ValidatedAt = &now
	c.UpdatedAt = now
}

// MarkRejected 标记为已拒绝
func (c *Candidate) MarkRejected(reason string) {
	c.Status = StatusRejected
	c.Reason = reason
	c.UpdatedAt = time.Now()
}

// MarkPromoted 标记为已晋级
func (c *Candidate) MarkPromoted() {
	c.Status = StatusPromoted
	c.UpdatedAt = time.Now()
}

// CandidateGenerator 候选生成器接口
type CandidateGenerator interface {
	// Generate 生成候选
	Generate(params GenerateParams) ([]*Candidate, error)
	// ValidateParams 验证参数
	ValidateParams(params GenerateParams) error
	// Source 返回来源类型
	Source() CandidateSource
}

// GenerateParams 生成参数
type GenerateParams struct {
	BatchID      string                 `json:"batch_id"`
	Source       CandidateSource        `json:"source"`
	Count        int                    `json:"count"`         // 生成数量上限
	SeedSchema   *ParadigmSchema        `json:"seed_schema"`   // 种子 Schema (可选)
	SearchConfig *SearchConfig          `json:"search_config"` // 搜索配置
	Filters      map[string]interface{} `json:"filters"`       // 过滤条件
}

// SearchConfig 搜索配置
type SearchConfig struct {
	MaxRules          int            `json:"max_rules"`          // 最大规则数
	MinConfidence     float64        `json:"min_confidence"`     // 最小置信度
	SearchBudget      int            `json:"search_budget"`      // 搜索预算 (生成次数)
	UsedBudget        int            `json:"used_budget"`        // 已用预算
	FeatureWhitelist  []string       `json:"feature_whitelist"`  // 允许的特征
	OperatorWhitelist []RuleOperator `json:"operator_whitelist"` // 允许的运算符
}

// CandidateStore 候选存储
type CandidateStore struct {
	candidates map[string]*Candidate // id -> candidate
	batches    map[string][]string   // batch_id -> [candidate_ids]
	quarantine []string              // 隔离区候选 IDs
}

// NewCandidateStore 创建候选存储
func NewCandidateStore() *CandidateStore {
	return &CandidateStore{
		candidates: make(map[string]*Candidate),
		batches:    make(map[string][]string),
		quarantine: make([]string, 0),
	}
}

// SaveCandidate 保存候选
func (cs *CandidateStore) SaveCandidate(candidate *Candidate) error {
	if candidate.ID == "" {
		return fmt.Errorf("candidate ID is required")
	}

	cs.candidates[candidate.ID] = candidate

	// 添加到批次
	if candidate.BatchID != "" {
		cs.batches[candidate.BatchID] = append(cs.batches[candidate.BatchID], candidate.ID)
	}

	// 添加到隔离区
	if candidate.Status == StatusQuarantine {
		cs.quarantine = append(cs.quarantine, candidate.ID)
	}

	return nil
}

// GetCandidate 获取候选
func (cs *CandidateStore) GetCandidate(id string) (*Candidate, error) {
	c, ok := cs.candidates[id]
	if !ok {
		return nil, fmt.Errorf("candidate not found: %s", id)
	}
	return c, nil
}

// GetBatch 获取批次所有候选
func (cs *CandidateStore) GetBatch(batchID string) ([]*Candidate, error) {
	ids, ok := cs.batches[batchID]
	if !ok {
		return nil, fmt.Errorf("batch not found: %s", batchID)
	}

	candidates := make([]*Candidate, 0, len(ids))
	for _, id := range ids {
		if c, ok := cs.candidates[id]; ok {
			candidates = append(candidates, c)
		}
	}
	return candidates, nil
}

// GetQuarantine 获取隔离区所有候选
func (cs *CandidateStore) GetQuarantine() []*Candidate {
	candidates := make([]*Candidate, 0, len(cs.quarantine))
	for _, id := range cs.quarantine {
		if c, ok := cs.candidates[id]; ok {
			candidates = append(candidates, c)
		}
	}
	return candidates
}

// UpdateCandidateState 更新候选状态
func (cs *CandidateStore) UpdateCandidateState(id string, status CandidateStatus) error {
	c, ok := cs.candidates[id]
	if !ok {
		return fmt.Errorf("candidate not found: %s", id)
	}

	c.Status = status
	c.UpdatedAt = time.Now()

	// 从隔离区移除 (如果不再是 quarantine)
	if status != StatusQuarantine {
		cs.removeFromQuarantine(id)
	}

	return nil
}

// removeFromQuarantine 从隔离区移除
func (cs *CandidateStore) removeFromQuarantine(id string) {
	for i, qid := range cs.quarantine {
		if qid == id {
			cs.quarantine = append(cs.quarantine[:i], cs.quarantine[i+1:]...)
			break
		}
	}
}

// GetAll 获取所有候选
func (cs *CandidateStore) GetAll() []*Candidate {
	candidates := make([]*Candidate, 0, len(cs.candidates))
	for _, c := range cs.candidates {
		candidates = append(candidates, c)
	}
	return candidates
}

// GetBySource 按来源获取候选
func (cs *CandidateStore) GetBySource(source CandidateSource) []*Candidate {
	var candidates []*Candidate
	for _, c := range cs.candidates {
		if c.Source == source {
			candidates = append(candidates, c)
		}
	}
	return candidates
}

// GetByStatus 按状态获取候选
func (cs *CandidateStore) GetByStatus(status CandidateStatus) []*Candidate {
	var candidates []*Candidate
	for _, c := range cs.candidates {
		if c.Status == status {
			candidates = append(candidates, c)
		}
	}
	return candidates
}

// ClearQuarantine 清空隔离区
func (cs *CandidateStore) ClearQuarantine() {
	// 将所有隔离区的候选标记为拒绝
	for _, id := range cs.quarantine {
		if c, ok := cs.candidates[id]; ok {
			c.MarkRejected("cleared from quarantine")
		}
	}
	cs.quarantine = make([]string, 0)
}

// RemoveCandidate 删除候选
func (cs *CandidateStore) RemoveCandidate(id string) error {
	if _, ok := cs.candidates[id]; !ok {
		return fmt.Errorf("candidate not found: %s", id)
	}

	delete(cs.candidates, id)
	cs.removeFromQuarantine(id)

	// 从批次中移除
	for batchID, ids := range cs.batches {
		for i, bid := range ids {
			if bid == id {
				cs.batches[batchID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}

	return nil
}

// QuarantineSize 获取隔离区大小
func (cs *CandidateStore) QuarantineSize() int {
	return len(cs.quarantine)
}

// TotalCandidates 获取总候选数
func (cs *CandidateStore) TotalCandidates() int {
	return len(cs.candidates)
}

// SourceDistribution 获取来源分布
func (cs *CandidateStore) SourceDistribution() map[CandidateSource]int {
	dist := make(map[CandidateSource]int)
	for _, c := range cs.candidates {
		dist[c.Source]++
	}
	return dist
}

// StatusDistribution 获取状态分布
func (cs *CandidateStore) StatusDistribution() map[CandidateStatus]int {
	dist := make(map[CandidateStatus]int)
	for _, c := range cs.candidates {
		dist[c.Status]++
	}
	return dist
}
