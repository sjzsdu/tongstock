package server

import (
	"testing"

	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

// TestIndicatorSettingsPayloadRoundTripsStateThresholds 守住一个很容易回归的点：
// GET/PUT /api/settings/indicator 是整体覆盖写回 indicator.yaml 的，一旦 payload
// 结构漏了 state 段，用户只要保存一次指标设置，params.yaml 里的状态阈值就被抹掉，
// 三态模型悄悄回落到默认口径。
func TestIndicatorSettingsPayloadRoundTripsStateThresholds(t *testing.T) {
	cfg := &param.ParamConfig{
		Defaults: param.CategoryParams{
			MA:   []int{5, 10, 20, 60},
			MACD: &ta.MACDConfig{Fast: 12, Slow: 26, Signal: 9},
			KDJ:  &ta.KDJConfig{N: 9, M1: 3, M2: 3},
			BOLL: &ta.BOLLConfig{N: 20, K: 2.0},
			RSI:  []int{6, 14},
		},
		State: &param.StateThresholds{
			Cross: &param.CrossThresholds{WrapRatio: 0.004},
			Osc:   &param.OscThresholds{OversoldJ: -10, OverboughtJ: 90},
			Boll:  &param.BollThresholds{ZoneRatio: 0.3},
			MA:    &param.MAThresholds{MinSpreadRatio: 0.002},
		},
	}

	payload := indicatorConfigToPayload(cfg)
	if payload.State == nil {
		t.Fatal("payload 丢了 state 段：保存指标设置会抹掉状态阈值配置")
	}

	back := payloadToIndicatorConfig(payload)
	if back.State == nil {
		t.Fatal("回写后 state 段丢失")
	}
	if back.State.Boll == nil || back.State.Boll.ZoneRatio != 0.3 {
		t.Errorf("boll zone ratio = %+v, want 0.3", back.State.Boll)
	}
	if back.State.Cross == nil || back.State.Cross.WrapRatio != 0.004 {
		t.Errorf("cross wrap ratio = %+v, want 0.004", back.State.Cross)
	}
	if back.State.Osc == nil || back.State.Osc.OversoldJ != -10 || back.State.Osc.OverboughtJ != 90 {
		t.Errorf("osc thresholds = %+v, want (-10, 90)", back.State.Osc)
	}
	if back.State.MA == nil || back.State.MA.MinSpreadRatio != 0.002 {
		t.Errorf("ma spread ratio = %+v, want 0.002", back.State.MA)
	}
}

// TestNormalizedStateThresholdsClampOutOfRange 验证保存时越界阈值会被夹紧，
// 而不是写进配置文件等状态计算时出错。
func TestNormalizedStateThresholdsClampOutOfRange(t *testing.T) {
	got := (&param.StateThresholds{
		Boll: &param.BollThresholds{ZoneRatio: 5},
		Osc:  &param.OscThresholds{OversoldJ: 200, OverboughtJ: -200},
	}).Normalized()
	if got.Boll == nil || got.Boll.ZoneRatio >= 0.5 {
		t.Fatalf("zone ratio not clamped: %+v", got.Boll)
	}
	if got.Osc == nil || got.Osc.OversoldJ >= got.Osc.OverboughtJ {
		t.Fatalf("inverted osc thresholds not fixed: %+v", got.Osc)
	}
	defaults := signalstate.DefaultDefinition()
	if got.Cross == nil || got.Cross.WrapRatio != defaults.CrossWrapRatio {
		t.Fatalf("missing cross threshold not filled with default: %+v", got.Cross)
	}
}
