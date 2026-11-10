package param

import "github.com/sjzsdu/tongstock/pkg/ta"

type StockCategory string

const (
	CategoryLargeCap StockCategory = "large_cap"
	CategoryMidCap   StockCategory = "mid_cap"
	CategorySmallCap StockCategory = "small_cap"
)

type CategoryParams struct {
	MA   []int          `yaml:"ma,omitempty"`
	MACD *ta.MACDConfig `yaml:"macd,omitempty"`
	KDJ  *ta.KDJConfig  `yaml:"kdj,omitempty"`
	BOLL *ta.BOLLConfig `yaml:"boll,omitempty"`
	RSI  []int          `yaml:"rsi,omitempty"`
}

type ParamConfig struct {
	Defaults   CategoryParams            `yaml:"defaults"`
	Categories map[string]CategoryParams `yaml:"categories,omitempty"`
	Overrides  map[string]CategoryParams `yaml:"overrides,omitempty"`
	// State 是三态状态模型的阈值（params.yaml 的 state 段）。它与个股/分类无关，
	// 全局一份；指标周期仍然复用上面三层覆盖的默认/分类/个股配置。
	State *StateThresholds `yaml:"state,omitempty" json:"state,omitempty"`
}

type ParamEntry struct {
	MA   []int
	MACD *ta.MACDConfig
	KDJ  *ta.KDJConfig
	BOLL *ta.BOLLConfig
	RSI  []int
}
