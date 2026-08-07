package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/app/discoveryapp"
)

func (s *Server) registerDiscoveryRoutes(api *gin.RouterGroup) {
	api.POST("/discover/run", s.handleDiscoverRun)
	api.GET("/discover/traces", s.handleDiscoverTraces)
}

func (s *Server) handleDiscoverRun(c *gin.Context) {
	if s.discoverRunner == nil {
		WriteError(c, 503, "discover_unavailable", "规律发现服务不可用")
		return
	}
	var req struct {
		PoolID       string   `json:"pool_id"`
		Codes        []string `json:"codes"`
		Question     string   `json:"question"`
		HoldDays     int      `json:"hold_days"`
		SearchBudget int      `json:"search_budget"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, 400, "invalid_request", "请求体格式错误")
		return
	}
	if req.PoolID == "" && len(req.Codes) == 0 {
		WriteError(c, 400, "invalid_request", "pool_id 或 codes 必填")
		return
	}
	result, err := s.discoverRunner.Run(c.Request.Context(), discoveryapp.RunRequest{
		PoolID: req.PoolID, Codes: req.Codes, Question: req.Question,
		HoldDays: req.HoldDays, SearchBudget: req.SearchBudget,
	})
	if err != nil {
		WriteError(c, 422, "discover_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"research_id":    result.ResearchID,
		"snapshot_id":    result.SnapshotID,
		"conclusion":     result.Conclusion,
		"candidate_count": len(result.Candidates),
		"candidates":     result.Candidates,
		"rejected_count": len(result.Rejected),
	})
}

func (s *Server) handleDiscoverTraces(c *gin.Context) {
	if s.discoverTraces == nil {
		WriteError(c, 503, "discover_unavailable", "规律发现服务不可用")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	summaries, err := s.discoverTraces.List(c.Request.Context(), limit)
	if err != nil {
		WriteError(c, 500, "discover_read_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"traces": summaries, "total": len(summaries)})
}

