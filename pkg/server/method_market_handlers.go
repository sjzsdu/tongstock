package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/methodautomation"
	"github.com/sjzsdu/tongstock/internal/methodhealth"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/selection"
)

// registerMethodMarketRoutes 挂载「方法市场 + 自动研究闭环」路由。
// 只新增端点，不改动既有 GET /methods* 契约。
func (s *Server) registerMethodMarketRoutes(api *gin.RouterGroup) {
	api.POST("/methods/research/run", s.handleMethodResearchRun)
	api.GET("/methods/research/last", s.handleMethodResearchLast)
	api.GET("/methods/reject-stats", s.handleMethodRejectStats)
	api.GET("/methods/forward-health", s.handleMethodForwardHealth)
	api.POST("/methods/:id/feedback", s.handleMethodFeedback)
	api.POST("/selections/run", s.handleSelectionRunCreate)
}

// ---------------------------------------------------------------------------
// 自动方法研究（阶段 A 供给）
// ---------------------------------------------------------------------------

// SetMethodAutomation 注册自动方法研究编排器。
func (s *Server) SetMethodAutomation(orch *methodautomation.Orchestrator) {
	s.methodAutomation = orch
}

func (s *Server) handleMethodResearchRun(c *gin.Context) {
	if s.methodAutomation == nil {
		WriteError(c, http.StatusServiceUnavailable, "method_research_unavailable", "自动方法研究服务不可用")
		return
	}
	var req struct {
		SnapshotID   string   `json:"snapshot_id"`
		Codes        []string `json:"codes"`
		HoldDays     []int    `json:"hold_days"`
		SearchBudget int      `json:"search_budget"`
		MaxCodes     int      `json:"max_codes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Minute)
	defer cancel()
	result, err := s.methodAutomation.Run(ctx, methodautomation.Request{
		SnapshotID: req.SnapshotID, Codes: req.Codes, HoldDays: req.HoldDays,
		SearchBudget: req.SearchBudget, MaxCodes: req.MaxCodes,
	})
	s.recordResearchResult(result, err)
	if err != nil {
		if errors.Is(err, methodautomation.ErrBusy) {
			WriteError(c, http.StatusConflict, "method_research_busy", err.Error())
			return
		}
		WriteError(c, http.StatusUnprocessableEntity, "method_research_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleMethodResearchLast(c *gin.Context) {
	s.researchMu.RLock()
	defer s.researchMu.RUnlock()
	if s.researchLast == nil {
		WriteError(c, http.StatusNotFound, "no_research_run", "还没有自动研究记录，先触发一次研究")
		return
	}
	c.JSON(http.StatusOK, s.researchLast)
}

func (s *Server) recordResearchResult(result *methodautomation.BatchResult, err error) {
	s.researchMu.Lock()
	defer s.researchMu.Unlock()
	if result != nil {
		s.researchLast = result
		return
	}
	if err != nil && s.researchLast == nil {
		s.researchLastError = err.Error()
	}
}

// StartMethodResearchScheduler 周期触发一轮自动方法研究（后台 goroutine）。
func (s *Server) StartMethodResearchScheduler(ctx context.Context, interval time.Duration) {
	if s.methodAutomation == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
			result, err := s.methodAutomation.Run(runCtx, methodautomation.Request{})
			s.recordResearchResult(result, err)
			if err != nil && !errors.Is(err, methodautomation.ErrBusy) && ctx.Err() == nil {
				log.Printf("method research: scheduled batch failed: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// 拒绝原因统计（reject 分布，可反哺研究）
// ---------------------------------------------------------------------------

func (s *Server) handleMethodRejectStats(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	ctx := c.Request.Context()
	stats, err := s.methodRegistry.RejectStats(ctx)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	rejected, err := s.methodRegistry.Cards(ctx, methodregistry.Query{Status: []methodregistry.Status{methodregistry.StatusRejected}, Limit: 100})
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_registry_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": stats, "total": len(stats), "rejected_methods": len(rejected)})
}

// ---------------------------------------------------------------------------
// 前向健康（阶段 D 监控）
// ---------------------------------------------------------------------------

// SetMethodHealth 注册前向健康评估器（带策略回写，供定时调度使用）。
func (s *Server) SetMethodHealth(e *methodhealth.Evaluator) {
	s.methodHealth = e
}

func (s *Server) handleMethodForwardHealth(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	if s.ledger == nil {
		WriteError(c, http.StatusServiceUnavailable, "ledger_unavailable", "前向账本不可用")
		return
	}
	// 只读评估：不在此处写回策略状态（状态转移由定时调度器执行）。
	evaluator, err := methodhealth.New(s.methodRegistry, s.ledger, false, nil)
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_health_failed", err.Error())
		return
	}
	items, err := evaluator.Evaluate(c.Request.Context())
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_health_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// StartMethodHealthScheduler 周期评估前向健康并回写策略状态
// （observing/degraded/retired 转移由 Policy.Health 决定）。
func (s *Server) StartMethodHealthScheduler(ctx context.Context, interval time.Duration) {
	if s.methodHealth == nil {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			if _, err := s.methodHealth.Evaluate(runCtx); err != nil && runCtx.Err() == nil {
				log.Printf("method health: scheduled evaluation failed: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// 用户反馈（好用 / 不好用 → 审计痕迹 → 反哺研究）
// ---------------------------------------------------------------------------

func (s *Server) handleMethodFeedback(c *gin.Context) {
	if !s.requireMethodRegistry(c) {
		return
	}
	var req struct {
		Useful  bool   `json:"useful"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}
	text := "feedback:useful=false"
	if req.Useful {
		text = "feedback:useful=true"
	}
	if comment := strings.TrimSpace(req.Comment); comment != "" {
		text += "; " + comment
	}
	m, err := s.methodRegistry.Annotate(c.Request.Context(), c.Param("id"), text, "user-feedback")
	if errors.Is(err, methodregistry.ErrNotFound) {
		WriteError(c, http.StatusNotFound, "method_not_found", "投资方法不存在")
		return
	}
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "method_feedback_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": m.ID, "status": m.Status,
		"annotations": m.Annotations, "updated_at": m.UpdatedAt,
	})
}

// ---------------------------------------------------------------------------
// 按方法选股（阶段 C 闭环）
// ---------------------------------------------------------------------------

// SetSelectionEngine 注册选股引擎与冻结快照列表器，支撑手动「用选中方法筛选」。
func (s *Server) SetSelectionEngine(engine *selection.Engine, snapshots readySnapshotLister) {
	s.selectionEngine = engine
	s.selectionSnapshots = snapshots
}

func (s *Server) handleSelectionRunCreate(c *gin.Context) {
	if s.selectionEngine == nil {
		WriteError(c, http.StatusServiceUnavailable, "selection_unavailable", "选股引擎不可用")
		return
	}
	var req struct {
		MarketSnapshotID  string   `json:"market_snapshot_id"`
		FeatureSnapshotID string   `json:"feature_snapshot_id"`
		MethodIDs         []string `json:"method_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}
	if len(req.MethodIDs) == 0 {
		WriteError(c, http.StatusBadRequest, "invalid_request", "method_ids 不能为空：本端点用于按选中方法筛选，全量选股走每日自动任务")
		return
	}
	if strings.TrimSpace(req.MarketSnapshotID) == "" {
		if s.selectionSnapshots == nil {
			WriteError(c, http.StatusBadRequest, "invalid_request", "market_snapshot_id 必填")
			return
		}
		id, err := latestFrozenSnapshot(s.selectionSnapshots)
		if err != nil {
			WriteError(c, http.StatusConflict, "no_ready_snapshot", err.Error())
			return
		}
		req.MarketSnapshotID = id
	}
	run, err := s.selectionEngine.Run(c.Request.Context(), selection.Request{
		MarketSnapshotID: req.MarketSnapshotID, FeatureSnapshotID: req.FeatureSnapshotID,
		MethodIDs: req.MethodIDs,
	})
	if err != nil {
		WriteError(c, http.StatusUnprocessableEntity, "selection_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

func latestFrozenSnapshot(snapshots readySnapshotLister) (string, error) {
	xs, err := snapshots.ListMarketSnapshots("", "", "ready")
	if err != nil {
		return "", err
	}
	for _, x := range xs {
		if x != nil && x.Frozen {
			return x.ID, nil
		}
	}
	return "", errors.New("没有已冻结就绪的市场快照，请先完成数据同步并冻结快照")
}
