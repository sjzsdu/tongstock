package methodregistry

import "testing"

// 拒绝原因必须去掉随机 hash 后缀再聚合：ai_critic 的原因码形如
// hard_blocker:ss-trades-<methodHash>，同一类别会被不同方法的 hash
// 拆成 N 条独立计数，统计分布失去意义。
func TestNormalizeRejectReasonStripsRandomSuffixes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hard_blocker:ss-trades-0797f2ae6b8fa808c7d66b8fb1036d66", "hard_blocker:ss-trades"},
		{"hard_blocker:bl-underperform-36ad20900a1016dff43a2882ec2604b4", "hard_blocker:bl-underperform"},
		{"hard_blocker:bl-excess-abc", "hard_blocker:bl-excess"},
		{"hard_blocker:bl-sharpe-abc", "hard_blocker:bl-sharpe"},
		{"hard_blocker:bl-return-abc", "hard_blocker:bl-return"},
		{"hard_blocker:ss-sample-abc", "hard_blocker:ss-sample"},
		{"multiple_testing_not_significant", "multiple_testing_not_significant"},
		{"oos_sharpe_below_threshold", "oos_sharpe_below_threshold"},
	}
	for _, c := range cases {
		if got := NormalizeRejectReason(c.in); got != c.want {
			t.Errorf("NormalizeRejectReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 归一后的原因必须落进可读类别（reject-stats 前端展示与反哺研究依赖它）。
func TestReasonCategoryForNormalizedReasons(t *testing.T) {
	cases := map[string]string{
		"hard_blocker:ss-trades":           "insufficient_trades",
		"hard_blocker:bl-underperform":     "below_baseline",
		"hard_blocker:bl-excess":           "below_baseline",
		"hard_blocker:bl-sharpe":           "below_baseline",
		"multiple_testing_not_significant": "multiple_testing",
		"oos_sharpe_below_threshold":       "weak_oos_performance",
	}
	for reason, want := range cases {
		if got := ReasonCategory(reason); got != want {
			t.Errorf("ReasonCategory(%q) = %q, want %q", reason, got, want)
		}
	}
}
