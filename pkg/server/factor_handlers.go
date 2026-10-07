package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/factorlab"
)

// factorResearchRunTimeout 是异步因子研究批次的防泄漏护栏（实测 300 只 × 9 因子
// 约 16 秒；全市场放开后分钟级内可控）。ctx 用 Background 而非请求上下文：
// handler 返回后 Request.Context 会被 net/http 取消，批次不能跟着被杀。
const factorResearchRunTimeout = 10 * time.Minute

// SetFactorLab 注册横截面因子研究服务。
func (s *Server) SetFactorLab(svc *factorlab.Service) { s.factorLab = svc }

// SetFactorPicks 注册因子通道 TopN 产出仓库（SQLite）。
func (s *Server) SetFactorPicks(store factorlab.PickStore) { s.factorPicks = store }

// registerFactorRoutes 挂载因子研究路由。
// 与自动方法研究一样异步启动：全市场快照上一轮可达分钟级，同步等待必然超时。
// 允许并发触发（compute 只读无互斥）；write_to_selection=true 时额外把显著因子
// TopN 落成因子通道持久化产出（pick-<截面日期>，幂等更新），供复核与跟 Laur。
func (s *Server) registerFactorRoutes(api *gin.RouterGroup) {
	api.POST("/factors/research/run", s.handleFactorResearchRun)
	api.GET("/factors/research/last", s.handleFactorResearchLast)
	api.GET("/factors/picks/last", s.handleFactorPicksLast)
}

func (s *Server) handleFactorResearchRun(c *gin.Context) {
	if s.factorLab == nil {
		WriteError(c, http.StatusServiceUnavailable, "factor_research_unavailable", "因子研究服务不可用")
		return
	}
	var req struct {
		SnapshotID       string `json:"snapshot_id"`
		HorizonDays      int    `json:"horizon_days"`
		TopK             int    `json:"top_k"`
		MaxCodes         int    `json:"max_codes"`
		WriteToSelection bool   `json:"write_to_selection"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}
	// 异步启动（仿 method research）：HTTP 立即返回，结果经 /last 获取，
	// 前端用轮询等待。无单飞互斥：并发触发只是重复计算，落库幂等。
	s.startFactorResearchRun(factorlab.Options{
		SnapshotID: req.SnapshotID, HorizonDays: req.HorizonDays,
		TopK: req.TopK, MaxCodes: req.MaxCodes,
	}, req.WriteToSelection)
	c.JSON(http.StatusOK, gin.H{
		"started":    true,
		"status_url": "/api/factors/research/last",
		"result_url": "/api/factors/research/last",
	})
}

// startFactorResearchRun 在后台 goroutine 执行一轮因子研究，记录结果并按需落因子通道。
func (s *Server) startFactorResearchRun(opts factorlab.Options, writeToSelection bool) {
	// 先置 running 态：last 端点立即从 no_completed_run 变为 running。
	s.factorMu.Lock()
	s.factorRunning = true
	s.factorMu.Unlock()
	s.backgroundWG.Add(1)
	startedAt := time.Now()
	go func() {
		defer s.backgroundWG.Done()
		defer func() {
			s.factorMu.Lock()
			s.factorRunning = false
			s.factorMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), factorResearchRunTimeout)
		defer cancel()
		result, err := s.factorLab.Run(ctx, opts)
		s.recordFactorResult(result, err)
		if err != nil && ctx.Err() == nil {
			log.Printf("factor research: batch failed after %s: %v", time.Since(startedAt).Round(time.Second), err)
		}
		if err == nil && writeToSelection && s.factorPicks != nil {
			s.saveFactorPicks(result)
		}
	}()
}

// recordFactorResult 持久化最近一轮因子研究结果（失败时保留旧结果并记录错误）。
func (s *Server) recordFactorResult(result *factorlab.RunResult, err error) {
	s.factorMu.Lock()
	defer s.factorMu.Unlock()
	if err != nil {
		s.factorLastError = err.Error()
		if result == nil {
			return // 失败且无结果：保留旧结果，错误经 last 端点透出
		}
	} else {
		s.factorLastError = ""
	}
	s.factorLast = result
}

// saveFactorPicks 把显著因子 TopN 写成因子通道持久化产出。幂等按 run_id
// （pick-<截面日期>）更新；last_date 属于历史截面的旧行不会干扰新截面。
// 注意：无显著因子不是研究失败（Run 返回空结果+诚实文案），只是这一轮
// 没有可落库的名单——如实记 info 级日志，绝不用错误措辞伪装成故障。
func (s *Server) saveFactorPicks(result *factorlab.RunResult) {
	pickRun, err := result.ToPickRun(0)
	if err != nil {
		log.Printf("factor research: 本轮无可落库名单，跳过持久化（%v）", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.factorPicks.SavePickRun(ctx, pickRun); err != nil {
		log.Printf("factor research: save pick run failed: %v", err)
	}
}

func (s *Server) handleFactorResearchLast(c *gin.Context) {
	s.factorMu.RLock()
	last := s.factorLast
	running := s.factorRunning
	lastError := s.factorLastError
	s.factorMu.RUnlock()
	resp := gin.H{"running": running}
	if lastError != "" {
		resp["last_error"] = lastError
	}
	if last != nil {
		resp["result"] = last
		resp["finished_at"] = last.FinishedAt
	} else if lastError != "" {
		resp["status"] = "failed"
	} else {
		resp["status"] = "no_completed_run"
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleFactorPicksLast(c *gin.Context) {
	if s.factorPicks == nil {
		c.JSON(http.StatusOK, gin.H{"status": "no_pick_run"})
		return
	}
	pickRun, err := s.factorPicks.GetLatestPickRun(c.Request.Context())
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "factor_picks_failed", err.Error())
		return
	}
	if pickRun == nil {
		c.JSON(http.StatusOK, gin.H{"status": "no_pick_run"})
		return
	}
	c.JSON(http.StatusOK, pickRun)
}
