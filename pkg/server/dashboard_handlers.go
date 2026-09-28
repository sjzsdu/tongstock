package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerDashboardRoutes(api *gin.RouterGroup) {
	api.GET("/dashboard/today", s.handleDashboardToday)
}

// handleDashboardToday 返回首屏「今日状态」：数据新鲜度、方法库、候选、持仓、
// 空榜单原因与下一步、工作日志、系统健康信号。
func (s *Server) handleDashboardToday(c *gin.Context) {
	if s.dashboardService == nil {
		WriteError(c, http.StatusServiceUnavailable, "dashboard_unavailable", "今日状态服务不可用")
		return
	}
	today, err := s.dashboardService.Today(c.Request.Context())
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "dashboard_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, today)
}
