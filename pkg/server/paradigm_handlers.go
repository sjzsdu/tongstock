package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// numberRangeRe parses expressions like "10-20%" or "10~20%".
var numberRangeRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[-~至到]\s*(\d+(?:\.\d+)?)\s*%?`)

type paradigmAnalyzeRequest struct {
	StockCode    string `json:"stock_code"`
	StockName    string `json:"stock_name,omitempty"`
	KlineType    string `json:"kline_type,omitempty"` // day, week, etc.
	Days         int    `json:"days,omitempty"`       // how many days of data to analyze
	ForceRefresh bool   `json:"force_refresh,omitempty"`
}

type paradigmAnalyzeResponse struct {
	StockCode        string                    `json:"stock_code"`
	StockName        string                    `json:"stock_name,omitempty"`
	Paradigm         *paradigms.Paradigm       `json:"paradigm,omitempty"`
	EvaluatedConfirm []paradigms.EvaluatedItem `json:"evaluated_confirm,omitempty"`
	EvaluatedInvalid []paradigms.EvaluatedItem `json:"evaluated_invalid,omitempty"`
	AgentText        string                    `json:"agent_text"`
	ExperimentID     string                    `json:"experiment_id,omitempty"`
	RunID            string                    `json:"run_id,omitempty"`
	EvidenceHash     string                    `json:"evidence_hash,omitempty"`
	Research         *agentResearchResponse    `json:"research,omitempty"`
	Cached           bool                      `json:"cached,omitempty"`
	Message          string                    `json:"message,omitempty"`
	Error            string                    `json:"error,omitempty"`
}

type paradigmListResponse struct {
	Paradigms []*paradigms.Paradigm `json:"paradigms"`
	Total     int                   `json:"total"`
}

func (s *Server) SetupParadigmRoutes(api *gin.RouterGroup) {
	p := api.Group("/paradigm")
	{
		p.POST("/analyze", s.handleParadigmAnalyze)
		p.POST("/evaluate", s.handleParadigmEvaluate)
		p.POST("/backtest", s.handleParadigmBacktest)
		p.GET("/list", s.handleParadigmList)
		p.GET("/alerts", s.handleParadigmAlerts)
		p.GET("/stats", s.handleParadigmStats)
		p.GET("/stock/:code", s.handleParadigmByStock)
		p.PUT("/:id/review", s.handleParadigmReview)
		p.DELETE("/:id", s.handleParadigmDelete)
	}
}

type paradigmAlert struct {
	ParadigmID string `json:"paradigm_id"`
	StockCode  string `json:"stock_code"`
	StockName  string `json:"stock_name,omitempty"`
	Side       string `json:"side"`
	Type       string `json:"type"`
	Condition  string `json:"condition"`
	Status     string `json:"status"`
	Value      string `json:"value,omitempty"`
	Severity   string `json:"severity"`
}

func (s *Server) StartParadigmAlertScanner(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		s.refreshParadigmAlertCache()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshParadigmAlertCache()
			}
		}
	}()
}

func (s *Server) refreshParadigmAlertCache() {
	alerts := s.collectParadigmAlerts("")
	s.paradigmAlertMu.Lock()
	s.paradigmAlertCache = alerts
	s.paradigmAlertLastScan = time.Now()
	s.paradigmAlertMu.Unlock()
}

func (s *Server) collectParadigmAlerts(code string) []paradigmAlert {
	if s.paradigmStore == nil {
		return []paradigmAlert{}
	}
	var list []*paradigms.Paradigm
	if code != "" {
		list = s.paradigmStore.ListByStockCode(code)
	} else {
		list = s.paradigmStore.List()
	}
	alerts := make([]paradigmAlert, 0)
	for _, p := range list {
		conds := s.evaluateParadigmConditions(p.StockCode, p)
		for _, ec := range conds {
			if ec.Status != "met" {
				continue
			}
			severity := "info"
			if ec.Type == "stop_loss" {
				severity = "critical"
			} else if ec.Type == "take_profit" {
				severity = "warning"
			}
			alerts = append(alerts, paradigmAlert{ParadigmID: p.ID, StockCode: p.StockCode, StockName: p.StockName, Side: p.Side, Type: ec.Type, Condition: ec.Condition, Status: ec.Status, Value: ec.Value, Severity: severity})
		}
	}
	return alerts
}

func (s *Server) handleParadigmAlerts(c *gin.Context) {
	code := c.Query("stock_code")
	if code == "" {
		s.paradigmAlertMu.RLock()
		cached := append([]paradigmAlert{}, s.paradigmAlertCache...)
		lastScan := s.paradigmAlertLastScan
		s.paradigmAlertMu.RUnlock()
		if !lastScan.IsZero() && c.Query("live") != "true" {
			c.JSON(http.StatusOK, gin.H{"alerts": cached, "total": len(cached), "last_scan": lastScan.Format(time.RFC3339), "cached": true})
			return
		}
	}
	alerts := s.collectParadigmAlerts(code)
	c.JSON(http.StatusOK, gin.H{"alerts": alerts, "total": len(alerts), "cached": false})
}

type paradigmStatsResponse struct {
	Total           int     `json:"total"`
	Reviewed        int     `json:"reviewed"`
	Verified        int     `json:"verified"`
	Rejected        int     `json:"rejected"`
	WinRate         float64 `json:"win_rate"`
	AverageReturn   float64 `json:"average_return"`
	AverageRating   float64 `json:"average_rating"`
	HighReliability int     `json:"high_reliability"`
}

func (s *Server) handleParadigmStats(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusOK, paradigmStatsResponse{})
		return
	}
	list := s.paradigmStore.List()
	resp := paradigmStatsResponse{Total: len(list)}
	var retSum, ratingSum float64
	var retN, ratingN, wins int
	for _, p := range list {
		switch p.ReviewStatus {
		case "reviewed":
			resp.Reviewed++
		case "verified":
			resp.Verified++
		case "rejected":
			resp.Rejected++
		}
		if p.Validation.ReliabilityLabel == "high" {
			resp.HighReliability++
		}
		if p.ActualReturn != nil {
			retSum += *p.ActualReturn
			retN++
			if *p.ActualReturn > 0 {
				wins++
			}
		}
		if p.ReviewRating > 0 {
			ratingSum += float64(p.ReviewRating)
			ratingN++
		}
	}
	if retN > 0 {
		resp.AverageReturn = retSum / float64(retN)
		resp.WinRate = float64(wins) / float64(retN)
	}
	if ratingN > 0 {
		resp.AverageRating = ratingSum / float64(ratingN)
	}
	c.JSON(http.StatusOK, resp)
}

type paradigmEvaluateRequest struct {
	StockCode string `json:"stock_code"`
}

type paradigmEvaluateResponse struct {
	StockCode  string               `json:"stock_code"`
	Conditions []EvaluatedCondition `json:"conditions"`
	Error      string               `json:"error,omitempty"`
}

func (s *Server) handleParadigmEvaluate(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusInternalServerError, paradigmEvaluateResponse{Error: "paradigm store not initialized"})
		return
	}

	var req paradigmEvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, paradigmEvaluateResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.StockCode == "" {
		c.JSON(http.StatusBadRequest, paradigmEvaluateResponse{Error: "stock_code is required"})
		return
	}

	existing := s.paradigmStore.GetByStockCode(req.StockCode)
	if existing == nil {
		c.JSON(http.StatusOK, paradigmEvaluateResponse{StockCode: req.StockCode, Conditions: []EvaluatedCondition{}})
		return
	}

	conditions := s.evaluateParadigmConditions(req.StockCode, existing)
	c.JSON(http.StatusOK, paradigmEvaluateResponse{
		StockCode:  req.StockCode,
		Conditions: conditions,
	})
}

func (s *Server) handleParadigmAnalyze(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusInternalServerError, paradigmAnalyzeResponse{Error: "agent not initialized"})
		return
	}
	if s.paradigmStore == nil {
		c.JSON(http.StatusInternalServerError, paradigmAnalyzeResponse{Error: "paradigm store not initialized"})
		return
	}

	var req paradigmAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, paradigmAnalyzeResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if req.StockCode == "" {
		c.JSON(http.StatusBadRequest, paradigmAnalyzeResponse{Error: "stock_code is required"})
		return
	}
	if req.Days == 0 {
		req.Days = 120
	}
	if req.KlineType == "" {
		req.KlineType = "day"
	}
	cacheKey := paradigmCacheKey(req.StockCode, req.KlineType, req.Days, "stock-paradigm-miner")

	// Check cache: return existing paradigm for this stock if available
	if !req.ForceRefresh {
		existing := s.paradigmStore.GetByCacheKey(cacheKey)
		if existing == nil {
			existing = s.paradigmStore.GetByStockCode(req.StockCode)
		}
		if existing != nil {
			s.respondWithVerifiedParadigmResearch(c, req, existing, true)
			return
		}
	}

	// Build data prompt from existing APIs
	prompt := s.buildParadigmPrompt(req.StockCode, req.StockName, req.Days)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()

	// Create runner for paradigm-miner agent
	s.agentState.mu.Lock()
	runner, err := s.agentState.rt.NewDirectRunner(pcwrap.RunOptions{
		Agent:          "stock-paradigm-miner",
		Model:          s.agentState.defaults.Model,
		Workspace:      s.agentState.workspace,
		Quiet:          true,
		EmbeddedAgents: s.agentState.embedded,
	})
	s.agentState.mu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, paradigmAnalyzeResponse{Error: err.Error()})
		return
	}
	defer runner.Close()

	agentResp, err := runner.ProcessDirectContext(ctx, pcwrap.RunOptions{
		Message:   prompt,
		Agent:     "stock-paradigm-miner",
		Session:   fmt.Sprintf("paradigm:%s:%d", req.StockCode, time.Now().UnixNano()),
		Workspace: s.agentState.workspace,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, paradigmAnalyzeResponse{Error: err.Error()})
		return
	}

	// Try to extract paradigm from agent response
	paradigm := extractParadigm(agentResp, req.StockCode, req.StockName)
	if paradigm != nil {
		paradigm.AgentText = agentResp
		paradigm.Source = paradigms.ParadigmSource{AgentVersion: "stock-paradigm-miner", Model: s.agentState.defaults.Model, KlineType: req.KlineType, Days: req.Days, GeneratedAt: time.Now().Format(time.RFC3339), CacheKey: cacheKey}
		paradigm.Validation = paradigms.ValidateParadigm(paradigm)
		if !paradigm.Validation.Valid {
			c.JSON(http.StatusOK, paradigmAnalyzeResponse{StockCode: req.StockCode, StockName: req.StockName, Paradigm: paradigm, AgentText: agentResp, Error: strings.Join(paradigm.Validation.Errors, "; ")})
			return
		}
		_ = s.paradigmStore.Save(paradigm)
	}

	if paradigm == nil {
		c.JSON(http.StatusUnprocessableEntity, paradigmAnalyzeResponse{
			StockCode: req.StockCode, StockName: req.StockName,
			AgentText: "AI 输出未形成可执行、可证伪的结构化假设，拒绝给出有效性结论。",
			Error:     "agent response did not contain a valid executable hypothesis",
		})
		return
	}
	s.respondWithVerifiedParadigmResearch(c, req, paradigm, false)
}

func (s *Server) respondWithVerifiedParadigmResearch(
	c *gin.Context,
	req paradigmAnalyzeRequest,
	p *paradigms.Paradigm,
	cached bool,
) {
	result, exp, run, err := s.conductVerifiedResearch(c.Request.Context(), agentResearchRequest{
		ParadigmID: p.ID,
		Question:   "验证 AI 生成的股票范式是否具有真实样本外证据",
	})
	if err != nil {
		response := paradigmAnalyzeResponse{
			StockCode: req.StockCode, StockName: req.StockName, Paradigm: p,
			AgentText: "真实数据、冻结快照或实验制品不足，拒绝判断该范式有效。",
			Error:     err.Error(), Cached: cached,
		}
		if exp != nil {
			response.ExperimentID = exp.ID
		}
		if run != nil {
			response.RunID = run.ID
		}
		c.JSON(http.StatusUnprocessableEntity, response)
		return
	}
	evalConfirm, evalInvalid := s.evaluateConditions(req.StockCode, p)
	c.JSON(http.StatusOK, paradigmAnalyzeResponse{
		StockCode: req.StockCode, StockName: req.StockName, Paradigm: p,
		EvaluatedConfirm: evalConfirm, EvaluatedInvalid: evalInvalid,
		AgentText: result.Answer, ExperimentID: result.Citation.ExperimentID,
		RunID: result.Citation.RunID, EvidenceHash: result.Citation.EvidenceHash,
		Research: result, Cached: cached,
		Message: "AI 假设已通过真实冻结快照运行实验；结论仅引用返回的 Evidence 与 critic 制品。",
	})
}

func (s *Server) handleParadigmList(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusOK, paradigmListResponse{Paradigms: []*paradigms.Paradigm{}, Total: 0})
		return
	}
	list := filterParadigms(s.paradigmStore.List(), c)
	total := len(list)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}
	if limit > 0 {
		end := offset + limit
		if offset > len(list) {
			list = []*paradigms.Paradigm{}
		} else {
			if end > len(list) {
				end = len(list)
			}
			list = list[offset:end]
		}
	}
	c.JSON(http.StatusOK, paradigmListResponse{Paradigms: list, Total: total})
}

func filterParadigms(list []*paradigms.Paradigm, c *gin.Context) []*paradigms.Paradigm {
	stockCode := strings.TrimSpace(c.Query("stock_code"))
	side := strings.TrimSpace(c.Query("side"))
	marketCap := strings.TrimSpace(c.Query("market_cap"))
	shareholder := strings.TrimSpace(c.Query("shareholder"))
	reviewStatus := strings.TrimSpace(c.Query("review_status"))
	reliability := strings.TrimSpace(c.Query("reliability"))
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	out := make([]*paradigms.Paradigm, 0, len(list))
	for _, p := range list {
		if stockCode != "" && p.StockCode != stockCode {
			continue
		}
		if side != "" && p.Side != side {
			continue
		}
		if marketCap != "" && p.Context.MarketCap != marketCap {
			continue
		}
		if shareholder != "" && p.Context.ShareholderDominant != shareholder {
			continue
		}
		if reviewStatus != "" && p.ReviewStatus != reviewStatus {
			continue
		}
		if reliability != "" && p.Validation.ReliabilityLabel != reliability {
			continue
		}
		if query != "" {
			hay := strings.ToLower(p.ID + " " + p.Name + " " + p.StockCode + " " + p.StockName)
			if !strings.Contains(hay, query) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) handleParadigmByStock(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusOK, paradigmListResponse{Paradigms: []*paradigms.Paradigm{}, Total: 0})
		return
	}
	code := c.Param("code")
	list := s.paradigmStore.ListByStockCode(code)
	c.JSON(http.StatusOK, paradigmListResponse{Paradigms: list, Total: len(list)})
}

type paradigmReviewRequest struct {
	ReviewStatus string   `json:"review_status"` // pending / reviewed / verified / rejected
	ReviewNote   string   `json:"review_note,omitempty"`
	ReviewRating int      `json:"review_rating,omitempty"` // 1-5
	ActualReturn *float64 `json:"actual_return,omitempty"`
}

func (s *Server) handleParadigmReview(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "paradigm store not initialized"})
		return
	}
	id := c.Param("id")
	p, err := s.paradigmStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req paradigmReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ReviewStatus == paradigms.StateVerified || req.ReviewStatus == paradigms.StatePromoted {
		evidence, evidenceErr := s.latestParadigmExperimentEvidence(id, "")
		if evidenceErr != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "无法验证真实证据: " + evidenceErr.Error()})
			return
		}
		if !evidence.PromotionEligible {
			c.JSON(http.StatusConflict, gin.H{
				"error":              "真实证据不完整，禁止将范式标记为已验证或已晋级",
				"promotion_blockers": evidence.PromotionBlockers,
				"evidence":           evidence,
			})
			return
		}
	}

	pCopy := *p
	if req.ReviewStatus != "" {
		pCopy.ReviewStatus = req.ReviewStatus
	}
	if req.ReviewNote != "" {
		pCopy.ReviewNote = req.ReviewNote
	}
	if req.ReviewRating >= 1 && req.ReviewRating <= 5 {
		pCopy.ReviewRating = req.ReviewRating
	}
	if req.ActualReturn != nil {
		pCopy.ActualReturn = req.ActualReturn
	}

	if err := s.paradigmStore.Save(&pCopy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, &pCopy)
}

func (s *Server) handleParadigmDelete(c *gin.Context) {
	if s.paradigmStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "paradigm store not initialized"})
		return
	}
	id := c.Param("id")
	if err := s.paradigmStore.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (s *Server) buildParadigmPrompt(code, name string, days int) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("请对股票 %s", code))
	if name != "" {
		b.WriteString(fmt.Sprintf("（%s）", name))
	}
	b.WriteString(fmt.Sprintf(" 进行范式挖掘分析。\n\n"))
	b.WriteString(fmt.Sprintf("分析范围：最近 %d 个交易日\n\n", days))

	// Fetch indicator data
	b.WriteString("## 技术指标数据\n")
	if indicator, err := s.fetchIndicator(code, "day"); err == nil {
		b.WriteString(formatIndicatorForPrompt(indicator))
	} else {
		b.WriteString(fmt.Sprintf("（获取指标数据失败: %v）\n", err))
	}

	// Fetch finance data
	b.WriteString("\n## 基本面数据\n")
	if finance, err := s.fetchFinance(code); err == nil {
		b.WriteString(formatFinanceForPrompt(finance))
	} else {
		b.WriteString(fmt.Sprintf("（获取财务数据失败: %v）\n", err))
	}

	// Shareholder profile analysis
	b.WriteString("\n## 股东结构画像\n")
	profile := s.buildShareholderProfile(code)
	b.WriteString(profile)

	b.WriteString(`
请先输出完整分析，然后必须在最后输出一个可机器解析的 JSON 代码块，格式如下：

` + "```json" + `
{
  "id": "paradigm-股票代码-短名称",
  "name": "范式名称",
  "side": "buy",
  "context": {"market_cap":"small|mid|large|mega", "shareholder_dominant":"retail|hot_money|foreign|institutional|state|mixed", "activity":"active|normal|quiet", "trend":"uptrend|downtrend|range|volatile"},
  "buy_conditions": [{"indicator":"close", "operator":"gt", "value":"MA20"}],
  "sell_conditions": {"take_profit":[{"indicator":"close", "operator":"gt", "value":"12.30"}], "stop_loss":[{"indicator":"close", "operator":"lt", "value":"MA60"}]},
  "confirmations": ["确认项"],
  "invalidations": ["失效规则"],
  "expectation": {"holding_period":"待验证", "expected_return":"待真实实验验证", "risk_reward_ratio":"待真实实验验证", "win_rate":0, "sample_size":0, "confidence":0},
  "rationale": "范式逻辑"
}
` + "```" + `

要求：JSON 中条件必须尽量结构化。operator 只使用 gt、lt、between、near、cross_above、cross_below、describe。
你只负责生成可证伪假设，不得声称回测通过，不得编造收益率、胜率、样本量、交易记录或置信度；
验证结论将由服务端真实冻结快照、持久化实验、Evidence 和 critic 生成。`)
	return b.String()
}

func paradigmCacheKey(stockCode, klineType string, days int, agentVersion string) string {
	return fmt.Sprintf("%s:%s:%d:%s", stockCode, klineType, days, agentVersion)
}

func (s *Server) buildShareholderProfile(code string) string {
	var b strings.Builder

	// Get finance data
	info, err := s.svc.FetchFinance(code)
	if err != nil {
		return "（无法获取股东结构数据）\n"
	}

	// Calculate per-capita holdings
	perCapita := 0.0
	if info.GuDongRenShu > 0 && info.LiuTongGuBen > 0 {
		// 流通市值 ≈ 流通股本 × 最新价（近似）
		// 这里用流通股本作为近似指标
		perCapita = info.LiuTongGuBen / float64(info.GuDongRenShu)
	}

	// Estimate market cap band
	marketCapDesc := "unknown"
	if info.ZongGuBen > 0 {
		// 粗略估算市值（万元单位的总股本 × 假设均价10元）
		// 更准确的方式需要最新价，这里用总股本做粗分类
		totalShares := info.ZongGuBen // 万股
		if totalShares < 50000 {
			marketCapDesc = "small(<50亿)"
		} else if totalShares < 200000 {
			marketCapDesc = "mid(50-200亿)"
		} else if totalShares < 1000000 {
			marketCapDesc = "large(200-1000亿)"
		} else {
			marketCapDesc = "mega(>1000亿)"
		}
	}

	b.WriteString(fmt.Sprintf("- 股东人数: %.0f 人\n", info.GuDongRenShu))
	b.WriteString(fmt.Sprintf("- 总股本: %.0f 万股\n", info.ZongGuBen))
	b.WriteString(fmt.Sprintf("- 流通股本: %.0f 万股\n", info.LiuTongGuBen))
	b.WriteString(fmt.Sprintf("- 估算市值规模: %s\n", marketCapDesc))

	if perCapita > 0 {
		b.WriteString(fmt.Sprintf("- 人均持股(万股): %.2f\n", perCapita))
		if perCapita < 1 {
			b.WriteString("- 画像推断: 散户主导（人均持股低，股东人数多）\n")
		} else if perCapita < 5 {
			b.WriteString("- 画像推断: 散户+游资混合\n")
		} else if perCapita < 20 {
			b.WriteString("- 画像推断: 机构参与度中等\n")
		} else {
			b.WriteString("- 画像推断: 机构/主力主导（人均持股高）\n")
		}
	}

	// Turnover rate analysis
	klines, err := s.svc.FetchKlineAll(code, 0)
	if err == nil && len(klines) >= 20 {
		avgTurnover := 0.0
		for i := len(klines) - 20; i < len(klines); i++ {
			avgTurnover += klines[i].Volume
		}
		avgTurnover = avgTurnover / 20
		if info.LiuTongGuBen > 0 {
			avgTurnoverRate := avgTurnover / info.LiuTongGuBen * 100
			b.WriteString(fmt.Sprintf("- 近20日平均换手率: %.2f%%\n", avgTurnoverRate))
			if avgTurnoverRate > 5 {
				b.WriteString("- 活跃度: 非常活跃（散户游资频繁交易）\n")
			} else if avgTurnoverRate > 2 {
				b.WriteString("- 活跃度: 活跃\n")
			} else if avgTurnoverRate > 0.5 {
				b.WriteString("- 活跃度: 正常\n")
			} else {
				b.WriteString("- 活跃度: 低（机构控盘或冷门股）\n")
			}
		}
	}

	return b.String()
}

func (s *Server) fetchIndicator(code, ktype string) (map[string]any, error) {
	// Use internal handler logic to get indicator data
	// We call the same logic as handleIndicator but capture the result
	klines, err := s.svc.FetchKlineAll(code, 0) // day type = 0
	if err != nil {
		return nil, err
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("no kline data")
	}

	// Return raw klines for the prompt
	result := map[string]any{
		"code":        code,
		"kline_count": len(klines),
	}
	if len(klines) > 0 {
		last := klines[len(klines)-1]
		result["latest_date"] = last.Time.Format("2006-01-02")
		result["latest_close"] = last.Close
		result["latest_volume"] = last.Volume
		result["latest_amount"] = last.Amount
	}
	return result, nil
}

func (s *Server) fetchFinance(code string) (map[string]any, error) {
	info, err := s.svc.FetchFinance(code)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"zong_gu_ben":      info.ZongGuBen,
		"liu_tong_gu_ben":  info.LiuTongGuBen,
		"zong_zi_chan":     info.ZongZiChan,
		"jing_zi_chan":     info.JingZiChan,
		"zhu_ying_shou_ru": info.ZhuYingShouRu,
		"jing_li_run":      info.JingLiRun,
		"mei_gu_jing_zi":   info.MeiGuJingZiChan,
		"gu_dong_ren_shu":  info.GuDongRenShu,
	}
	return result, nil
}

func formatIndicatorForPrompt(data map[string]any) string {
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b) + "\n"
}

func formatFinanceForPrompt(data map[string]any) string {
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b) + "\n"
}

// extractParadigm parses a paradigm from agent text output
func extractParadigm(text, stockCode, stockName string) *paradigms.Paradigm {
	if text == "" {
		return nil
	}
	if p := extractParadigmJSON(text, stockCode, stockName); p != nil {
		return p
	}

	p := &paradigms.Paradigm{
		StockCode: stockCode,
		StockName: stockName,
	}

	// Extract id
	if id := extractField(text, "id:"); id != "" {
		p.ID = id
	} else {
		p.ID = fmt.Sprintf("paradigm-%s-%d", stockCode, time.Now().UnixMilli())
	}

	// Extract name - try several patterns
	if name := extractField(text, "name:"); name != "" {
		p.Name = name
	} else if name := extractField(text, "范式名称:"); name != "" {
		p.Name = name
	} else {
		p.Name = fmt.Sprintf("%s 范式", stockName)
	}

	// Extract side (buy/sell) — check field, then ID, then name, then buy_conditions comment
	p.Side = extractField(text, "side:")
	if p.Side == "" {
		// Check ID
		idLower := strings.ToLower(p.ID)
		if strings.Contains(idLower, "sell") || strings.Contains(idLower, "short") || strings.Contains(idLower, "bearish") {
			p.Side = "sell"
		}
	}
	if p.Side == "" {
		// Check name
		nameLower := strings.ToLower(p.Name)
		if strings.Contains(nameLower, "卖出") || strings.Contains(nameLower, "做空") || strings.Contains(nameLower, "空头") || strings.Contains(nameLower, "减仓") || strings.Contains(nameLower, "sell") || strings.Contains(nameLower, "short") {
			p.Side = "sell"
		}
	}
	if p.Side == "" {
		// Check buy_conditions comment for "做空" or "卖出"
		bcComment := extractField(text, "buy_conditions:")
		if strings.Contains(bcComment, "做空") || strings.Contains(bcComment, "卖出") || strings.Contains(bcComment, "空仓") {
			p.Side = "sell"
		}
	}
	if p.Side == "" {
		p.Side = "buy"
	}

	// Extract context
	p.Context.MarketCap = extractField(text, "market_cap:")
	if p.Context.MarketCap == "" {
		p.Context.MarketCap = extractField(text, "市值:")
	}
	p.Context.ShareholderDominant = extractField(text, "shareholder_dominant:")
	if p.Context.ShareholderDominant == "" {
		p.Context.ShareholderDominant = extractField(text, "股东:")
	}
	p.Context.Trend = extractField(text, "trend:")
	if p.Context.Trend == "" {
		p.Context.Trend = extractField(text, "趋势:")
	}
	p.Context.Activity = extractField(text, "activity:")

	// Extract buy conditions - look for lines with indicators
	p.BuyConds = extractConditions(text, "buy_conditions", "买入条件")
	if len(p.BuyConds) == 0 {
		p.BuyConds = extractConditionsFromSection(text, "买入条件", "卖出条件")
	}

	// Extract sell conditions
	if tp := extractConditions(text, "take_profit", "止盈"); len(tp) > 0 {
		p.SellConds.TakeProfit = tp
	}
	if sl := extractConditions(text, "stop_loss", "止损"); len(sl) > 0 {
		p.SellConds.StopLoss = sl
	}
	if len(p.SellConds.TakeProfit) == 0 && len(p.SellConds.StopLoss) == 0 {
		// Try to extract from numbered/bulleted list
		p.SellConds.TakeProfit = extractConditionsFromSection(text, "止盈", "止损")
		p.SellConds.StopLoss = extractConditionsFromSection(text, "止损", "确认")
	}

	// Extract confirmations and invalidations
	p.Confirm = extractList(text, "confirmations", "确认项")
	p.Invalid = extractList(text, "invalidations", "失效规则")

	// Extract expectation fields with flexible patterns
	p.Expectation = extractExpectation(text)

	// Extract rationale
	p.Rationale = extractField(text, "rationale:")
	if p.Rationale == "" {
		p.Rationale = extractField(text, "范式逻辑:")
	}

	return p
}

func extractParadigmJSON(text, stockCode, stockName string) *paradigms.Paradigm {
	blocks := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```").FindAllStringSubmatch(text, -1)
	try := make([]string, 0, len(blocks)+1)
	for _, b := range blocks {
		if len(b) > 1 {
			try = append(try, b[1])
		}
	}
	if start := strings.LastIndex(text, "{"); start >= 0 {
		try = append(try, text[start:])
	}
	for _, raw := range try {
		var p paradigms.Paradigm
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		if p.ID == "" {
			p.ID = fmt.Sprintf("paradigm-%s-%d", stockCode, time.Now().UnixMilli())
		}
		if p.Name == "" {
			p.Name = fmt.Sprintf("%s 范式", stockName)
		}
		p.StockCode = stockCode
		p.StockName = stockName
		if p.Side == "" {
			p.Side = "buy"
		}
		return &p
	}
	return nil
}

func extractField(text, field string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, field) {
			idx := strings.Index(trimmed, field)
			val := strings.TrimSpace(trimmed[idx+len(field):])
			val = strings.Trim(val, "\"'")
			// Take only the first line if multi-line
			if nl := strings.IndexAny(val, "\n\r"); nl > 0 {
				val = val[:nl]
			}
			if val != "" && val != "|" && val != ">" {
				return val
			}
		}
	}
	return ""
}

func extractConditions(text, yamlKey, cnKey string) []paradigms.Condition {
	var conds []paradigms.Condition
	// Look for lines like "- indicator: MACD.DIF" or "- indicator: xxx, operator: xxx, value: xxx"
	lines := strings.Split(text, "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, yamlKey+":") || strings.Contains(trimmed, cnKey+"：") {
			inBlock = true
			continue
		}
		if inBlock && trimmed == "" {
			break
		}
		if inBlock && strings.HasPrefix(trimmed, "- ") {
			cond := paradigms.Condition{}
			parts := strings.TrimSpace(trimmed[2:])
			// Parse "indicator: X operator: Y value: Z" or plain text like "MA20 > MA10"
			if idx := strings.Index(parts, "indicator:"); idx >= 0 {
				cond.Indicator = extractInline(parts[idx+10:])
				if oidx := strings.Index(parts, "operator:"); oidx >= 0 {
					cond.Operator = extractInline(parts[oidx+9:])
				}
				if vidx := strings.Index(parts, "value:"); vidx >= 0 {
					cond.Value = extractInline(parts[vidx+6:])
				}
			} else {
				// Plain text condition like "MA20 > MA10 (多头排列)"
				cond.Indicator = parts
				cond.Operator = "describe"
			}
			if cond.Indicator != "" {
				conds = append(conds, cond)
			}
		}
	}
	return conds
}

func extractInline(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'")
	if nl := strings.IndexAny(s, "\n\r"); nl > 0 {
		s = s[:nl]
	}
	return s
}

func extractConditionsFromSection(text, startKey, endKey string) []paradigms.Condition {
	var conds []paradigms.Condition
	lines := strings.Split(text, "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, startKey) && (strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "：") || strings.HasSuffix(trimmed, "\n")) {
			inBlock = true
			continue
		}
		if inBlock && endKey != "" && strings.Contains(trimmed, endKey) {
			break
		}
		if inBlock && trimmed == "" {
			continue
		}
		if inBlock && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "· ")) {
			item := strings.TrimSpace(trimmed[2:])
			cond := paradigms.Condition{Indicator: item, Operator: "describe", Value: ""}
			conds = append(conds, cond)
		}
	}
	return conds
}

func extractList(text, yamlKey, cnKey string) []string {
	var items []string
	lines := strings.Split(text, "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, yamlKey+":") || strings.Contains(trimmed, cnKey+"：") {
			inBlock = true
			continue
		}
		if inBlock && trimmed == "" {
			break
		}
		if inBlock && strings.HasPrefix(trimmed, "- ") {
			items = append(items, strings.TrimSpace(trimmed[2:]))
		}
	}
	return items
}

func extractExpectation(text string) paradigms.Expectation {
	e := paradigms.Expectation{}

	// Try multiple patterns for each field
	if v := extractField(text, "holding_period:"); v != "" {
		e.HoldingPeriod = v
	} else if v := extractField(text, "持仓周期:"); v != "" {
		e.HoldingPeriod = v
	}

	if v := extractField(text, "expected_return:"); v != "" {
		e.ExpectedReturn = v
	} else if v := extractField(text, "预期收益:"); v != "" {
		e.ExpectedReturn = v
	} else if v := extractNumberRange(text, "期望收益"); v != "" {
		e.ExpectedReturn = v
	}

	if v := extractField(text, "risk_reward_ratio:"); v != "" {
		e.RiskReward = v
	} else if v := extractField(text, "盈亏比:"); v != "" {
		e.RiskReward = v
	}

	if v := extractField(text, "confidence:"); v != "" {
		fmt.Sscanf(v, "%f", &e.Confidence)
	} else if v := extractField(text, "置信度:"); v != "" {
		fmt.Sscanf(v, "%f", &e.Confidence)
	}

	if v := extractField(text, "win_rate:"); v != "" {
		fmt.Sscanf(v, "%f", &e.WinRate)
	} else if v := extractField(text, "胜率:"); v != "" {
		fmt.Sscanf(v, "%f", &e.WinRate)
	}

	if v := extractField(text, "sample_size:"); v != "" {
		fmt.Sscanf(v, "%d", &e.SampleSize)
	} else if v := extractField(text, "样本量:"); v != "" {
		fmt.Sscanf(v, "%d", &e.SampleSize)
	}

	return e
}

func extractNumberRange(text, keyword string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, keyword) {
			if match := numberRangeRe.FindStringSubmatch(trimmed); len(match) >= 3 {
				rangeStr := match[1] + "-" + match[2]
				if strings.Contains(trimmed, "%") {
					rangeStr += "%"
				}
				return rangeStr
			}
		}
	}
	return ""
}

// evaluateConditions checks confirmations and invalidations against current stock data
func (s *Server) evaluateConditions(stockCode string, p *paradigms.Paradigm) ([]paradigms.EvaluatedItem, []paradigms.EvaluatedItem) {
	if p == nil {
		return nil, nil
	}

	// Fetch current indicator data for evaluation
	indicator := s.fetchCurrentIndicator(stockCode)

	evalConfirm := make([]paradigms.EvaluatedItem, 0, len(p.Confirm))
	for _, c := range p.Confirm {
		item := paradigms.EvaluatedItem{Text: c, Status: "unknown"}
		evalConfirm = append(evalConfirm, item)
	}

	evalInvalid := make([]paradigms.EvaluatedItem, 0, len(p.Invalid))
	for _, rule := range p.Invalid {
		item := paradigms.EvaluatedItem{Text: rule, Status: "unknown"}
		// Try to evaluate invalidation rules against indicator data
		if indicator != nil {
			s.evaluateSingleInvalidation(&item, rule, indicator)
		}
		evalInvalid = append(evalInvalid, item)
	}

	return evalConfirm, evalInvalid
}

func (s *Server) fetchCurrentIndicator(stockCode string) map[string]float64 {
	klines, err := s.svc.FetchKlineAll(stockCode, 0)
	if err != nil || len(klines) < 20 {
		return nil
	}

	result := make(map[string]float64)
	last := klines[len(klines)-1]
	prev := klines[len(klines)-2]
	result["close"] = last.Close
	result["prev_close"] = prev.Close
	result["volume"] = last.Volume
	result["prev_volume"] = prev.Volume

	// Calculate simple MA60
	for _, period := range []int{5, 10, 20, 60} {
		if len(klines) >= period {
			result[fmt.Sprintf("ma%d", period)] = calcSMA(klines, period, len(klines)-1)
		}
		if len(klines) >= period+1 {
			result[fmt.Sprintf("prev_ma%d", period)] = calcSMA(klines, period, len(klines)-2)
		}
	}

	// Backward-compatible explicit MA60 key
	if len(klines) >= 60 {
		sum := 0.0
		for i := len(klines) - 60; i < len(klines); i++ {
			sum += klines[i].Close
		}
		result["ma60"] = sum / 60
	}

	// Calculate 20-day average volume
	if len(klines) >= 20 {
		volSum := 0.0
		for i := len(klines) - 20; i < len(klines); i++ {
			volSum += klines[i].Volume
		}
		result["avg_volume_20"] = volSum / 20
	}

	// Simple MACD approximation (for validation only)
	if len(klines) >= 26 {
		ema12 := calcEMA(klines, 12)
		ema26 := calcEMA(klines, 26)
		if ema12 > 0 && ema26 > 0 {
			result["macd_dif"] = ema12 - ema26
		}
		prevEMA12 := calcEMA(klines[:len(klines)-1], 12)
		prevEMA26 := calcEMA(klines[:len(klines)-1], 26)
		if prevEMA12 > 0 && prevEMA26 > 0 {
			result["prev_macd_dif"] = prevEMA12 - prevEMA26
		}
	}

	// Simple RSI14 calculation
	if len(klines) >= 15 {
		result["rsi14"] = calcRSI(klines, 14)
	}

	return result
}

func calcSMA(klines []*protocol.Kline, period, end int) float64 {
	if end < 0 || end >= len(klines) || end-period+1 < 0 {
		return 0
	}
	sum := 0.0
	for i := end - period + 1; i <= end; i++ {
		sum += klines[i].Close
	}
	return sum / float64(period)
}

func calcEMA(klines []*protocol.Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}
	return ema
}

func calcRSI(klines []*protocol.Kline, period int) float64 {
	if len(klines) < period+1 {
		return 50
	}
	gainSum := 0.0
	lossSum := 0.0
	for i := len(klines) - period; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gainSum += change
		} else {
			lossSum -= change
		}
	}
	if lossSum == 0 {
		return 100
	}
	rs := gainSum / lossSum
	return 100 - (100 / (1 + rs))
}

func (s *Server) evaluateSingleInvalidation(item *paradigms.EvaluatedItem, rule string, indicator map[string]float64) {
	ruleLower := strings.ToLower(rule)
	close, hasClose := indicator["close"]

	// Check "跌破XXX.XX" — generic price level
	if strings.Contains(ruleLower, "跌破") {
		if price := extractPrice(rule); price > 0 && hasClose {
			if close < price {
				item.Status = "met"
				item.Reason = fmt.Sprintf("当前价 %.2f < %.2f", close, price)
			} else {
				item.Status = "not_met"
				item.Reason = fmt.Sprintf("当前价 %.2f >= %.2f", close, price)
			}
			return
		}
	}

	// Check "突破XXX.XX" — generic price level
	if strings.Contains(ruleLower, "突破") {
		if price := extractPrice(rule); price > 0 && hasClose {
			if close > price {
				item.Status = "met"
				item.Reason = fmt.Sprintf("当前价 %.2f > %.2f", close, price)
			} else {
				item.Status = "not_met"
				item.Reason = fmt.Sprintf("当前价 %.2f <= %.2f", close, price)
			}
			return
		}
	}

	// Check "收盘价跌破 MA60"
	if strings.Contains(ruleLower, "ma60") || strings.Contains(ruleLower, "ma 60") {
		if hasClose {
			if ma60, ok := indicator["ma60"]; ok {
				if close < ma60 {
					item.Status = "met"
					item.Reason = fmt.Sprintf("当前价 %.2f < MA60 %.2f", close, ma60)
				} else {
					item.Status = "not_met"
					item.Reason = fmt.Sprintf("当前价 %.2f >= MA60 %.2f", close, ma60)
				}
				return
			}
		}
	}

	// Check "MACD 继续死叉" or "DIF ≤ DEA"
	if strings.Contains(ruleLower, "macd") && (strings.Contains(ruleLower, "死叉") || strings.Contains(ruleLower, "dif")) {
		if dif, ok := indicator["macd_dif"]; ok {
			if dif < 0 {
				item.Status = "met"
				item.Reason = fmt.Sprintf("MACD DIF=%.4f < 0 (死叉状态)", dif)
			} else {
				item.Status = "not_met"
				item.Reason = fmt.Sprintf("MACD DIF=%.4f >= 0 (非死叉)", dif)
			}
			return
		}
	}

	// Check "RSI" rules like "RSI14 > 70"
	if strings.Contains(ruleLower, "rsi") {
		if rsi, ok := indicator["rsi14"]; ok {
			// Extract threshold from rule
			threshold := 70.0
			if strings.Contains(ruleLower, "> 70") || strings.Contains(ruleLower, ">70") {
				threshold = 70
			} else if strings.Contains(ruleLower, "< 30") || strings.Contains(ruleLower, "<30") {
				threshold = 30
			}
			if strings.Contains(ruleLower, ">") || strings.Contains(ruleLower, "超买") {
				if rsi > threshold {
					item.Status = "met"
					item.Reason = fmt.Sprintf("RSI14=%.1f > %.0f (超买)", rsi, threshold)
				} else {
					item.Status = "not_met"
					item.Reason = fmt.Sprintf("RSI14=%.1f <= %.0f", rsi, threshold)
				}
				return
			}
			if strings.Contains(ruleLower, "<") || strings.Contains(ruleLower, "超卖") {
				if rsi < threshold {
					item.Status = "met"
					item.Reason = fmt.Sprintf("RSI14=%.1f < %.0f (超卖)", rsi, threshold)
				} else {
					item.Status = "not_met"
					item.Reason = fmt.Sprintf("RSI14=%.1f >= %.0f", rsi, threshold)
				}
				return
			}
		}
		item.Status = "unknown"
		item.Reason = "RSI 数据不足"
		return
	}

	// Check "KDJ" rules
	if strings.Contains(ruleLower, "kdj") || strings.Contains(ruleLower, "k值") || strings.Contains(ruleLower, "j值") {
		item.Status = "unknown"
		item.Reason = "KDJ 需要完整K线序列计算"
		return
	}

	// Check "成交量" related rules
	if strings.Contains(ruleLower, "成交量") || strings.Contains(ruleLower, "净卖出") {
		item.Status = "unknown"
		item.Reason = "需要实时交易数据判断"
		return
	}

	// Default: can't evaluate
	item.Status = "unknown"
	item.Reason = "无法自动判断"
}

// extractPrice extracts a numeric price from text like "跌破40.00" or "突破50.84"
func extractPrice(text string) float64 {
	// Find digits with optional decimal point
	for i := 0; i < len(text); i++ {
		if text[i] >= '0' && text[i] <= '9' {
			j := i
			for j < len(text) && ((text[j] >= '0' && text[j] <= '9') || text[j] == '.') {
				j++
			}
			var price float64
			if _, err := fmt.Sscanf(text[i:j], "%f", &price); err == nil && price > 0 {
				return price
			}
		}
	}
	return 0
}
