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
}

type ParamEntry struct {
	MA   []int
	MACD *ta.MACDConfig
	KDJ  *ta.KDJConfig
	BOLL *ta.BOLLConfig
	RSI  []int
}
