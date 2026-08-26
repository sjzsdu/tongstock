package quality

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Severity 问题严重程度。
type Severity string

const (
	SeverityCritical Severity = "critical" // 严重: 阻止实验
	SeverityWarning  Severity = "warning"  // 警告: 不阻止, 但需记录
	SeverityInfo     Severity = "info"     // 信息: 仅提示
)

// Category 质量检查分类。
type Category string

const (
	CategoryMissingData       Category = "missing_data"       // 数据缺失
	CategoryDuplicate         Category = "duplicate"          // 重复记录
	CategoryAbnormalPrice     Category = "abnormal_price"     // 异常价格
	CategoryAbnormalVolume    Category = "abnormal_volume"    // 异常成交量
	CategoryTimeReversal      Category = "time_reversal"      // 时间倒序
	CategoryFinancialLag      Category = "financial_lag"      // 财务数据滞后
	CategoryPoolCoverage      Category = "pool_coverage"      // 证券池覆盖
	CategorySourceDegradation Category = "source_degradation" // 数据源降级
)

// QualityIssue 单个数据质量问题。
type QualityIssue struct {
	ID          string   `json:"id"`
	Category    Category `json:"category"`
	Severity    Severity `json:"severity"`
	StockCode   string   `json:"stock_code,omitempty"`
	Date        string   `json:"date,omitempty"`
	Metric      string   `json:"metric"`      // 指标名, e.g. "close", "volume", "coverage"
	Expected    string   `json:"expected"`    // 期望值/范围描述
	Actual      string   `json:"actual"`      // 实际值
	Description string   `json:"description"` // 可读描述
}

// QualityReport 数据质量报告。每个数据快照/实验对应一个报告。
type QualityReport struct {
	ID         string         `json:"id"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
	StockCode  string         `json:"stock_code,omitempty"`
	DateRange  DateRangeInfo  `json:"date_range"`
	Source     string         `json:"source"` // 数据源: kline / finance / xdxr
	AsOf       time.Time      `json:"as_of"`
	Issues     []QualityIssue `json:"issues"`
	Passed     bool           `json:"passed"`  // 是否通过质量门
	Blocked    bool           `json:"blocked"` // 是否被阻止 (存在 critical)
	Summary    QualitySummary `json:"summary"`
	CreatedAt  time.Time      `json:"created_at"`
}

// DateRangeInfo 日期范围信息。
type DateRangeInfo struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// QualitySummary 质量摘要统计。
type QualitySummary struct {
	TotalIssues     int     `json:"total_issues"`
	CriticalCount   int     `json:"critical_count"`
	WarningCount    int     `json:"warning_count"`
	InfoCount       int     `json:"info_count"`
	CheckedRecords  int     `json:"checked_records"`
	PassedRecords   int     `json:"passed_records"`
	FailedRecords   int     `json:"failed_records"`
	CoveragePercent float64 `json:"coverage_percent"`
}

// QualityGateConfig 质量门配置: 定义各检查的阈值和严重度。
type QualityGateConfig struct {
	// 价格跳变阈值 (百分比), 超过则判定为异常
	MaxPriceChangePct float64 `json:"max_price_change_pct"` // 默认 5.0 (500%)

	// 成交量异常阈值 (相对于历史均值的倍数)
	MaxVolumeRatio float64 `json:"max_volume_ratio"` // 默认 10.0

	// 覆盖率阈值 (低于此百分比触发 warning)
	MinCoveragePercent float64 `json:"min_coverage_percent"` // 默认 95.0

	// 最大允许缺失天数 (超过则 critical)
	MaxMissingDays int `json:"max_missing_days"` // 默认 5

	// 财务数据最大滞后天数
	MaxFinancialLagDays int `json:"max_financial_lag_days"` // 默认 60

	// 是否阻止实验 (存在 critical 时)
	BlockOnCritical bool `json:"block_on_critical"` // 默认 true
}

// DefaultQualityGateConfig 返回默认质量门配置。
func DefaultQualityGateConfig() QualityGateConfig {
	return QualityGateConfig{
		MaxPriceChangePct:   5.0,
		MaxVolumeRatio:      10.0,
		MinCoveragePercent:  95.0,
		MaxMissingDays:      5,
		MaxFinancialLagDays: 60,
		BlockOnCritical:     true,
	}
}

// QualityGateResult 质量门决策结果。
type QualityGateResult struct {
	Passed    bool      `json:"passed"`
	Blocked   bool      `json:"blocked"`
	Decision  string    `json:"decision"` // "pass" / "warn" / "block"
	Reason    string    `json:"reason"`
	ReportID  string    `json:"report_id"`
	Issues    []string  `json:"issues,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// ShouldBlock 判断报告是否应被阻止。
func (r *QualityReport) ShouldBlock(cfg QualityGateConfig) bool {
	if !cfg.BlockOnCritical {
		return false
	}
	for _, issue := range r.Issues {
		if issue.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// IssueCountBySeverity 按严重度统计问题数。
func (r *QualityReport) IssueCountBySeverity(s Severity) int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == s {
			count++
		}
	}
	return count
}

// HasCriticalIssues 是否存在严重问题。
func (r *QualityReport) HasCriticalIssues() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// ComputeHash 计算报告的哈希值, 用于版本比对。
func (r *QualityReport) ComputeHash() string {
	data := fmt.Sprintf("%s|%s|%s|%d",
		r.SnapshotID, r.StockCode, r.Source, len(r.Issues))
	b, _ := json.Marshal(r.Issues)
	data += string(b)
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

// NewQualityIssue 创建质量问题。
func NewQualityIssue(category Category, severity Severity, stockCode, date, metric, expected, actual, description string) QualityIssue {
	id := fmt.Sprintf("%s-%s-%s-%s-%s",
		category, stockCode, date, metric, severity)
	return QualityIssue{
		ID:          id,
		Category:    category,
		Severity:    severity,
		StockCode:   stockCode,
		Date:        date,
		Metric:      metric,
		Expected:    expected,
		Actual:      actual,
		Description: description,
	}
}
