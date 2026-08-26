package validation

import (
	"context"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methods"
)

// BarProvider 按 code + 日期范围加载真实历史 K 线，转换为回测消费的 BacktestBar。
// 适配器在 internal/adapter 下实现；域层不依赖 SQLite/TDX。
// Fail closed：缺失或不可信数据必须返回错误，禁止返回补值或空切片冒充。
type BarProvider interface {
	// LoadBars 返回 [dateStart, dateEnd] 闭区间内按日期升序排列的真实 K 线。
	// snapshotID 必须指向含冻结 K 线的不可变数据快照。
	LoadBars(ctx context.Context, snapshotID, code, dateStart, dateEnd string) ([]BacktestBar, error)
}

// BenchmarkProvider 加载基准代码（如沪深300）的日收益序列，用于计算超额收益。
// 返回的日期->收益率映射必须与 BarProvider 使用同一数据口径。
type BenchmarkProvider interface {
	LoadDailyReturns(ctx context.Context, snapshotID, code, dateStart, dateEnd string) (map[string]float64, error)
}

// EvidenceRepository 持久化可被方法库和 AI 按稳定哈希引用的验证制品。
type EvidenceRepository interface {
	Save(ctx context.Context, bundle *EvidenceBundle) error
	Get(ctx context.Context, resultHash string) (*EvidenceBundle, error)
	ListByMethod(ctx context.Context, methodHash string, limit int) ([]*EvidenceBundle, error)
}

// FactoryDeps 装配验证工厂所需的外部依赖。
// 由 Composition Root 注入；域层不构造具体适配器。
type FactoryDeps struct {
	Method    *methods.CompiledMethod
	Bars      BarProvider
	Benchmark BenchmarkProvider // 可为 nil（无基准时跳过超额收益）
	Snapshot  *marketsnapshot.MarketSnapshot
	Calendar  marketsnapshot.TradingCalendar // 可为 nil
}

// SegmentSpec 描述一个回测段（训练/验证/样本外）。
type SegmentSpec struct {
	Name      string // train / valid / test
	DateStart string
	DateEnd   string
}

// Validate 检查段定义。
func (s SegmentSpec) Validate() error {
	if s.Name == "" {
		return errEmpty("segment name")
	}
	if s.DateStart == "" || s.DateEnd == "" {
		return errEmpty("segment date range")
	}
	return nil
}

type fieldError struct{ field string }

func (e *fieldError) Error() string { return e.field + " is required" }

func errEmpty(field string) error { return &fieldError{field: field} }
