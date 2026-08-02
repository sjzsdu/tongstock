package methodresearch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

var numberPattern = regexp.MustCompile(`\d`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

type Researcher struct {
	provider SourceProvider
	repo     Repository
	now      func() time.Time
}

func New(provider SourceProvider, repo Repository) (*Researcher, error) {
	if provider == nil {
		return nil, fmt.Errorf("source provider is required")
	}
	return &Researcher{provider: provider, repo: repo, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (r *Researcher) Run(ctx context.Context, input ResearchInput) (*ResearchResult, error) {
	input.Value = strings.TrimSpace(input.Value)
	input.StockCode = strings.TrimSpace(input.StockCode)
	if input.Value == "" {
		return nil, fmt.Errorf("research input is required")
	}
	if input.Kind != InputName && input.Kind != InputURL && input.Kind != InputText {
		return nil, fmt.Errorf("unsupported input kind %q", input.Kind)
	}
	draft, err := r.provider.Research(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("collect source evidence: %w", err)
	}
	if draft == nil {
		return nil, fmt.Errorf("source provider returned nil draft")
	}
	now := r.now()
	result := &ResearchResult{ResearchID: fmt.Sprintf("method-research-%d", now.UnixNano()), Input: input, MethodName: strings.TrimSpace(draft.MethodName), Summary: strings.TrimSpace(draft.Summary), CreatedAt: now}
	result.FamilyID = computeFamilyID(input, result.MethodName)
	result.Sources, result.Citations, result.Claims, result.RejectionReasons = verifyEvidence(draft)
	result.Conflicts = detectConflicts(result.Claims)

	if !sourceSufficient(input, result.Sources, result.Claims) {
		result.Status = StatusInsufficient
		result.RejectionReasons = appendUnique(result.RejectionReasons, "没有足够可靠且可追溯的规则来源，拒绝生成伪精确方法")
	} else if len(result.Conflicts) > 0 {
		result.Status = StatusConflict
	} else {
		result.Status = StatusComplete
	}

	if result.Status != StatusInsufficient {
		result.Compilations = compileVariants(result.FamilyID, draft.Variants, result.Claims, result.Conflicts)
		for _, report := range result.Compilations {
			if report.Executable {
				job := ValidationHandoff{JobID: result.ResearchID + ":" + report.VariantID, FamilyID: result.FamilyID, VariantID: report.VariantID, MethodHash: report.Method.ContentHash, StockCode: input.StockCode, Status: "queued"}
				if input.StockCode == "" {
					job.Scope = "universe_all"
				} else {
					job.Scope = "single_stock"
				}
				result.ValidationJobs = append(result.ValidationJobs, job)
			}
		}
	}
	result.ResultHash = result.ComputeHash()
	if r.repo != nil {
		if err := r.repo.Save(ctx, result); err != nil {
			return nil, fmt.Errorf("persist source research: %w", err)
		}
	}
	return result, nil
}

func verifyEvidence(d *ResearchDraft) ([]SourceDocument, []Citation, []RuleClaim, []string) {
	var sources []SourceDocument
	var citations []Citation
	var claims []RuleClaim
	var rejected []string
	sourceIDs := map[string]bool{}
	sourceTiers := map[string]SourceTier{}
	for _, s := range d.Sources {
		u, err := url.ParseRequestURI(strings.TrimSpace(s.URL))
		validURL := err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
		if strings.TrimSpace(s.ID) == "" || !validURL || strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Publisher) == "" || s.RetrievedAt.IsZero() || (s.Tier != TierPrimary && s.Tier != TierSecondary) || !sha256Pattern.MatchString(strings.TrimSpace(s.ContentHash)) {
			rejected = append(rejected, "invalid source evidence: "+s.ID)
			continue
		}
		if sourceIDs[s.ID] {
			rejected = append(rejected, "duplicate source id: "+s.ID)
			continue
		}
		sourceIDs[s.ID] = true
		sourceTiers[s.ID] = s.Tier
		sources = append(sources, s)
	}
	citationIDs := map[string]bool{}
	citationSources := map[string]string{}
	for _, c := range d.Citations {
		c.Excerpt = strings.TrimSpace(c.Excerpt)
		if c.ID == "" || !sourceIDs[c.SourceID] || strings.TrimSpace(c.Locator) == "" || c.Excerpt == "" || len([]rune(c.Excerpt)) > 500 {
			rejected = append(rejected, "invalid citation evidence: "+c.ID)
			continue
		}
		if citationIDs[c.ID] {
			rejected = append(rejected, "duplicate citation id: "+c.ID)
			continue
		}
		citationIDs[c.ID] = true
		citationSources[c.ID] = c.SourceID
		citations = append(citations, c)
	}
	claimIDs := map[string]bool{}
	for _, c := range d.Claims {
		valid := c.ID != "" && c.Field != "" && c.Key != "" && strings.TrimSpace(c.Value) != "" && !claimIDs[c.ID]
		if c.Provenance == ProvenancePrimary || c.Provenance == ProvenanceSecondary {
			if len(c.CitationIDs) == 0 {
				valid = false
			}
			for _, id := range c.CitationIDs {
				if !citationIDs[id] {
					valid = false
				}
			}
			if c.Provenance == ProvenancePrimary {
				hasPrimary := false
				for _, id := range c.CitationIDs {
					if sourceTiers[citationSources[id]] == TierPrimary {
						hasPrimary = true
					}
				}
				if !hasPrimary {
					valid = false
				}
			}
		} else if c.Provenance != ProvenanceInference && c.Provenance != ProvenanceUser {
			valid = false
		}
		if !valid {
			rejected = append(rejected, "invalid or untraceable claim: "+c.ID)
			continue
		}
		claimIDs[c.ID] = true
		claims = append(claims, c)
	}
	return sources, citations, claims, rejected
}

func sourceSufficient(input ResearchInput, sources []SourceDocument, claims []RuleClaim) bool {
	if len(sources) == 0 || len(claims) == 0 {
		return false
	}
	if input.Kind == InputName && len(sources) < 2 {
		return false
	}
	hasEntry, hasExit := false, false
	for _, c := range claims {
		trustedForName := c.Provenance == ProvenancePrimary || c.Provenance == ProvenanceSecondary
		if c.Field == "entry" && (input.Kind != InputName || trustedForName) {
			hasEntry = true
		}
		if c.Field == "exit" && (input.Kind != InputName || trustedForName) {
			hasExit = true
		}
	}
	return hasEntry && hasExit
}

func detectConflicts(claims []RuleClaim) []Conflict {
	groups := map[string]map[string][]string{}
	for _, c := range claims {
		if c.Provenance == ProvenanceInference {
			continue
		}
		value := strings.ToLower(strings.Join(strings.Fields(c.Value), " "))
		if groups[c.Key] == nil {
			groups[c.Key] = map[string][]string{}
		}
		groups[c.Key][value] = append(groups[c.Key][value], c.ID)
	}
	var out []Conflict
	for key, values := range groups {
		if len(values) < 2 {
			continue
		}
		c := Conflict{Key: key}
		for v, ids := range values {
			c.Values = append(c.Values, v)
			c.ClaimIDs = append(c.ClaimIDs, ids...)
		}
		sort.Strings(c.Values)
		sort.Strings(c.ClaimIDs)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func compileVariants(familyID string, variants []VariantDraft, claims []RuleClaim, conflicts []Conflict) []CompilationReport {
	claimByID := map[string]RuleClaim{}
	for _, c := range claims {
		claimByID[c.ID] = c
	}
	var out []CompilationReport
	for _, v := range variants {
		report := CompilationReport{FamilyID: familyID, VariantID: v.ID}
		selected := map[string]bool{}
		fields := map[string]bool{}
		for _, id := range v.ClaimIDs {
			if c, ok := claimByID[id]; ok {
				selected[id] = true
				fields[c.Field] = true
			} else {
				report.Blockers = append(report.Blockers, "variant references unverified claim: "+id)
			}
		}
		if !fields["entry"] || !fields["exit"] || !fields["position"] {
			report.Blockers = append(report.Blockers, "variant must include cited entry, exit, and position claims")
		}
		for _, conflict := range conflicts {
			count := 0
			for _, id := range conflict.ClaimIDs {
				if selected[id] {
					count++
				}
			}
			if count != 1 {
				report.Blockers = append(report.Blockers, "variant must choose exactly one value for conflict "+conflict.Key)
			}
		}
		for id := range selected {
			c := claimByID[id]
			if c.Provenance == ProvenanceInference && numberPattern.MatchString(c.Value) {
				report.Blockers = append(report.Blockers, "AI inference cannot invent a numeric threshold: "+id)
			}
		}
		m, diags, err := methods.Compile(&v.Candidate)
		report.Method = m
		report.Diagnostics = diags
		if err != nil {
			report.Blockers = append(report.Blockers, err.Error())
		}
		report.Executable = len(report.Blockers) == 0 && m != nil && m.IsExecutable()
		out = append(out, report)
	}
	return out
}

func computeFamilyID(input ResearchInput, methodName string) string {
	name := strings.ToLower(strings.Join(strings.Fields(firstResearchName(methodName, input.Value)), " "))
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("method-family-%x", sum[:8])
}

func firstResearchName(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "unnamed"
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}
