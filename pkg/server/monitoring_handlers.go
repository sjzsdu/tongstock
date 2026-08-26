package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/monitoring"
)

// ============================================================================
// 监控 API 处理函数
// ============================================================================

// monitoringReportResponse 监控报告响应
type monitoringReportResponse struct {
	Report monitoring.MonitorReport `json:"report"`
}

// handleMonitoringReport 获取最近监控报告
// GET /api/monitoring/report
func (s *Server) handleMonitoringReport(c *gin.Context) {
	s.monitoringMu.RLock()
	defer s.monitoringMu.RUnlock()
	if s.monitoringReport == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"available": false,
			"error":     "尚无基于真实观测输入生成的监控报告",
		})
		return
	}
	c.JSON(http.StatusOK, monitoringReportResponse{Report: *s.monitoringReport})
}

// handleMonitoringAlerts 获取预警列表
// GET /api/monitoring/alerts
func (s *Server) handleMonitoringAlerts(c *gin.Context) {
	s.monitoringMu.RLock()
	defer s.monitoringMu.RUnlock()
	active := s.monitoringEngine.AlertEngine.GetActiveAlerts()
	summary := s.monitoringEngine.AlertEngine.GetAlertSummary()

	c.JSON(http.StatusOK, gin.H{
		"alerts":  active,
		"summary": summary,
	})
}

// handleMonitoringAlertAck 确认预警
// POST /api/monitoring/alerts/:id/ack
func (s *Server) handleMonitoringAlertAck(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alert id is required"})
		return
	}

	user := c.Query("user")
	if user == "" {
		user = "system"
	}

	s.monitoringMu.Lock()
	defer s.monitoringMu.Unlock()
	err := s.monitoringEngine.AlertEngine.AcknowledgeAlert(alertID, user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "acknowledged", "id": alertID})
}

// handleMonitoringAlertResolve 解决预警
// POST /api/monitoring/alerts/:id/resolve
func (s *Server) handleMonitoringAlertResolve(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alert id is required"})
		return
	}

	s.monitoringMu.Lock()
	defer s.monitoringMu.Unlock()
	err := s.monitoringEngine.AlertEngine.ResolveAlert(alertID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved", "id": alertID})
}

// handleMonitoringConfig 获取/更新监控配置
// GET /api/monitoring/config
func (s *Server) handleMonitoringConfig(c *gin.Context) {
	s.monitoringMu.RLock()
	defer s.monitoringMu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"config": s.monitoringEngine.Config})
}

// handleMonitoringHealth 健康检查
// GET /api/monitoring/health
func (s *Server) handleMonitoringHealth(c *gin.Context) {
	s.monitoringMu.RLock()
	defer s.monitoringMu.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"report_available": s.monitoringReport != nil,
		"engine_source":    s.monitoringEngine.Config.Source,
		"alert_summary":    s.monitoringEngine.AlertEngine.GetAlertSummary(),
	})
}

// registerMonitoringRoutes 注册监控路由
func (s *Server) registerMonitoringRoutes(api *gin.RouterGroup) {
	m := api.Group("/monitoring")
	{
		m.GET("/report", s.handleMonitoringReport)
		m.GET("/alerts", s.handleMonitoringAlerts)
		m.POST("/alerts/:id/ack", s.handleMonitoringAlertAck)
		m.POST("/alerts/:id/resolve", s.handleMonitoringAlertResolve)
		m.GET("/config", s.handleMonitoringConfig)
		m.GET("/health", s.handleMonitoringHealth)
	}
}
