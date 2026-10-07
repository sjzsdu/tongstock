// Package methodautomation 把「规律发现 → 机器验证 → 方法库晋级/拒绝」串成
// 可定时、可手动触发的自动闭环供给线。它复用既有引擎，不新增任何事实来源：
//
//   - discovery.Researcher：在冻结真实 K 线上做模板扫描（未触碰尾部保留样本）；
//   - validation.Factory：在 discovery 从未触碰的保留窗口上做全池样本外回测；
//   - methodregistry.Registry：由机器证据（置信度 + 门槛）决定 verified / rejected。
//
// 红线：绝不硬编码 verified；候选空间扩大必须计入 DiscoveryTrials 多重检验校正。
package methodautomation

import (
	"context"
	"time"

	"github.com/sjzsdu/tongstock/internal/validation"
)

// Request 描述一轮自动方法研究。
type Request struct {
	// SnapshotID 复用已冻结真实 K 线快照；空 = 自动选最新可用的冻结快照。
	SnapshotID string
	// Codes 显式研究代码；空 = 由 Universe resolver 从快照解析。
	Codes []string
	// HoldDays 要扫描的持有期集合；空 = [5, 20]。
	HoldDays []int
	// SearchBudget 每个持有期的模板搜索预算；0 = 24。
	SearchBudget int
	// MaxCodes 从快照解析股票池时的上限；0 = 300。
	MaxCodes int
}

// CandidateOutcome 是一个候选方法在闭环里的真实结局。
type CandidateOutcome struct {
	TemplateID  string   `json:"template_id"`
	MethodID    string   `json:"method_id,omitempty"`
	MethodHash  string   `json:"method_hash,omitempty"`
	Status      string   `json:"status"` // verified / rejected / failed / skipped_registered / skipped_feedback
	Stage       string   `json:"stage"`  // discovery / validation / registration
	Confidence  string   `json:"confidence,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	OOSTrades   int      `json:"oos_trades,omitempty"`
	OOSReturn   float64  `json:"oos_return,omitempty"`
	OOSWinRate  float64  `json:"oos_win_rate,omitempty"`
	SharpeRatio *float64 `json:"sharpe_ratio,omitempty"`
}

// HoldBatchResult 是单个持有期一轮发现+验证的汇总。
type HoldBatchResult struct {
	HoldDays   int    `json:"hold_days"`
	ResearchID string `json:"research_id,omitempty"`
	// DiscoveryTrials 是本轮扫描评估的模板数（计入多重检验）。
	DiscoveryTrials int    `json:"discovery_trials"`
	Candidates      int    `json:"candidates"`
	Registered      int    `json:"registered"`
	Verified        int    `json:"verified"`
	Rejected        int    `json:"rejected"`
	Error           string `json:"error,omitempty"`
}

// BatchResult 是一轮自动研究的整体结果。
type BatchResult struct {
	StartedAt        time.Time          `json:"started_at"`
	FinishedAt       time.Time          `json:"finished_at"`
	SnapshotID       string             `json:"snapshot_id"`
	UniverseSize     int                `json:"universe_size"`
	ValidationStart  string             `json:"validation_start,omitempty"`
	ValidationEnd    string             `json:"validation_end,omitempty"`
	TrialsThisBatch  int                `json:"trials_this_batch"`
	TrialsCumulative int64              `json:"trials_cumulative"`
	Batches          []HoldBatchResult  `json:"batches"`
	Outcomes         []CandidateOutcome `json:"outcomes"`
	Registered       int                `json:"registered"`
	Verified         int                `json:"verified"`
	Rejected         int                `json:"rejected"`
}

// EvidenceSink 持久化验证制品（与 methodseed.EvidenceSink 一致的形状）。
type EvidenceSink interface {
	Save(ctx context.Context, bundle *validation.EvidenceBundle) error
}
