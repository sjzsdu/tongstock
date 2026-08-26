package marketsnapshot

// Repository 定义 marketsnapshot 域的持久化端口。
// 适配器（例如 SQLite）在 internal/adapter/* 下实现，以保持域层存储无关。
type Repository interface {
	// SaveMarketSnapshot 幂等写入 market_snapshot + code_state。
	// 当已存在相同 ID 时，若 frozen=1 则返回错误；否则覆盖写入。
	SaveMarketSnapshot(s *MarketSnapshot) error
	// LoadMarketSnapshot 按 ID 加载（Codes 可选填充）。
	LoadMarketSnapshot(id string, includeCodes bool) (*MarketSnapshot, error)
	// FindMarketSnapshot 按 (date, universe, adj) 找最近一个快照。
	FindMarketSnapshot(date string, universeName string, adj string) (*MarketSnapshot, error)
	// ListMarketSnapshots 按日期范围列所有快照元信息。
	ListMarketSnapshots(dateStart string, dateEnd string, status string) ([]*MarketSnapshot, error)
	// Freeze 将快照切换为 frozen，禁止后续修改。
	FreezeMarketSnapshot(id string) error

	SaveFeatureSnapshot(s *FeatureSnapshot) error
	LoadFeatureSnapshot(id string, includeValues bool) (*FeatureSnapshot, error)
	ListFeatureSnapshots(marketSnapshotID string) ([]*FeatureSnapshot, error)
}

// UniverseProvider 生成 point-in-time 股票宇宙。
type UniverseProvider interface {
	// BuildUniverse 返回截至指定交易日的所有候选证券及 point-in-time 状态。
	// 返回的 UniverseMember 同时包含被 Exclude 的条目（Selected=false，带原因），
	// 以便 readiness 审计。
	BuildUniverse(date string, def UniverseDefinition) ([]UniverseMember, error)
}

// WatermarkProvider 查询单股数据水位（K 线 / quote / finance / xdxr）。
type WatermarkProvider interface {
	FetchWatermarks(date string, codes []string) (map[string]CodeStatus, error)
}

// FeatureEngine 对 universe 中的代码批量 point-in-time 计算特征。
type FeatureEngine interface {
	Compute(date string, codes []string, features []FeatureSpec) (map[string]map[string]float64, error)
}

// TradingCalendar 提供交易日判断。
type TradingCalendar interface {
	IsTradingDay(date string) (bool, error)
	PrevTradingDay(date string) (string, error)
}
