package signalstate_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

// 状态带图层的前端文件（图层绘制 + 图例/读数）。它们只允许消费后端下发的元数据，
// 不允许自带一套状态文案或买卖方向映射 —— 旧 web/src/types/screen.ts 的
// SIGNAL_OPTIONS 就是在前端另写一套之后跟后端漂移的。
var stateLayerSources = []string{
	"../../web/src/components/charts/stateBands.ts",
	"../../web/src/components/charts/StateBandOverlay.tsx",
}

// TestFrontendStateLayerHasNoHardcodedLabels 验证前端状态带图层没有硬编码状态文案/方向：
// 三态文案与配色必须全部来自后端枚举（验收口径 ④）。
func TestFrontendStateLayerHasNoHardcodedLabels(t *testing.T) {
	forbidden := []string{
		"金叉", "死叉", "超买", "超卖",
		"突破上轨", "跌破下轨", "多头排列", "空头排列",
		"缠绕", "数据不足",
	}

	for _, path := range stateLayerSources {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取状态带图层源码失败（图层文件被移动/删除？）: %v", err)
		}
		src := string(data)
		for _, label := range forbidden {
			if strings.Contains(src, label) {
				t.Errorf("%s 硬编码了状态文案 %q：状态标签必须来自后端 pkg/signalstate 元数据", path, label)
			}
		}
		// 方向字面量同理：前端不得自己判断 buy/sell。
		if strings.Contains(src, "=== 'buy'") || strings.Contains(src, `=== "buy"`) {
			t.Errorf("%s 出现 === 'buy' 方向判断：买卖方向只能由后端枚举决定", path)
		}
	}
}

// TestFrontendStateLayerReadsBackendMetadata 验证图层确实从 dimensions 元数据取标签/配色，
// 而不是自带一份映射表。
func TestFrontendStateLayerReadsBackendMetadata(t *testing.T) {
	for _, path := range stateLayerSources {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		src := string(data)
		if !strings.Contains(src, "dimensions") {
			t.Errorf("%s 没有消费后端 dimensions 元数据", path)
		}
	}
}

// TestMetadataLabelsCoverFrozenDirectionTable 是前端契约的反向断言：后端必须为四维四态
// 都下发文案与配色 —— 一旦漏配，前端只能渲染空白。
func TestMetadataLabelsCoverFrozenDirectionTable(t *testing.T) {
	for _, meta := range signalstate.Metadata() {
		if len(meta.States) != 4 {
			t.Fatalf("dim %s 下发了 %d 个状态元数据，应为 4 个", meta.Dim, len(meta.States))
		}
		for _, state := range meta.States {
			if state.Label == "" || state.Color == "" {
				t.Fatalf("dim %s state %s 缺少 label/color", meta.Dim, state.State)
			}
		}
	}
}
