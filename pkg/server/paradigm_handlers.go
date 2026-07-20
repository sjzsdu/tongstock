package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// ParadigmStore is set from main.go to avoid import cycles
var ParadigmStore *paradigms.Store

var numberRangeRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[-~至到]\s*(\d+(?:\.\d+)?)\s*%?`)

type paradigmAnalyzeRequest struct {
	StockCode string `json:"stock_code"`
	StockName string `json:"stock_name,omitempty"`
	KlineType string `json:"kline_type,omitempty"` // day, week, etc.
	Days      int    `json:"days,omitempty"`       // how many days of data to analyze
}

type paradigmAnalyzeResponse struct {
	StockCode        string                    `json:"stock_code"`
	StockName        string                    `json:"stock_name,omitempty"`
	Paradigm         *paradigms.Paradigm       `json:"paradigm,omitempty"`
	EvaluatedConfirm []paradigms.EvaluatedItem `json:"evaluated_confirm,omitempty"`
	EvaluatedInvalid []paradigms.EvaluatedItem `json:"evaluated_invalid,omitempty"`
	AgentText        string                    `json:"agent_text"`
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
		p.GET("/list", s.handleParadigmList)
		p.GET("/:id", s.handleParadigmGet)
		p.DELETE("/:id", s.handleParadigmDelete)
	}
}

func (s *Server) handleParadigmAnalyze(c *gin.Context) {
	if s.agentState == nil {
		c.JSON(http.StatusInternalServerError, paradigmAnalyzeResponse{Error: "agent not initialized"})
		return
	}
	if ParadigmStore == nil {
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

	// Check cache: return existing paradigm for this stock if available
	if existing := ParadigmStore.GetByStockCode(req.StockCode); existing != nil {
		evalConfirm, evalInvalid := s.evaluateConditions(req.StockCode, existing)
		c.JSON(http.StatusOK, paradigmAnalyzeResponse{
			StockCode:        req.StockCode,
			StockName:        req.StockName,
			Paradigm:         existing,
			EvaluatedConfirm: evalConfirm,
			EvaluatedInvalid: evalInvalid,
			AgentText:        existing.AgentText,
		})
		return
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
		ParadigmStore.Save(paradigm)
	}

	// Evaluate confirmations and invalidations against current data
	evalConfirm, evalInvalid := s.evaluateConditions(req.StockCode, paradigm)

	c.JSON(http.StatusOK, paradigmAnalyzeResponse{
		StockCode:        req.StockCode,
		StockName:        req.StockName,
		Paradigm:         paradigm,
		EvaluatedConfirm: evalConfirm,
		EvaluatedInvalid: evalInvalid,
		AgentText:        agentResp,
	})
}

func (s *Server) handleParadigmList(c *gin.Context) {
	if ParadigmStore == nil {
		c.JSON(http.StatusOK, paradigmListResponse{Paradigms: []*paradigms.Paradigm{}, Total: 0})
		return
	}

	marketCap := c.Query("market_cap")
	shareholder := c.Query("shareholder")

	var list []*paradigms.Paradigm
	if marketCap != "" || shareholder != "" {
		list = ParadigmStore.ListByContext(paradigms.Context{
			MarketCap:           marketCap,
			ShareholderDominant: shareholder,
		})
	} else {
		list = ParadigmStore.List()
	}

	c.JSON(http.StatusOK, paradigmListResponse{Paradigms: list, Total: len(list)})
}

func (s *Server) handleParadigmGet(c *gin.Context) {
	if ParadigmStore == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "paradigm store not initialized"})
		return
	}
	id := c.Param("id")
	p, err := ParadigmStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) handleParadigmDelete(c *gin.Context) {
	if ParadigmStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "paradigm store not initialized"})
		return
	}
	id := c.Param("id")
	if err := ParadigmStore.Delete(id); err != nil {
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

	b.WriteString("\n请按照你的标准分析流程，输出完整的条件范式。")
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
	result["close"] = last.Close
	result["volume"] = last.Volume

	// Calculate simple MA60
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
	}

	// Simple RSI14 calculation
	if len(klines) >= 15 {
		result["rsi14"] = calcRSI(klines, 14)
	}

	return result
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
