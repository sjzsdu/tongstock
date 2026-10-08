// Package paradigmspromote 把「范式 → 编译 → 真实冻结数据机器验证 → 方法库注册」
// 串成一条可审计的晋级链路，补上范式库与可信方法库之间缺失的通道。
//
// 红线：
//   - 晋级与否完全由机器证据（真实回测 + 置信度门槛）决定，绝不硬编码；
//   - 编译 fail-closed：cross/near/describe 等无法单日求值的条件拒绝晋级；
//   - 未过门槛的范式如实标 rejected + 原因；方法库按既有惯例保留 rejected
//     登记作为审计痕迹（与 methodautomation 一致）；
//   - 多重检验：DiscoveryTrials 沿用自动研究的全局累计计数。
package paradigmspromote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/internal/validation"
)

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

// EvidenceSink 持久化验证制品（与 methodseed/methodautomation 的 EvidenceSink 一致）。
type EvidenceSink interface {
	Save(ctx context.Context, bundle *validation.EvidenceBundle) error
}

// TrialsFunc 返回全局累计的发现尝试数（methodautomation.Orchestrator.Trials 天然满足），
// 用于验证的多重检验校正。
type TrialsFunc func() int64

// minUniverseCodes 与 validation / methodautomation 的 fail-closed 门槛一致。
const minUniverseCodes = 5

// Deps 是晋级链路的装配依赖。
type Deps struct {
	Paradigms *paradigms.Store
	Registry  *methodregistry.Registry
	Snapshots SnapshotStore
	Universe  UniverseResolver
	Bars      validation.BarProvider
	Benchmark validation.BenchmarkProvider
	Evidence  EvidenceSink
	Trials    TrialsFunc // 可选；nil = 不做多重检验惩罚
	// Now 可注入时钟；零值使用 UTC 真实时钟。
	Now func() time.Time
}

// Options 是单次晋级的输入。
type Options struct {
	// SnapshotID 复用已冻结真实 K 线快照；空 = 自动选最新可用的多票快照。
	SnapshotID string
	// MaxCodes 从快照解析股票池的上限；0 = 300。
	MaxCodes int
}

// Outcome 是一次晋级尝试的真实结局。
type Outcome struct {
	ParadigmID string   `json:"paradigm_id"`
	// Status: promoted / rejected / blocked / failed
	Status       string   `json:"status"`
	MethodID     string   `json:"method_id,omitempty"`
	MethodStatus string   `json:"method_status,omitempty"`
	Blockers     []string `json:"blockers,omitempty"`
	Confidence   string   `json:"confidence,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	OOSTrades    int      `json:"oos_trades,omitempty"`
	OOSReturn    float64  `json:"oos_return,omitempty"`
	OOSWinRate   float64  `json:"oos_win_rate,omitempty"`
	ResultHash   string   `json:"result_hash,omitempty"`
	SnapshotID   string   `json:"snapshot_id,omitempty"`
}

// Service 编排范式晋级。
type Service struct {
	deps Deps
	now  func() time.Time
}

// New 构造晋级服务。
func New(deps Deps) (*Service, error) {
	if deps.Paradigms == nil {
		return nil, fmt.Errorf("paradigmspromote requires a paradigm store")
	}
	if deps.Registry == nil {
		return nil, fmt.Errorf("paradigmspromote requires a method registry")
	}
	if deps.Snapshots == nil || deps.Universe == nil {
		return nil, fmt.Errorf("paradigmspromote requires snapshot store and universe resolver")
	}
	if deps.Bars == nil || deps.Benchmark == nil {
		return nil, fmt.Errorf("paradigmspromote requires bar and benchmark providers")
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{deps: deps, now: now}, nil
}

// Promote 执行一次晋级：编译 → 机器验证 → 注册 → 回写范式。
// blocked（fail-closed 拦截）与 failed（验证失败）都不改 ReviewStatus，
// 只把原因写进范式的 PromotionBlockers 供前端展示。
func (s *Service) Promote(ctx context.Context, paradigmID string, opts Options) (*Outcome, error) {
	p, err := s.deps.Paradigms.Get(paradigmID)
	if err != nil {
		return nil, err
	}
	out := &Outcome{ParadigmID: p.ID}

	candidate, blockers, err := paradigms.ToCandidate(p)
	if err != nil {
		return s.recordBlocked(p, out, []string{err.Error()}), nil
	}
	if len(blockers) > 0 {
		return s.recordBlocked(p, out, blockers), nil
	}

	compiled, diags, err := methods.Compile(candidate)
	if err != nil {
		return s.recordBlocked(p, out, []string{"compile: " + err.Error()}), nil
	}
	if compiled == nil || !compiled.IsExecutable() {
		return s.recordBlocked(p, out, []string{"compiled method is not executable: " + diagnosticsSummary(diags)}), nil
	}
	out.Blockers = nil

	// 同 family + methodHash 已带可用证据登记过 → 跳过重复验证，直接对齐范式状态。
	familyID := "paradigm-" + p.ID
	if existing, ok := s.registeredUsable(ctx, familyID, compiled.ContentHash); ok {
		out.MethodID = existing.ID
		out.MethodStatus = string(existing.Status)
		status, err := s.alignParadigmState(p, existing, "", "")
		if err != nil {
			return nil, err
		}
		out.Status = status
		return out, nil
	}

	snapshot, err := s.resolveSnapshot(opts.SnapshotID)
	if err != nil {
		return s.recordBlocked(p, out, []string{err.Error()}), nil
	}
	out.SnapshotID = snapshot.ID
	codes, err := s.resolveCodes(ctx, snapshot.ID, opts.MaxCodes)
	if err != nil {
		return s.recordBlocked(p, out, []string{err.Error()}), nil
	}

	trials := 0
	if s.deps.Trials != nil {
		trials = int(s.deps.Trials())
	}
	job := validation.ValidationJob{
		MethodHash:      compiled.ContentHash,
		MethodName:      compiled.Name,
		SnapshotID:      snapshot.ID,
		Universe:        codes,
		DateStart:       snapshot.DateRange.Start,
		DateEnd:         snapshot.DateRange.End,
		DiscoveryTrials: trials,
	}
	factory, err := validation.NewFactory(validation.FactoryDeps{
		Method: compiled, Bars: s.deps.Bars, Benchmark: s.deps.Benchmark,
	})
	if err != nil {
		return s.recordBlocked(p, out, []string{"validation factory: " + err.Error()}), nil
	}
	bundle, err := factory.Run(ctx, job)
	if err != nil {
		return s.recordBlocked(p, out, []string{"validation: " + err.Error()}), nil
	}
	if s.deps.Evidence != nil {
		if saveErr := s.deps.Evidence.Save(ctx, bundle); saveErr != nil {
			return nil, fmt.Errorf("persist validation evidence: %w", saveErr)
		}
	}
	out.Confidence = string(bundle.Confidence)
	out.Reason = bundle.ConfidenceReason
	out.OOSTrades = bundle.OosStats.TotalTrades
	out.OOSReturn = bundle.OosStats.TotalReturn
	out.OOSWinRate = bundle.OosStats.WinRate
	out.ResultHash = bundle.ResultHash
	for _, b := range bundle.Blockers {
		out.Blockers = append(out.Blockers, b.Code+": "+b.Description)
	}

	m, err := s.deps.Registry.Register(ctx, methodregistry.Registration{
		FamilyID:         familyID,
		VariantID:        compiled.ContentHash,
		Name:             "[范式] " + p.Name,
		SourceResearchID: p.ID,
		ValidationJobID:  bundle.JobHash,
		Market:           "A",
		TriggerFrequency: "daily",
		EntrySummary:     p.Rationale,
		ExitSummary:      exitSummary(compiled),
		Method:           compiled,
		Evidence:         methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		return nil, fmt.Errorf("register method: %w", err)
	}
	out.MethodID = m.ID
	out.MethodStatus = string(m.Status)

	status, err := s.alignParadigmState(p, m, bundle.ResultHash, out.Reason)
	if err != nil {
		return nil, err
	}
	out.Status = status
	return out, nil
}

// alignParadigmState 按注册后的方法状态回写范式：
// verified → promoted（带 MethodID）；其余 → rejected + 原因。每次流转写审计。
func (s *Service) alignParadigmState(p *paradigms.Paradigm, m *methodregistry.Method, evidenceHash, reason string) (string, error) {
	pCopy := *p
	pCopy.MethodID = m.ID
	now := s.now()

	to := paradigms.StatePromoted
	action := "promote"
	note := "机器验证通过，已晋级为可信方法 " + m.ID
	if m.Status != methodregistry.StatusVerified {
		to = paradigms.StateRejected
		action = "reject"
		note = "机器验证未过门槛：" + firstNonEmpty(reason, string(m.Status))
	}
	pCopy.ReviewStatus = to
	pCopy.ReviewNote = note

	// 晋级资格按最终结局重写：verified = 可晋级且无 blocker；rejected 不可晋级。
	eligibility := &paradigms.ParadigmEvidence{Eligible: to == paradigms.StatePromoted}
	if to == paradigms.StatePromoted {
		eligibility.Level = "promoted"
		eligibility.Score = 1
		eligibility.Reasons = []string{"真实冻结数据机器验证通过：" + note}
	} else {
		eligibility.Level = "rejected"
		eligibility.MustFix = []string{note}
	}
	pCopy.Evidence = eligibility

	transition := paradigms.StateTransition{
		ID:           fmt.Sprintf("%s-%s-%d", p.ID, action, now.UnixNano()),
		ParadigmID:   p.ID,
		From:         p.ReviewStatus,
		To:           to,
		Action:       action,
		Reason:       note,
		Actor:        "paradigm-promotion",
		EvidenceHash: evidenceHash,
		Auto:         true,
		CreatedAt:    now,
	}
	pCopy.Transitions = append(append([]paradigms.StateTransition{}, p.Transitions...), transition)

	if err := s.deps.Paradigms.Save(&pCopy); err != nil {
		// 回写失败不影响方法注册结果，但必须暴露给调用方。
		return "", fmt.Errorf("register method %s ok, but paradigm write-back failed: %w", m.ID, err)
	}
	if to == paradigms.StatePromoted {
		return "promoted", nil
	}
	return "rejected", nil
}

// recordBlocked 把 fail-closed 拦截原因写进范式的 PromotionBlockers（不改 ReviewStatus）。
func (s *Service) recordBlocked(p *paradigms.Paradigm, out *Outcome, blockers []string) *Outcome {
	out.Status = "blocked"
	out.Blockers = blockers
	out.Reason = "晋级被 fail-closed 拦截：" + strings.Join(blockers, "；")

	pCopy := *p
	card := paradigms.ParadigmEvidence{Eligible: false, Level: "blocked", MustFix: blockers}
	if p.Evidence != nil {
		card = *p.Evidence
		card.Eligible = false
		card.MustFix = blockers
	}
	pCopy.Evidence = &card
	if err := s.deps.Paradigms.Save(&pCopy); err != nil {
		out.Reason += "（blockers 回写失败: " + err.Error() + "）"
	}
	return out
}

// registeredUsable 判断同 family 下相同方法哈希是否已带可用证据登记过，
// 避免重复跑昂贵回测。与 methodautomation.registeredWithUsableEvidence 同口径。
func (s *Service) registeredUsable(ctx context.Context, familyID, methodHash string) (*methodregistry.Method, bool) {
	cards, err := s.deps.Registry.Cards(ctx, methodregistry.Query{FamilyID: familyID, Limit: 50})
	if err != nil {
		return nil, false
	}
	for _, card := range cards {
		if card.VariantID != methodHash {
			continue
		}
		if card.Evidence == nil || !card.Evidence.Passable {
			return nil, false
		}
		m, err := s.deps.Registry.Get(ctx, card.ID)
		if err != nil {
			return nil, false
		}
		return m, true
	}
	return nil, false
}

// resolveSnapshot 选定冻结快照：显式 ID 优先；否则按 created_at 倒序分页扫描，
// 取最新一个股票池足以通过验证门槛且内容校验通过的快照。
func (s *Service) resolveSnapshot(snapshotID string) (*paradigm.DatasetSnapshot, error) {
	if id := strings.TrimSpace(snapshotID); id != "" {
		snapshot, err := s.deps.Snapshots.GetByID(id)
		if err != nil {
			return nil, fmt.Errorf("load frozen snapshot %s: %w", id, err)
		}
		if err := s.deps.Snapshots.VerifyContent(id); err != nil {
			return nil, fmt.Errorf("verify frozen snapshot %s: %w", id, err)
		}
		return snapshot, nil
	}
	const (
		scanPage = 50
		scanMax  = 500
	)
	scanned, tooSmall, unverifiable := 0, 0, 0
	for offset := 0; offset < scanMax; offset += scanPage {
		snaps, err := s.deps.Snapshots.List(scanPage, offset)
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
			if err := s.deps.Snapshots.VerifyContent(snap.ID); err != nil {
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
		"no usable frozen dataset snapshot: scanned %d snapshots, %d with universe < %d codes, %d failed content verification; run a multi-stock AI research (it freezes one) or pass snapshot_id explicitly",
		scanned, tooSmall, minUniverseCodes, unverifiable)
}

func (s *Service) resolveCodes(ctx context.Context, snapshotID string, maxCodes int) ([]string, error) {
	if maxCodes <= 0 {
		maxCodes = 300
	}
	codes, skipped, err := s.deps.Universe.ResolveUniverse(ctx, snapshotID, 30, maxCodes)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot universe: %w", err)
	}
	if len(codes) < minUniverseCodes {
		return nil, fmt.Errorf("snapshot %s resolved only %d usable codes (< %d): fail closed", snapshotID, len(codes), minUniverseCodes)
	}
	_ = skipped
	return codes, nil
}

func diagnosticsSummary(diags []methods.Diagnostic) string {
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Level == "error" || d.Level == "ambiguous" {
			parts = append(parts, d.Code+": "+d.Detail)
		}
	}
	if len(parts) == 0 {
		return "no diagnostics"
	}
	return strings.Join(parts, "; ")
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
