// Package ledger 实现范式前向观察的不可回填信号账本与 Paper Trading 引擎。
//
// 核心能力:
//   - SignalLedger: 只追加信号账本, 每条信号冻结当时的数据快照与元信息,
//     禁止事后用修订数据重写历史 (append-only, content-hash verified)。
//   - PaperTradeEngine: 基于已有的 trading.ExecutionEngine 做前向模拟,
//     遵守与回测完全一致的 A 股约束 (T+1 / 涨跌停 / 最小单位 / 停牌)。
//   - ComparisonReport: 对比理论回测与实际前向表现, 量化执行差距。
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/trading"
)

// ============================================================================
// 信号账本 (SignalLedger)
// ============================================================================

// SignalEntry 账本中的单条信号记录。一旦写入不可修改。
type SignalEntry struct {
	ID                string          `json:"id"`
	RunID             string          `json:"run_id"`
	ParadigmVersionID string          `json:"paradigm_version_id"`
	StockCode         string          `json:"stock_code"`
	Direction         string          `json:"direction"` // buy / sell
	SignalDate        time.Time       `json:"signal_date"`
	ExecutionDate     time.Time       `json:"execution_date"`
	Price             float64         `json:"price"`       // 信号生成时的价格
	PreClose          float64         `json:"pre_close"`   // 前收盘价 (用于计算涨跌停)
	LimitUp           float64         `json:"limit_up"`    // 当日涨停价
	LimitDown         float64         `json:"limit_down"`  // 当日跌停价
	Suspended         bool            `json:"suspended"`
	Board             string          `json:"board"`
	Confidence        float64         `json:"confidence"`
	// DataSnapshot 记录信号生成时的数据版本哈希, 确保不可回填
	DataSnapshot      DataSnapshot   `json:"data_snapshot"`
	// Source 记录触发信号的规则与上下文
	Source            SignalSource   `json:"source"`
	// Execution 记录 Paper Trade 的执行结果 (nil = 尚未尝试)
	Execution         *ExecutionRecord `json:"execution,omitempty"`
	// ContentHash 基于所有字段计算, 用于不可回填校验
	ContentHash       string          `json:"content_hash"`
	CreatedAt         time.Time       `json:"created_at"`
}

// DataSnapshot 信号生成时的数据快照, 用于确保不可回填。
type DataSnapshot struct {
	// DatasetID 原始数据集标识
	DatasetID string `json:"dataset_id"`
	// FeatureSetID 使用的特征集版本
	FeatureSetID string `json:"feature_set_id"`
	// RuleSetID 使用的规则集版本
	RuleSetID string `json:"rule_set_id"`
	// DataHash 当时代理人看到的数据哈希 (例如 K 线数据的摘要哈希)
	DataHash string `json:"data_hash"`
	// CapturedAt 快照采集时间 (必须 <= SignalDate)
	CapturedAt time.Time `json:"captured_at"`
}

// SignalSource 触发信号的规则与上下文
type SignalSource struct {
	RuleID       string            `json:"rule_id"`
	RuleDesc     string            `json:"rule_desc"`
	TriggeredBy  string            `json:"triggered_by"` // 规则描述 (可读)
	ContextTags  map[string]string `json:"context_tags,omitempty"`
}

// ExecutionRecord Paper Trade 执行结果
type ExecutionRecord struct {
	Status       string    `json:"status"` // pending / filled / partial / rejected / cancelled
	ExecPrice    float64   `json:"exec_price"`
	ExecQty      int       `json:"exec_qty"`
	Fee          float64   `json:"fee"`
	PnL          float64   `json:"pnl"`      // 已实现盈亏
	HoldQty      int       `json:"hold_qty"` // 持仓数量 (T+1)
	HoldCost     float64   `json:"hold_cost"` // 持仓成本
	RejectReason string    `json:"reject_reason,omitempty"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// ============================================================================
// 前向运行 (ForwardRun)
// ============================================================================

// ForwardRun 描述范式的一次 Paper Trading 运行
type ForwardRun struct {
	ID                string    `json:"id"`
	ParadigmVersionID string    `json:"paradigm_version_id"`
	StartDate         time.Time `json:"start_date"`
	EndDate           *time.Time `json:"end_date,omitempty"`
	Status            string    `json:"status"` // active / completed / stopped
	InitialCash       float64   `json:"initial_cash"`
	FinalCash         float64   `json:"final_cash"`
	FinalPositionValue float64  `json:"final_position_value"`
	TotalPnL          float64   `json:"total_pnl"`
	TotalReturn       float64   `json:"total_return"` // (final_value - initial_cash) / initial_cash
	SignalCount       int       `json:"signal_count"`
	FilledCount       int       `json:"filled_count"`
	RejectedCount     int       `json:"rejected_count"`
	ExecutedCount     int       `json:"executed_count"`
	MaxDrawdown       float64   `json:"max_drawdown"`
	WinRate           float64   `json:"win_rate"`
	SharpeRatio       float64   `json:"sharpe_ratio"`
	// ConstraintsSnapshot 记录运行使用的约束配置, 用于复现
	ConstraintsSnapshot ConstraintsSnapshot `json:"constraints_snapshot"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ConstraintsSnapshot 前向运行使用的约束配置快照
type ConstraintsSnapshot struct {
	EnableT1         bool    `json:"enable_t_1"`
	EnablePriceLimit bool    `json:"enable_price_limit"`
	EnableSuspension bool    `json:"enable_suspension"`
	Board            string  `json:"board"`
	MinTradeUnit     int     `json:"min_trade_unit"`
	CommissionRate   float64 `json:"commission_rate"`
	SlippageBps      float64 `json:"slippage_bps"`
	StampDutyRate    float64 `json:"stamp_duty_rate"`
}

// ============================================================================
// 信号账本
// ============================================================================

// SignalLedger 只追加、不可回填的信号账本
type SignalLedger struct {
	mu     sync.RWMutex
	entries map[string]SignalEntry // entry.ID -> entry
	runs   map[string]*ForwardRun
	// 索引
	byRun       map[string][]string
	byParadigm  map[string][]string
	byStock     map[string][]string
	byDate      map[string][]string // yyyy-mm-dd -> entry IDs
}

// NewSignalLedger 创建空账本
func NewSignalLedger() *SignalLedger {
	return &SignalLedger{
		entries:    make(map[string]SignalEntry),
		runs:       make(map[string]*ForwardRun),
		byRun:      make(map[string][]string),
		byParadigm: make(map[string][]string),
		byStock:    make(map[string][]string),
		byDate:     make(map[string][]string),
	}
}

// NewForwardRun 创建一个新的前向运行
func (l *SignalLedger) NewForwardRun(
	paradigmVersionID string,
	startDate time.Time,
	initialCash float64,
	constraints trading.TradingConstraints,
	costModel trading.CostModel,
) (*ForwardRun, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	runID := fmt.Sprintf("fr-%s-%s", paradigmVersionID, startDate.Format("20060102"))
	if _, exists := l.runs[runID]; exists {
		return nil, fmt.Errorf("forward run %s already exists", runID)
	}

	run := &ForwardRun{
		ID:                runID,
		ParadigmVersionID: paradigmVersionID,
		StartDate:         startDate,
		Status:            "active",
		InitialCash:       initialCash,
		FinalCash:         initialCash,
		TotalPnL:          0,
		TotalReturn:       0,
		SignalCount:       0,
		FilledCount:       0,
		RejectedCount:     0,
		ExecutedCount:     0,
		MaxDrawdown:       0,
		WinRate:           0,
		SharpeRatio:       0,
		ConstraintsSnapshot: ConstraintsSnapshot{
			EnableT1:         constraints.EnableT1,
			EnablePriceLimit: constraints.EnablePriceLimit,
			EnableSuspension: constraints.EnableSuspension,
			Board:            string(constraints.Board),
			MinTradeUnit:     constraints.MinTradeUnit,
			CommissionRate:   costModel.CommissionRate,
			SlippageBps:      costModel.SlippageBps,
			StampDutyRate:    costModel.StampDutyRate,
		},
		CreatedAt: startDate,
		UpdatedAt: startDate,
	}

	l.runs[runID] = run
	return run, nil
}

// AppendSignal 追加一条新信号 (只追加, 不修改)
func (l *SignalLedger) AppendSignal(entry SignalEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.ID == "" {
		return fmt.Errorf("signal ID is required")
	}
	if _, exists := l.entries[entry.ID]; exists {
		return fmt.Errorf("signal %s already exists (ledger is append-only)", entry.ID)
	}

	// 计算 content hash 确保不可回填
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	hash := computeSignalHash(entry)
	entry.ContentHash = hash

	l.entries[entry.ID] = entry
	l.byRun[entry.RunID] = append(l.byRun[entry.RunID], entry.ID)
	l.byParadigm[entry.ParadigmVersionID] = append(l.byParadigm[entry.ParadigmVersionID], entry.ID)
	l.byStock[entry.StockCode] = append(l.byStock[entry.StockCode], entry.ID)

	dateKey := entry.SignalDate.Format("2006-01-02")
	l.byDate[dateKey] = append(l.byDate[dateKey], entry.ID)

	// 更新 run 统计
	if run, ok := l.runs[entry.RunID]; ok {
		run.SignalCount++
		run.UpdatedAt = time.Now()
	}

	return nil
}

// UpdateExecution 更新信号的执行结果 (仅更新 Execution 字段, 不触碰原始数据)
func (l *SignalLedger) UpdateExecution(entryID string, execution ExecutionRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[entryID]
	if !ok {
		return fmt.Errorf("signal %s not found", entryID)
	}

	// 重新计算 hash, 确保 Execution 的修改被纳入
	entry.Execution = &execution
	entry.ContentHash = computeSignalHash(entry)
	l.entries[entryID] = entry

	// 更新 run 统计
	if run, ok := l.runs[entry.RunID]; ok {
		run.ExecutedCount++
		switch execution.Status {
		case "filled", "partial":
			run.FilledCount++
		case "rejected":
			run.RejectedCount++
		}
		run.UpdatedAt = time.Now()
	}

	return nil
}

// GetSignal 获取单条信号 (验证 hash)
func (l *SignalLedger) GetSignal(id string) (SignalEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.entries[id]
	if !ok {
		return SignalEntry{}, fmt.Errorf("signal %s not found", id)
	}
	// 验证 hash 确保数据未被篡改
	expectedHash := computeSignalHash(entry)
	if entry.ContentHash != expectedHash {
		return SignalEntry{}, fmt.Errorf("signal %s hash mismatch: data may have been tampered", id)
	}
	return entry, nil
}

// ListByRun 列出某个 run 的所有信号 (按日期升序)
func (l *SignalLedger) ListByRun(runID string) []SignalEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	ids := l.byRun[runID]
	entries := make([]SignalEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := l.entries[id]; ok {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SignalDate.Before(entries[j].SignalDate)
	})
	return entries
}

// ListByParadigm 列出某个范式版本的所有信号
func (l *SignalLedger) ListByParadigm(versionID string) []SignalEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	ids := l.byParadigm[versionID]
	entries := make([]SignalEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := l.entries[id]; ok {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SignalDate.Before(entries[j].SignalDate)
	})
	return entries
}

// ListByDate 列出某个日期的所有信号
func (l *SignalLedger) ListByDate(date time.Time) []SignalEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	dateKey := date.Format("2006-01-02")
	ids := l.byDate[dateKey]
	entries := make([]SignalEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := l.entries[id]; ok {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SignalDate.Before(entries[j].SignalDate)
	})
	return entries
}

// GetRun 获取前向运行
func (l *SignalLedger) GetRun(runID string) (*ForwardRun, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	run, ok := l.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	return run, nil
}

// ListRuns 列出所有前向运行 (按创建日期降序)
func (l *SignalLedger) ListRuns(n int) []*ForwardRun {
	l.mu.RLock()
	defer l.mu.RUnlock()

	runs := make([]*ForwardRun, 0, len(l.runs))
	for _, r := range l.runs {
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	if n > 0 && len(runs) > n {
		runs = runs[:n]
	}
	return runs
}

// FinalizeRun 结束一个前向运行, 计算最终统计
func (l *SignalLedger) FinalizeRun(runID string, endDate time.Time) (*ForwardRun, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	run, ok := l.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}

	// 计算已实现 P&L 和胜率
	entries := make([]SignalEntry, 0)
	for _, id := range l.byRun[runID] {
		if e, ok := l.entries[id]; ok {
			entries = append(entries, e)
		}
	}

	var totalPnL float64
	var wins, losses int
	var peakEquity float64
	equityCurve := []float64{run.InitialCash}
	cash := run.InitialCash
	var positionValue float64

	for _, e := range entries {
		if e.Execution == nil {
			continue
		}
		if e.Execution.Status == "filled" || e.Execution.Status == "partial" {
			totalPnL += e.Execution.PnL
			if e.Execution.PnL > 0 {
				wins++
			} else if e.Execution.PnL < 0 {
				losses++
			}
			// 更新现金
			if e.Direction == "buy" {
				cash -= e.Execution.ExecPrice * float64(e.Execution.ExecQty) + e.Execution.Fee
			} else {
				cash += e.Execution.ExecPrice * float64(e.Execution.ExecQty) - e.Execution.Fee
			}
		}
	}

	// 计算当前持仓市值 (简化: 用最后价格估算)
	for _, e := range entries {
		if e.Execution != nil && e.Direction == "buy" && e.Execution.Status == "filled" {
			positionValue += e.Price * float64(e.Execution.HoldQty)
		}
	}

	finalValue := cash + positionValue
	run.FinalCash = cash
	run.FinalPositionValue = positionValue
	run.TotalPnL = totalPnL
	run.TotalReturn = (finalValue - run.InitialCash) / run.InitialCash
	if run.InitialCash > 0 {
		run.TotalReturn = finalValue/run.InitialCash - 1
	}
	if wins+losses > 0 {
		run.WinRate = float64(wins) / float64(wins+losses)
	}
	run.Status = "completed"
	run.EndDate = &endDate
	run.UpdatedAt = time.Now()

	// 计算最大回撤 (简化: 基于权益曲线)
	for _, v := range equityCurve {
		if v > peakEquity {
			peakEquity = v
		}
		if peakEquity > 0 {
			drawdown := (peakEquity - v) / peakEquity
			if drawdown > run.MaxDrawdown {
				run.MaxDrawdown = drawdown
			}
		}
	}

	return run, nil
}

// computeSignalHash 基于信号全部内容计算不可回填哈希
func computeSignalHash(entry SignalEntry) string {
	data, _ := json.Marshal(struct {
		ID                string          `json:"id"`
		RunID             string          `json:"run_id"`
		ParadigmVersionID string          `json:"paradigm_version_id"`
		StockCode         string          `json:"stock_code"`
		Direction         string          `json:"direction"`
		SignalDate        time.Time       `json:"signal_date"`
		ExecutionDate     time.Time       `json:"execution_date"`
		Price             float64         `json:"price"`
		PreClose          float64         `json:"pre_close"`
		LimitUp           float64         `json:"limit_up"`
		LimitDown         float64         `json:"limit_down"`
		Suspended         bool            `json:"suspended"`
		Board             string          `json:"board"`
		Confidence        float64         `json:"confidence"`
		DataSnapshot      DataSnapshot    `json:"data_snapshot"`
		Source            SignalSource    `json:"source"`
		Execution         *ExecutionRecord `json:"execution,omitempty"`
		CreatedAt         time.Time       `json:"created_at"`
	}{
		ID:                entry.ID,
		RunID:             entry.RunID,
		ParadigmVersionID: entry.ParadigmVersionID,
		StockCode:         entry.StockCode,
		Direction:         entry.Direction,
		SignalDate:        entry.SignalDate,
		ExecutionDate:     entry.ExecutionDate,
		Price:             entry.Price,
		PreClose:          entry.PreClose,
		LimitUp:           entry.LimitUp,
		LimitDown:         entry.LimitDown,
		Suspended:         entry.Suspended,
		Board:             entry.Board,
		Confidence:        entry.Confidence,
		DataSnapshot:      entry.DataSnapshot,
		Source:            entry.Source,
		Execution:         entry.Execution,
		CreatedAt:         entry.CreatedAt,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidateSignals 校验账本中所有信号的哈希完整性
func (l *SignalLedger) ValidateSignals() (int, []string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var invalid int
	var issues []string
	for id, entry := range l.entries {
		expected := computeSignalHash(entry)
		if entry.ContentHash != expected {
			invalid++
			issues = append(issues, fmt.Sprintf("signal %s: hash mismatch", id))
		}
	}
	return invalid, issues
}
