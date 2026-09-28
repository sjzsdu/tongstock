package methodseed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
)

// KlineTypeDaily 与 validationrepo.KlineTypeDaily 一致（通达信 A 股日线）。
const KlineTypeDaily uint8 = 9

// SnapshotStore 负责创建/读取不可变冻结快照。实现见 paradigm.DatasetSnapshotStore。
type SnapshotStore interface {
	CreateKlineSnapshot(snapshot *paradigm.DatasetSnapshot, ktype uint8) error
	GetByID(id string) (*paradigm.DatasetSnapshot, error)
	VerifyContent(snapshotID string) error
}

// EvidenceSink 持久化验证制品（可选）。缺失时只注册方法、不落证据制品。
type EvidenceSink interface {
	Save(ctx context.Context, bundle *validation.EvidenceBundle) error
}

// UniverseResolver 从冻结快照解析真实代码列表（可选）。
type UniverseResolver interface {
	ResolveUniverse(ctx context.Context, snapshotID string, minBars, maxCodes int) ([]string, int, error)
}

// Deps 是内置示例方法载入的装配依赖。
type Deps struct {
	Registry  *methodregistry.Registry
	Snapshots SnapshotStore
	Bars      validation.BarProvider
	Benchmark validation.BenchmarkProvider
	Evidence  EvidenceSink
	Universe  UniverseResolver
}

// Service 编排「编译 → 真实回测 → 注册」。
type Service struct {
	deps Deps
	now  func() time.Time
}

// NewService 构造内置示例方法服务。
func NewService(deps Deps) (*Service, error) {
	if deps.Registry == nil {
		return nil, fmt.Errorf("methodseed requires a method registry")
	}
	if deps.Snapshots == nil || deps.Bars == nil {
		return nil, fmt.Errorf("methodseed requires snapshot store and bar provider")
	}
	return &Service{deps: deps, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Run 载入内置示例方法：逐个编译、在冻结真实数据上做全池回测、按真实结果注册到方法库。
// 任何方法是否可信完全由验证工厂决定；未过门槛的方法如实标注，绝不硬编码为 verified。
func (s *Service) Run(ctx context.Context, opts Options) (*Result, error) {
	snapshot, universe, skippedUniverse, err := s.resolveInput(ctx, opts)
	if err != nil {
		return nil, err
	}

	seeds := filterSeeds(opts.Keys)
	res := &Result{
		SnapshotID:   snapshot.ID,
		UniverseSize: len(universe) + skippedUniverse,
		DateStart:    snapshot.DateRange.Start,
		DateEnd:      snapshot.DateRange.End,
		Outcomes:     make([]Outcome, 0, len(seeds)),
		FinishedAt:   s.now(),
	}
	if len(seeds) == 0 {
		return res, fmt.Errorf("no seed method matched keys %v", opts.Keys)
	}

	for _, seed := range seeds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outcome := s.loadOne(ctx, seed, snapshot, universe, opts)
		res.Outcomes = append(res.Outcomes, outcome)
		if outcome.MethodID != "" {
			res.Registered++
		}
		if outcome.Status == string(methodregistry.StatusVerified) {
			res.Verified++
		}
	}
	res.FinishedAt = s.now()
	return res, nil
}

// resolveInput 确定快照与验证股票池。
//   - 提供 SnapshotID：复用已冻结快照，股票池从快照解析（或由 opts.Universe 覆盖）。
//   - 未提供：要求 Universe + 日期区间，创建一个专用冻结快照。
func (s *Service) resolveInput(ctx context.Context, opts Options) (*paradigm.DatasetSnapshot, []string, int, error) {
	if id := strings.TrimSpace(opts.SnapshotID); id != "" {
		snapshot, err := s.deps.Snapshots.GetByID(id)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("load frozen snapshot %s: %w", id, err)
		}
		if err := s.deps.Snapshots.VerifyContent(id); err != nil {
			return nil, nil, 0, fmt.Errorf("verify frozen snapshot %s: %w", id, err)
		}
		universe, skipped, err := s.resolveUniverse(ctx, id, opts)
		if err != nil {
			return nil, nil, 0, err
		}
		return snapshot, universe, skipped, nil
	}

	if len(opts.Universe) == 0 {
		return nil, nil, 0, fmt.Errorf("creating a seed snapshot requires an explicit universe list")
	}
	if opts.DateStart == "" || opts.DateEnd == "" {
		return nil, nil, 0, fmt.Errorf("creating a seed snapshot requires date_start and date_end")
	}
	universe := dedupeCodes(opts.Universe)
	snapshot := &paradigm.DatasetSnapshot{
		ID:              fmt.Sprintf("methodseed-%d", s.now().UnixNano()),
		Version:         "methodseed-v1",
		Universe:        universe,
		DateRange:       paradigm.DateRange{Start: opts.DateStart, End: opts.DateEnd},
		Market:          "A",
		PriceAdjustment: paradigm.PriceRaw,
		Description:     "built-in seed methods: auto-frozen real daily K-lines",
		CreatedAt:       s.now(),
	}
	if err := s.deps.Snapshots.CreateKlineSnapshot(snapshot, KlineTypeDaily); err != nil {
		return nil, nil, 0, fmt.Errorf("freeze seed snapshot: %w", err)
	}
	return snapshot, universe, 0, nil
}

func (s *Service) resolveUniverse(ctx context.Context, snapshotID string, opts Options) ([]string, int, error) {
	if len(opts.Universe) > 0 {
		return dedupeCodes(opts.Universe), 0, nil
	}
	if s.deps.Universe == nil {
		return nil, 0, fmt.Errorf("no explicit universe and no universe resolver configured")
	}
	maxCodes := opts.MaxCodes
	if maxCodes <= 0 {
		maxCodes = 300
	}
	codes, skipped, err := s.deps.Universe.ResolveUniverse(ctx, snapshotID, 30, maxCodes)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve snapshot universe: %w", err)
	}
	if len(codes) == 0 {
		return nil, 0, fmt.Errorf("snapshot %s has no code with enough real bars", snapshotID)
	}
	return codes, skipped, nil
}

// loadOne 编译、验证并注册单个种子方法。
func (s *Service) loadOne(ctx context.Context, seed methods.SeedCandidate, snapshot *paradigm.DatasetSnapshot, universe []string, opts Options) Outcome {
	out := Outcome{Key: seed.Key}
	if seed.Candidate == nil {
		out.Status = "invalid"
		out.Error = "seed candidate is nil"
		return out
	}
	out.Name = seed.Candidate.Name

	compiled, diags, err := methods.Compile(seed.Candidate)
	if err != nil {
		out.Status = "invalid"
		out.Error = fmt.Sprintf("compile: %v", err)
		return out
	}
	if compiled == nil || !compiled.IsExecutable() {
		out.Status = "invalid"
		out.Error = "compiled method is not executable: " + diagnosticsSummary(diags)
		return out
	}
	out.MethodHash = compiled.ContentHash

	job := validation.ValidationJob{
		MethodHash:      compiled.ContentHash,
		MethodName:      compiled.Name,
		SnapshotID:      snapshot.ID,
		Universe:        universe,
		DateStart:       snapshot.DateRange.Start,
		DateEnd:         snapshot.DateRange.End,
		SplitType:       opts.SplitType,
		DiscoveryTrials: opts.DiscoveryTrials,
	}
	factory, err := validation.NewFactory(validation.FactoryDeps{
		Method: compiled, Bars: s.deps.Bars, Benchmark: s.deps.Benchmark,
	})
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	bundle, err := factory.Run(ctx, job)
	if err != nil {
		out.Status = "failed"
		out.Error = fmt.Sprintf("validation: %v", err)
		return out
	}
	if s.deps.Evidence != nil {
		if saveErr := s.deps.Evidence.Save(ctx, bundle); saveErr != nil {
			out.Status = "failed"
			out.Error = fmt.Sprintf("persist evidence: %v", saveErr)
			return out
		}
	}
	out.Confidence = string(bundle.Confidence)
	out.Passable = bundle.Passable
	out.OOSTrades = bundle.OosStats.TotalTrades
	out.OOSReturn = bundle.OosStats.TotalReturn
	out.OOSMaxDrawdown = bundle.OosStats.MaxDrawdown
	out.ResultHash = bundle.ResultHash
	for _, b := range bundle.Blockers {
		out.Blockers = append(out.Blockers, b.Code)
	}

	m, err := s.deps.Registry.Register(ctx, methodregistry.Registration{
		FamilyID:         "seed-" + seed.Key,
		VariantID:        compiled.ContentHash,
		ValidationJobID:  bundle.JobHash,
		Market:           "A",
		TriggerFrequency: "daily",
		EntrySummary:     seed.Rationale,
		ExitSummary:      exitSummary(compiled),
		Method:           compiled,
		Evidence:         methodregistry.ValidationEvidence{Bundle: bundle},
	})
	if err != nil {
		out.Status = "failed"
		out.Error = fmt.Sprintf("register: %v", err)
		return out
	}
	out.MethodID = m.ID
	out.Status = string(m.Status)
	return out
}

func filterSeeds(keys []string) []methods.SeedCandidate {
	all := methods.SeedCandidates()
	if len(keys) == 0 {
		return all
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[strings.TrimSpace(k)] = true
	}
	out := make([]methods.SeedCandidate, 0, len(all))
	for _, s := range all {
		if want[s.Key] {
			out = append(out, s)
		}
	}
	return out
}

func dedupeCodes(codes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func diagnosticsSummary(diags []methods.Diagnostic) string {
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Level == "error" || d.Level == "ambiguous" {
			parts = append(parts, d.Code)
		}
	}
	if len(parts) == 0 {
		return "no diagnostics"
	}
	return strings.Join(parts, ",")
}

func exitSummary(m *methods.CompiledMethod) string {
	if m.Holding.MaxDays > 0 {
		return fmt.Sprintf("按退出规则，最长持有 %d 个交易日", m.Holding.MaxDays)
	}
	return "按退出规则离场"
}
