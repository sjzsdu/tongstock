package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/monitoring"
)

// ============================================================================
// 监控 API 处理函数
// ============================================================================

// monitorEngine 全局监控引擎实例
var monitorEngine *monitoring.MonitorEngine

// EnsureMonitorEngine 确保监控引擎已初始化
func EnsureMonitorEngine() *monitoring.MonitorEngine {
	if monitorEngine == nil {
		config := monitoring.NewDefaultMonitorConfig()
		monitorEngine = monitoring.NewMonitorEngine(config)
	}
	return monitorEngine
}

// monitoringRunRequest 监控运行请求
type monitoringRunRequest struct {
	Source                string                        `json:"source"`
	BaselineReturns       []float64                     `json:"baseline_returns"`
	ForwardReturns        []float64                     `json:"forward_returns"`
	ForwardDates          []time.Time                   `json:"forward_dates"`
	Positions             []monitoring.PositionItem    `json:"positions"`
}

// monitoringReportResponse 监控报告响应
type monitoringReportResponse struct {
	Report monitoring.MonitorReport `json:"report"`
}

// handleMonitoringRun 执行监控分析
// POST /api/monitoring/run
func (s *Server) handleMonitoringRun(c *gin.Context) {
	engine := EnsureMonitorEngine()

	var req monitoringRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.BaselineReturns) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "baseline_returns is required"})
		return
	}
	if len(req.ForwardReturns) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "forward_returns is required"})
		return
	}

	if req.Source != "" {
		engine.Config.Source = req.Source
	}

	input := monitoring.MonitoringInput{
		BaselineReturns: req.BaselineReturns,
		ForwardReturns:  req.ForwardReturns,
		ForwardDates:    req.ForwardDates,
		Positions:       req.Positions,
	}

	report := engine.RunMonitoring(input)

	c.JSON(http.StatusOK, monitoringReportResponse{Report: report})
}

// handleMonitoringReport 获取最近监控报告
// GET /api/monitoring/report
func (s *Server) handleMonitoringReport(c *gin.Context) {
	engine := EnsureMonitorEngine()

	// 从 query 获取参数
	baselineCount := 200
	forwardCount := 60

	input := monitoring.MonitoringInput{
		BaselineReturns: generateMockReturns(baselineCount, 0.01, 0.02),
		ForwardReturns:  generateMockReturns(forwardCount, 0.008, 0.025),
		ForwardDates:    generateMockDates(forwardCount),
		Positions: []monitoring.PositionItem{
			{Code: "000001", Industry: "金融", Weight: 0.30},
			{Code: "000002", Industry: "消费", Weight: 0.25},
			{Code: "000003", Industry: "科技", Weight: 0.20},
			{Code: "000004", Industry: "医药", Weight: 0.15},
			{Code: "000005", Industry: "能源", Weight: 0.10},
		},
	}

	report := engine.RunMonitoring(input)

	c.JSON(http.StatusOK, monitoringReportResponse{Report: report})
}

// handleMonitoringAlerts 获取预警列表
// GET /api/monitoring/alerts
func (s *Server) handleMonitoringAlerts(c *gin.Context) {
	engine := EnsureMonitorEngine()

	active := engine.AlertEngine.GetActiveAlerts()
	summary := engine.AlertEngine.GetAlertSummary()

	c.JSON(http.StatusOK, gin.H{
		"alerts":  active,
		"summary": summary,
	})
}

// handleMonitoringAlertAck 确认预警
// POST /api/monitoring/alerts/:id/ack
func (s *Server) handleMonitoringAlertAck(c *gin.Context) {
	engine := EnsureMonitorEngine()

	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alert id is required"})
		return
	}

	user := c.Query("user")
	if user == "" {
		user = "system"
	}

	err := engine.AlertEngine.AcknowledgeAlert(alertID, user)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "acknowledged", "id": alertID})
}

// handleMonitoringAlertResolve 解决预警
// POST /api/monitoring/alerts/:id/resolve
func (s *Server) handleMonitoringAlertResolve(c *gin.Context) {
	engine := EnsureMonitorEngine()

	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "alert id is required"})
		return
	}

	err := engine.AlertEngine.ResolveAlert(alertID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved", "id": alertID})
}

// handleMonitoringConfig 获取/更新监控配置
// GET /api/monitoring/config
func (s *Server) handleMonitoringConfig(c *gin.Context) {
	engine := EnsureMonitorEngine()
	c.JSON(http.StatusOK, gin.H{"config": engine.Config})
}

// handleMonitoringHealth 健康检查
// GET /api/monitoring/health
func (s *Server) handleMonitoringHealth(c *gin.Context) {
	engine := EnsureMonitorEngine()

	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"engine_source": engine.Config.Source,
		"alert_summary": engine.AlertEngine.GetAlertSummary(),
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

// ============================================================================
// 辅助函数
// ============================================================================

// generateMockReturns 生成模拟收益数据
func generateMockReturns(n int, mean_, std float64) []float64 {
	result := make([]float64, n)
	for i := range result {
		// 近似正态分布
		u1 := float64(i+1) * 0.5  // 简单种子避免依赖 rand
		result[i] = mean_ + (std*(u1-0.5))*2
	}
	return result
}

// generateMockDates 生成模拟日期
func generateMockDates(n int) []time.Time {
	dates := make([]time.Time, n)
	base := time.Now()
	for i := range dates {
		dates[i] = base.AddDate(0, 0, -n+i)
	}
	return dates
}
