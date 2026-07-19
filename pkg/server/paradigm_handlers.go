package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pcwrap "github.com/sjzsdu/tongstock/internal/picoclaw"
	"github.com/sjzsdu/tongstock/internal/paradigms"
)

// ParadigmStore is set from main.go to avoid import cycles
var ParadigmStore *paradigms.Store

type paradigmAnalyzeRequest struct {
	StockCode string `json:"stock_code"`
	StockName string `json:"stock_name,omitempty"`
	KlineType string `json:"kline_type,omitempty"` // day, week, etc.
	Days      int    `json:"days,omitempty"`       // how many days of data to analyze
}

type paradigmAnalyzeResponse struct {
	StockCode string            `json:"stock_code"`
	StockName string            `json:"stock_name,omitempty"`
	Paradigm  *paradigms.Paradigm `json:"paradigm,omitempty"`
	AgentText string            `json:"agent_text"`
	Error     string            `json:"error,omitempty"`
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
		ParadigmStore.Save(paradigm)
	}

	c.JSON(http.StatusOK, paradigmAnalyzeResponse{
		StockCode: req.StockCode,
		StockName: req.StockName,
		Paradigm:  paradigm,
		AgentText: agentResp,
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
			MarketCap:          marketCap,
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
		"code":       code,
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
		"zong_gu_ben":    info.ZongGuBen,
		"liu_tong_gu_ben": info.LiuTongGuBen,
		"zong_zi_chan":    info.ZongZiChan,
		"jing_zi_chan":    info.JingZiChan,
		"zhu_ying_shou_ru": info.ZhuYingShouRu,
		"jing_li_run":    info.JingLiRun,
		"mei_gu_jing_zi": info.MeiGuJingZiChan,
		"gu_dong_ren_shu": info.GuDongRenShu,
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

// extractParadigm tries to parse a YAML paradigm from agent text output
func extractParadigm(text, stockCode, stockName string) *paradigms.Paradigm {
	// Look for paradigm block in agent output
	if !strings.Contains(text, "paradigm:") && !strings.Contains(text, "id:") {
		return nil
	}

	// Simple extraction: look for key fields
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

	// Extract name
	if name := extractField(text, "name:"); name != "" {
		p.Name = name
	} else {
		p.Name = fmt.Sprintf("%s 范式", stockCode)
	}

	// Extract market_cap
	p.Context.MarketCap = extractField(text, "market_cap:")
	p.Context.ShareholderDominant = extractField(text, "shareholder_dominant:")

	// Extract confidence
	if conf := extractField(text, "confidence:"); conf != "" {
		fmt.Sscanf(conf, "%f", &p.Expectation.Confidence)
	}

	// Extract win_rate
	if wr := extractField(text, "win_rate:"); wr != "" {
		fmt.Sscanf(wr, "%f", &p.Expectation.WinRate)
	}

	// Extract rationale
	if rat := extractBlock(text, "rationale:"); rat != "" {
		p.Rationale = rat
	}

	return p
}

func extractField(text, field string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, field) {
			val := strings.TrimSpace(strings.TrimPrefix(line, field))
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}

func extractBlock(text, field string) string {
	idx := strings.Index(text, field)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(field):]
	// Find next line that starts with a top-level field (no indentation or different key)
	lines := strings.Split(rest, "\n")
	var block []string
	for i, line := range lines {
		if i == 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed == "|" || trimmed == ">" {
				continue
			}
			block = append(block, trimmed)
			continue
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && strings.Contains(line, ":") {
			break
		}
		block = append(block, strings.TrimSpace(line))
	}
	return strings.TrimSpace(strings.Join(block, "\n"))
}
