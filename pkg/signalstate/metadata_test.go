package signalstate_test

import (
	"testing"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

// TestMetadataCoversEveryDimAndState 验证元数据（标签/方向/颜色）四维四态齐全，
// 且顺序与 Dims 一致 —— 前端只靠这份元数据渲染，缺一项就会渲染成空白。
func TestMetadataCoversEveryDimAndState(t *testing.T) {
	meta := signalstate.Metadata()
	dims := signalstate.Dims()
	if len(meta) != len(dims) {
		t.Fatalf("metadata dims = %d, want %d", len(meta), len(dims))
	}
	for i, dim := range dims {
		if meta[i].Dim != dim {
			t.Fatalf("metadata order mismatch at %d: want %s got %s", i, dim, meta[i].Dim)
		}
		if meta[i].Label == "" || meta[i].Description == "" {
			t.Fatalf("dim %s missing label/description", dim)
		}
		want := []signalstate.State{
			signalstate.StateBuy, signalstate.StateSell,
			signalstate.StateNeutral, signalstate.StateInsufficient,
		}
		if len(meta[i].States) != len(want) {
			t.Fatalf("dim %s states = %d, want %d", dim, len(meta[i].States), len(want))
		}
		for j, state := range want {
			got := meta[i].States[j]
			if got.State != state {
				t.Fatalf("dim %s state order mismatch at %d: want %s got %s", dim, j, state, got.State)
			}
			if got.Label == "" || got.Color == "" || got.Description == "" {
				t.Fatalf("dim %s state %s missing label/color/description: %+v", dim, state, got)
			}
			if got.Direction != signalstate.DirectionOf(state) {
				t.Fatalf("dim %s state %s direction=%s, want %s", dim, state, got.Direction, signalstate.DirectionOf(state))
			}
		}
	}
}

// TestStateColorsAndLabelsAreDistinct 验证四态的配色/文案互不重复：
// 特别是数据不足必须与中性在视觉上可区分（验收口径 ③）。
func TestStateColorsAndLabelsAreDistinct(t *testing.T) {
	states := []signalstate.State{
		signalstate.StateBuy, signalstate.StateSell,
		signalstate.StateNeutral, signalstate.StateInsufficient,
	}
	labels := map[signalstate.Dim]map[string]bool{}
	colors := map[string]signalstate.State{}
	for _, dim := range signalstate.Dims() {
		labels[dim] = map[string]bool{}
		for _, state := range states {
			meta, ok := signalstate.StateMetaFor(dim, state)
			if !ok {
				t.Fatalf("dim %s state %s meta missing", dim, state)
			}
			if labels[dim][meta.Label] {
				t.Fatalf("dim %s has duplicate state label %q", dim, meta.Label)
			}
			labels[dim][meta.Label] = true
			if prev, ok := colors[meta.Color]; ok && prev != state {
				t.Fatalf("state %s and %s share color %s", prev, state, meta.Color)
			}
			colors[meta.Color] = state
		}
	}

	neutral, _ := signalstate.StateMetaFor(signalstate.DimBoll, signalstate.StateNeutral)
	insufficient, _ := signalstate.StateMetaFor(signalstate.DimBoll, signalstate.StateInsufficient)
	if neutral.Color == insufficient.Color {
		t.Fatalf("neutral and insufficient share color %s", neutral.Color)
	}
	if neutral.Label == insufficient.Label {
		t.Fatalf("neutral and insufficient share label %q", neutral.Label)
	}
}

// TestDirectionsFollowOneTable 验证方向表：三态方向 = 自身语义，
// 数据不足不是三态之一。
func TestDirectionsFollowOneTable(t *testing.T) {
	cases := map[signalstate.State]signalstate.Direction{
		signalstate.StateBuy:          signalstate.DirectionBuy,
		signalstate.StateSell:         signalstate.DirectionSell,
		signalstate.StateNeutral:      signalstate.DirectionNeutral,
		signalstate.StateInsufficient: signalstate.DirectionInsufficient,
	}
	for state, want := range cases {
		if got := signalstate.DirectionOf(state); got != want {
			t.Fatalf("DirectionOf(%s) = %s, want %s", state, got, want)
		}
	}
}
