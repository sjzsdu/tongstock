// Package methodseed 是「内置示例方法」应用服务：把编译、真实数据回测验证、
// 方法库注册串成一条可复现的链路，让冷启动时方法库立刻有经过真实回测的方法，
// 而不是一个永远为 0 的榜单。
//
// 它不产生任何虚假事实：每个方法是否可信完全由验证工厂在冻结真实数据上的
// 回测结果决定；未过门槛的方法会被如实标注为未通过，绝不硬编码为 verified。
package methodseed

import "time"

// Options 控制一次内置示例方法载入。
type Options struct {
	// SnapshotID 复用已冻结的真实数据快照；空 = 新建一个专用快照。
	SnapshotID string
	// Universe 验证用的真实代码列表；空 = 由 UniverseSource 解析。
	Universe []string
	// DateStart / DateEnd 验证区间；空 = 由 UniverseSource 的默认区间。
	DateStart string
	DateEnd   string
	// MaxCodes 解析股票池时的上限（0 = 使用默认）。
	MaxCodes int
	// Keys 只载入指定 key 的种子方法；空 = 全部。
	Keys []string
	// SplitType 回测切分类型: fixed / walk_forward（空 = 默认 walk_forward）。
	SplitType string
	// DiscoveryTrials 发现阶段搜索尝试次数（用于多重检验惩罚，0 = 不惩罚）。
	DiscoveryTrials int
}

// Outcome 是单个内置方法载入后的真实结果。
type Outcome struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	MethodID       string   `json:"method_id,omitempty"`
	MethodHash     string   `json:"method_hash,omitempty"`
	Status         string   `json:"status"`
	Confidence     string   `json:"confidence,omitempty"`
	Passable       bool     `json:"passable"`
	OOSTrades      int      `json:"oos_trades"`
	OOSReturn      float64  `json:"oos_return"`
	OOSMaxDrawdown float64  `json:"oos_max_drawdown"`
	ResultHash     string   `json:"result_hash,omitempty"`
	Blockers       []string `json:"blockers,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// Result 是一次内置示例方法载入的整体结果。
type Result struct {
	SnapshotID   string    `json:"snapshot_id"`
	UniverseSize int       `json:"universe_size"`
	DateStart    string    `json:"date_start"`
	DateEnd      string    `json:"date_end"`
	Outcomes     []Outcome `json:"outcomes"`
	Registered   int       `json:"registered"`
	Verified     int       `json:"verified"`
	FinishedAt   time.Time `json:"finished_at"`
}
