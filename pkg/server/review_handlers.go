package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/review"
)

// ============================================================================
// 复盘 API 处理函数
// ============================================================================

var (
	reviewGenerator    = review.NewReviewGenerator()
	failureAnalyzer    = review.NewFailureAnalyzer()
	feedbackGenerator  = review.NewFeedbackGenerator()
	reviewStore        []*review.ReviewReport
	feedbackStore      []*review.FeedbackPortfolio
)

// reviewGenerateRequest 复盘生成请求
type reviewGenerateRequest struct {
	SourceID         string    `json:"source_id" binding:"required"`
	SourceType       string    `json:"source_type" binding:"required,oneof=paradigm run system"`
	Type             string    `json:"type" binding:"required,oneof=post_mortem retrospective param_audit failure_analysis"`
	Period           string    `json:"period" binding:"required,oneof=weekly monthly quarterly"`
	PeriodStart      time.Time `json:"period_start"`
	PeriodEnd        time.Time `json:"period_end"`
	Author           string    `json:"author"`

	SignalCount      int       `json:"signal_count"`
	ExecutedCount    int       `json:"executed_count"`
	FailedCount      int       `json:"failed_count"`
	UnexecutedCount  int       `json:"unexecuted_count"`
	Returns          []float64 `json:"returns"`
	PnL              float64   `json:"pnl"`
	StatusChanges    int       `json:"status_changes"`
	ParamChanges     int       `json:"param_changes"`
	DataQualityScore float64   `json:"data_quality_score"`

	Failures         []review.FailureEvent `json:"failures,omitempty"`
	Decisions        []review.ReviewDecision `json:"decisions,omitempty"`
}

// reviewGenerateResponse 复盘生成响应
type reviewGenerateResponse struct {
	Report *review.ReviewReport `json:"report"`
}

// reviewListResponse 复盘列表响应
type reviewListResponse struct {
	Reports []*review.ReviewReport `json:"reports"`
	Total   int                    `json:"total"`
}

// reviewFailureAnalysisResponse 失败分析响应
type reviewFailureAnalysisResponse struct {
	Analysis review.FailureAnalysisResult `json:"analysis"`
	Failures []review.FailureEvent       `json:"failures"`
}

// reviewFeedbackResponse 反馈响应
type reviewFeedbackResponse struct {
	Portfolio *review.FeedbackPortfolio `json:"portfolio"`
}

// reviewFeedbackListResponse 反馈列表响应
type reviewFeedbackListResponse struct {
	Portfolios []*review.FeedbackPortfolio `json:"portfolios"`
	Total      int                         `json:"total"`
}

// reviewFeedbackUpdateRequest 反馈更新请求
type reviewFeedbackUpdateRequest struct {
	Status  string `json:"status"`
	Note    string `json:"note"`
	Actor   string `json:"actor"`
}

// ============================================================================
// 复盘处理函数
// ============================================================================

// handleReviewGenerate 生成复盘报告
// POST /api/review/generate
func (s *Server) handleReviewGenerate(c *gin.Context) {
	var req reviewGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.PeriodStart.IsZero() {
		req.PeriodStart = time.Now().AddDate(0, 0, -7)
	}
	if req.PeriodEnd.IsZero() {
		req.PeriodEnd = time.Now()
	}
	if req.Author == "" {
		req.Author = "system"
	}

	input := review.ReviewInput{
		SourceID:         req.SourceID,
		SourceType:       req.SourceType,
		Type:             review.ReviewType(req.Type),
		Period:           review.ReviewPeriod(req.Period),
		PeriodStart:      req.PeriodStart,
		PeriodEnd:        req.PeriodEnd,
		Author:           req.Author,
		SignalCount:      req.SignalCount,
		ExecutedCount:    req.ExecutedCount,
		FailedCount:      req.FailedCount,
		UnexecutedCount:  req.UnexecutedCount,
		Returns:          req.Returns,
		PnL:              req.PnL,
		StatusChanges:    req.StatusChanges,
		ParamChanges:     req.ParamChanges,
		DataQualityScore: req.DataQualityScore,
		Failures:         req.Failures,
		Decisions:        req.Decisions,
	}

	report := reviewGenerator.GenerateReview(input)
	reviewStore = append(reviewStore, report)

	c.JSON(http.StatusOK, reviewGenerateResponse{Report: report})
}

// handleReviewList 获取复盘报告列表
// GET /api/review/list
func (s *Server) handleReviewList(c *gin.Context) {
	sourceID := c.Query("source_id")
	sourceType := c.Query("source_type")
	reviewType := c.Query("type")

	var filtered []*review.ReviewReport
	for _, r := range reviewStore {
		if sourceID != "" && r.SourceID != sourceID {
			continue
		}
		if sourceType != "" && r.SourceType != sourceType {
			continue
		}
		if reviewType != "" && string(r.Type) != reviewType {
			continue
		}
		filtered = append(filtered, r)
	}

	c.JSON(http.StatusOK, reviewListResponse{
		Reports: filtered,
		Total:   len(filtered),
	})
}

// handleReviewGet 获取单个复盘报告
// GET /api/review/:id
func (s *Server) handleReviewGet(c *gin.Context) {
	id := c.Param("id")

	for _, r := range reviewStore {
		if r.ID == id {
			c.JSON(http.StatusOK, reviewGenerateResponse{Report: r})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
}

// handleReviewFailureAnalysis 失败分析
// POST /api/review/failure-analysis
func (s *Server) handleReviewFailureAnalysis(c *gin.Context) {
	var req struct {
		Failures []review.FailureEvent `json:"failures" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 自动分类
	for i := range req.Failures {
		if req.Failures[i].Category == "" {
			req.Failures[i].Category = failureAnalyzer.ClassifyFailure(req.Failures[i])
		}
	}

	analysis := failureAnalyzer.AnalyzeFailures(req.Failures)

	c.JSON(http.StatusOK, reviewFailureAnalysisResponse{
		Analysis: analysis,
		Failures: req.Failures,
	})
}

// handleReviewFailurePatterns 获取失败模式
// GET /api/review/failure-patterns
func (s *Server) handleReviewFailurePatterns(c *gin.Context) {
	// 收集所有报告中的失败
	var allFailures []review.FailureEvent
	for _, r := range reviewStore {
		allFailures = append(allFailures, r.Failures...)
	}

	analysis := failureAnalyzer.AnalyzeFailures(allFailures)

	c.JSON(http.StatusOK, gin.H{
		"patterns": analysis.Patterns,
		"total":    len(allFailures),
	})
}

// handleReviewFeedbackGenerate 从复盘报告生成反馈
// POST /api/review/feedback/generate
func (s *Server) handleReviewFeedbackGenerate(c *gin.Context) {
	var req struct {
		ReviewID   string `json:"review_id" binding:"required"`
		AutoUpdate bool   `json:"auto_update"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var targetReport *review.ReviewReport
	for _, r := range reviewStore {
		if r.ID == req.ReviewID {
			targetReport = r
			break
		}
	}

	if targetReport == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
		return
	}

	portfolio := feedbackGenerator.GenerateFromReview(targetReport)
	feedbackStore = append(feedbackStore, &portfolio)

	// 更新报告的反馈生成状态
	targetReport.FeedbackGenerated = true
	targetReport.FeedbackID = portfolio.ID

	c.JSON(http.StatusOK, reviewFeedbackResponse{Portfolio: &portfolio})
}

// handleReviewFeedbackList 获取反馈列表
// GET /api/review/feedback/list
func (s *Server) handleReviewFeedbackList(c *gin.Context) {
	status := c.Query("status")
	priority := c.Query("priority")

	var filtered []*review.FeedbackPortfolio
	for _, p := range feedbackStore {
		if status != "" {
			match := false
			for _, item := range p.Items {
				if string(item.Status) == status {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if priority != "" {
			match := false
			for _, item := range p.Items {
				if string(item.Priority) == priority {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	totalItems := 0
	for _, p := range filtered {
		totalItems += len(p.Items)
	}

	c.JSON(http.StatusOK, reviewFeedbackListResponse{
		Portfolios: filtered,
		Total:      totalItems,
	})
}

// handleReviewFeedbackUpdate 更新反馈状态
// PUT /api/review/feedback/:id
func (s *Server) handleReviewFeedbackUpdate(c *gin.Context) {
	portfolioID := c.Param("id")
	itemID := c.Query("item_id")

	var req reviewFeedbackUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newStatus := review.FeedbackStatus(req.Status)
	if newStatus == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}

	for _, p := range feedbackStore {
		if p.ID == portfolioID {
			for i := range p.Items {
				if itemID == "" || p.Items[i].ID == itemID {
					feedbackGenerator.UpdateFeedback(&p.Items[i], newStatus, req.Note, req.Actor)
				}
			}
			c.JSON(http.StatusOK, reviewFeedbackResponse{Portfolio: p})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "feedback portfolio not found"})
}

// handleReviewFeedbackImplement 实施反馈
// POST /api/review/feedback/:id/implement
func (s *Server) handleReviewFeedbackImplement(c *gin.Context) {
	portfolioID := c.Param("id")

	var req struct {
		ItemID      string `json:"item_id" binding:"required"`
		NewVersion  string `json:"new_version" binding:"required"`
		Actor       string `json:"actor"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, p := range feedbackStore {
		if p.ID == portfolioID {
			for i := range p.Items {
				if p.Items[i].ID == req.ItemID {
					feedbackGenerator.ImplementFeedback(&p.Items[i], req.NewVersion, req.Actor)
					c.JSON(http.StatusOK, reviewFeedbackResponse{Portfolio: p})
					return
				}
			}
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "feedback item not found"})
}

// ============================================================================
// 复盘路由注册
// ============================================================================

// registerReviewRoutes 注册复盘路由
func (s *Server) registerReviewRoutes(api *gin.RouterGroup) {
	r := api.Group("/review")
	{
		r.POST("/generate", s.handleReviewGenerate)
		r.GET("/list", s.handleReviewList)
		r.GET("/:id", s.handleReviewGet)
		r.POST("/failure-analysis", s.handleReviewFailureAnalysis)
		r.GET("/failure-patterns", s.handleReviewFailurePatterns)
		r.POST("/feedback/generate", s.handleReviewFeedbackGenerate)
		r.GET("/feedback/list", s.handleReviewFeedbackList)
		r.PUT("/feedback/:id", s.handleReviewFeedbackUpdate)
		r.POST("/feedback/:id/implement", s.handleReviewFeedbackImplement)
	}
}
