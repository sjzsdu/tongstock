package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigmspromote"
)

// 范式→方法晋级（阶段 A P0）：
//   - POST /api/paradigm/:id/promote           异步启动晋级（编译→机器验证→注册→回写）
//   - GET  /api/paradigm/:id/promotion/status  查询晋级进度与结果
//   - GET  /api/paradigm/:id/evidence          最新范式实验证据卡（晋级体检输入）
//
// 红线：晋级结果完全由机器证据决定；本层只做异步调度与状态透出。

// paradigmPromotionTimeout 是单次晋级的防泄漏护栏。
// 真实库多票快照的全池样本外回测可能要几十分钟，异步在后台跑，HTTP 立即返回。
const paradigmPromotionTimeout = 30 * time.Minute

// paradigmPromotionRun 是单个范式的晋级运行状态。
type paradigmPromotionRun struct {
	Status     string                    `json:"status"` // running / done / failed
	StartedAt  time.Time                 `json:"started_at"`
	FinishedAt time.Time                 `json:"finished_at,omitempty"`
	Error      string                    `json:"error,omitempty"`
	Outcome    *paradigmspromote.Outcome `json:"outcome,omitempty"`
}

type paradigmPromoteRequest struct {
	SnapshotID string `json:"snapshot_id,omitempty"`
	MaxCodes   int    `json:"max_codes,omitempty"`
}

// SetupParadigmPromoteRoutes 把晋级端点挂到既有 /api/paradigm 路由组，
// 不改动任何既有端点契约。
func (s *Server) SetupParadigmPromoteRoutes(p *gin.RouterGroup) {
	p.POST("/:id/promote", s.handleParadigmPromote)
	p.GET("/:id/promotion/status", s.handleParadigmPromotionStatus)
	p.GET("/:id/evidence", s.handleParadigmEvidence)
}

func (s *Server) handleParadigmPromote(c *gin.Context) {
	if s.paradigmPromote == nil {
		WriteError(c, http.StatusServiceUnavailable, "paradigm_promote_unavailable", "范式晋级服务不可用（未装配）")
		return
	}
	if s.paradigmStore == nil {
		WriteError(c, http.StatusServiceUnavailable, "paradigm_store_unavailable", "范式库未初始化")
		return
	}
	id := c.Param("id")
	p, err := s.paradigmStore.Get(id)
	if err != nil {
		WriteError(c, http.StatusNotFound, "paradigm_not_found", err.Error())
		return
	}
	if p.Side != "buy" {
		WriteError(c, http.StatusUnprocessableEntity, "paradigm_not_promotable", "side=sell 的范式当前不可晋级为做多方法")
		return
	}

	var req paradigmPromoteRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}

	// 单飞互斥：同范式晋级已在跑直接 409。
	s.promotionMu.Lock()
	if run, ok := s.promotionRuns[id]; ok && run.Status == "running" {
		s.promotionMu.Unlock()
		msg := "该范式的晋级已在进行中"
		if !run.StartedAt.IsZero() {
			msg += "（已运行 " + time.Since(run.StartedAt).Round(time.Second).String() + "）"
		}
		WriteError(c, http.StatusConflict, "paradigm_promotion_busy", msg)
		return
	}
	s.promotionRuns[id] = &paradigmPromotionRun{Status: "running", StartedAt: time.Now().UTC()}
	s.promotionMu.Unlock()

	s.startParadigmPromotion(id, req)
	c.JSON(http.StatusOK, gin.H{
		"started":     true,
		"paradigm_id": id,
		"status_url":  "/api/paradigm/" + id + "/promotion/status",
	})
}

// startParadigmPromotion 在后台 goroutine 执行晋级。
// ctx 用 Background 而非请求上下文：handler 返回后 Request.Context 会被取消。
func (s *Server) startParadigmPromotion(id string, req paradigmPromoteRequest) {
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), paradigmPromotionTimeout)
		defer cancel()
		outcome, err := s.paradigmPromote.Promote(ctx, id, paradigmspromote.Options{
			SnapshotID: req.SnapshotID, MaxCodes: req.MaxCodes,
		})
		s.promotionMu.Lock()
		defer s.promotionMu.Unlock()
		run := s.promotionRuns[id]
		if run == nil {
			run = &paradigmPromotionRun{}
			s.promotionRuns[id] = run
		}
		run.FinishedAt = time.Now().UTC()
		if err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			return
		}
		run.Status = "done"
		run.Outcome = outcome
	}()
}

func (s *Server) handleParadigmPromotionStatus(c *gin.Context) {
	if s.paradigmPromote == nil {
		WriteError(c, http.StatusServiceUnavailable, "paradigm_promote_unavailable", "范式晋级服务不可用（未装配）")
		return
	}
	id := c.Param("id")
	s.promotionMu.RLock()
	defer s.promotionMu.RUnlock()
	run, ok := s.promotionRuns[id]
	if !ok {
		c.JSON(http.StatusOK, gin.H{"paradigm_id": id, "status": "idle"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"paradigm_id": id, "run": run})
}

// handleParadigmEvidence 返回范式最新持久化实验的证据卡（晋级体检输入）。
func (s *Server) handleParadigmEvidence(c *gin.Context) {
	if s.paradigmStore == nil || s.experimentRegistry == nil {
		WriteError(c, http.StatusServiceUnavailable, "paradigm_evidence_unavailable", "范式证据服务不可用")
		return
	}
	id := c.Param("id")
	experimentID := c.Query("experiment_id")
	card, err := s.latestParadigmExperimentEvidence(id, experimentID)
	if err != nil {
		WriteError(c, http.StatusNotFound, "paradigm_evidence_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, card)
}
