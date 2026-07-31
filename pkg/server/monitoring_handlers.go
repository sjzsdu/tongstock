package server

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/monitoring"
)

// ============================================================================
// 监控 API 处理函数
// ============================================================================

// monitoringRunRequest 监控运行请求
type monitoringRunRequest struct {
	Source          string                    `json:"source"`
	BaselineReturns []float64                 `json:"baseline_returns"`
	ForwardReturns  []float64                 `json:"forward_returns"`
	ForwardDates    []time.Time               `json:"forward_dates"`
	Positions       []monitoring.PositionItem `json:"positions"`
}

// monitoringReportResponse 监控报告响应
type monitoringReportResponse struct {
	Report monitoring.MonitorReport `json:"report"`
}

// handleMonitoringRun 执行监控分析
// POST /api/monitoring/run
func (s *Server) handleMonitoringRun(c *gin.Context) {
	var req monitoringRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := validateMonitoringRunRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := monitoring.MonitoringInput{
		BaselineReturns: req.BaselineReturns,
		ForwardReturns:  req.ForwardReturns,
		ForwardDates:    req.ForwardDates,
		Positions:       req.Positions,
	}

	s.monitoringMu.Lock()
	defer s.monitoringMu.Unlock()
	config := s.monitoringEngine.Config
	config.Source = strings.TrimSpace(req.Source)
	s.monitoringEngine.Config = config
	report := s.monitoringEngine.RunMonitoring(input)
	report.Period.StartDate = req.ForwardDates[0]
	report.Period.EndDate = req.ForwardDates[len(req.ForwardDates)-1]
	report.Period.WindowDays = int(report.Period.EndDate.Sub(report.Period.StartDate).Hours()/24) + 1
	s.monitoringReport = &report

	c.JSON(http.StatusOK, monitoringReportResponse{Report: report})
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
		m.POST("/run", s.handleMonitoringRun)
		m.GET("/report", s.handleMonitoringReport)
		m.GET("/alerts", s.handleMonitoringAlerts)
		m.POST("/alerts/:id/ack", s.handleMonitoringAlertAck)
		m.POST("/alerts/:id/resolve", s.handleMonitoringAlertResolve)
		m.GET("/config", s.handleMonitoringConfig)
		m.GET("/health", s.handleMonitoringHealth)
	}
}

func validateMonitoringRunRequest(req monitoringRunRequest) error {
	if strings.TrimSpace(req.Source) == "" {
		return fmt.Errorf("source is required to identify the real observation source")
	}
	if len(req.BaselineReturns) == 0 {
		return fmt.Errorf("baseline_returns is required")
	}
	if len(req.ForwardReturns) == 0 {
		return fmt.Errorf("forward_returns is required")
	}
	if len(req.ForwardDates) != len(req.ForwardReturns) {
		return fmt.Errorf("forward_dates must contain one date per forward return")
	}
	for _, series := range [][]float64{req.BaselineReturns, req.ForwardReturns} {
		for _, value := range series {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("returns must contain only finite observed values")
			}
		}
	}
	for index, date := range req.ForwardDates {
		if date.IsZero() {
			return fmt.Errorf("forward_dates[%d] is required", index)
		}
		if index > 0 && date.Before(req.ForwardDates[index-1]) {
			return fmt.Errorf("forward_dates must be ordered ascending")
		}
	}
	return nil
}
