package sources

import "testing"

func TestNormalizeCLSStockIDVariants(t *testing.T) {
	cases := map[string]string{
		"sh688012": "688012",
		"SZ000001": "000001",
		"600519":   "600519",
		"":         "",
		"abc":      "",
		"sh12345":  "",
	}
	for in, want := range cases {
		if got := normalizeCLSStockID(in); got != want {
			t.Fatalf("normalizeCLSStockID(%q) = %q, want %q", in, got, want)
		}
	}
}

// 财联社的原生 stock_list 应被解析为确定关联
func TestCaiLianSheStockRefsAreNative(t *testing.T) {
	item := caiLianSheItem{
		ID:    123,
		Brief: "中微公司公告",
		StockList: []caiLianSheStock{
			{Name: "中微公司", StockID: "sh688012"},
		},
	}
	refs := item.stockRefs()
	if len(refs) != 1 {
		t.Fatalf("refs = %+v", refs)
	}
	if refs[0].Code != "688012" || refs[0].MatchType != "native" || refs[0].Confidence != 1.0 {
		t.Fatalf("refs[0] = %+v", refs[0])
	}
}
