package server

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"net/http"
	"strconv"
)

func (s *Server) registerPositionDecisionRoutes(api *gin.RouterGroup) {
	api.POST("/position-decisions/run", s.handlePositionDecisionRun)
	api.GET("/position-decisions/today", s.handlePositionDecisionToday)
	api.GET("/position-decisions/runs/:id", s.handlePositionDecisionGet)
}
func (s *Server) handlePositionDecisionRun(c *gin.Context) {
	if s.positionEngine == nil {
		WriteError(c, 503, "position_decision_unavailable", "持仓判断服务不可用")
		return
	}
	var req struct {
		MarketSnapshotID  string `json:"market_snapshot_id"`
		FeatureSnapshotID string `json:"feature_snapshot_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, 400, "invalid_request", err.Error())
		return
	}
	x, err := s.positionEngine.Run(c.Request.Context(), positiondecision.Request{MarketSnapshotID: req.MarketSnapshotID, FeatureSnapshotID: req.FeatureSnapshotID})
	if err != nil {
		WriteError(c, 422, "position_decision_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, x)
}
func (s *Server) handlePositionDecisionToday(c *gin.Context) {
	if s.positionRuns == nil {
		WriteError(c, 503, "position_decision_unavailable", "持仓判断服务不可用")
		return
	}
	limit, _ := strconv.Atoi("1")
	xs, err := s.positionRuns.List(c.Request.Context(), "", limit)
	if err != nil {
		WriteError(c, 500, "position_decision_read_failed", err.Error())
		return
	}
	if len(xs) == 0 {
		WriteError(c, 404, "position_decision_not_found", "尚无持仓判断")
		return
	}
	c.JSON(200, xs[0])
}
func (s *Server) handlePositionDecisionGet(c *gin.Context) {
	if s.positionRuns == nil {
		WriteError(c, 503, "position_decision_unavailable", "持仓判断服务不可用")
		return
	}
	x, err := s.positionRuns.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, positiondecision.ErrNotFound) {
		WriteError(c, 404, "position_decision_not_found", "持仓判断不存在")
		return
	}
	if err != nil {
		WriteError(c, 500, "position_decision_read_failed", err.Error())
		return
	}
	c.JSON(200, x)
}
