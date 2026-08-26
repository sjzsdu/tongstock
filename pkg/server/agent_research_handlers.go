package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/ai_critic"
	"github.com/sjzsdu/tongstock/internal/ai_tools"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/paradigms"
)

type agentResearchRequest struct {
	ParadigmID string                    `json:"paradigm_id"`
	Question   string                    `json:"question,omitempty"`
	SnapshotID string                    `json:"snapshot_id,omitempty"`
	StartDate  string                    `json:"start_date,omitempty"`
	EndDate    string                    `json:"end_date,omitempty"`
	Split      experiment.SplitConfigRef `json:"split,omitempty"`
}

type researchCitation struct {
	ExperimentID string   `json:"experiment_id"`
	RunID        string   `json:"run_id"`
	SnapshotID   string   `json:"snapshot_id"`
	EvidenceHash string   `json:"evidence_hash"`
	ResultHash   string   `json:"result_hash"`
	TradeIDs     []string `json:"trade_ids,omitempty"`
}

type agentResearchResponse struct {
	Conclusion string                   `json:"conclusion"`
	Answer     string                   `json:"answer"`
	Citation   researchCitation         `json:"citation"`
	Evidence   *paradigms.EvidenceCard  `json:"evidence"`
	Critic     *ai_critic.ReviewOutcome `json:"critic"`
	ToolTrace  *ai_tools.ToolCallLog    `json:"tool_trace,omitempty"`
}

// verifiedResearchEvidenceTool is the production read-only research tool used
// by the analysis endpoint. It can only return evidence rebuilt from a frozen
// snapshot and persisted experiment artifacts.
type verifiedResearchEvidenceTool struct {
	server *Server
}

func (t *verifiedResearchEvidenceTool) Name() string { return "verified_research_evidence" }
func (t *verifiedResearchEvidenceTool) Description() string {
	return "按 paradigm_id 和 experiment_id 查询冻结快照、持久化交易制品及 critic 审查组成的可验证研究证据。"
}
func (t *verifiedResearchEvidenceTool) Version() string { return "1.0.0" }
func (t *verifiedResearchEvidenceTool) Permissions() []ai_tools.ToolPermission {
	return []ai_tools.ToolPermission{ai_tools.PermRead}
}
func (t *verifiedResearchEvidenceTool) Invoke(
	_ ai_tools.AccessContext,
	params map[string]any,
) (*ai_tools.ToolResult, error) {
	paradigmID, _ := params["paradigm_id"].(string)
	experimentID, _ := params["experiment_id"].(string)
	if strings.TrimSpace(paradigmID) == "" || strings.TrimSpace(experimentID) == "" {
		return nil, fmt.Errorf("paradigm_id and experiment_id are required")
	}
	card, err := t.server.latestParadigmExperimentEvidence(paradigmID, experimentID)
	if err != nil {
		return nil, err
	}
	if !card.Available {
		return &ai_tools.ToolResult{
			Success: false, Data: card, Version: t.Version(),
			Summary: "真实实验证据不完整，拒绝生成验证结论",
		}, nil
	}
	return &ai_tools.ToolResult{
		Success: true, Data: card, Version: t.Version(),
		Summary: fmt.Sprintf("实验 %s 的真实证据已验证，evidence_hash=%s",
			experimentID, card.EvidenceHash),
		Metadata: map[string]any{
			"experiment_id": experimentID, "run_id": card.RunID,
			"snapshot_id": card.SnapshotID, "evidence_hash": card.EvidenceHash,
		},
	}, nil
}

func (s *Server) handleAgentResearch(c *gin.Context) {
	if s.researchTools == nil || s.experimentRegistry == nil || s.paradigmStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "真实研究工具、实验注册表或范式仓储未初始化",
		})
		return
	}
	var req agentResearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ParadigmID = strings.TrimSpace(req.ParadigmID)
	if req.ParadigmID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "paradigm_id is required；AI 不会仅凭自然语言生成已验证结论",
		})
		return
	}
	result, exp, run, err := s.conductVerifiedResearch(c.Request.Context(), req)
	if err != nil {
		body := gin.H{
			"error":      err.Error(),
			"conclusion": "insufficient_data",
			"answer":     "真实数据、冻结快照、实验制品或工具证据不足，拒绝给出有效性结论。",
		}
		if exp != nil {
			body["experiment_id"] = exp.ID
		}
		if run != nil {
			body["run_id"] = run.ID
		}
		c.JSON(http.StatusUnprocessableEntity, body)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (s *Server) conductVerifiedResearch(
	ctx context.Context,
	req agentResearchRequest,
) (*agentResearchResponse, *experiment.Experiment, *experiment.ExperimentRun, error) {
	p, exp, run, _, err := s.executeParadigmExperiment(ctx,
		paradigmBacktestRequest{
			ParadigmID: req.ParadigmID, SnapshotID: req.SnapshotID,
			StartDate: req.StartDate, EndDate: req.EndDate, Split: req.Split,
		}, "agent-research")
	if err != nil {
		return nil, exp, run, err
	}
	toolResult, err := s.researchTools.Call(ai_tools.AccessContext{
		AgentID: "agent-research", SessionID: "research:" + exp.ID,
		Role: "researcher", Timestamp: time.Now(),
	}, "verified_research_evidence", map[string]any{
		"paradigm_id": p.ID, "experiment_id": exp.ID,
	})
	if err != nil {
		return nil, exp, run, err
	}
	card, ok := toolResult.Data.(*paradigms.EvidenceCard)
	if !ok || card == nil {
		return nil, exp, run, fmt.Errorf("research tool returned invalid evidence")
	}
	critic, err := criticOutcomeFromRun(run)
	if err != nil {
		return nil, exp, run, err
	}
	tradeIDs := make([]string, 0, len(card.TradeSamples))
	for _, trade := range card.TradeSamples {
		tradeIDs = append(tradeIDs, trade.TradeID)
	}
	conclusion := "not_verified"
	if card.PromotionEligible && critic.Passed() {
		conclusion = "evidence_passed"
	}
	logs := s.researchTools.GetLogs("agent-research", 1)
	var trace *ai_tools.ToolCallLog
	if len(logs) > 0 {
		trace = logs[0]
	}
	return &agentResearchResponse{
		Conclusion: conclusion,
		Answer:     canonicalResearchAnswer(card, critic),
		Citation: researchCitation{
			ExperimentID: exp.ID, RunID: run.ID, SnapshotID: card.SnapshotID,
			EvidenceHash: card.EvidenceHash, ResultHash: card.ResultHash,
			TradeIDs: tradeIDs,
		},
		Evidence: card, Critic: critic, ToolTrace: trace,
	}, exp, run, nil
}

func criticOutcomeFromRun(run *experiment.ExperimentRun) (*ai_critic.ReviewOutcome, error) {
	artifact := findEvidenceArtifact(run.Artifacts, "critic_review")
	if artifact == nil {
		return nil, fmt.Errorf("critic_review artifact is missing")
	}
	var outcome ai_critic.ReviewOutcome
	if err := json.Unmarshal(artifact.Content, &outcome); err != nil {
		return nil, fmt.Errorf("decode critic review: %w", err)
	}
	return &outcome, nil
}

func canonicalResearchAnswer(
	card *paradigms.EvidenceCard,
	critic *ai_critic.ReviewOutcome,
) string {
	reference := fmt.Sprintf(
		"[experiment_id=%s run_id=%s snapshot_id=%s evidence_hash=%s]",
		card.ExperimentID, card.RunID, card.SnapshotID, card.EvidenceHash,
	)
	if !card.Available || card.OutOfSample == nil {
		return "真实样本外证据不足，拒绝判断该范式有效。" + reference
	}
	sample := card.OutOfSample
	metrics := fmt.Sprintf("样本外完成交易 %d 笔", sample.TradesCount)
	if sample.TotalReturn != nil {
		metrics += fmt.Sprintf("，净收益率 %.4f", *sample.TotalReturn)
	}
	if sample.MaxDrawdown != nil {
		metrics += fmt.Sprintf("，最大回撤 %.4f", *sample.MaxDrawdown)
	}
	if sample.WinRate != nil {
		metrics += fmt.Sprintf("，胜率 %.4f", *sample.WinRate)
	}
	if !card.PromotionEligible || !critic.Passed() {
		return metrics + "。critic 或晋级门未通过，当前不能称为已验证获利范式。" + reference
	}
	return metrics + "。真实证据及 critic 门均通过。" + reference
}
