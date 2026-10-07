package factorlab

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// ---------------------------------------------------------------------------
// fakes：复用 methodautomation 的依赖形状
// ---------------------------------------------------------------------------

type fakeSnapshotStore struct{ snaps []*paradigm.DatasetSnapshot }

func (f fakeSnapshotStore) List(limit, offset int) ([]*paradigm.DatasetSnapshot, error) {
	if offset >= len(f.snaps) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.snaps) {
		end = len(f.snaps)
	}
	return f.snaps[offset:end], nil
}
func (f fakeSnapshotStore) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	for _, s := range f.snaps {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errors.New("not found")
}
func (fakeSnapshotStore) VerifyContent(string) error { return nil }

type fakeUniverse struct{ codes []string }

func (f fakeUniverse) ResolveUniverse(context.Context, string, int, int) ([]string, int, error) {
	return f.codes, 0, nil
} // crossSectionalBars 生成确定的横截面数据：每只股票在长周期（40 天）里经历
// 20 天匀速上涨 + 20 天匀速下跌，相位按股票错开。5 日动量窗大多落在同一
// 趋势段内部 → 过去 5 日涨的股票未来 5 日大概率继续涨（momentum_5d 正显著）。
func crossSectionalBars(codes []string, days int) []validation.BacktestBar {
	out := make([]validation.BacktestBar, 0, len(codes)*days)
	date := func(i int) string {
		return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
	}
	const period = 40
	for ci, code := range codes {
		price := 10.0
		for i := 0; i < days; i++ {
			phase := (i + 7*ci) % period
			ret := -0.005
			if phase < period/2 {
				ret = 0.005
			}
			price *= 1 + ret
			out = append(out, validation.BacktestBar{
				Code: code, Date: date(i),
				Open: price, High: price * 1.01, Low: price * 0.99, Close: price,
				Volume: 1_000_000, Amount: 10_000_000,
			})
		}
	}
	return out
}

type barsProvider struct {
	byCode map[string][]validation.BacktestBar
}

func (p barsProvider) LoadBars(_ context.Context, _, code, _, _ string) ([]validation.BacktestBar, error) {
	if bars, ok := p.byCode[code]; ok {
		return bars, nil
	}
	return nil, errors.New("no bars")
}

func newTestService(t *testing.T, codes []string, days int) (*Service, *RunResult) {
	t.Helper()
	bars := crossSectionalBars(codes, days)
	byCode := map[string][]validation.BacktestBar{}
	for _, b := range bars {
		byCode[b.Code] = append(byCode[b.Code], b)
	}
	codesForUniverse := make([]string, 0, len(byCode))
	for code := range byCode {
		codesForUniverse = append(codesForUniverse, code)
	}
	svc, err := New(methodautomation.Deps{
		Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-1", Universe: codesForUniverse}}},
		Universe:  fakeUniverse{codes: codesForUniverse},
		Bars:      barsProvider{byCode: byCode},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, res
}

// ---------------------------------------------------------------------------
// 统计函数单测
// ---------------------------------------------------------------------------

func TestRankICPerfectPositive(t *testing.T) {
	fv := map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	fr := map[string]float64{"a": 0.01, "b": 0.02, "c": 0.03, "d": 0.04, "e": 0.05}
	ic, ok := rankIC(fv, fr)
	if !ok || math.Abs(ic-1) > 1e-9 {
		t.Fatalf("perfectly monotonic data should give IC=1, got %v ok=%v", ic, ok)
	}
}

func TestRankICTiesAveraged(t *testing.T) {
	// x 有并列（1,1,2），秩应为 (1.5,1.5,3)；与完全单调的 y 相关应 < 1。
	fv := map[string]float64{"a": 1, "b": 1, "c": 2, "d": 3, "e": 4}
	fr := map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	ic, ok := rankIC(fv, fr)
	if !ok || ic >= 1 {
		t.Fatalf("ties should reduce IC below 1, got %v ok=%v", ic, ok)
	}
}

func TestRankICZeroVarianceRejected(t *testing.T) {
	fv := map[string]float64{"a": 7, "b": 7, "c": 7, "d": 7, "e": 7}
	fr := map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	if _, ok := rankIC(fv, fr); ok {
		t.Fatal("zero-variance factor section must be rejected")
	}
	if _, ok := rankIC(map[string]float64{"a": 1, "b": 2}, fr); ok {
		t.Fatal("cross-section below minimum size must be rejected")
	}
}

func TestRanksAverageForTies(t *testing.T) {
	out, ok := ranks([]float64{10, 20, 10, 30})
	if !ok {
		t.Fatal("ranks should succeed")
	}
	// 排序后值: [10,10,20,30] → 秩 [1.5,1.5,3,4]，映射回原顺序。
	want := []float64{1.5, 3, 1.5, 4}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("rank[%d]=%v want %v (all=%v)", i, out[i], want[i], out)
		}
	}
}

func TestForwardReturnAtLooksForward(t *testing.T) {
	bar := func(i int, close float64) validation.BacktestBar {
		return validation.BacktestBar{Date: fmt.Sprintf("2024-01-%02d", i+1), Close: close}
	}
	bars := []validation.BacktestBar{bar(0, 10), bar(1, 11), bar(2, 12), bar(3, 20)}
	ret, ok := forwardReturnAt(bars, 0, 3)
	if !ok || math.Abs(ret-1.0) > 1e-9 { // 10 → 20 = +100%
		t.Fatalf("forward return must look forward, got %v ok=%v", ret, ok)
	}
	if _, ok := forwardReturnAt(bars, 1, 3); ok {
		t.Fatal("window beyond data end must be rejected")
	}
}

// ---------------------------------------------------------------------------
// 服务级测试
// ---------------------------------------------------------------------------

func TestRunEvaluatesFactorsAndProducesTopPicks(t *testing.T) {
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005", "600006", "600007"}
	_, res := newTestService(t, codes, 120)

	if res.EngineVersion != EngineVersion || res.SnapshotID != "snap-1" || res.Codes != len(codes) {
		t.Fatalf("unexpected run meta: %+v", res)
	}
	if res.Sections == 0 || res.LastDate == "" {
		t.Fatalf("run should observe cross-sections, got %+v", res)
	}
	if len(res.Factors) == 0 {
		t.Fatal("factor evals missing")
	}
	for _, f := range res.Factors {
		if f.Sections > 0 && f.Pairs <= 0 {
			t.Fatalf("factor %s has sections without pairs: %+v", f.Key, f)
		}
	}
	// 构造数据里截面动量持续分化：momentum_5d 应当显著为正。
	foundMomentum := false
	for _, f := range res.Factors {
		if f.Key == "momentum_5d" {
			foundMomentum = true
			if !f.Significant || f.MeanIC <= 0 {
				t.Fatalf("momentum_5d should be significantly positive on constructed data: %+v", f)
			}
		}
	}
	if !foundMomentum {
		t.Fatal("momentum_5d missing from evals")
	}
	if len(res.TopPicks) == 0 || len(res.TopPicks) > res.TopK {
		t.Fatalf("top picks missing or oversized: %d", len(res.TopPicks))
	}
	// Top1 必须等于「显著因子组合分」的最大者：按因子定义与 z 分在测试里重算，
	// 锁死组合契约（数据方向 × |IC| 权重 × 截面 z 分 / 总权重）。
	sigFactors := map[string]FactorEval{}
	for _, ev := range res.Factors {
		if ev.Significant {
			sigFactors[ev.Key] = ev
		}
	}
	factorsByKey := map[string]Factor{}
	for _, f := range BuiltinFactors() {
		factorsByKey[f.Key] = f
	}
	lastBars := groupByCode(crossSectionalBars(codes, 120))
	scores, totalWeight := map[string]float64{}, 0.0
	for key, ev := range sigFactors {
		vals := map[string]float64{}
		for code, bars := range lastBars {
			if v, ok := factorsByKey[key].Compute(bars); ok {
				vals[code] = v
			}
		}
		zs, ok := zscores(vals)
		if !ok {
			continue
		}
		w := math.Abs(ev.MeanIC)
		totalWeight += w
		for code, z := range zs {
			scores[code] += ev.Direction * w * z
		}
		// 去重叠：t 统计量的独立样本量应约为全截面数 / horizon。
		if ev.EffectiveSections <= 0 || ev.EffectiveSections > ev.Sections {
			t.Fatalf("factor %s effective sections %d must be in (0, %d]", key, ev.EffectiveSections, ev.Sections)
		}
		if ev.EffectiveSections < ev.Sections/5-1 || ev.EffectiveSections > ev.Sections/5+1 {
			t.Fatalf("factor %s effective sections %d should be ~= sections %d / horizon 5", key, ev.EffectiveSections, ev.Sections)
		}
	}
	if totalWeight <= 0 {
		t.Fatal("significant factors must produce a usable combination")
	}
	bestCode, bestScore := "", math.Inf(-1)
	for code, sc := range scores {
		if s := sc / totalWeight; s > bestScore {
			bestCode, bestScore = code, s
		}
	}
	if res.TopPicks[0].Code != bestCode {
		t.Fatalf("top pick %s must match combined-score argmax %s", res.TopPicks[0].Code, bestCode)
	}
	if math.Abs(res.TopPicks[0].Score-bestScore) > 1e-9 {
		t.Fatalf("top pick score %.6f != recomputed %.6f", res.TopPicks[0].Score, bestScore)
	}
	if len(res.TopPicks[0].Contributions) == 0 {
		t.Fatal("top pick must expose score contributions")
	}
	if res.Note == "" {
		t.Fatal("human-readable note missing")
	}
}

func TestRunWithoutSignificantFactorsOmitsPicksButStaysHonest(t *testing.T) {
	// 每只股票完全相同的常数价格路径：因子值零离散、前向收益零离散，
	// 全部截面无效 → 无显著因子、无 Top 名单，但结果必须如实成立。
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005"}
	svc, err := New(methodautomation.Deps{
		Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-flat", Universe: codes}}},
		Universe:  fakeUniverse{codes: codes},
		Bars:      barsProvider{byCode: groupByCode(flatBars(codes, 60))},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Factors {
		if f.Significant {
			t.Fatalf("constant data cannot be significant: %+v", f)
		}
	}
	if len(res.TopPicks) != 0 {
		t.Fatalf("no significant factors → no picks, got %d", len(res.TopPicks))
	}
	if res.Note == "" {
		t.Fatal("honest note required even without findings")
	}
}

// ToPickRun 的两条契约：有名单时 run_id 按截面日期幂等（同日重跑同键）；
// 无名单时硬拒绝（持久化层面不允许空名单占位），但这是「无产出」而非研究失败。
func TestToPickRunIdempotentKeyAndEmptyRefusal(t *testing.T) {
	res := &RunResult{
		SnapshotID: "snap-x", LastDate: "2024-02-09", SnapshotDateEnd: "2024-02-09",
		StartedAt:  time.Date(2024, 2, 9, 8, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2024, 2, 9, 8, 0, 5, 0, time.UTC),
		TopPicks:   []TopPick{{Code: "600000", Score: 0.5, Contributions: map[string]float64{"momentum_5d": 0.5}}},
		Note:       "note",
	}
	pick, err := res.ToPickRun(0)
	if err != nil {
		t.Fatal(err)
	}
	if pick.RunID != "pick-2024-02-09" || pick.AsOf != "2024-02-09" {
		t.Fatalf("run_id must be pick-<as_of>, got %s/%s", pick.RunID, pick.AsOf)
	}
	if len(pick.Picks) != 1 || pick.Picks[0].Code != "600000" {
		t.Fatalf("picks must carry through, got %+v", pick.Picks)
	}
	if len(pick.FactorsSnapshot) != 0 {
		t.Fatalf("factors snapshot should pass through (empty here), got %d", len(pick.FactorsSnapshot))
	}

	// topK>0 截断。
	res.TopPicks = append(res.TopPicks, TopPick{Code: "600001", Score: 0.4}, TopPick{Code: "600002", Score: 0.3})
	pick, err = res.ToPickRun(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(pick.Picks) != 2 || pick.Picks[0].Code != "600000" || pick.Picks[1].Code != "600001" {
		t.Fatalf("topK=2 must truncate keeping order, got %+v", pick.Picks)
	}

	// 无名单：持久化硬拒绝（Run 层面同场景返回空结果，非故障）。
	_, err = (&RunResult{LastDate: "2024-02-09"}).ToPickRun(0)
	if err == nil || !strings.Contains(err.Error(), "no significant factors") {
		t.Fatalf("empty picks must be refused with honest reason, got %v", err)
	}
}

func groupByCode(bars []validation.BacktestBar) map[string][]validation.BacktestBar {
	out := map[string][]validation.BacktestBar{}
	for _, b := range bars {
		out[b.Code] = append(out[b.Code], b)
	}
	return out
}

func flatBars(codes []string, days int) []validation.BacktestBar {
	out := make([]validation.BacktestBar, 0, len(codes)*days)
	for _, code := range codes {
		for i := 0; i < days; i++ {
			d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
			out = append(out, validation.BacktestBar{Code: code, Date: d, Open: 10, High: 10, Low: 10, Close: 10, Volume: 1_000_000, Amount: 10_000_000})
		}
	}
	return out
}

func TestRunRequiresUsableSnapshot(t *testing.T) {
	svc, err := New(methodautomation.Deps{
		Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{{ID: "tiny", Universe: []string{"600000"}}}},
		Universe:  fakeUniverse{codes: []string{"600000"}},
		Bars:      barsProvider{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Run(context.Background(), Options{}); err == nil {
		t.Fatal("single-code universe must fail closed")
	}
}

// reversalBars 生成反转结构：周期 10（5 天跌 + 5 天涨）相位错开，5 日动量窗
// 恰好落在一个相位内，过去 5 日与未来 5 日反相 → 动量 IC 显著为负。
// 用来锁死「方向由数据决定，与先验相反时如实反向」的契约。
// 注意：整体翻转收益符号不改变延续结构（正负同翻，秩相关不变），
// 必须缩短相位周期才能造出反转。
func reversalBars(codes []string, days int) map[string][]validation.BacktestBar {
	out := make(map[string][]validation.BacktestBar, len(codes))
	date := func(i int) string {
		return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format("2006-01-02")
	}
	const period = 10
	for ci, code := range codes {
		price := 10.0
		for i := 0; i < days; i++ {
			phase := (i + 3*ci) % period
			ret := 0.005
			if phase < period/2 {
				ret = -0.005
			}
			price *= 1 + ret
			out[code] = append(out[code], validation.BacktestBar{
				Code: code, Date: date(i),
				Open: price, High: price * 1.01, Low: price * 0.99, Close: price,
				Volume: 1_000_000, Amount: 10_000_000,
			})
		}
	}
	return out
}

func TestRunLetsDataDecideFactorDirection(t *testing.T) {
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005", "600006", "600007"}
	svc, err := New(methodautomation.Deps{
		Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-inv", Universe: codes}}},
		Universe:  fakeUniverse{codes: codes},
		Bars:      barsProvider{byCode: reversalBars(codes, 120)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// momentum_5d 先验 +1，但构造数据里短期动量显著为负：方向必须翻转而非判不显著。
	found := false
	for _, f := range res.Factors {
		if f.Key != "momentum_5d" {
			continue
		}
		found = true
		if !f.Significant {
			t.Fatalf("momentum_5d must stay significant on inverted data (direction flips, not dropped): %+v", f)
		}
		if f.MeanIC >= 0 || f.Direction != -1 || f.Prior != 1 {
			t.Fatalf("momentum_5d direction must come from data: %+v", f)
		}
	}
	if !found {
		t.Fatal("momentum_5d missing from evals")
	}
	if len(res.TopPicks) == 0 {
		t.Fatal("flipped factors must still contribute to picks")
	}
	if !strings.Contains(res.Note, "与先验相反") {
		t.Fatalf("note must disclose direction flips: %s", res.Note)
	}
}

func TestRunReportsSnapshotStaleness(t *testing.T) {
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005"}
	makeSvc := func(end string, now time.Time) *Service {
		svc, err := New(methodautomation.Deps{
			Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{
				{ID: "snap-stale", Universe: codes, DateRange: paradigm.DateRange{End: end}},
			}},
			Universe: fakeUniverse{codes: codes},
			Bars:     barsProvider{byCode: groupByCode(flatBars(codes, 60))},
			Now:      func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	today := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

	fresh := makeSvc("2026-10-06", today)
	res, err := fresh.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StaleDays != 1 || res.SnapshotDateEnd != "2026-10-06" {
		t.Fatalf("fresh snapshot must report staleness honestly: end=%s stale=%d", res.SnapshotDateEnd, res.StaleDays)
	}
	if strings.Contains(res.Note, "过时") {
		t.Fatalf("fresh snapshot must not warn stale: %s", res.Note)
	}

	old := makeSvc("2026-08-01", today)
	res, err = old.Run(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StaleDays != 67 {
		t.Fatalf("stale snapshot must count days honestly, got %d", res.StaleDays)
	}
	if !strings.Contains(res.Note, "过时") || !strings.Contains(res.Note, "2026-08-01") {
		t.Fatalf("stale snapshot must warn in note: %s", res.Note)
	}
}

func TestRunHorizonAndTopKOptions(t *testing.T) {
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005", "600006", "600007"}
	bars := crossSectionalBars(codes, 40)
	byCode := map[string][]validation.BacktestBar{}
	for _, b := range bars {
		byCode[b.Code] = append(byCode[b.Code], b)
	}
	svc, err := New(methodautomation.Deps{
		Snapshots: fakeSnapshotStore{snaps: []*paradigm.DatasetSnapshot{{ID: "snap-1", Universe: codes}}},
		Universe:  fakeUniverse{codes: codes},
		Bars:      barsProvider{byCode: byCode},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Run(context.Background(), Options{HorizonDays: 3, TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.HorizonDays != 3 || res.TopK != 2 {
		t.Fatalf("options not respected: %+v", res)
	}
	if len(res.TopPicks) > 2 {
		t.Fatalf("topK must cap picks, got %d", len(res.TopPicks))
	}
}
