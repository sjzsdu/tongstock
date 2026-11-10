package param

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"gopkg.in/yaml.v3"
)

var globalConfig *ParamConfig

func cloneConfig(cfg *ParamConfig) (*ParamConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var cloned ParamConfig
	if err := yaml.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func Init(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg ParamConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	cloned, err := cloneConfig(&cfg)
	if err != nil {
		return err
	}
	globalConfig = cloned
	return nil
}

func LoadDefault() {
	globalConfig = &ParamConfig{
		Defaults: CategoryParams{
			MA:   []int{5, 10, 20, 60, 120},
			MACD: &ta.MACDConfig{Fast: 12, Slow: 26, Signal: 9},
			KDJ:  &ta.KDJConfig{N: 9, M1: 3, M2: 3},
			BOLL: &ta.BOLLConfig{N: 20, K: 2.0},
			RSI:  []int{6, 12, 24},
		},
		Categories: map[string]CategoryParams{
			"large_cap": {
				MA:   []int{5, 10, 20, 60, 120},
				MACD: &ta.MACDConfig{Fast: 12, Slow: 26, Signal: 9},
			},
			"small_cap": {
				MA:   []int{5, 10, 20},
				MACD: &ta.MACDConfig{Fast: 8, Slow: 17, Signal: 9},
				KDJ:  &ta.KDJConfig{N: 7, M1: 3, M2: 3},
			},
		},
		Overrides: map[string]CategoryParams{
			"000001": {
				KDJ: &ta.KDJConfig{N: 5, M1: 3, M2: 3},
			},
		},
		State: DefaultStateThresholds(),
	}
}

func AutoInit() error {
	if globalConfig != nil {
		return nil
	}

	path := config.IndicatorConfigPath()
	if err := config.EnsureHomeDir(); err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefaultConfig(path); err != nil {
			return fmt.Errorf("写入默认指标配置失败: %w", err)
		}
		fmt.Fprintf(os.Stderr, "已生成默认指标配置: %s\n", path)
	}

	return Init(path)
}

func GetConfig() (*ParamConfig, error) {
	if err := AutoInit(); err != nil {
		return nil, err
	}
	return cloneConfig(globalConfig)
}

func SaveConfig(cfg *ParamConfig) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	if err := config.EnsureHomeDir(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := config.IndicatorConfigPath()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	cloned, err := cloneConfig(cfg)
	if err != nil {
		return err
	}
	globalConfig = cloned
	return nil
}

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultYAML), 0644)
}

const defaultYAML = `# TongStock 技术指标参数配置
# 用户可自行修改此文件以自定义指标参数

defaults:
  ma: [5, 10, 20, 60, 120]
  macd:
    fast: 12
    slow: 26
    signal: 9
  kdj:
    n: 9
    m1: 3
    m2: 3
  boll:
    n: 20
    k: 2.0
  rsi: [6, 12, 24]

# 三态状态模型（买入/卖出/中性）的阈值。指标周期复用上面的 defaults /
# categories / overrides，这里只放「判定阈值」。0 或不写 = 用默认值。
state:
  cross:
    # |DIF - DEA| / 收盘价 <= 该比例 视为 DIF/DEA 缠绕（中性）。默认 0.002（0.2%）
    wrap_ratio: 0.002
  osc:
    # J <= oversold_j 为超卖（买入）；J >= overbought_j 为超买（卖出）。默认 0 / 100
    oversold_j: 0
    overbought_j: 100
  boll:
    # 以上轨/下轨为中心的触发带宽度 = zone_ratio x 布林带宽（上轨-下轨），默认 0.15
    zone_ratio: 0.15
  ma:
    # 均线排列的最小间距（最短均线-最长均线占收盘价比例），默认 0.001（0.1%）
    min_spread_ratio: 0.001

categories:
  large_cap:
    ma: [5, 10, 20, 60, 120]
    macd:
      fast: 12
      slow: 26
      signal: 9
  small_cap:
    ma: [5, 10, 20]
    macd:
      fast: 8
      slow: 17
      signal: 9
    kdj:
      n: 7
      m1: 3
      m2: 3

overrides:
  "000001":
    kdj:
      n: 5
      m1: 3
      m2: 3
`

func Resolve(code string, category StockCategory) *ta.IndicatorConfig {
	if globalConfig == nil {
		LoadDefault()
	}

	def := &globalConfig.Defaults
	if category != "" && globalConfig.Categories != nil {
		if catParams, ok := globalConfig.Categories[string(category)]; ok {
			def = mergeCategoryParams(def, &catParams)
		}
	}
	if globalConfig.Overrides != nil {
		if stockParams, ok := globalConfig.Overrides[code]; ok {
			def = mergeCategoryParams(def, &stockParams)
		}
	}

	return toIndicatorConfig(def)
}

func mergeCategoryParams(base, override *CategoryParams) *CategoryParams {
	result := *base
	if override.MA != nil {
		result.MA = override.MA
	}
	if override.MACD != nil {
		result.MACD = override.MACD
	}
	if override.KDJ != nil {
		result.KDJ = override.KDJ
	}
	if override.BOLL != nil {
		result.BOLL = override.BOLL
	}
	if override.RSI != nil {
		result.RSI = override.RSI
	}
	return &result
}

func toIndicatorConfig(p *CategoryParams) *ta.IndicatorConfig {
	return &ta.IndicatorConfig{
		MA:   p.MA,
		MACD: p.MACD,
		KDJ:  p.KDJ,
		BOLL: p.BOLL,
		RSI:  p.RSI,
	}
}
