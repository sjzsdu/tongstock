// Package methodresearch turns named investment-method research into cited,
// auditable rule variants. It deliberately keeps source evidence separate from
// market-data validation evidence.
package methodresearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

type InputKind string

const (
	InputName InputKind = "name"
	InputURL  InputKind = "url"
	InputText InputKind = "text"
)

type ResearchInput struct {
	Kind      InputKind `json:"kind"`
	Value     string    `json:"value"`
	StockCode string    `json:"stock_code,omitempty"`
}

type SourceTier string

const (
	TierPrimary   SourceTier = "primary"
	TierSecondary SourceTier = "secondary"
)

type SourceDocument struct {
	ID          string     `json:"id"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Author      string     `json:"author,omitempty"`
	Publisher   string     `json:"publisher"`
	PublishedAt string     `json:"published_at,omitempty"`
	RetrievedAt time.Time  `json:"retrieved_at"`
	Tier        SourceTier `json:"tier"`
	ContentHash string     `json:"content_hash"`
}

type Citation struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Locator  string `json:"locator"`
	Excerpt  string `json:"excerpt"` // short excerpt/paraphrase; never a copied article
}

type Provenance string

const (
	ProvenancePrimary   Provenance = "primary_source"
	ProvenanceSecondary Provenance = "secondary_source"
	ProvenanceInference Provenance = "ai_inference"
	ProvenanceUser      Provenance = "user_input"
)

// RuleClaim is one atomic assertion. Key identifies the semantic slot used for
// conflict detection (for example entry.time or exit.deadline).
type RuleClaim struct {
	ID          string     `json:"id"`
	Field       string     `json:"field"`
	Key         string     `json:"key"`
	Value       string     `json:"value"`
	Provenance  Provenance `json:"provenance"`
	CitationIDs []string   `json:"citation_ids,omitempty"`
}

type VariantDraft struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	ClaimIDs  []string          `json:"claim_ids"`
	Candidate methods.Candidate `json:"candidate"`
}

// ResearchDraft is untrusted AI/provider output. Researcher verifies every
// reference and compiles candidates itself.
type ResearchDraft struct {
	MethodName string           `json:"method_name"`
	Summary    string           `json:"summary,omitempty"`
	Sources    []SourceDocument `json:"sources"`
	Citations  []Citation       `json:"citations"`
	Claims     []RuleClaim      `json:"claims"`
	Variants   []VariantDraft   `json:"variants"`
}

type Conflict struct {
	Key      string   `json:"key"`
	ClaimIDs []string `json:"claim_ids"`
	Values   []string `json:"values"`
}

type CompilationReport struct {
	FamilyID    string                  `json:"family_id"`
	VariantID   string                  `json:"variant_id"`
	Method      *methods.CompiledMethod `json:"method,omitempty"`
	Diagnostics []methods.Diagnostic    `json:"diagnostics,omitempty"`
	Executable  bool                    `json:"executable"`
	Blockers    []string                `json:"blockers,omitempty"`
}

type ValidationHandoff struct {
	JobID      string `json:"job_id"`
	FamilyID   string `json:"family_id"`
	VariantID  string `json:"variant_id"`
	MethodHash string `json:"method_hash"`
	StockCode  string `json:"stock_code,omitempty"`
	Scope      string `json:"scope"`
	Status     string `json:"status"`
	Blocker    string `json:"blocker,omitempty"`
}

type Status string

const (
	StatusComplete     Status = "source_complete"
	StatusConflict     Status = "source_conflict"
	StatusInsufficient Status = "source_insufficient"
)

type ResearchResult struct {
	ResearchID       string              `json:"research_id"`
	FamilyID         string              `json:"family_id"`
	ResultHash       string              `json:"result_hash"`
	Input            ResearchInput       `json:"input"`
	MethodName       string              `json:"method_name,omitempty"`
	Summary          string              `json:"summary,omitempty"`
	Status           Status              `json:"status"`
	Sources          []SourceDocument    `json:"sources"`
	Citations        []Citation          `json:"citations"`
	Claims           []RuleClaim         `json:"claims"`
	Conflicts        []Conflict          `json:"conflicts,omitempty"`
	Compilations     []CompilationReport `json:"compilations,omitempty"`
	ValidationJobs   []ValidationHandoff `json:"validation_jobs,omitempty"`
	RejectionReasons []string            `json:"rejection_reasons,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
}

func (r *ResearchResult) ComputeHash() string {
	if r == nil {
		return ""
	}
	type stableCompilation struct {
		FamilyID    string               `json:"family_id"`
		VariantID   string               `json:"variant_id"`
		MethodHash  string               `json:"method_hash,omitempty"`
		Executable  bool                 `json:"executable"`
		Diagnostics []methods.Diagnostic `json:"diagnostics,omitempty"`
		Blockers    []string             `json:"blockers,omitempty"`
	}
	type stableValidation struct {
		FamilyID   string `json:"family_id"`
		VariantID  string `json:"variant_id"`
		MethodHash string `json:"method_hash"`
		StockCode  string `json:"stock_code,omitempty"`
		Scope      string `json:"scope"`
		Status     string `json:"status"`
		Blocker    string `json:"blocker,omitempty"`
	}
	stable := make([]stableCompilation, 0, len(r.Compilations))
	for _, c := range r.Compilations {
		item := stableCompilation{FamilyID: c.FamilyID, VariantID: c.VariantID, Executable: c.Executable, Diagnostics: c.Diagnostics, Blockers: c.Blockers}
		if c.Method != nil {
			item.MethodHash = c.Method.ContentHash
		}
		stable = append(stable, item)
	}
	jobs := make([]stableValidation, 0, len(r.ValidationJobs))
	for _, j := range r.ValidationJobs {
		jobs = append(jobs, stableValidation{j.FamilyID, j.VariantID, j.MethodHash, j.StockCode, j.Scope, j.Status, j.Blocker})
	}
	payload := struct {
		Input          ResearchInput       `json:"input"`
		FamilyID       string              `json:"family_id"`
		MethodName     string              `json:"method_name"`
		Status         Status              `json:"status"`
		Sources        []SourceDocument    `json:"sources"`
		Citations      []Citation          `json:"citations"`
		Claims         []RuleClaim         `json:"claims"`
		Conflicts      []Conflict          `json:"conflicts"`
		Compilations   []stableCompilation `json:"compilations"`
		ValidationJobs []stableValidation  `json:"validation_jobs"`
		Rejections     []string            `json:"rejections"`
	}{r.Input, r.FamilyID, r.MethodName, r.Status, r.Sources, r.Citations, r.Claims, r.Conflicts, stable, jobs, r.RejectionReasons}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type SourceProvider interface {
	Research(context.Context, ResearchInput) (*ResearchDraft, error)
}
type Repository interface {
	Save(context.Context, *ResearchResult) error
}
