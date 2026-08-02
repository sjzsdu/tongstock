package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"net/http"
	"strconv"
	"time"
)

type readySnapshotLister interface {
	ListMarketSnapshots(string, string, string) ([]*marketsnapshot.MarketSnapshot, error)
}

func (s *Server) registerAutomationRoutes(api *gin.RouterGroup) {
	api.POST("/automation/run", s.handleAutomationRun)
	api.GET("/automation/jobs", s.handleAutomationJobs)
	api.GET("/automation/outbox", s.handleAutomationOutbox)
}
func (s *Server) handleAutomationRun(c *gin.Context) {
	if s.automationEngine == nil {
		WriteError(c, 503, "automation_unavailable", "自动任务服务不可用")
		return
	}
	var req struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SnapshotID == "" {
		WriteError(c, 400, "invalid_request", "snapshot_id 必填")
		return
	}
	j, err := s.automationEngine.Run(c.Request.Context(), req.SnapshotID)
	if err != nil {
		WriteError(c, 422, "automation_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, j)
}
func (s *Server) handleAutomationJobs(c *gin.Context) {
	if s.automationRuns == nil {
		WriteError(c, 503, "automation_unavailable", "自动任务服务不可用")
		return
	}
	n, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	xs, err := s.automationRuns.ListJobs(c.Request.Context(), n)
	if err != nil {
		WriteError(c, 500, "automation_read_failed", err.Error())
		return
	}
	c.JSON(200, gin.H{"items": xs, "total": len(xs)})
}
func (s *Server) handleAutomationOutbox(c *gin.Context) {
	if s.automationRuns == nil {
		WriteError(c, 503, "automation_unavailable", "自动任务服务不可用")
		return
	}
	xs, err := s.automationRuns.ListEvents(c.Request.Context(), c.Query("status"), 100)
	if err != nil {
		WriteError(c, 500, "automation_read_failed", err.Error())
		return
	}
	c.JSON(200, gin.H{"items": xs, "total": len(xs)})
}
func (s *Server) StartAutomationScheduler(ctx context.Context, snapshots readySnapshotLister) {
	if s.automationEngine == nil || snapshots == nil {
		return
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		run := func() {
			xs, err := snapshots.ListMarketSnapshots("", "", "ready")
			if err != nil {
				return
			}
			for _, x := range xs {
				if x.Frozen {
					_, _ = s.automationEngine.Run(ctx, x.ID)
					return
				}
			}
		}
		run()
		ticker := time.NewTicker(5 * time.Minute)
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
