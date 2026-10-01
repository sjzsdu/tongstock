package protocol

import "testing"

// 回归：指数 K 线代码市场路由。此前 000001 被按股票规则路由到深市，
// 上证指数查成了平安银行，返回乱码数据（负价格、异常日期）。
func TestDecodeIndexCode(t *testing.T) {
	cases := []struct {
		code   string
		market byte
	}{
		{"000001", byte(ExchangeSH)}, // 上证指数
		{"000300", byte(ExchangeSH)}, // 沪深300（通达信代码）
		{"999999", byte(ExchangeSH)}, // 上证指数（旧代码）
		{"399001", byte(ExchangeSZ)}, // 深证成指
		{"399006", byte(ExchangeSZ)}, // 创业板指
		{"399300", byte(ExchangeSZ)}, // 沪深300（深交所真实代码）
		{"880003", byte(ExchangeSH)}, // 通达信自编板块指数
		{"sh000001", byte(ExchangeSH)},
		{"sz399001", byte(ExchangeSZ)},
	}
	for _, tc := range cases {
		ex, num, err := DecodeIndexCode(tc.code)
		if err != nil {
			t.Fatalf("DecodeIndexCode(%q): %v", tc.code, err)
		}
		if ex != tc.market {
			t.Errorf("DecodeIndexCode(%q) market = %d, want %d", tc.code, ex, tc.market)
		}
		if num != tc.code[len(tc.code)-6:] {
			t.Errorf("DecodeIndexCode(%q) num = %q", tc.code, num)
		}
	}

	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, _, err := DecodeIndexCode(bad); err == nil {
			t.Errorf("DecodeIndexCode(%q) should fail", bad)
		}
	}
}
