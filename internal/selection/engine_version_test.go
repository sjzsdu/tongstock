package selection_test

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// 本文件锁定「EngineVersion 参与 run 缓存身份」这条约定。
//
// 背景：输出语义一变就必须 bump EngineVersion，否则旧 run（输入完全相同时）会
// 从 run_hash 缓存里被原样返回，表现为「功能合了但没生效」，且没有任何报错。
// per-method buy 上限（PR #21）就是这样落进 selection-v3 的：常量不动，旧缓存
// 就永远命中。
//
// 因此下面的测试不是断言「常量等于某个字符串」，而是构造一条 v3 时期真实存在
// 的落库记录，在 v4 下用完全相同的输入重跑，要求引擎不得复用它——同时要求
// 缓存本身仍然生效（否则「关掉缓存」也能让断言通过）。

// legacyRunHash 复算「某个历史 EngineVersion 下」同一批输入的 run_hash，
// 用来把旧的缓存记录放进它当年真正的键位上。
func legacyRunHash(t *testing.T, env *capEnv, featureID, version string) string {
	t.Helper()
	market, err := env.snapshots.LoadMarketSnapshot(env.marketID, true)
	if err != nil {
		t.Fatalf("load market snapshot: %v", err)
	}
	feature, err := env.snapshots.LoadFeatureSnapshot(featureID, true)
	if err != nil {
		t.Fatalf("load feature snapshot: %v", err)
	}
	items, err := env.methodsRepo.Query(env.ctx, methodregistry.Query{Limit: 10000})
	if err != nil {
		t.Fatalf("load methods: %v", err)
	}
	// 与 engine.Run 一致：方法按 ID 排序后进哈希。
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return selection.ExportRunHashForVersion(version, market, feature, items, nil, nil)
}

// TestEngineVersionBumpDoesNotReusePreviousVersionRun 是本条的核心回归：
// v3 落库的 run 在 v4 下必须重算，而不是被 run_hash 缓存原样返回。
//
// 用「只改常量、命中路径不变」（即 run_hash 不含 EngineVersion）的假实现跑这条
// 测试会失败：那种实现下 v3 与 v4 算出同一个 run_hash，引擎直接返回 9 个 buy
// 的旧产物。
func TestEngineVersionBumpDoesNotReusePreviousVersionRun(t *testing.T) {
	const legacyVersion = "selection-v3"
	// 单方法触发 9 只：v3 时代没有 per-method 上限，旧产物会是 9 个 buy；
	// 名额机制生效后只能是 5 个 buy + 4 个 watch。
	codes := seqCodes("410", 9)
	env := newCapEnv(t, capFixture{
		codes:    codes,
		methods:  []capMethodSpec{{id: "m-a", family: "fam-a", signal: "sig_a", evidence: capEvidence("strong", 120, -.1)}},
		triggers: map[string][]string{"m-a": codes},
	})
	featureID := env.addFeatureSnapshot(codes)

	// 1) 造一条 v3 时期落库的记录：键位是 v3 的 run_hash，产物里没有名额字段
	//    （BuyCapPerMethod/BuyCountByMethod/KeptByMethodIDs 全为零值，和升级前
	//    真实写下的 JSON 一致）。
	legacyHash := legacyRunHash(t, env, featureID, legacyVersion)
	legacyCreated := time.Date(2024, 1, 2, 15, 0, 0, 0, time.UTC)
	legacy := &selection.Run{
		ID: "selection-" + legacyHash[:16], RunHash: legacyHash, EngineVersion: legacyVersion,
		SnapshotID: env.marketID, FeatureSnapshotID: featureID, SnapshotDate: "2024-01-02",
		Status: "completed", EligibleMethods: 1, ScannedStocks: len(codes),
		ActionCounts: map[string]int{selection.ActionBuy: 0, selection.ActionWatch: 0, selection.ActionAvoid: 0, selection.ActionInsufficientData: 0},
		Candidates:   []selection.Candidate{}, Exclusions: []selection.Exclusion{}, CreatedAt: legacyCreated,
	}
	for i, code := range codes {
		legacy.Candidates = append(legacy.Candidates, selection.Candidate{
			Rank: i + 1, Code: code, Action: selection.ActionBuy, Score: 0.76, DataDate: "2024-01-02",
			SnapshotID: env.marketID, FeatureSnapshotID: featureID,
			Triggers: []selection.Trigger{{MethodID: "m-a", MethodVersionID: "m-a-v1", MethodName: "m-a 截面信号方法", FamilyID: "fam-a", Score: 0.76}},
		})
		legacy.ActionCounts[selection.ActionBuy]++
	}
	legacy.CandidateCount = len(legacy.Candidates)
	legacy.BuyCount = len(legacy.Candidates)
	if err := env.runs.Save(env.ctx, legacy); err != nil {
		t.Fatalf("save legacy run: %v", err)
	}

	// 2) 完全相同的输入、在当前版本下再跑一次：必须重算。
	run := env.run(featureID)

	if run.RunHash == legacyHash {
		t.Fatalf("v4 与 %s 算出同一个 run_hash %s：EngineVersion 没有参与缓存身份，旧缓存永远不会失效", legacyVersion, legacyHash)
	}
	if run.ID == legacy.ID {
		t.Fatalf("复用了 %s 时期的记录 %s（engine_version=%s）", legacyVersion, legacy.ID, run.EngineVersion)
	}
	if run.EngineVersion != selection.EngineVersion {
		t.Fatalf("run.EngineVersion = %q, want %q", run.EngineVersion, selection.EngineVersion)
	}
	if run.CreatedAt.Equal(legacyCreated) {
		t.Fatalf("run.CreatedAt = %s 仍是旧记录的时间戳，说明返回的是缓存里的旧产物", run.CreatedAt)
	}

	// 3) 重算结果必须带上当前版本的语义：per-method 上限生效（9 → 5）。
	if run.BuyCount != 5 {
		t.Fatalf("重算后的 BuyCount = %d, want 5（旧记录是 9，说明买到了 v3 的产物）", run.BuyCount)
	}
	if !reflect.DeepEqual(run.BuyCountByMethod, map[string]int{"m-a": 5}) {
		t.Fatalf("BuyCountByMethod = %v, want map[m-a:5]", run.BuyCountByMethod)
	}
	if run.TruncatedFromBuy != 4 {
		t.Fatalf("TruncatedFromBuy = %d, want 4", run.TruncatedFromBuy)
	}

	// 4) 旧记录留在自己的键位上、没有被覆盖：历史 run 仍可审计。
	stored, err := env.runs.Get(env.ctx, "hash:"+legacyHash, "")
	if err != nil {
		t.Fatalf("旧记录应当仍在库中（可审计），Get 失败：%v", err)
	}
	if stored.EngineVersion != legacyVersion || stored.BuyCount != len(codes) {
		t.Fatalf("旧记录被改写了：engine_version=%q buy_count=%d", stored.EngineVersion, stored.BuyCount)
	}
	all, err := env.runs.List(env.ctx, "", "", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("库中应有「旧 v3 记录 + 新 v4 记录」两条，实际 %d 条", len(all))
	}

	// 5) 缓存本身没有被关掉：同输入再跑必须复用刚算出的 v4 run。
	again := env.run(featureID)
	if again.ID != run.ID || !again.CreatedAt.Equal(run.CreatedAt) {
		t.Fatalf("同输入第二次 Run 应命中缓存（id=%s created=%s），实际 id=%s created=%s", run.ID, run.CreatedAt, again.ID, again.CreatedAt)
	}
}
