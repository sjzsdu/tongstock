// Package factorlab 把选股从「技术形态投票」升级为「横截面排序预测」：
// 在冻结快照的每个交易日截面上计算候选因子，用「因子值 vs 未来 N 日收益」的
// 截面秩相关（RankIC）度量预测力，并产出当日预测分最高的 TopN 股票。
//
// 与 methodautomation 的证据哲学一致：一切结论来自机器可复现的统计，
// 不显著、不稳定的因子如实在结果中标注，绝不包装成「必涨名单」。
package factorlab

import (
	"context"
	"time"

	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// EngineVersion 是因子研究结果的结构版本，用于结果溯源。
const EngineVersion = "factorlab-v1"

// SnapshotStore 读取不可变冻结快照（形状与 paradigm.DatasetSnapshotStore 对齐）。
type SnapshotStore interface {
	List(limit, offset int) ([]*paradigm.DatasetSnapshot, error)
	GetByID(id string) (*paradigm.DatasetSnapshot, error)
	VerifyContent(snapshotID string) error
}

// UniverseResolver 从冻结快照解析真实代码列表（与 methodautomation 对齐）。
type UniverseResolver interface {
	ResolveUniverse(ctx context.Context, snapshotID string, minBars, maxCodes int) ([]string, int, error)
}

// Factor 是一个横截面因子：Compute 在「截至当日（含）」的升序 K 线上
// 返回该股票当日截面取值。ok=false 表示该股票数据不足，如实跳过。
type Factor struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Prior 是经验先验方向（+1 值越大越可能领涨 / -1 值越小越可能领涨），
	// 仅作为待检验假设的文档记录：实际显著性与方向由数据决定
	// （FactorEval.Direction = sign(MeanIC)），与先验相反就如实反向。
	Prior   float64                                                         `json:"prior"`
	Compute func(history []validation.BacktestBar) (value float64, ok bool) `json:"-"`
}

// FactorEval 是单因子在整段历史上的预测力评估。
type FactorEval struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Prior       float64 `json:"prior"`
	// Sections 是该因子有效截面数（截面内股票数达标且因子值/前向收益均有离散度）。
	Sections int `json:"sections"`
	// Pairs 是参与统计的（股票, 截面）样本对总数。
	Pairs int `json:"pairs"`
	// Coverage 是有效截面里「因子可计算股票数 / 截面股票数」的均值。
	Coverage float64 `json:"coverage"`
	// MeanIC / ICStd / ICIR / TStat 是逐截面 RankIC 序列的统计。
	// 前向窗口长 horizon 时相邻截面日的前向窗口重叠，IC 序列自相关，
	// 直接算 t 会严重夸大显著性；因此 ICStd/TStat 用「步长 = horizon 的
	// 不重叠独立子序列」计算（EffectiveSections 是独立截面数）。
	MeanIC float64 `json:"mean_ic"`
	ICStd  float64 `json:"ic_std"`
	ICIR   float64 `json:"icir"`
	TStat  float64 `json:"t_stat"`
	// EffectiveSections 是去重叠后的独立截面数（t 统计量的真实样本量）。
	EffectiveSections int `json:"effective_sections"`
	// Significant 是研究参考线判定：|TStat| ≥ 2 且 |MeanIC| ≥ MinIC。
	// 因子方向不写死：Significant 时 Direction 取数据方向 sign(MeanIC)，
	// 与先验相反就如实反向（A股短周期普遍是反转而非动量）。这是研究
	// 产出参考线，不等于方法库晋级门槛（后者仍由 validation 工厂决定）。
	Significant bool `json:"significant"`
	// Direction 是数据决定的方向：+1 值大→领涨 / -1 值小→领涨；不显著为 0。
	Direction float64 `json:"direction"`
}

// TopPick 是最后一个截面日按组合预测分排序的头部股票。
// Score = Σ(合格因子权重 × 该因子截面 z 分) / Σ|权重|，贡献分解随结果透出，
// 保持「为何选它」可解释。
type TopPick struct {
	Code  string  `json:"code"`
	Score float64 `json:"score"`
	// Contributions 是各合格因子对得分的贡献（已按总权重归一）。
	Contributions map[string]float64 `json:"contributions,omitempty"`
}

// RunResult 是一轮因子研究的完整结果。
type RunResult struct {
	EngineVersion string    `json:"engine_version"`
	SnapshotID    string    `json:"snapshot_id"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	HorizonDays   int       `json:"horizon_days"`
	TopK          int       `json:"top_k"`
	// Codes 是参与研究的股票数；Sections 是有效截面数；LastDate 是最后截面日。
	Codes    int    `json:"codes"`
	Sections int    `json:"sections"`
	LastDate string `json:"last_date,omitempty"`
	// SnapshotDateEnd 是冻结快照的数据截止日；StaleDays 是它距今的自然日数。
	// 预测必须基于新鲜快照：过时的截面排序只是历史陈迹，前端必须醒目提示。
	SnapshotDateEnd string `json:"snapshot_date_end,omitempty"`
	StaleDays       int    `json:"stale_days"`
	// Factors 按显著性降序、|t| 降序排列。
	Factors  []FactorEval `json:"factors"`
	TopPicks []TopPick    `json:"top_picks"`
	// Note 是结论的人话解释（显著因子数、TopN 产出情况），前端直接展示。
	Note string `json:"note"`
}
