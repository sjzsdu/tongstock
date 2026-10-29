package selection_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/factorlab"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// 本文件覆盖「每个选股方法在单次 run 中最多贡献 5 个 buy 候选」的口径。
//
// 方法分数档位由 evidence 决定（见 engine.go methodScore），测试固定用：
//
//	strong + OOSTrades>=100                     → 0.76
//	strong + OOSTrades<100                      → 0.72
//	strong + OOSTrades>=100 + 回撤超 20%        → 0.68
//
// 候选聚合分 = 最高 methodScore + 其余每个触发方法 min(0.03, score*0.04)，
// 所以同方法内（amount 都有值）methodScore 全等，排名差异只可能来自「还触发了
// 哪些别的方法」——这正是 per-method 截断必须看聚合分、不能看 Code 的原因。

// capMethodSpec 描述一个参与名额竞争的方法：signal 是它读取的截面特征名
// （1=触发、0=不触发），evidence 决定它的 methodScore。
type capMethodSpec struct {
	id       string
	family   string
	signal   string
	evidence *methodregistry.EvidenceSummary
}

// variant 让同族方法在注册表里各自成立（family_id+variant_id 唯一）。
func (s capMethodSpec) variant() string { return s.id }

type capFixture struct {
	codes   []string
	methods []capMethodSpec
	// triggers: 方法 ID → 该方法 signal=1 的股票集合。未列出的方法只触发空集。
	triggers map[string][]string
}

type capEnv struct {
	t           *testing.T
	ctx         context.Context
	snapshots   *marketsnapshotrepo.SQLiteRepository
	methodsRepo *methodregistryrepo.SQLiteRepository
	runs        *selectionrepo.SQLiteRepository
	engine      *selection.Engine
	marketID    string
	signals     map[string]map[string]float64
	featureIDs  []string
}

func capEvidence(confidence string, oosTrades int, maxDrawdown float64) *methodregistry.EvidenceSummary {
	return &methodregistry.EvidenceSummary{
		ResultHash:     fmt.Sprintf("ev-%s-%d", confidence, oosTrades),
		SnapshotID:     "validation-cap",
		Confidence:     confidence,
		Passable:       true,
		OOSTrades:      oosTrades,
		OOSMaxDrawdown: maxDrawdown,
	}
}

// seqCodes 生成 prefix+"001"..prefix+"00N" 这样的连续代码。
func seqCodes(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("%s%03d", prefix, i))
	}
	return out
}

// newCapEnv 建一套真实仓库（sqlite）+ 市场快照 + 各方法，返回可直接 Run 的环境。
func newCapEnv(t *testing.T, f capFixture) *capEnv {
	t.Helper()
	ctx := context.Background()
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "buycap.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	snapshots, _ := marketsnapshotrepo.New(store)
	methodsRepo, _ := methodregistryrepo.New(store)
	runs, _ := selectionrepo.New(store)

	// 每个方法的 signal 在每只股票上都必须存在（否则会走 insufficient_data）。
	triggered := map[string]map[string]bool{}
	signalByMethod := make([]string, 0, len(f.methods))
	for _, spec := range f.methods {
		signalByMethod = append(signalByMethod, spec.signal)
		set := map[string]bool{}
		for _, code := range f.triggers[spec.id] {
			set[code] = true
		}
		triggered[spec.id] = set
	}

	signals := map[string]map[string]float64{}
	members := make([]marketsnapshot.UniverseMember, 0, len(f.codes))
	statuses := make([]marketsnapshot.CodeStatus, 0, len(f.codes))
	for _, code := range f.codes {
		members = append(members, marketsnapshot.UniverseMember{Code: code, Selected: true, Status: "normal"})
		statuses = append(statuses, marketsnapshot.CodeStatus{Code: code, UniverseMember: true, KlineLastDate: "2024-01-02", KlineRowCount: 260})
		values := map[string]float64{"close": 10.4, "open": 10.1, "high": 10.5, "low": 10.0, "volume": 1e6, "ma20": 9.8, "amount": 1.2e8}
		for _, spec := range f.methods {
			values[spec.signal] = 0
			if triggered[spec.id][code] {
				values[spec.signal] = 1
			}
		}
		signals[code] = values
	}

	market := &marketsnapshot.MarketSnapshot{ID: "cap-market", SnapshotDate: "2024-01-02", Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, Market: "CN-A", PriceAdjustment: "forward", ExpectedKlineCodes: len(f.codes), ReadyKlineCodes: len(f.codes), CoveragePct: 1, Status: marketsnapshot.StatusReady, UniverseMembers: members, Codes: statuses}
	market.UniverseHash = marketsnapshot.ComputeUniverseHash(market.UniverseMembers)
	market.ContentHash, _ = marketsnapshot.ComputeContentHash(market)
	if err := snapshots.SaveMarketSnapshot(market); err != nil {
		t.Fatal(err)
	}
	if err := snapshots.FreezeMarketSnapshot(market.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	stop, take, pct := -.05, .1, .08
	for _, spec := range f.methods {
		compiled, _, err := methods.Compile(&methods.Candidate{
			Name:           spec.id + " 截面信号方法",
			Universe:       "universe_usable",
			Entry:          map[string]any{"type": "compare", "left": map[string]any{"type": "indicator", "indicator": spec.signal}, "right": map[string]any{"type": "constant", "value": 0.5}, "op": "gt"},
			HoldingMaxDays: 10, StopLossPct: &stop, TakeProfitPct: &take, PositionMode: "pct_equity", PositionPct: &pct,
		})
		if err != nil {
			t.Fatalf("compile %s: %v", spec.id, err)
		}
		method := &methodregistry.Method{ID: spec.id, FamilyID: spec.family, VariantID: spec.variant(), Name: compiled.Name, Status: methodregistry.StatusVerified, Market: "A", Universe: "universe_usable", HoldingMaxDays: 10, CurrentVersion: 1, Versions: []methodregistry.MethodVersion{{ID: spec.id + "-v1", Version: 1, MethodHash: compiled.ContentHash, Method: compiled, Evidence: spec.evidence, CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
		if err := methodsRepo.Save(ctx, method, methodregistry.AuditEvent{ID: "audit-" + spec.id, MethodID: spec.id, From: methodregistry.StatusCandidate, To: methodregistry.StatusVerified, Action: "policy", Automatic: true, CreatedAt: now}); err != nil {
			t.Fatalf("save %s: %v", spec.id, err)
		}
	}

	engine, err := selection.NewEngine(snapshots, methodsRepo, runs)
	if err != nil {
		t.Fatal(err)
	}
	return &capEnv{t: t, ctx: ctx, snapshots: snapshots, methodsRepo: methodsRepo, runs: runs, engine: engine, marketID: market.ID, signals: signals}
}

// addFeatureSnapshot 只为给定代码建特征快照（用于「截断前/截断后」同快照对比）。
func (e *capEnv) addFeatureSnapshot(codes []string) string {
	e.t.Helper()
	values := map[string]map[string]float64{}
	featureIDs := []string{"close", "open", "high", "low", "volume", "ma20", "amount"}
	for _, code := range codes {
		src, ok := e.signals[code]
		if !ok {
			e.t.Fatalf("code %s is not part of the fixture", code)
		}
		dst := make(map[string]float64, len(src))
		for name, value := range src {
			dst[name] = value
			if strings.HasPrefix(name, "sig_") {
				featureIDs = append(featureIDs, name)
			}
		}
		values[code] = dst
	}
	sort.Strings(featureIDs)
	featureIDs = dedupeCapStrings(featureIDs)
	id := fmt.Sprintf("cap-features-%d", len(e.featureIDs)+1)
	feature := &marketsnapshot.FeatureSnapshot{ID: id, MarketSnapshotID: e.marketID, SnapshotDate: "2024-01-02", FeatureIDs: featureIDs, FeatureTotal: len(featureIDs), RowsWritten: len(values), LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady, AsOfNs: time.Date(2024, 1, 2, 15, 0, 0, 0, time.Local).UnixNano(), Values: values}
	feature.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(feature)
	if err := e.snapshots.SaveFeatureSnapshot(feature); err != nil {
		e.t.Fatal(err)
	}
	e.featureIDs = append(e.featureIDs, id)
	return id
}

func (e *capEnv) run(featureID string) *selection.Run {
	e.t.Helper()
	run, err := e.engine.Run(e.ctx, selection.Request{MarketSnapshotID: e.marketID, FeatureSnapshotID: featureID})
	if err != nil {
		e.t.Fatal(err)
	}
	return run
}

func dedupeCapStrings(in []string) []string {
	out := in[:0]
	seen := map[string]bool{}
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func capCandidate(t *testing.T, run *selection.Run, code string) selection.Candidate {
	t.Helper()
	for _, c := range run.Candidates {
		if c.Code == code {
			return c
		}
	}
	t.Fatalf("candidate %s missing from run (candidates: %v)", code, capCodes(run))
	return selection.Candidate{}
}

func capCodes(run *selection.Run) []string {
	out := make([]string, 0, len(run.Candidates))
	for _, c := range run.Candidates {
		out = append(out, c.Code)
	}
	sort.Strings(out)
	return out
}

// keptCodes 返回某个方法保住的候选代码（已排序），truncatedCodes 返回被它截断的。
func keptCodes(run *selection.Run, methodID string) []string {
	out := []string{}
	for _, c := range run.Candidates {
		for _, id := range c.KeptByMethodIDs {
			if id == methodID {
				out = append(out, c.Code)
			}
		}
	}
	sort.Strings(out)
	return out
}

func truncatedCodes(run *selection.Run, methodID string) []string {
	out := []string{}
	for _, c := range run.Candidates {
		for _, id := range c.TruncatedByMethodIDs {
			if id == methodID {
				out = append(out, c.Code)
			}
		}
	}
	sort.Strings(out)
	return out
}

func capBuys(run *selection.Run) []string {
	out := []string{}
	for _, c := range run.Candidates {
		if c.Action == selection.ActionBuy {
			out = append(out, c.Code)
		}
	}
	sort.Strings(out)
	return out
}

func assertCodes(t *testing.T, label string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

// T1：单方法触发 8 只全达标 → 出 5 只，BuyCountByMethod[m]==5，3 只降级 watch。
func TestPerMethodBuyCapT1SingleMethodKeepsFive(t *testing.T) {
	codes := seqCodes("300", 8)
	env := newCapEnv(t, capFixture{
		codes:   codes,
		methods: []capMethodSpec{{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)}},
		triggers: map[string][]string{
			"m-a": codes,
		},
	})
	run := env.run(env.addFeatureSnapshot(codes))

	if run.BuyCapPerMethod != 5 {
		t.Fatalf("BuyCapPerMethod = %d, want 5", run.BuyCapPerMethod)
	}
	if run.BuyCount != 5 {
		t.Fatalf("BuyCount = %d, want 5 (买: %v)", run.BuyCount, capBuys(run))
	}
	if got := run.BuyCountByMethod["m-a"]; got != 5 {
		t.Fatalf("BuyCountByMethod[m-a] = %d, want 5 (map: %v)", got, run.BuyCountByMethod)
	}
	if run.TruncatedFromBuy != 3 {
		t.Fatalf("TruncatedFromBuy = %d, want 3", run.TruncatedFromBuy)
	}
	// 同方法内 methodScore 全等，并列按 Code 升序：前 5 只留下，后 3 只降级。
	assertCodes(t, "buy codes", capBuys(run), codes[:5])
	for _, code := range codes[5:] {
		c := capCandidate(t, run, code)
		if c.Action != selection.ActionWatch || !c.TruncatedFromBuy {
			t.Fatalf("%s 应降级 watch 且标记 TruncatedFromBuy，实际 action=%s truncated=%v", code, c.Action, c.TruncatedFromBuy)
		}
		if len(c.KeptByMethodIDs) != 0 {
			t.Fatalf("%s 不应被任何方法保住，实际 %v", code, c.KeptByMethodIDs)
		}
		if !reflect.DeepEqual(c.TruncatedByMethodIDs, []string{"m-a"}) {
			t.Fatalf("%s TruncatedByMethodIDs = %v, want [m-a]", code, c.TruncatedByMethodIDs)
		}
	}
	// Rank 仍按全局排序连续编号。
	for i, c := range run.Candidates {
		if c.Rank != i+1 {
			t.Fatalf("rank 不连续：第 %d 个候选 rank=%d", i, c.Rank)
		}
	}
}

// T2：两方法各 8 只无交集 → 总 buy 10，各 5。
func TestPerMethodBuyCapT2TwoDisjointMethodsKeepFiveEach(t *testing.T) {
	codesA := seqCodes("310", 8)
	codesB := seqCodes("320", 8)
	all := append(append([]string{}, codesA...), codesB...)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{"m-a": codesA, "m-b": codesB},
	})
	run := env.run(env.addFeatureSnapshot(all))

	if run.BuyCount != 10 {
		t.Fatalf("BuyCount = %d, want 10 (买: %v)", run.BuyCount, capBuys(run))
	}
	want := map[string]int{"m-a": 5, "m-b": 5}
	if !reflect.DeepEqual(run.BuyCountByMethod, want) {
		t.Fatalf("BuyCountByMethod = %v, want %v", run.BuyCountByMethod, want)
	}
	for _, count := range run.BuyCountByMethod {
		if count > 5 {
			t.Fatalf("单个方法名额被突破：%v", run.BuyCountByMethod)
		}
	}
}

// T3：两方法各 3 只 → 总 6，各 3（不跨方法借名额）。
func TestPerMethodBuyCapT3NoCrossMethodBorrowing(t *testing.T) {
	codesA := seqCodes("330", 3)
	codesB := seqCodes("340", 3)
	all := append(append([]string{}, codesA...), codesB...)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{"m-a": codesA, "m-b": codesB},
	})
	run := env.run(env.addFeatureSnapshot(all))

	if run.BuyCount != 6 {
		t.Fatalf("BuyCount = %d, want 6 (买: %v)", run.BuyCount, capBuys(run))
	}
	want := map[string]int{"m-a": 3, "m-b": 3}
	if !reflect.DeepEqual(run.BuyCountByMethod, want) {
		t.Fatalf("BuyCountByMethod = %v, want %v", run.BuyCountByMethod, want)
	}
	if run.TruncatedFromBuy != 0 {
		t.Fatalf("名额没满就不该有降级，TruncatedFromBuy = %d", run.TruncatedFromBuy)
	}
}

// T4：票 X 被 A 排第 3、被 B 排第 8 → buy，KeptByMethodIDs==[A]、TruncatedByMethodIDs==[B]。
func TestPerMethodBuyCapT4KeptByOneTruncatedByOther(t *testing.T) {
	w1, w2 := "100001", "100002"
	x := "100003"
	high := []string{"100011", "100012", "100013", "100014", "100015", "100016", "100017"}
	all := append([]string{w1, w2, x}, high...)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			// A / C 同为 0.76，B 0.72，D 0.68。
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
			{id: "m-c", family: "fam-c", signal: "sig_c", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-d", family: "fam-d", signal: "sig_d", evidence: capEvidence("strong", 120, -.25)},
		},
		triggers: map[string][]string{
			// W1/W2 = A+C → 0.76+0.03 = 0.79
			"m-a": {w1, w2, x},
			// X = A+B → 0.76+0.0288 = 0.7888；被 A 排第 3（S_A 只有 3 只）
			"m-b": append([]string{x}, high...),
			// 高位票 = B+C+D → 0.76+0.0288+0.0272 = 0.816 > X，把 X 挤到 B 的第 8 名
			"m-c": append([]string{w1, w2}, high...),
			"m-d": high,
		},
	})
	run := env.run(env.addFeatureSnapshot(all))

	c := capCandidate(t, run, x)
	if c.Action != selection.ActionBuy {
		t.Fatalf("X 应保住 buy（被 A 排第 3），实际 action=%s truncated=%v", c.Action, c.TruncatedFromBuy)
	}
	if !reflect.DeepEqual(c.KeptByMethodIDs, []string{"m-a"}) {
		t.Fatalf("X KeptByMethodIDs = %v, want [m-a]", c.KeptByMethodIDs)
	}
	if !reflect.DeepEqual(c.TruncatedByMethodIDs, []string{"m-b"}) {
		t.Fatalf("X TruncatedByMethodIDs = %v, want [m-b]", c.TruncatedByMethodIDs)
	}
	if c.Score != 0.7888 {
		t.Fatalf("X Score = %v, want 0.7888", c.Score)
	}
	if c.TruncatedFromBuy {
		t.Fatal("保住名额的候选不该带 TruncatedFromBuy")
	}
	// X 在方法 A 内排第 3：A 只触发 W1/W2/X，三只都在名额内。
	assertCodes(t, "m-a kept", keptCodes(run, "m-a"), []string{w1, w2, x})
	// X 在方法 B 内被截断（B 触发 8 只，X 排最后）。
	if got := truncatedCodes(run, "m-b"); len(got) != 3 || !containsCode(got, x) {
		t.Fatalf("m-b truncated = %v, want 含 X 的 3 只", got)
	}
}

// T5：票 Y 被 A 排第 7、被 B 排第 9 → watch，TruncatedFromBuy==true。
func TestPerMethodBuyCapT5TruncatedEverywhereDowngrades(t *testing.T) {
	others := seqCodes("200", 8) // A、B、C 都触发，聚合分 0.8188
	y := "200999"                // 只被 A、B 触发，聚合分 0.7888
	all := append(append([]string{}, others...), y)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
			{id: "m-c", family: "fam-c", signal: "sig_c", evidence: capEvidence("strong", 120, -.1)},
		},
		triggers: map[string][]string{
			"m-a": append([]string{}, all...),
			"m-b": append([]string{}, all...),
			"m-c": others,
		},
	})
	run := env.run(env.addFeatureSnapshot(all))

	c := capCandidate(t, run, y)
	if c.Score < 0.65 || !c.Exit.Complete {
		t.Fatalf("前置假设失败：Y 本应达到 buy 门槛 score=%v exit.complete=%v", c.Score, c.Exit.Complete)
	}
	if c.Action != selection.ActionWatch {
		t.Fatalf("Y Action = %s, want watch（被两个方法都截在名额外）", c.Action)
	}
	if !c.TruncatedFromBuy {
		t.Fatal("Y 应标记 TruncatedFromBuy")
	}
	if len(c.KeptByMethodIDs) != 0 {
		t.Fatalf("Y KeptByMethodIDs = %v, want 空", c.KeptByMethodIDs)
	}
	if !reflect.DeepEqual(c.TruncatedByMethodIDs, []string{"m-a", "m-b"}) {
		t.Fatalf("Y TruncatedByMethodIDs = %v, want [m-a m-b]", c.TruncatedByMethodIDs)
	}
	// Y 在 A、B 内都排第 9（8 只高分票都在它前面）。
	if got := keptCodes(run, "m-a"); containsCode(got, y) {
		t.Fatalf("Y 不该在 m-a 名额内：%v", got)
	}
	if got := keptCodes(run, "m-b"); containsCode(got, y) {
		t.Fatalf("Y 不该在 m-b 名额内：%v", got)
	}
	if run.BuyCount != 5 {
		t.Fatalf("BuyCount = %d, want 5 (买: %v)", run.BuyCount, capBuys(run))
	}
}

// T6：同族方法各 8 只 → 族内只留代表计名额，总 buy ≤ 5。
func TestPerMethodBuyCapT6SameFamilyCountsOnce(t *testing.T) {
	codes := seqCodes("350", 8)
	env := newCapEnv(t, capFixture{
		codes: codes,
		methods: []capMethodSpec{
			{id: "m-a1", family: "fam-same", signal: "sig_a1", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-a2", family: "fam-same", signal: "sig_a2", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{"m-a1": codes, "m-a2": codes},
	})
	run := env.run(env.addFeatureSnapshot(codes))

	if run.BuyCount != 5 {
		t.Fatalf("BuyCount = %d, want 5（同族不重复占额，买: %v）", run.BuyCount, capBuys(run))
	}
	want := map[string]int{"m-a1": 5}
	if !reflect.DeepEqual(run.BuyCountByMethod, want) {
		t.Fatalf("BuyCountByMethod = %v, want %v（族内只认代表 m-a1）", run.BuyCountByMethod, want)
	}
	for _, c := range run.Candidates {
		if len(c.Triggers) != 1 {
			t.Fatalf("%s 的 triggers 应已同族去重，实际 %d 条", c.Code, len(c.Triggers))
		}
		if containsCode(c.KeptByMethodIDs, "m-a2") || containsCode(c.TruncatedByMethodIDs, "m-a2") {
			t.Fatalf("%s 不该把被挤出同族的 m-a2 计入名额：kept=%v truncated=%v", c.Code, c.KeptByMethodIDs, c.TruncatedByMethodIDs)
		}
	}
}

// T7：同快照重跑 → BuyCountByMethod、KeptByMethodIDs 完全一致。
func TestPerMethodBuyCapT7ReplayIsDeterministic(t *testing.T) {
	codes := seqCodes("360", 9)
	others := seqCodes("370", 4)
	all := append(append([]string{}, codes...), others...)
	fixture := func() capFixture {
		return capFixture{
			codes: all,
			methods: []capMethodSpec{
				{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
				{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
				{id: "m-c", family: "fam-c", signal: "sig_c", evidence: capEvidence("strong", 120, -.1)},
			},
			triggers: map[string][]string{
				"m-a": append(append([]string{}, codes...), others...),
				"m-b": codes,
				"m-c": append(append([]string{}, codes[:4]...), others...),
			},
		}
	}
	// 两个独立环境（独立库、独立计算路径），排除 map 迭代顺序带来的抖动。
	firstEnv := newCapEnv(t, fixture())
	first := firstEnv.run(firstEnv.addFeatureSnapshot(all))
	secondEnv := newCapEnv(t, fixture())
	second := secondEnv.run(secondEnv.addFeatureSnapshot(all))

	if !reflect.DeepEqual(first.BuyCountByMethod, second.BuyCountByMethod) {
		t.Fatalf("两次独立运行的 BuyCountByMethod 不一致：%v vs %v", first.BuyCountByMethod, second.BuyCountByMethod)
	}
	if first.BuyCount != second.BuyCount || len(first.Candidates) != len(second.Candidates) {
		t.Fatalf("两次独立运行结果规模不一致：buy %d/%d candidates %d/%d", first.BuyCount, second.BuyCount, len(first.Candidates), len(second.Candidates))
	}
	for _, c := range first.Candidates {
		other := capCandidate(t, second, c.Code)
		if !reflect.DeepEqual(c.KeptByMethodIDs, other.KeptByMethodIDs) {
			t.Fatalf("%s KeptByMethodIDs 不稳定：%v vs %v", c.Code, c.KeptByMethodIDs, other.KeptByMethodIDs)
		}
		if !reflect.DeepEqual(c.TruncatedByMethodIDs, other.TruncatedByMethodIDs) {
			t.Fatalf("%s TruncatedByMethodIDs 不稳定：%v vs %v", c.Code, c.TruncatedByMethodIDs, other.TruncatedByMethodIDs)
		}
		if c.Action != other.Action || c.Score != other.Score {
			t.Fatalf("%s 判定不稳定：%s/%v vs %s/%v", c.Code, c.Action, c.Score, other.Action, other.Score)
		}
	}
	// 同一个引擎重放（走 run hash 缓存）也必须逐位一致。
	replay := firstEnv.run(firstEnv.featureIDs[0])
	if replay.ID != first.ID {
		t.Fatalf("同快照重跑没有命中同一 run：%s vs %s", replay.ID, first.ID)
	}
	if !reflect.DeepEqual(replay.BuyCountByMethod, first.BuyCountByMethod) {
		t.Fatalf("重放后 BuyCountByMethod 变了：%v vs %v", replay.BuyCountByMethod, first.BuyCountByMethod)
	}
}

// T8：票 X 被 B 截断前后 → Score 逐位相等（截断不参与、不修改分数）。
func TestPerMethodBuyCapT8ScoreIsInvariantUnderTruncation(t *testing.T) {
	plain := seqCodes("000", 8) // 0000001..，X 取字典序最大，同分时排最后
	x := "000009"
	all := append(append([]string{}, plain...), x)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{
			"m-a": append([]string{}, all...),
			"m-b": append([]string{}, all...),
		},
	})
	// 截断前：快照里只有 3 只，X 在名额内 → buy。
	before := env.run(env.addFeatureSnapshot([]string{plain[0], plain[1], x}))
	// 截断后：快照里 9 只同分，X 按 Code 排在最后 → 被两个方法都截断。
	after := env.run(env.addFeatureSnapshot(all))

	beforeX := capCandidate(t, before, x)
	afterX := capCandidate(t, after, x)
	if beforeX.Action != selection.ActionBuy || beforeX.TruncatedFromBuy {
		t.Fatalf("截断前 X 应保住 buy，实际 action=%s truncated=%v", beforeX.Action, beforeX.TruncatedFromBuy)
	}
	if afterX.Action != selection.ActionWatch || !afterX.TruncatedFromBuy {
		t.Fatalf("截断后 X 应降级 watch 并标记 TruncatedFromBuy，实际 action=%s truncated=%v", afterX.Action, afterX.TruncatedFromBuy)
	}
	if !reflect.DeepEqual(afterX.TruncatedByMethodIDs, []string{"m-a", "m-b"}) {
		t.Fatalf("截断后 X TruncatedByMethodIDs = %v, want [m-a m-b]", afterX.TruncatedByMethodIDs)
	}
	// 显式断言：分数逐位相等（截断前后 Score 必须一字不差）。
	if math.Float64bits(beforeX.Score) != math.Float64bits(afterX.Score) {
		t.Fatalf("截断改变了 Score：before=%v after=%v", beforeX.Score, afterX.Score)
	}
	// 且分数仍等于「全量 triggers 聚合」的结果（A 0.76 + B 0.72*0.04）。
	if beforeX.Score != 0.7888 || afterX.Score != 0.7888 {
		t.Fatalf("X Score 应为 0.7888（全量聚合），实际 before=%v after=%v", beforeX.Score, afterX.Score)
	}
	// 触发链在截断前后完全一致：截断只是输出过滤，不改 triggers。
	if len(afterX.Triggers) != 2 || len(beforeX.Triggers) != 2 {
		t.Fatalf("截断不得改动 Triggers：before=%d after=%d", len(beforeX.Triggers), len(afterX.Triggers))
	}
}

// T9：同方法下 8 只 amount 全有值（methodScore 全等）→ 必须由聚合 Score 区分前 5，
// 而不是退化成「按 Code 取前 5」。
func TestPerMethodBuyCapT9RanksByAggregateScoreNotCode(t *testing.T) {
	plain := seqCodes("400", 5)                       // 400001..400005，只被 A 触发 → 0.76
	boosted := []string{"400100", "400101", "400102"} // A+B → 0.7888，Code 全部最大
	all := append(append([]string{}, plain...), boosted...)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{
			"m-a": append([]string{}, all...),
			"m-b": boosted,
		},
	})
	run := env.run(env.addFeatureSnapshot(all))

	// 前置假设：同方法内 amount 都有值，methodScore 全等。
	plainScores := map[float64]bool{}
	for _, code := range plain {
		plainScores[capCandidate(t, run, code).Score] = true
	}
	if len(plainScores) != 1 {
		t.Fatalf("同方法内只触发 A 的候选分数应全等，实际 %v", plainScores)
	}
	if _, ok := plainScores[0.76]; !ok {
		t.Fatalf("只触发 A 的候选分数应为 0.76，实际 %v", plainScores)
	}

	// 显式断言：前三名必须是聚合分更高的 400100/400101/400102，而不是按 Code 取前 5。
	for _, code := range boosted {
		c := capCandidate(t, run, code)
		if c.Score != 0.7888 {
			t.Fatalf("%s Score = %v, want 0.7888（A+B 聚合）", code, c.Score)
		}
		if c.Action != selection.ActionBuy {
			t.Fatalf("%s 应保住 buy（聚合分高于只触发 A 的票），实际 action=%s", code, c.Action)
		}
		if !containsCode(c.KeptByMethodIDs, "m-a") {
			t.Fatalf("%s 应在 m-a 名额内，实际 kept=%v truncated=%v", code, c.KeptByMethodIDs, c.TruncatedByMethodIDs)
		}
	}
	if reflect.DeepEqual(capBuys(run), plain) {
		t.Fatalf("截断退化成按 Code 取前 5：buy=%v", capBuys(run))
	}
	// m-a 名额 = 3 只高分票 + 2 只 Code 最小的低分票（并列时的确定性排序键）。
	assertCodes(t, "m-a kept", keptCodes(run, "m-a"), []string{"400001", "400002", "400100", "400101", "400102"})
	assertCodes(t, "m-a truncated", truncatedCodes(run, "m-a"), []string{"400003", "400004", "400005"})
}

// T10：新字段全部 omitempty —— 历史 run 反序列化不受影响，也不污染旧产物。
func TestPerMethodBuyCapT10FieldsStayBackwardCompatible(t *testing.T) {
	// 1) 没有新字段的历史 run JSON 能反序列化，新字段取零值。
	legacy := `{
		"id":"selection-legacy","run_hash":"legacy-hash","engine_version":"selection-v3",
		"snapshot_id":"s1","feature_snapshot_id":"f1","snapshot_date":"2024-01-02",
		"status":"completed","eligible_methods":1,"scanned_stocks":1,"candidate_count":1,"buy_count":1,
		"action_counts":{"buy":1,"watch":0,"avoid":0,"insufficient_data":0},
		"candidates":[{"rank":1,"code":"000001","action":"buy","score":0.76,"data_date":"2024-01-02","snapshot_id":"s1","feature_snapshot_id":"f1","triggers":[{"method_id":"m-a"}],"position_cap_pct":0.08,"buy_window":"下一交易日","exit":{"complete":true},"explanation":"旧产物"}],
		"exclusions":[],"created_at":"2024-01-02T15:00:00Z"
	}`
	var old selection.Run
	if err := json.Unmarshal([]byte(legacy), &old); err != nil {
		t.Fatalf("历史 run 反序列化失败：%v", err)
	}
	if old.BuyCapPerMethod != 0 || old.BuyCountByMethod != nil || old.TruncatedFromBuy != 0 {
		t.Fatalf("历史 run 的新字段应为零值：cap=%d byMethod=%v truncated=%d", old.BuyCapPerMethod, old.BuyCountByMethod, old.TruncatedFromBuy)
	}
	if old.Candidates[0].Action != selection.ActionBuy || old.Candidates[0].TruncatedFromBuy {
		t.Fatal("历史候选的判定不该被新字段改写")
	}
	if old.Candidates[0].KeptByMethodIDs != nil || old.Candidates[0].TruncatedByMethodIDs != nil {
		t.Fatal("历史候选不该凭空获得名额归属")
	}

	// 2) 未设置新字段时编组不出新键（omitempty 生效）。
	minimal, err := json.Marshal(selection.Run{ID: "selection-min", ActionCounts: map[string]int{}, Candidates: []selection.Candidate{{Code: "000001", Action: selection.ActionBuy}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"buy_cap_per_method", "buy_count_by_method", "truncated_from_buy", "kept_by_method_ids", "truncated_by_method_ids"} {
		if strings.Contains(string(minimal), key) {
			t.Fatalf("零值不该编组出 %q：%s", key, minimal)
		}
	}

	// 3) 真正跑一次：新字段落库，且仍能按 run_hash 读回（JSON round-trip）。
	codes := seqCodes("380", 7)
	env := newCapEnv(t, capFixture{
		codes:    codes,
		methods:  []capMethodSpec{{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)}},
		triggers: map[string][]string{"m-a": codes},
	})
	run := env.run(env.addFeatureSnapshot(codes))
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"buy_cap_per_method":5`, `"buy_count_by_method":{"m-a":5}`, `"truncated_from_buy":2`, `"kept_by_method_ids":["m-a"]`, `"truncated_by_method_ids":["m-a"]`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("新产物缺少 %s：%s", key, raw)
		}
	}
	var round selection.Run
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(round.BuyCountByMethod, run.BuyCountByMethod) || round.TruncatedFromBuy != run.TruncatedFromBuy {
		t.Fatalf("round-trip 后名额信息丢失：%v/%d", round.BuyCountByMethod, round.TruncatedFromBuy)
	}
}

// 因子通道（watch-only）不占名额：既不出现在 BuyCountByMethod，也不获得名额归属。
func TestPerMethodBuyCapFactorChannelDoesNotConsumeSlots(t *testing.T) {
	codes := seqCodes("390", 3)
	env := newCapEnv(t, capFixture{
		codes:    codes,
		methods:  []capMethodSpec{{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)}},
		triggers: map[string][]string{"m-a": codes},
	})
	stub := &factorPickStub{pick: &factorlab.PickRun{
		RunID: "pick-2024-02-09", SnapshotID: "factor-snap", AsOf: "2024-02-09", StaleDays: 5,
		UpdatedAt: time.Now().UnixMilli(),
		Picks:     []factorlab.TopPick{{Code: "600001", Score: 0.8}, {Code: "600002", Score: 0.7}},
	}}
	env.engine.SetFactorPicks(stub)
	run := env.run(env.addFeatureSnapshot(codes))

	if _, ok := run.BuyCountByMethod[selection.FactorMethodID]; ok {
		t.Fatalf("因子通道不该出现在 BuyCountByMethod：%v", run.BuyCountByMethod)
	}
	want := map[string]int{"m-a": 3}
	if !reflect.DeepEqual(run.BuyCountByMethod, want) {
		t.Fatalf("BuyCountByMethod = %v, want %v", run.BuyCountByMethod, want)
	}
	factorSeen := 0
	for _, c := range run.Candidates {
		if c.Triggers[0].MethodID != selection.FactorMethodID {
			continue
		}
		factorSeen++
		if len(c.KeptByMethodIDs) != 0 || len(c.TruncatedByMethodIDs) != 0 {
			t.Fatalf("因子候选不该有名额归属：kept=%v truncated=%v", c.KeptByMethodIDs, c.TruncatedByMethodIDs)
		}
	}
	if factorSeen != 2 {
		t.Fatalf("因子候选数 = %d, want 2", factorSeen)
	}
}

// 按 method_id 读取单次 run（GET /api/selection/runs/:id?method_id=...）时，
// 名额聚合必须跟着被过滤的候选一起重算，否则 sum(BuyCountByMethod) 会与 BuyCount 对不上。
func TestPerMethodBuyCapFilteredReadRecomputesAggregates(t *testing.T) {
	codesA := seqCodes("510", 8)
	codesB := seqCodes("520", 8)
	all := append(append([]string{}, codesA...), codesB...)
	env := newCapEnv(t, capFixture{
		codes: all,
		methods: []capMethodSpec{
			{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)},
			{id: "m-b", family: "fam-b", signal: "sig_b", evidence: capEvidence("strong", 40, -.1)},
		},
		triggers: map[string][]string{"m-a": codesA, "m-b": codesB},
	})
	run := env.run(env.addFeatureSnapshot(all))
	if want := map[string]int{"m-a": 5, "m-b": 5}; !reflect.DeepEqual(run.BuyCountByMethod, want) {
		t.Fatalf("全量 run BuyCountByMethod = %v, want %v", run.BuyCountByMethod, want)
	}

	for methodID, wantCount := range map[string]int{"m-a": 5, "m-b": 5} {
		filtered, err := env.runs.Get(env.ctx, run.ID, methodID)
		if err != nil {
			t.Fatal(err)
		}
		if filtered.BuyCount != wantCount {
			t.Fatalf("按 %s 过滤后 BuyCount = %d, want %d", methodID, filtered.BuyCount, wantCount)
		}
		want := map[string]int{methodID: wantCount}
		if !reflect.DeepEqual(filtered.BuyCountByMethod, want) {
			t.Fatalf("按 %s 过滤后 BuyCountByMethod = %v, want %v", methodID, filtered.BuyCountByMethod, want)
		}
		if filtered.TruncatedFromBuy != 3 {
			t.Fatalf("按 %s 过滤后 TruncatedFromBuy = %d, want 3", methodID, filtered.TruncatedFromBuy)
		}
	}
}

func containsCode(codes []string, code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}
