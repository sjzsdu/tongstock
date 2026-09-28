package dashboard

import "testing"

func actionKinds(actions []Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Kind)
	}
	return out
}

func hasAction(actions []Action, kind string) bool {
	for _, a := range actions {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// 空榜分诊必须按真实原因给出不同的原因码与下一步，否则首屏会退化成
// 一句「暂无数据」，用户无法判断是自己没同步还是方法没过门槛。
func TestDecideEmptyStateTriage(t *testing.T) {
	fresh := DataStatus{Fresh: true, LatestSnapshotID: "snap-1", Detail: "快照覆盖 98.0%，数据已就绪"}

	cases := []struct {
		name       string
		data       DataStatus
		methods    MethodStatus
		sel        *SelectionStatus
		wantReason string
		wantAction string
	}{
		{
			name:       "行情未同步优先于方法问题",
			data:       DataStatus{Fresh: false, Detail: "还没有任何市场快照"},
			methods:    MethodStatus{Total: 3, Rejected: 3},
			sel:        nil,
			wantReason: ReasonDataNotSynced,
			wantAction: ActionSync,
		},
		{
			name:       "有方法但全部未通过验证",
			data:       fresh,
			methods:    MethodStatus{Total: 3, Rejected: 3},
			sel:        nil,
			wantReason: ReasonNoVerifiedMethods,
			wantAction: ActionSeedMethods,
		},
		{
			name:       "方法库为空也算未通过验证",
			data:       fresh,
			methods:    MethodStatus{},
			sel:        nil,
			wantReason: ReasonNoVerifiedMethods,
			wantAction: ActionSeedMethods,
		},
		{
			name:       "方法与数据就绪但未跑选股",
			data:       fresh,
			methods:    MethodStatus{Total: 1, Verified: 1},
			sel:        nil,
			wantReason: ReasonSelectionNotRun,
			wantAction: ActionRunSelection,
		},
		{
			name:       "已扫描但没有候选是正常结果",
			data:       fresh,
			methods:    MethodStatus{Total: 1, Verified: 1},
			sel:        &SelectionStatus{SnapshotDate: "2026-04-24", ScannedStocks: 300, EligibleMethods: 1},
			wantReason: ReasonNoCandidates,
			wantAction: ActionViewExclusions,
		},
		{
			name:       "有候选则不再展示空状态",
			data:       fresh,
			methods:    MethodStatus{Total: 1, Verified: 1},
			sel:        &SelectionStatus{SnapshotDate: "2026-04-24", ScannedStocks: 300, CandidateCount: 2},
			wantReason: ReasonHasCandidates,
			wantAction: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideEmptyState(tc.data, tc.methods, tc.sel)
			if got.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q (message=%q)", got.Reason, tc.wantReason, got.Message)
			}
			if got.Title == "" || got.Message == "" {
				t.Fatalf("empty state must explain why: %+v", got)
			}
			if tc.wantAction == "" {
				if len(got.Actions) != 0 {
					t.Fatalf("has_candidates should not offer actions, got %v", actionKinds(got.Actions))
				}
				return
			}
			if !hasAction(got.Actions, tc.wantAction) {
				t.Fatalf("actions = %v, want to contain %q", actionKinds(got.Actions), tc.wantAction)
			}
			// 每个空状态必须恰好有一个主行动，避免用户无从下手。
			primary := 0
			for _, a := range got.Actions {
				if a.Primary {
					primary++
				}
			}
			if primary != 1 {
				t.Fatalf("want exactly one primary action, got %d: %v", primary, actionKinds(got.Actions))
			}
		})
	}
}

// 未通过验证时的文案必须带上真实计数，不能让用户以为是系统故障。
func TestDecideEmptyStateReportsRejectedCount(t *testing.T) {
	got := decideEmptyState(
		DataStatus{Fresh: true, LatestSnapshotID: "snap-1"},
		MethodStatus{Total: 3, Rejected: 3},
		nil,
	)
	want := "方法库里有 3 个方法，但没有一个通过验证门槛（其中 3 个被拒绝）。"
	if got.Message != want {
		t.Fatalf("message = %q, want %q", got.Message, want)
	}
}

func TestBuildHealthBlocksWhenNoDataAndNoMethods(t *testing.T) {
	health := buildHealth(
		DataStatus{Detail: "还没有任何市场快照"},
		MethodStatus{},
		nil,
		PositionStatus{},
	)
	if health.Overall != "blocked" {
		t.Fatalf("overall = %q, want blocked", health.Overall)
	}
	if len(health.Signals) != 4 {
		t.Fatalf("want 4 signals (data/methods/candidates/positions), got %d", len(health.Signals))
	}
	byKey := map[string]Signal{}
	for _, s := range health.Signals {
		byKey[s.Key] = s
	}
	if byKey["data"].Status != "blocked" {
		t.Fatalf("data signal = %+v, want blocked", byKey["data"])
	}
	if byKey["methods"].Status != "blocked" {
		t.Fatalf("methods signal = %+v, want blocked", byKey["methods"])
	}
	if byKey["candidates"].Status != "attention" {
		t.Fatalf("candidates signal = %+v, want attention", byKey["candidates"])
	}
}

// 数据新鲜但没有可用方法（无验证、无观察）时，系统确实无法产出候选，
// 因此整体判定为 blocked；一旦有方法在观察期，则降级为 attention。
func TestBuildHealthMethodSignalSeverity(t *testing.T) {
	data := DataStatus{Fresh: true, LatestSnapshotID: "snap-1", LatestSnapshotDate: "2026-04-24", Detail: "ok"}

	blocked := buildHealth(data, MethodStatus{Total: 3, Rejected: 3}, nil, PositionStatus{})
	if blocked.Overall != "blocked" {
		t.Fatalf("overall = %q, want blocked", blocked.Overall)
	}

	attention := buildHealth(data, MethodStatus{Total: 3, Observing: 1, Rejected: 2}, nil, PositionStatus{})
	if attention.Overall != "attention" {
		t.Fatalf("overall = %q, want attention", attention.Overall)
	}
}

func TestBuildWorkLogGroupsExclusions(t *testing.T) {
	sel := &SelectionStatus{
		SnapshotDate:     "2026-04-24",
		ScannedStocks:    300,
		EligibleMethods:  0,
		CandidateCount:   0,
		ExclusionCounts:  map[string]int{"insufficient_data": 12, "method_not_eligible": 288},
		SampleExclusions: nil,
	}
	log := buildWorkLog(sel, MethodStatus{Total: 3, Rejected: 3})
	if !log.Available {
		t.Fatal("work log should be available when a run exists")
	}
	byKey := map[string]WorkLogLine{}
	for _, l := range log.Lines {
		byKey[l.Key] = l
	}
	if got := byKey["insufficient"].Value; got != 12 {
		t.Fatalf("insufficient = %d, want 12", got)
	}
	if got := byKey["method_gated"].Value; got != 288 {
		t.Fatalf("method_gated = %d, want 288", got)
	}
	if got := byKey["scanned"].Value; got != 300 {
		t.Fatalf("scanned = %d, want 300", got)
	}
}

func TestBuildWorkLogUnavailableWithoutRun(t *testing.T) {
	if log := buildWorkLog(nil, MethodStatus{}); log.Available {
		t.Fatal("work log must be unavailable when no selection run exists")
	}
}
