// Package discovery 从冻结真实历史中确定性地挖掘可证伪候选规律。
// 本包只生成 candidate，不会把发现阶段收益当成验证结论。
package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

const GeneratorVersion = "price-pattern-v2"

type Request struct {
	SnapshotID   string   `json:"snapshot_id"`
	StockCodes   []string `json:"stock_codes"`
	Question     string   `json:"question,omitempty"`
	HoldDays     int      `json:"hold_days,omitempty"`
	SearchBudget int      `json:"search_budget,omitempty"`
	// CandidateSpecs is the provider-neutral contract for AI/discovery
	// candidates. Each candidate carries its own scope and outcome definition.
	// When empty, Researcher uses the deterministic templates as a compatibility
	// fallback; those templates are not presented as AI-generated discoveries.
	CandidateSpecs []methods.Candidate `json:"candidate_specs,omitempty"`
	// OnProgress 可选：模板扫描每完成一只股票回调一次（codes_done, codes_total），
	// 供编排器把长扫描阶段透出到状态端点，用户不用对着 0/0 干等。
	OnProgress func(codesDone, codesTotal int)
}

// CandidateGenerator allows a real AI/provider to supply structured method
// candidates without coupling discovery to an LLM implementation.
type CandidateGenerator interface {
	Generate(context.Context, Request) ([]methods.Candidate, error)
}

func (r *Request) Normalize() error {
	if r.SnapshotID == "" {
		return fmt.Errorf("snapshot_id is required")
	}
	if len(r.StockCodes) == 0 {
		return fmt.Errorf("at least one stock code is required")
	}
	if r.HoldDays == 0 {
		r.HoldDays = 5
	}
	if r.HoldDays < 1 || r.HoldDays > 60 {
		return fmt.Errorf("hold_days must be in [1,60]")
	}
	if r.SearchBudget == 0 {
		r.SearchBudget = 24
	}
	if r.SearchBudget < 1 || r.SearchBudget > 100 {
		return fmt.Errorf("search_budget must be in [1,100]")
	}
	for i := range r.CandidateSpecs {
		if err := validateCandidateSpec(r.CandidateSpecs[i], i); err != nil {
			return err
		}
	}
	return nil
}

// validateCandidateSpec keeps provider output explicit. A mined candidate may
// choose its own horizon, target and supported price/success semantics; the
// discovery service must not silently turn a missing target into a generic
// positive-return label.
func validateCandidateSpec(candidate methods.Candidate, index int) error {
	o := candidate.Outcome
	if o == nil {
		return fmt.Errorf("candidate_specs[%d].outcome is required", index)
	}
	if o.HorizonDays < 1 || o.HorizonDays > 252 {
		return fmt.Errorf("candidate_specs[%d].outcome.horizon_days must be in [1,252]", index)
	}
	if o.TargetReturnPct == nil {
		return fmt.Errorf("candidate_specs[%d].outcome.target_return_pct is required", index)
	}
	if !supportedOutcomeBasis(o.PriceBasis) {
		return fmt.Errorf("candidate_specs[%d].outcome.price_basis %q is unsupported", index, o.PriceBasis)
	}
	if o.Success != "close_gte_target" && o.Success != "price_gte_target" && o.Success != "price_lte_target" {
		return fmt.Errorf("candidate_specs[%d].outcome.success %q is unsupported", index, o.Success)
	}
	if o.Success == "close_gte_target" && o.PriceBasis != "close" {
		return fmt.Errorf("candidate_specs[%d] uses close_gte_target with price_basis %q", index, o.PriceBasis)
	}
	return nil
}

func supportedOutcomeBasis(value string) bool {
	switch value {
	case "close", "open", "high", "low":
		return true
	default:
		return false
	}
}

type BarProvider interface {
	Load(ctx context.Context, snapshotID, code string) ([]methods.Bar, error)
}

type CandidateEvidence struct {
	Rank               int                     `json:"rank"`
	TemplateID         string                  `json:"template_id"`
	Method             *methods.CompiledMethod `json:"method"`
	Observations       int                     `json:"observations"`
	MeanForwardReturn  float64                 `json:"mean_forward_return"`
	WinRate            float64                 `json:"win_rate"`
	BaselineReturn     float64                 `json:"baseline_return"`
	Lift               float64                 `json:"lift"`
	TStatistic         float64                 `json:"t_statistic"`
	Rationale          string                  `json:"rationale"`
	Source             string                  `json:"source"`
	Outcome            methods.OutcomeRule     `json:"outcome,omitempty"`
	ValidationJobs     []ValidationHandoff     `json:"validation_jobs"`
	ValidationEvidence []ValidationEvidenceRef `json:"validation_evidence,omitempty"`
}

// ValidationHandoff 只指向发现阶段从未触碰的保留样本。
type ValidationHandoff struct {
	MethodHash      string `json:"method_hash"`
	SnapshotID      string `json:"snapshot_id"`
	StockCode       string `json:"stock_code"`
	DateStart       string `json:"date_start"`
	DateEnd         string `json:"date_end"`
	DiscoveryTrials int    `json:"discovery_trials"`
}

type ValidationEvidenceRef struct {
	StockCode  string `json:"stock_code"`
	Status     string `json:"status"` // completed / failed
	ResultHash string `json:"result_hash,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Passable   bool   `json:"passable"`
	Error      string `json:"error,omitempty"`
}

type RejectedCandidate struct {
	TemplateID   string `json:"template_id"`
	Reason       string `json:"reason"`
	Observations int    `json:"observations"`
}

type CodeBoundary struct {
	Code              string `json:"code"`
	FirstDate         string `json:"first_date"`
	DiscoveryEndDate  string `json:"discovery_end_date"`
	ReservedStartDate string `json:"reserved_start_date"`
	LastDate          string `json:"last_date"`
	DiscoveryBars     int    `json:"discovery_bars"`
	ReservedBars      int    `json:"reserved_bars"`
}

type Result struct {
	ResearchID       string              `json:"research_id"`
	SnapshotID       string              `json:"snapshot_id"`
	GeneratorVersion string              `json:"generator_version"`
	GeneratedAt      time.Time           `json:"generated_at"`
	Question         string              `json:"question,omitempty"`
	HoldDays         int                 `json:"hold_days"`
	SearchBudget     int                 `json:"search_budget"`
	CandidateSpecs   []methods.Candidate `json:"candidate_specs,omitempty"`
	DiscoveryTrials  int                 `json:"discovery_trials"`
	Boundaries       []CodeBoundary      `json:"boundaries"`
	Candidates       []CandidateEvidence `json:"candidates,omitempty"`
	Rejected         []RejectedCandidate `json:"rejected,omitempty"`
	Conclusion       string              `json:"conclusion"` // ranked_hypotheses / insufficient_evidence
	ResultHash       string              `json:"result_hash"`
}

func (r *Result) ComputeHash() string {
	type candidateDigest struct {
		Rank               int
		TemplateID         string
		MethodHash         string
		Observations       int
		MeanForwardReturn  float64
		WinRate            float64
		BaselineReturn     float64
		Lift               float64
		TStatistic         float64
		Rationale          string
		Source             string
		ValidationJobs     []ValidationHandoff
		ValidationEvidence []ValidationEvidenceRef
		Outcome            methods.OutcomeRule
	}
	candidates := make([]candidateDigest, len(r.Candidates))
	for i, candidate := range r.Candidates {
		methodHash := ""
		if candidate.Method != nil {
			methodHash = candidate.Method.ContentHash
		}
		candidates[i] = candidateDigest{
			Rank: candidate.Rank, TemplateID: candidate.TemplateID, MethodHash: methodHash,
			Observations: candidate.Observations, MeanForwardReturn: candidate.MeanForwardReturn,
			WinRate: candidate.WinRate, BaselineReturn: candidate.BaselineReturn,
			Lift: candidate.Lift, TStatistic: candidate.TStatistic,
			Rationale: candidate.Rationale, Source: candidate.Source,
			Outcome:            candidate.Outcome,
			ValidationJobs:     candidate.ValidationJobs,
			ValidationEvidence: candidate.ValidationEvidence,
		}
	}
	payload := struct {
		ResearchID, SnapshotID, Version, Question string
		HoldDays, SearchBudget, Trials            int
		Boundaries                                []CodeBoundary
		Candidates                                []candidateDigest
		CandidateSpecs                            []methods.Candidate
		Rejected                                  []RejectedCandidate
		Conclusion                                string
	}{ResearchID: r.ResearchID, SnapshotID: r.SnapshotID, Version: r.GeneratorVersion, Question: r.Question,
		HoldDays: r.HoldDays, SearchBudget: r.SearchBudget, Trials: r.DiscoveryTrials,
		Boundaries: r.Boundaries, Candidates: candidates, CandidateSpecs: r.CandidateSpecs,
		Rejected: r.Rejected, Conclusion: r.Conclusion}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
