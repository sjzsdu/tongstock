package methodautomation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// Version 是自动研究编排的版本号，用于结果溯源。
const Version = "method-automation-v1"

// ErrBusy 表示上一轮研究尚未结束。
var ErrBusy = errors.New("method research batch is already running")

// minUniverseCodes 与 validation 的 fail-closed 门槛保持一致：
// 股票池小于该规模的快照必然验证失败，不值得作为自动研究输入。
const minUniverseCodes = 5

// SnapshotStore 读取不可变冻结快照（形状与 paradigm.DatasetSnapshotStore 对齐）。
type SnapshotStore interface {
	List(limit, offset int) ([]*paradigm.DatasetSnapshot, error)
	GetByID(id string) (*paradigm.DatasetSnapshot, error)
	VerifyContent(snapshotID string) error
}

// UniverseResolver 从冻结快照解析真实代码列表。
type UniverseResolver interface {
	ResolveUniverse(ctx context.Context, snapshotID string, minBars, maxCodes int) ([]string, int, error)
}

// Discoverer 是规律发现引擎的接口（discovery.Researcher 天然满足）。
type Discoverer interface {
	Run(ctx context.Context, request discovery.Request) (*discovery.Result, error)
}

// TraceSink 持久化研究轨迹（discoveryrepo.TraceRepository 天然满足）。
type TraceSink interface {
	Save(ctx context.Context, result *discovery.Result) error
}

// Deps 是自动研究编排的装配依赖。
type Deps struct {
	Registry   *methodregistry.Registry
	Snapshots  SnapshotStore
	Universe   UniverseResolver
	Bars       validation.BarProvider
	Benchmark  validation.BenchmarkProvider
	Evidence   EvidenceSink
	Discoverer Discoverer
	Traces     TraceSink // 可选
	// Now 可注入时钟；零值使用 UTC 真实时钟。
	Now func() time.Time
}

// Phase 描述批次当前所处的阶段（用于状态端点的实时进度展示）。
type Phase string

const (
	PhasePreparing  Phase = "preparing"  // 解析冻结快照与股票池
	PhaseDiscovery  Phase = "discovery"  // 在快照上扫描模板
	PhaseValidation Phase = "validation" // 在保留窗口上做样本外回测
)

// Progress 是运行中批次的实时快照，由 status 端点透出给前端。
type Progress struct {
	Phase        Phase `json:"phase"`
	HoldDays     int   `json:"hold_days,omitempty"`
	UniverseSize int   `json:"universe_size,omitempty"`
	// DiscoveryCodes:模板扫描阶段的逐码进度（300 只需要逐个加载 K 线扫描，
	// 是批次里最长的一段，必须可见，不能让用户对着 0/0 干等）。
	DiscoveryCodesDone  int `json:"discovery_codes_done,omitempty"`
	DiscoveryCodesTotal int `json:"discovery_codes_total,omitempty"`
	TotalCandidates     int `json:"total_candidates,omitempty"`
	CandidatesDone      int `json:"candidates_done,omitempty"`
	Verified            int `json:"verified"`
	Rejected            int `json:"rejected"`
}

// Orchestrator 编排「发现 → 验证 → 晋级/拒绝」批次，并累计 DiscoveryTrials。
type Orchestrator struct {
	deps Deps
	now  func() time.Time

	mu           sync.Mutex
	running      bool
	runningSince time.Time
	progress     Progress     // 运行中批次的实时进度（随阶段更新）
	trials       atomic.Int64 // 全局累计的发现尝试数，传入验证的多重检验校正
}

// New 构造自动研究编排器。
func New(deps Deps) (*Orchestrator, error) {
	if deps.Registry == nil {
		return nil, fmt.Errorf("methodautomation requires a method registry")
	}
	if deps.Snapshots == nil || deps.Universe == nil {
		return nil, fmt.Errorf("methodautomation requires snapshot store and universe resolver")
	}
	if deps.Bars == nil || deps.Benchmark == nil {
		return nil, fmt.Errorf("methodautomation requires bar and benchmark providers")
	}
	if deps.Discoverer == nil {
		return nil, fmt.Errorf("methodautomation requires a discovery engine")
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Orchestrator{deps: deps, now: now}, nil
}

// Trials 返回全局累计的发现尝试数。
func (o *Orchestrator) Trials() int64 { return o.trials.Load() }

// Running 报告当前是否有批次在运行、开始时间与实时进度（用于 HTTP 层展示）。
func (o *Orchestrator) Running() (bool, time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running, o.runningSince
}

// RunningProgress 在批次运行中返回其实时进度快照；空闲时返回 false。
func (o *Orchestrator) RunningProgress() (Progress, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.running {
		return Progress{}, false
	}
	return o.progress, true
}

// Run 执行一轮完整研究：每个持有期做模板扫描 → 保留窗口全池验证 →
// 通过机器证据门槛自动晋级 verified，未过门槛如实记录 rejected 及原因。
func (o *Orchestrator) Run(ctx context.Context, req Request) (*BatchResult, error) {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return nil, ErrBusy
	}
	o.running = true
	o.runningSince = o.now()
	o.progress = Progress{Phase: PhasePreparing}
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.running = false
		o.mu.Unlock()
	}()

	result := &BatchResult{StartedAt: o.now()}
	snapshot, err := o.resolveSnapshot(req.SnapshotID)
	if err != nil {
		return nil, err
	}
	result.SnapshotID = snapshot.ID
	codes, err := o.resolveCodes(ctx, snapshot, req)
	if err != nil {
		return nil, err
	}
	result.UniverseSize = len(codes)
	o.mu.Lock()
	o.progress.UniverseSize = len(codes)
	o.mu.Unlock()

	holdDays := normalizeHoldDays(req.HoldDays)
	for _, hold := range holdDays {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		o.mu.Lock()
		o.progress = Progress{Phase: PhaseDiscovery, HoldDays: hold, UniverseSize: len(codes), Verified: o.progress.Verified, Rejected: o.progress.Rejected}
		o.mu.Unlock()
		batch, err := o.runHoldDays(ctx, result, snapshot, codes, hold, req.SearchBudget)
		if err != nil {
			batch.Error = err.Error()
		}
		result.Batches = append(result.Batches, *batch)
	}
	result.TrialsCumulative = o.trials.Load()
	result.FinishedAt = o.now()
	return result, nil
}

// runHoldDays 对单个持有期执行发现与验证。
func (o *Orchestrator) runHoldDays(ctx context.Context, result *BatchResult, snapshot *paradigm.DatasetSnapshot, codes []string, hold, budget int) (*HoldBatchResult, error) {
	batch := &HoldBatchResult{HoldDays: hold}
	research, err := o.deps.Discoverer.Run(ctx, discovery.Request{
		SnapshotID: snapshot.ID, StockCodes: codes, HoldDays: hold, SearchBudget: budget,
		// 逐码回调把扫描进度写进 progress；锁内轻量赋值，回调频率 = 股票数。
		OnProgress: func(done, total int) {
			o.mu.Lock()
			o.progress.DiscoveryCodesDone = done
			o.progress.DiscoveryCodesTotal = total
			o.mu.Unlock()
		},
	})
	if err != nil {
		return batch, fmt.Errorf("discovery: %w", err)
	}
	batch.ResearchID = research.ResearchID
	batch.DiscoveryTrials = research.DiscoveryTrials
	o.mu.Lock()
	o.progress.DiscoveryCodesDone = o.progress.DiscoveryCodesTotal
	o.mu.Unlock()
	batch.Candidates = len(research.Candidates)
	result.TrialsThisBatch += research.DiscoveryTrials
	o.trials.Add(int64(research.DiscoveryTrials))
	if o.deps.Traces != nil {
		if saveErr := o.deps.Traces.Save(ctx, research); saveErr != nil {
			return batch, fmt.Errorf("persist research trace: %w", saveErr)
		}
	}

	// 验证窗口取各代码保留样本的全局交集起点：任何代码在该窗口内的 K 线
	// 都不早于它自己的 ReservedStartDate，即全部位于发现阶段从未触碰的区域。
	start, end := reservedWindow(research.Boundaries)
	if start == "" {
		return batch, fmt.Errorf("discovery produced no reserved boundaries: fail closed")
	}
	result.ValidationStart, result.ValidationEnd = start, end

	o.mu.Lock()
	o.progress = Progress{
		Phase: PhaseValidation, HoldDays: batch.HoldDays, UniverseSize: len(codes),
		TotalCandidates: len(research.Candidates), Verified: o.progress.Verified, Rejected: o.progress.Rejected,
	}
	o.mu.Unlock()

	for i := range research.Candidates {
		if err := ctx.Err(); err != nil {
			return batch, err
		}
		outcome := o.processCandidate(ctx, research, &research.Candidates[i], codes, start, end)
		result.Outcomes = append(result.Outcomes, outcome)
		switch {
		case outcome.Status == "verified":
			batch.Verified++
			result.Verified++
			batch.Registered++
			result.Registered++
		case outcome.Status == "rejected":
			batch.Rejected++
			result.Rejected++
			// 未过门槛的候选同样登记进方法库（保留审计痕迹），只是状态是 rejected。
			if outcome.MethodID != "" {
				batch.Registered++
				result.Registered++
			}
		}
		o.mu.Lock()
		o.progress.CandidatesDone = i + 1
		o.progress.Verified = result.Verified
		o.progress.Rejected = result.Rejected
		o.mu.Unlock()
	}
	// discovery 阶段就被拒绝的模板（样本不足 / 无正收益）同样记入分布。
	for _, rejected := range research.Rejected {
		result.Outcomes = append(result.Outcomes, CandidateOutcome{
			TemplateID: rejected.TemplateID, Status: "rejected", Stage: "discovery", Reason: rejected.Reason,
		})
		batch.Rejected++
		result.Rejected++
	}
	return batch, nil
}

// processCandidate 对单个候选执行验证与注册。
func (o *Orchestrator) processCandidate(ctx context.Context, research *discovery.Result, candidate *discovery.CandidateEvidence, codes []string, start, end string) CandidateOutcome {
	out := CandidateOutcome{TemplateID: candidate.TemplateID, Stage: "validation"}
	if candidate.Method == nil {
		out.Status = "failed"
		out.Reason = "candidate has no compiled method"
		return out
	}
	out.MethodHash = candidate.Method.ContentHash
	familyID := "auto-" + candidate.TemplateID
	variantID := candidate.Method.ContentHash

	if o.registeredWithUsableEvidence(ctx, familyID, variantID) {
		out.Status = "skipped_registered"
		out.Stage = "registration"
		out.Reason = "same method hash already registered with usable evidence for this template"
		return out
	}
	if o.negativeFeedback(ctx, familyID) {
		out.Status = "skipped_feedback"
		out.Stage = "registration"
		out.Reason = "user feedback: family has dominant negative feedback"
		return out
	}

	factory, err := validation.NewFactory(validation.FactoryDeps{
		Method: candidate.Method, Bars: o.deps.Bars, Benchmark: o.deps.Benchmark,
	})
	if err != nil {
		out.Status = "failed"
		out.Reason = err.Error()
		return out
	}
	job := validation.ValidationJob{
		MethodHash: candidate.Method.ContentHash, MethodName: candidate.Method.Name,
		SnapshotID: research.SnapshotID, Universe: codes,
		DateStart: start, DateEnd: end,
		DiscoveryTrials: int(o.trials.Load()),
	}
	bundle, err := factory.Run(ctx, job)
	if err != nil {
		out.Status = "failed"
		out.Reason = "validation: " + err.Error()
		return out
	}
	if o.deps.Evidence != nil {
		if saveErr := o.deps.Evidence.Save(ctx, bundle); saveErr != nil {
			out.Status = "failed"
			out.Reason = "persist evidence: " + saveErr.Error()
			return out
		}
	}
	out.Confidence = string(bundle.Confidence)
	out.OOSTrades = bundle.OosStats.TotalTrades
	out.OOSReturn = bundle.OosStats.TotalReturn
	out.OOSWinRate = bundle.OosStats.WinRate
	sharpe := bundle.OosStats.SharpeRatio
	out.SharpeRatio = &sharpe
	if bundle.ConfidenceReason != "" {
		out.Reason = bundle.ConfidenceReason
	}

	m, err := o.deps.Registry.Register(ctx, methodregistry.Registration{
		FamilyID:  familyID,
		VariantID: variantID,
		// 显示名带上模板与持有期：不同 hold 的同一模板是不同方法，仅靠
		// 模板自动生成的名字（如「RSI14 高于 65」）在列表里无法区分。
		Name:             fmt.Sprintf("[自动] %s（持有 %d 天）", candidate.Method.Name, candidate.Method.Holding.MaxDays),
		SourceResearchID: research.ResearchID,
		ValidationJobID:  bundle.JobHash,
		Market:           "A",
		TriggerFrequency: "daily",
		EntrySummary:     candidate.Rationale,
		ExitSummary:      exitSummary(candidate.Method),
		Method:           candidate.Method,
		Evidence:         methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		out.Status = "failed"
		out.Stage = "registration"
		out.Reason = "register: " + err.Error()
		return out
	}
	out.Stage = "registration"
	out.MethodID = m.ID
	out.Status = string(m.Status)
	return out
}

// registeredWithUsableEvidence 判断同模板下相同方法哈希是否已带着可用证据登记过，
// 避免反复跑同一候选时在方法库里堆叠重复版本。
//
// 只在「已有版本的证据 passable」时才跳过：历史版本可能被旧门槛（如已降级的
// bl-underperform 硬拒绝）误杀成 rejected——这类方法必须放行重新验证，
// Registry.Register 会追加新版本并由 policy.Initial 按当前标准重算状态，
// 误杀的方法就能在新门槛下翻回 verified。若旧版本本就 passable，
// 重复验证只会产出等价证据，跳过以免浪费预算。
//
// 没有 EvidenceSummary 的历史登记（理论上不存在）同样放行，宁多验不漏放。
func (o *Orchestrator) registeredWithUsableEvidence(ctx context.Context, familyID, methodHash string) bool {
	cards, err := o.deps.Registry.Cards(ctx, methodregistry.Query{FamilyID: familyID, Limit: 50})
	if err != nil {
		return false
	}
	for _, card := range cards {
		if card.VariantID != methodHash {
			continue
		}
		if card.Evidence == nil || !card.Evidence.Passable {
			return false
		}
		return true
	}
	return false
}

// negativeFeedback 实现「用户反馈反哺研究」：某方法族累计 >=2 条负反馈且无正反馈时，
// 跳过该族的新候选，把预算让给用户认可的方向。
func (o *Orchestrator) negativeFeedback(ctx context.Context, familyID string) bool {
	cards, err := o.deps.Registry.Cards(ctx, methodregistry.Query{FamilyID: familyID, Limit: 50})
	if err != nil {
		return false
	}
	positive, negative := 0, 0
	for _, card := range cards {
		m, err := o.deps.Registry.Get(ctx, card.ID)
		if err != nil {
			continue
		}
		for _, a := range m.Annotations {
			text := strings.TrimSpace(a.Text)
			if !strings.HasPrefix(text, "feedback:") {
				continue
			}
			switch {
			case strings.Contains(text, "useful=true"):
				positive++
			case strings.Contains(text, "useful=false"):
				negative++
			}
		}
	}
	return negative >= 2 && negative > positive
}

// resolveSnapshot 选定冻结快照：显式 ID 优先；否则按 created_at 倒序分页扫描，
// 取最新一个股票池足以通过验证门槛且内容校验通过的快照。
// 范式分析、单股 AI 研究会产生大量 universe=1 的单票快照，只扫最近 20 条会把
// 更早的多股票快照挤出窗口（线上已踩过：34 个快照里只有 1 个多股票，排在第 21+ 位），
// 所以这里分页扫到上限为止，找到即返回最新的合格者。
func (o *Orchestrator) resolveSnapshot(snapshotID string) (*paradigm.DatasetSnapshot, error) {
	if id := strings.TrimSpace(snapshotID); id != "" {
		snapshot, err := o.deps.Snapshots.GetByID(id)
		if err != nil {
			return nil, fmt.Errorf("load frozen snapshot %s: %w", id, err)
		}
		if err := o.deps.Snapshots.VerifyContent(id); err != nil {
			return nil, fmt.Errorf("verify frozen snapshot %s: %w", id, err)
		}
		return snapshot, nil
	}
	const (
		scanPage = 50
		scanMax  = 500 // 防御上限：快照表被单票快照灌满时不至于无限扫，正常场景前几页就命中
	)
	scanned, tooSmall, unverifiable := 0, 0, 0
	for offset := 0; offset < scanMax; offset += scanPage {
		snaps, err := o.deps.Snapshots.List(scanPage, offset)
		if err != nil {
			return nil, fmt.Errorf("list frozen snapshots: %w", err)
		}
		for _, snap := range snaps {
			if snap == nil {
				continue
			}
			scanned++
			if len(snap.Universe) < minUniverseCodes {
				tooSmall++
				continue
			}
			if err := o.deps.Snapshots.VerifyContent(snap.ID); err != nil {
				unverifiable++
				continue
			}
			return snap, nil
		}
		if len(snaps) < scanPage {
			break
		}
	}
	return nil, fmt.Errorf(
		"no usable frozen dataset snapshot: scanned %d snapshots, %d with universe < %d codes, %d failed content verification; run a multi-stock AI research (it freezes one) or freeze any multi-code snapshot first",
		scanned, tooSmall, minUniverseCodes, unverifiable)
}

func (o *Orchestrator) resolveCodes(ctx context.Context, snapshot *paradigm.DatasetSnapshot, req Request) ([]string, error) {
	if len(req.Codes) > 0 {
		return dedupeSorted(req.Codes), nil
	}
	maxCodes := req.MaxCodes
	if maxCodes <= 0 {
		maxCodes = 300
	}
	codes, skipped, err := o.deps.Universe.ResolveUniverse(ctx, snapshot.ID, 30, maxCodes)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot universe: %w", err)
	}
	if len(codes) == 0 {
		return nil, fmt.Errorf("snapshot %s has no code with enough real bars", snapshot.ID)
	}
	_ = skipped
	return codes, nil
}

// reservedWindow 返回保留样本的全局 [start, end]：start 取各代码
// ReservedStartDate 的最大值，保证窗口对每个代码都落在未触碰区域。
func reservedWindow(boundaries []discovery.CodeBoundary) (string, string) {
	start, end := "", ""
	for _, b := range boundaries {
		if b.ReservedStartDate > start {
			start = b.ReservedStartDate
		}
		if b.LastDate > end {
			end = b.LastDate
		}
	}
	return start, end
}

func normalizeHoldDays(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, v := range values {
		if v >= 1 && v <= 60 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []int{5, 20}
	}
	sort.Ints(out)
	return out
}

func dedupeSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func exitSummary(m *methods.CompiledMethod) string {
	if m == nil {
		return "按退出规则离场"
	}
	if m.Holding.MaxDays > 0 {
		return fmt.Sprintf("按退出规则，最长持有 %d 个交易日", m.Holding.MaxDays)
	}
	return "按退出规则离场"
}
