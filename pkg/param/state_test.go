package param

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"gopkg.in/yaml.v3"
)

func writeTempParams(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "params.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp params: %v", err)
	}
	return path
}

// TestResolveStateDefinitionUsesStateBlock 验证 state 段能被解析并作用到状态定义上，
// 且指标周期仍然来自 defaults / categories / overrides 三层覆盖。
func TestResolveStateDefinitionUsesStateBlock(t *testing.T) {
	t.Cleanup(LoadDefault)
	path := writeTempParams(t, `
defaults:
  ma: [5, 10, 20, 60]
  macd: {fast: 12, slow: 26, signal: 9}
  kdj: {n: 9, m1: 3, m2: 3}
  boll: {n: 20, k: 2.0}
  rsi: [6, 14]
state:
  cross:
    wrap_ratio: 0.003
  osc:
    oversold_j: -5
    overbought_j: 95
  boll:
    zone_ratio: 0.25
  ma:
    min_spread_ratio: 0.004
overrides:
  "000001":
    kdj: {n: 5, m1: 3, m2: 3}
`)
	if err := Init(path); err != nil {
		t.Fatalf("init params: %v", err)
	}

	def := ResolveStateDefinition("000001")
	if def.CrossWrapRatio != 0.003 {
		t.Errorf("cross wrap ratio = %v, want 0.003", def.CrossWrapRatio)
	}
	if def.OscOversoldJ != -5 || def.OscOverboughtJ != 95 {
		t.Errorf("osc thresholds = (%v, %v), want (-5, 95)", def.OscOversoldJ, def.OscOverboughtJ)
	}
	if def.BollZoneRatio != 0.25 {
		t.Errorf("boll zone ratio = %v, want 0.25", def.BollZoneRatio)
	}
	if def.MAMinSpreadRatio != 0.004 {
		t.Errorf("ma min spread ratio = %v, want 0.004", def.MAMinSpreadRatio)
	}
	// 指标周期必须与图表指标同源：000001 的 KDJ 覆盖成 n=5。
	if def.KDJ.N != 5 {
		t.Errorf("kdj n = %d, want 5 (per-stock override must reach the state kernel)", def.KDJ.N)
	}
	if got, want := def.MinBarsFor(signalstate.DimOsc), 5; got != want {
		t.Errorf("osc warmup = %d, want %d", got, want)
	}
}

// TestResolveStateDefinitionFallsBackToDefaults 验证没配 state 段时回落到内置默认值，
// 而不是变成零值（零值会把上下触发带算成重叠）。
func TestResolveStateDefinitionFallsBackToDefaults(t *testing.T) {
	t.Cleanup(LoadDefault)
	path := writeTempParams(t, `
defaults:
  ma: [5, 10, 20, 60]
  macd: {fast: 12, slow: 26, signal: 9}
  kdj: {n: 9, m1: 3, m2: 3}
  boll: {n: 20, k: 2.0}
  rsi: [6, 14]
`)
	if err := Init(path); err != nil {
		t.Fatalf("init params: %v", err)
	}

	def := ResolveStateDefinition("600000")
	want := signalstate.DefaultDefinition()
	if def.BollZoneRatio != want.BollZoneRatio {
		t.Errorf("boll zone ratio = %v, want default %v", def.BollZoneRatio, want.BollZoneRatio)
	}
	if def.CrossWrapRatio != want.CrossWrapRatio {
		t.Errorf("cross wrap ratio = %v, want default %v", def.CrossWrapRatio, want.CrossWrapRatio)
	}
	if def.OscOverboughtJ != want.OscOverboughtJ || def.OscOversoldJ != want.OscOversoldJ {
		t.Errorf("osc thresholds = (%v, %v), want defaults (%v, %v)",
			def.OscOversoldJ, def.OscOverboughtJ, want.OscOversoldJ, want.OscOverboughtJ)
	}
}

// TestDefaultConfigTemplateCarriesStateBlock 验证写进 ~/.tongstock/indicator.yaml 的
// 默认模板就带 state 段：用户改阈值不必去翻源码找参数名。
func TestDefaultConfigTemplateCarriesStateBlock(t *testing.T) {
	var cfg ParamConfig
	if err := unmarshalYAML([]byte(defaultYAML), &cfg); err != nil {
		t.Fatalf("default template is invalid yaml: %v", err)
	}
	if cfg.State == nil {
		t.Fatal("default template has no state block")
	}
	def := DefaultStateThresholds()
	if cfg.State.Boll == nil || cfg.State.Boll.ZoneRatio != def.Boll.ZoneRatio {
		t.Fatalf("default template boll zone ratio = %+v, want %v", cfg.State.Boll, def.Boll.ZoneRatio)
	}
	if cfg.State.Cross == nil || cfg.State.Cross.WrapRatio != def.Cross.WrapRatio {
		t.Fatalf("default template cross wrap ratio = %+v, want %v", cfg.State.Cross, def.Cross.WrapRatio)
	}
	if cfg.State.MA == nil || cfg.State.MA.MinSpreadRatio != def.MA.MinSpreadRatio {
		t.Fatalf("default template ma spread ratio = %+v, want %v", cfg.State.MA, def.MA.MinSpreadRatio)
	}
}

// unmarshalYAML 是测试用的 yaml 解析入口（与 Init 用同一个解析器）。
func unmarshalYAML(data []byte, cfg *ParamConfig) error {
	return yaml.Unmarshal(data, cfg)
}
