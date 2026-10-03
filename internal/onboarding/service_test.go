package onboarding_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/onboarding"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// ---- fakes ----

type fakeFreshness struct{ latest string }

func (f *fakeFreshness) LatestKlineDate() (string, error) { return f.latest, nil }

type fakeUniverse struct{ codes []string }

func (f *fakeUniverse) BuildUniverse(date string, def marketsnapshot.UniverseDefinition) ([]marketsnapshot.UniverseMember, error) {
	out := make([]marketsnapshot.UniverseMember, 0, len(f.codes))
	for _, c := range f.codes {
		out = append(out, marketsnapshot.UniverseMember{Code: c, Name: "stock " + c, Status: "normal", Selected: true})
	}
	return out, nil
}

// fakeWatermarks 复刻线上现场：全宇宙只差最后一天——绝大多数代码日线到
// 2026-09-28，仅极少数“种子”代码到了 2026-09-30。
type fakeWatermarks struct {
	majorityDate string
	aheadCodes   map[string]string // code -> 更新的 last_date
}

func (f *fakeWatermarks) FetchWatermarks(date string, codes []string) (map[string]marketsnapshot.CodeStatus, error) {
	out := map[string]marketsnapshot.CodeStatus{}
	for _, c := range codes {
		last := f.majorityDate
		if ahead, ok := f.aheadCodes[c]; ok {
			last = ahead
		}
		out[c] = marketsnapshot.CodeStatus{Code: c, KlineLastDate: last}
	}
	return out, nil
}

type fakeFeatures struct{}

func (fakeFeatures) Compute(date string, codes []string, features []marketsnapshot.FeatureSpec) (map[string]map[string]float64, error) {
	out := map[string]map[string]float64{}
	for _, c := range codes {
		out[c] = map[string]float64{"close": 10.0}
	}
	return out, nil
}

type memRepo struct {
	market  map[string]*marketsnapshot.MarketSnapshot
	feature map[string]*marketsnapshot.FeatureSnapshot
}

func newMemRepo() *memRepo {
	return &memRepo{market: map[string]*marketsnapshot.MarketSnapshot{}, feature: map[string]*marketsnapshot.FeatureSnapshot{}}
}

func (r *memRepo) SaveMarketSnapshot(s *marketsnapshot.MarketSnapshot) error {
	r.market[s.ID] = s
	return nil
}
func (r *memRepo) LoadMarketSnapshot(id string, includeCodes bool) (*marketsnapshot.MarketSnapshot, error) {
	return r.market[id], nil
}
func (r *memRepo) FindMarketSnapshot(date, universeName, adj string) (*marketsnapshot.MarketSnapshot, error) {
	return nil, nil
}
func (r *memRepo) ListMarketSnapshots(dateStart, dateEnd, status string) ([]*marketsnapshot.MarketSnapshot, error) {
	return nil, nil
}
func (r *memRepo) FreezeMarketSnapshot(id string) error {
	if s := r.market[id]; s != nil {
		s.Frozen = true
	}
	return nil
}
func (r *memRepo) SaveFeatureSnapshot(s *marketsnapshot.FeatureSnapshot) error {
	r.feature[s.ID] = s
	return nil
}
func (r *memRepo) LoadFeatureSnapshot(id string, includeValues bool) (*marketsnapshot.FeatureSnapshot, error) {
	return r.feature[id], nil
}
func (r *memRepo) ListFeatureSnapshots(marketSnapshotID string) ([]*marketsnapshot.FeatureSnapshot, error) {
	return nil, nil
}

type fakeSelection struct{}

func (fakeSelection) Run(ctx context.Context, req selection.Request) (*selection.Run, error) {
	return &selection.Run{ID: "run_fake", CandidateCount: 1, BuyCount: 1, ScannedStocks: 300, EligibleMethods: 1}, nil
}

// ---- the bug ----

// 线上现场（SUN-4）：一键走通 blocked，快照状态 failed：coverage=0.33% 低于 80%。
// 库内 5222 只股票里 5109 只日线到 2026-09-28，只有 17 只到 2026-09-30；
// Freshness.LatestKlineDate 用 MAX(date) 取到 2026-09-30，一键流程就以它为
// 交易日构建快照 —— 而该日全宇宙只有 0.33% 的代码有数据。第 1 步刚宣布
// “已就绪”，第 2 步就被覆盖率门槛阻断，自相矛盾。
//
// 期望：交易日应取宇宙数据真正支撑的日期（2026-09-28，覆盖率 99.7%），
// 一键流程完整走通。这里用 300 只股票、1 只领先复刻同样的现场
// （1/300 = 0.33%）。当前实现下该测试失败，即为本 bug 的最小复现。
func TestRunCompletesWhenUniverseDataLagsFreshnessMax(t *testing.T) {
	codes := make([]string, 0, 300)
	for i := 1; i <= 300; i++ {
		codes = append(codes, fmt.Sprintf("%06d", i))
	}
	repo := newMemRepo()
	svc, err := onboarding.NewService(onboarding.Deps{
		Builder: marketsnapshot.NewBuilder(&fakeUniverse{codes: codes}, &fakeWatermarks{
			majorityDate: "2026-09-28",
			aheadCodes:   map[string]string{"000001": "2026-09-30"},
		}, nil),
		Snapshots: repo,
		Features:  fakeFeatures{},
		Selection: fakeSelection{},
		Freshness: &fakeFreshness{latest: "2026-09-30"},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), onboarding.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != onboarding.StatusCompleted {
		t.Fatalf("one-click flow blocked: status=%s blocked_reason=%q steps=%+v",
			res.Status, res.BlockedReason, res.Steps)
	}
	if res.TradeDate != "2026-09-28" {
		t.Fatalf("trade date = %s, want 2026-09-28 (the date the universe data actually supports)", res.TradeDate)
	}
	if res.SelectionRunID == "" {
		t.Fatalf("selection never ran; steps=%+v", res.Steps)
	}
}
