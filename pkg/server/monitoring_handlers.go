package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/monitoring"
)

// ============================================================================
// 监控 API 处理函数
// ============================================================================

// monitoringReportResponse 监控报告响应
type monitoringReportResponse struct {
	Report monitoring.MonitorReport `json:"report"`
	Input  MonitoringInputStatus    `json:"input"`
}

// monitoringReportUnavailable 构造“没有真实观测”的失败响应。
// 响应体带上 input 诊断, 让前端说清楚缺什么, 而不是静默无反应。
func monitoringReportUnavailable(status MonitoringInputStatus) (int, gin.H) {
	return http.StatusNotFound, gin.H{
		"available": false,
		"error":     "尚无基于真实观测输入的监控报告",
		"input":     status,
	}
}

// ensureMonitoringReport 返回缓存内的监控报告, 过期或缺失时用真实观测重算。
// 输入构造可能涉及行情 IO, 只在 monitoringBuildMu 下串行执行;
// 重算完成后才短暂持有 monitoringMu 写回报告。
func (s *Server) ensureMonitoringReport(c *gin.Context, force bool) (*monitoring.MonitorReport, MonitoringInputStatus, bool) {
	if !force {
		if report, status, ok := s.cachedMonitoringReport(); ok {
			return report, status, true
		}
	}

	s.monitoringBuildMu.Lock()
	defer s.monitoringBuildMu.Unlock()

	// 拿到串行锁后可能已被并发请求算过
	if !force {
		if report, status, ok := s.cachedMonitoringReport(); ok {
			return report, status, true
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), monitoringBuildTimeout)
	defer cancel()

	input, status, err := s.buildMonitoringInput(ctx)
	if err != nil {
		s.monitoringMu.Lock()
		s.monitoringInputStatus = status
		s.monitoringMu.Unlock()
		return nil, status, false
	}

	s.monitoringMu.Lock()
	defer s.monitoringMu.Unlock()
	s.monitoringEngine.AlertEngine.PruneSuppressed()
	generated := s.monitoringEngine.RunMonitoring(input)
	generated.Source = status.Source
	generated.Period = monitoring.MonitoringPeriod{
		StartDate:  status.ForwardStart,
		EndDate:    status.End,
		WindowDays: status.ForwardCount,
	}
	report := &generated
	s.monitoringReport = report
	s.monitoringInputStatus = status
	s.monitoringGeneratedAt = time.Now()
	return report, status, true
}

func (s *Server) cachedMonitoringReport() (*monitoring.MonitorReport, MonitoringInputStatus, bool) {
	s.monitoringMu.RLock()
	defer s.monitoringMu.RUnlock()
	if s.monitoringReport == nil || time.Since(s.monitoringGeneratedAt) >= monitoringReportTTL {
		return nil, s.monitoringInputStatus, false
	}
	return s.monitoringReport, s.monitoringInputStatus, true
}

// handleMonitoringReport 获取监控报告 (必要时用真实观测重算)
// GET /api/monitoring/report
func (s *Server) handleMonitoringReport(c *gin.Context) {
	report, status, ok := s.ensureMonitoringReport(c, false)
	if !ok {
		code, body := monitoringReportUnavailable(status)
		c.JSON(code, body)
		return
	}
	c.JSON(http.StatusOK, monitoringReportResponse{Report: *report, Input: status})
}

// handleMonitoringReportRefresh 按当前真实观测强制重算监控报告
// POST /api/monitoring/report/refresh
func (s *Server) handleMonitoringReportRefresh(c *gin.Context) {
	report, status, ok := s.ensureMonitoringReport(c, true)
	if !ok {
		code, body := monitoringReportUnavailable(status)
		c.JSON(code, body)
		return
	}
	c.JSON(http.StatusOK, monitoringReportResponse{Report: *report, Input: status})
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
		code, message := statusError(http.StatusBadRequest)
		WriteError(c, http.StatusBadRequest, code, message)
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
		code, message := statusError(http.StatusNotFound)
		WriteError(c, http.StatusNotFound, code, message)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "acknowledged", "id": alertID})
}

// handleMonitoringAlertResolve 解决预警
// POST /api/monitoring/alerts/:id/resolve
func (s *Server) handleMonitoringAlertResolve(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		code, message := statusError(http.StatusBadRequest)
		WriteError(c, http.StatusBadRequest, code, message)
		return
	}

	s.monitoringMu.Lock()
	defer s.monitoringMu.Unlock()
	err := s.monitoringEngine.AlertEngine.ResolveAlert(alertID)
	if err != nil {
		code, message := statusError(http.StatusNotFound)
		WriteError(c, http.StatusNotFound, code, message)
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
		"input":            s.monitoringInputStatus,
	})
}

// registerMonitoringRoutes 注册监控路由
func (s *Server) registerMonitoringRoutes(api *gin.RouterGroup) {
	m := api.Group("/monitoring")
	{
		m.GET("/report", s.handleMonitoringReport)
		m.POST("/report/refresh", s.handleMonitoringReportRefresh)
		m.GET("/alerts", s.handleMonitoringAlerts)
		m.POST("/alerts/:id/ack", s.handleMonitoringAlertAck)
		m.POST("/alerts/:id/resolve", s.handleMonitoringAlertResolve)
		m.GET("/config", s.handleMonitoringConfig)
		m.GET("/health", s.handleMonitoringHealth)
	}
}
