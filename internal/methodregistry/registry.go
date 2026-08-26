package methodregistry

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

type Registry struct {
	repo   Repository
	policy Policy
	now    func() time.Time
}

func New(repo Repository) (*Registry, error) {
	if repo == nil {
		return nil, fmt.Errorf("method registry repository is required")
	}
	return &Registry{repo: repo, policy: Policy{}, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (r *Registry) Register(ctx context.Context, in Registration) (*Method, error) {
	if err := validateRegistration(in); err != nil {
		return nil, err
	}
	now := r.now()
	e := EvidenceInput{}
	if in.Evidence != nil {
		e = in.Evidence.RegistryEvidence()
		if e.MethodHash != in.Method.ContentHash {
			return nil, fmt.Errorf("validation method hash mismatch")
		}
	}
	status, reason := r.policy.Initial(in.Method.IsExecutable(), in.Method.Scope.Universe, e)
	id := stableID("method", in.FamilyID, in.VariantID)
	m, err := r.repo.Get(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	from := StatusDraft
	if m == nil {
		m = &Method{ID: id, FamilyID: in.FamilyID, VariantID: in.VariantID, CreatedAt: now}
	} else {
		from = m.Status
	}
	nextVersion := m.CurrentVersion + 1
	m.Name = in.Method.Name
	m.Status = status
	m.Market = first(in.Market, "A")
	m.Universe = in.Method.Scope.Universe
	m.HoldingMinDays = in.Method.Holding.MinDays
	m.HoldingMaxDays = in.Method.Holding.MaxDays
	m.TriggerFrequency = first(in.TriggerFrequency, "daily")
	m.EntrySummary = in.EntrySummary
	m.ExitSummary = in.ExitSummary
	m.Invalidations = append([]string{}, in.Invalidations...)
	m.CurrentVersion = nextVersion
	m.UpdatedAt = now
	v := MethodVersion{ID: stableID(id, fmt.Sprintf("%d", nextVersion), in.Method.ContentHash), Version: nextVersion, MethodHash: in.Method.ContentHash, CompilerVersion: in.Method.CompilerVersion, SourceResearchID: in.SourceResearchID, ValidationJobID: in.ValidationJobID, Method: cloneCompiled(in.Method), CreatedAt: now}
	if e.ResultHash != "" {
		v.Evidence = &EvidenceSummary{ResultHash: e.ResultHash, SnapshotID: e.SnapshotID, JobHash: e.JobHash, Confidence: e.Confidence, Passable: e.Passable, OOSTrades: e.OOSTrades, OOSReturn: e.OOSReturn, OOSWinRate: e.OOSWinRate, OOSMaxDrawdown: e.OOSMaxDrawdown}
	}
	m.Versions = append(m.Versions, v)
	event := AuditEvent{ID: stableID(id, now.Format(time.RFC3339Nano)), MethodID: id, From: from, To: status, Action: "register", Reason: reason, Actor: "policy-engine", EvidenceHash: e.ResultHash, Automatic: true, CreatedAt: now}
	if err := r.repo.Save(ctx, m, event); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *Registry) ApplyHealth(ctx context.Context, id string, h HealthState) (*Method, error) {
	m, err := r.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	from := m.Status
	to, reason := r.policy.Health(from, h)
	m.Health = &h
	m.Status = to
	m.UpdatedAt = r.now()
	event := AuditEvent{ID: stableID(id, h.AsOf.Format(time.RFC3339Nano), string(to)), MethodID: id, From: from, To: to, Action: "health_policy", Reason: reason, Actor: "policy-engine", EvidenceHash: h.EvidenceHash, Automatic: true, CreatedAt: r.now()}
	if err := r.repo.Save(ctx, m, event); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *Registry) Annotate(ctx context.Context, id, text, actor string) (*Method, error) {
	if strings.TrimSpace(text) == "" || strings.TrimSpace(actor) == "" {
		return nil, fmt.Errorf("annotation text and actor are required")
	}
	m, err := r.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := r.now()
	m.Annotations = append(m.Annotations, Annotation{Text: text, Actor: actor, CreatedAt: now})
	m.UpdatedAt = now
	event := AuditEvent{ID: stableID(id, now.Format(time.RFC3339Nano)), MethodID: id, From: m.Status, To: m.Status, Action: "annotate", Reason: text, Actor: actor, Automatic: false, CreatedAt: now}
	if err := r.repo.Save(ctx, m, event); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *Registry) ManualTransition(ctx context.Context, id string, to Status, reason, actor string) (*Method, error) {
	if to != StatusRetired && to != StatusRejected {
		return nil, fmt.Errorf("manual transition to %s forbidden; verified/observing/degraded are policy-controlled", to)
	}
	m, err := r.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	from := m.Status
	m.Status = to
	m.UpdatedAt = r.now()
	event := AuditEvent{ID: stableID(id, m.UpdatedAt.Format(time.RFC3339Nano)), MethodID: id, From: from, To: to, Action: "manual_transition", Reason: reason, Actor: actor, Automatic: false, CreatedAt: m.UpdatedAt}
	if err := r.repo.Save(ctx, m, event); err != nil {
		return nil, err
	}
	return m, nil
}
func (r *Registry) Get(ctx context.Context, id string) (*Method, error) { return r.repo.Get(ctx, id) }
func (r *Registry) Card(ctx context.Context, id string) (Card, error) {
	m, err := r.repo.Get(ctx, id)
	if err != nil {
		return Card{}, err
	}
	return toCard(m), nil
}
func (r *Registry) Cards(ctx context.Context, q Query) ([]Card, error) {
	items, err := r.repo.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(items))
	for _, m := range items {
		cards = append(cards, toCard(m))
	}
	return cards, nil
}
func (r *Registry) Audit(ctx context.Context, id string) ([]AuditEvent, error) {
	return r.repo.ListAudit(ctx, id)
}

func toCard(m *Method) Card {
	c := Card{ID: m.ID, FamilyID: m.FamilyID, VariantID: m.VariantID, Name: m.Name, Status: m.Status, Market: m.Market, Universe: m.Universe, TriggerFrequency: m.TriggerFrequency, HoldingPeriod: holdingText(m.HoldingMinDays, m.HoldingMaxDays), EntrySummary: m.EntrySummary, ExitSummary: m.ExitSummary, Invalidations: append([]string{}, m.Invalidations...), Health: m.Health, UpdatedAt: m.UpdatedAt}
	if len(m.Versions) > 0 {
		c.Evidence = m.Versions[len(m.Versions)-1].Evidence
	}
	return c
}
func holdingText(min, max int) string {
	switch {
	case min > 0 && max > 0:
		return fmt.Sprintf("%d-%d个交易日", min, max)
	case max > 0:
		return fmt.Sprintf("最多%d个交易日", max)
	case min > 0:
		return fmt.Sprintf("至少%d个交易日", min)
	default:
		return "由退出规则决定"
	}
}
func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%s-%x", parts[0], sum[:8])
}
func first(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
func cloneCompiled(m *methods.CompiledMethod) *methods.CompiledMethod {
	if m == nil {
		return nil
	}
	copy := *m
	return &copy
}
