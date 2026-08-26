package main

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/internal/adapter/discoveryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/validationrepo"
	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/spf13/cobra"
)

var methodLibraryCmd = &cobra.Command{Use: "library", Short: "可信投资方法库与机器生命周期"}
var methodLibrarySyncCmd = &cobra.Command{Use: "sync-discovery [research-id]", Short: "把真实发现与验证制品同步到方法库", Args: cobra.RangeArgs(0, 1), RunE: func(cmd *cobra.Command, args []string) error {
	store, err := openValidationStorage()
	if err != nil {
		return err
	}
	defer store.Close()
	researchID := ""
	if len(args) > 0 {
		researchID = strings.TrimSpace(args[0])
	} else {
		err = store.DB().QueryRowContext(cmd.Context(), `SELECT research_id FROM discovery_research_trace ORDER BY created_at_ns DESC LIMIT 1`).Scan(&researchID)
		if err == sql.ErrNoRows {
			return fmt.Errorf("no discovery research trace available")
		}
		if err != nil {
			return err
		}
	}
	traceRepo, err := discoveryrepo.NewTraceRepository(store)
	if err != nil {
		return err
	}
	trace, err := traceRepo.Get(cmd.Context(), researchID)
	if err != nil {
		return err
	}
	registryRepo, err := methodregistryrepo.New(store)
	if err != nil {
		return err
	}
	registry, err := methodregistry.New(registryRepo)
	if err != nil {
		return err
	}
	evidenceRepo, err := validationrepo.NewEvidenceRepository(store)
	if err != nil {
		return err
	}
	registered := make([]*methodregistry.Method, 0, len(trace.Candidates))
	for _, candidate := range trace.Candidates {
		if candidate.Method == nil {
			continue
		}
		if len(candidate.ValidationEvidence) == 0 {
			m, err := registerDiscoveryMethod(cmd, registry, trace, candidate, nil)
			if err != nil {
				return err
			}
			registered = append(registered, m)
			continue
		}
		for _, ref := range candidate.ValidationEvidence {
			if ref.ResultHash == "" {
				continue
			}
			bundle, err := evidenceRepo.Get(cmd.Context(), ref.ResultHash)
			if err != nil {
				return fmt.Errorf("load evidence %s: %w", ref.ResultHash, err)
			}
			m, err := registerDiscoveryMethod(cmd, registry, trace, candidate, bundle)
			if err != nil {
				return err
			}
			registered = append(registered, m)
		}
	}
	return printValidationJSON(map[string]any{"research_id": researchID, "registered": registered, "count": len(registered)})
}}

func registerDiscoveryMethod(cmd *cobra.Command, registry *methodregistry.Registry, trace *discovery.Result, candidate discovery.CandidateEvidence, bundle *validation.EvidenceBundle) (*methodregistry.Method, error) {
	var evidence methodregistry.Evidence
	if bundle != nil {
		evidence = methodregistry.ValidationEvidence{Bundle: bundle}
	}
	return registry.Register(cmd.Context(), methodregistry.Registration{FamilyID: "discovery-family-" + candidate.TemplateID, VariantID: candidate.Method.ContentHash, SourceResearchID: trace.ResearchID, ValidationJobID: validationJobID(candidate, bundle), Market: "A", TriggerFrequency: "daily", EntrySummary: candidate.Rationale, ExitSummary: fmt.Sprintf("按退出规则，最长持有%d个交易日", candidate.Method.Holding.MaxDays), Method: candidate.Method, Evidence: evidence})
}
func validationJobID(candidate discovery.CandidateEvidence, bundle *validation.EvidenceBundle) string {
	if bundle != nil {
		return bundle.JobHash
	}
	if len(candidate.ValidationJobs) > 0 {
		return candidate.ValidationJobs[0].MethodHash + ":" + candidate.ValidationJobs[0].SnapshotID
	}
	return ""
}

func init() {
	methodLibraryCmd.AddCommand(methodLibrarySyncCmd)
	methodCmd.AddCommand(methodLibraryCmd)
}
