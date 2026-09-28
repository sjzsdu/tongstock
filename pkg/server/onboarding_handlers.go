package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/onboarding"
)

func (s *Server) registerOnboardingRoutes(api *gin.RouterGroup) {
	api.POST("/onboarding/run", s.handleOnboardingRun)
}

// handleOnboardingRun 一键走通：同步/检查行情 → 冻结快照 → 物化特征 → 运行选股。
// 返回每一步的真实状态与最终候选数，供首次引导使用。
func (s *Server) handleOnboardingRun(c *gin.Context) {
	if s.onboardingService == nil {
		WriteError(c, http.StatusServiceUnavailable, "onboarding_unavailable", "首次引导服务不可用")
		return
	}
	var req struct {
		Date              string  `json:"date"`
		Universe          string  `json:"universe"`
		CoverageThreshold float64 `json:"coverage_threshold"`
		MaxGappedCodes    int     `json:"max_gapped_codes"`
		Sync              bool    `json:"sync"`
		Force             bool    `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "请求体格式错误: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.onboardingService.Run(ctx, onboarding.RunOptions{
		Date:              req.Date,
		Universe:          req.Universe,
		CoverageThreshold: req.CoverageThreshold,
		MaxGappedCodes:    req.MaxGappedCodes,
		Sync:              req.Sync,
		Force:             req.Force,
	})
	if err != nil {
		WriteError(c, http.StatusInternalServerError, "onboarding_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
