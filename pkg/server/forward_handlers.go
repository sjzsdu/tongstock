package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/trading"
)

// registerForwardRunRoutes registers forward run and signal ledger routes
func (s *Server) registerForwardRunRoutes(api *gin.RouterGroup) {
	fr := api.Group("/forward")
	{
		fr.GET("/runs", s.handleForwardRunsList)
		fr.POST("/runs", s.handleForwardRunCreate)
		fr.GET("/runs/:id", s.handleForwardRunGet)
		fr.POST("/runs/:id/execute", s.handleForwardRunExecute)
		fr.POST("/runs/:id/finalize", s.handleForwardRunFinalize)
		fr.GET("/runs/:id/signals", s.handleForwardRunSignals)
		fr.POST("/runs/:id/signals", s.handleForwardSignalAppend)
		fr.GET("/runs/:id/equity", s.handleForwardRunEquity)
		fr.GET("/runs/:id/compare", s.handleForwardRunCompare)
		fr.GET("/signals/:id", s.handleForwardSignalGet)
		fr.GET("/signals", s.handleForwardSignalsList)
	}
}

// ============================================================================
// Forward Run Handlers
// ============================================================================

type forwardRunCreateRequest struct {
	ParadigmVersionID string  `json:"paradigm_version_id"`
	StartDate         string  `json:"start_date"`
	InitialCash       float64 `json:"initial_cash"`
	EnableT1          *bool   `json:"enable_t_1,omitempty"`
	EnablePriceLimit  *bool   `json:"enable_price_limit,omitempty"`
	EnableSuspension  *bool   `json:"enable_suspension,omitempty"`
	Board             string  `json:"board,omitempty"`
	CommissionRate    float64 `json:"commission_rate,omitempty"`
	SlippageBps       float64 `json:"slippage_bps,omitempty"`
	StampDutyRate     float64 `json:"stamp_duty_rate,omitempty"`
}

type forwardRunResponse struct {
	Run *ledger.ForwardRun `json:"run"`
}

func (s *Server) handleForwardRunsList(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	runs := s.ledger.ListRuns(limit)
	c.JSON(http.StatusOK, gin.H{"runs": runs, "total": len(runs)})
}

func (s *Server) handleForwardRunCreate(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}

	var req forwardRunCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ParadigmVersionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paradigm_version_id is required"})
		return
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date format (expected YYYY-MM-DD)"})
		return
	}

	if req.InitialCash <= 0 {
		req.InitialCash = 1000000
	}

	constraints := trading.DefaultTradingConstraints()
	if req.EnableT1 != nil {
		constraints.EnableT1 = *req.EnableT1
	}
	if req.EnablePriceLimit != nil {
		constraints.EnablePriceLimit = *req.EnablePriceLimit
	}
	if req.EnableSuspension != nil {
		constraints.EnableSuspension = *req.EnableSuspension
	}
	if req.Board != "" {
		constraints.Board = trading.Board(req.Board)
	}

	costModel := trading.DefaultCostModel()
	if req.CommissionRate > 0 {
		costModel.CommissionRate = req.CommissionRate
	}
	if req.SlippageBps > 0 {
		costModel.SlippageBps = req.SlippageBps
	}
	if req.StampDutyRate > 0 {
		costModel.StampDutyRate = req.StampDutyRate
	}

	run, err := s.ledger.NewForwardRun(req.ParadigmVersionID, startDate, req.InitialCash, constraints, costModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, forwardRunResponse{Run: run})
}

func (s *Server) handleForwardRunGet(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	id := c.Param("id")
	run, err := s.ledger.GetRun(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, forwardRunResponse{Run: run})
}

type forwardRunExecuteRequest struct {
	FromDate string `json:"from_date,omitempty"`
	ToDate   string `json:"to_date,omitempty"`
	SignalID string `json:"signal_id,omitempty"` // 执行单条信号
}

type forwardRunExecuteResponse struct {
	Executed int              `json:"executed"`
	Rejected int              `json:"rejected"`
	Results  []*ledger.ExecutionRecord `json:"results,omitempty"`
}

func (s *Server) handleForwardRunExecute(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")

	run, err := s.ledger.GetRun(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req forwardRunExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body = 执行全部
	}

	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	engine, err := ledger.NewPaperTradeEngine(s.ledger, runID, constraints, costModel, run.InitialCash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var executed, rejected int

	// 单条信号执行
	if req.SignalID != "" {
		entry, err := s.ledger.GetSignal(req.SignalID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		exec, err := engine.ExecuteSignal(entry)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if exec.Status == "rejected" {
			rejected++
		} else {
			executed++
		}
		c.JSON(http.StatusOK, forwardRunExecuteResponse{
			Executed: executed,
			Rejected: rejected,
			Results:  []*ledger.ExecutionRecord{exec},
		})
		return
	}

	// 按日期范围执行
	if req.FromDate != "" && req.ToDate != "" {
		from, ferr := time.Parse("2006-01-02", req.FromDate)
		to, terr := time.Parse("2006-01-02", req.ToDate)
		if ferr != nil || terr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
			return
		}
		executed, rejected, err = engine.ExecuteByDate(from, to)
	} else {
		executed, rejected, err = engine.ExecuteAllPending()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, forwardRunExecuteResponse{
		Executed: executed,
		Rejected: rejected,
	})
}

func (s *Server) handleForwardRunFinalize(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")

	endDate := time.Now()
	if d := c.Query("end_date"); d != "" {
		if parsed, err := time.Parse("2006-01-02", d); err == nil {
			endDate = parsed
		}
	}

	run, err := s.ledger.FinalizeRun(runID, endDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, forwardRunResponse{Run: run})
}

func (s *Server) handleForwardRunSignals(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")
	entries := s.ledger.ListByRun(runID)
	c.JSON(http.StatusOK, gin.H{"signals": entries, "total": len(entries)})
}

// ============================================================================
// Signal Handlers
// ============================================================================

type forwardSignalAppendRequest struct {
	ID                string            `json:"id"`
	ParadigmVersionID string            `json:"paradigm_version_id"`
	StockCode         string            `json:"stock_code"`
	Direction         string            `json:"direction"`
	SignalDate        string            `json:"signal_date"`
	ExecutionDate     string            `json:"execution_date"`
	Price             float64           `json:"price"`
	PreClose          float64           `json:"pre_close"`
	LimitUp           float64           `json:"limit_up"`
	LimitDown         float64           `json:"limit_down"`
	Suspended         bool              `json:"suspended"`
	Board             string            `json:"board"`
	Confidence        float64           `json:"confidence"`
	DataSnapshot      map[string]interface{} `json:"data_snapshot"`
	Source            map[string]interface{} `json:"source"`
}

func (s *Server) handleForwardSignalAppend(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")

	var req forwardSignalAppendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	signalDate, err := time.Parse("2006-01-02", req.SignalDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signal_date"})
		return
	}

	execDate := signalDate.AddDate(0, 0, 1)
	if req.ExecutionDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.ExecutionDate); err == nil {
			execDate = parsed
		}
	}

	entry := ledger.SignalEntry{
		ID:                req.ID,
		RunID:             runID,
		ParadigmVersionID: req.ParadigmVersionID,
		StockCode:         req.StockCode,
		Direction:         req.Direction,
		SignalDate:        signalDate,
		ExecutionDate:     execDate,
		Price:             req.Price,
		PreClose:          req.PreClose,
		LimitUp:           req.LimitUp,
		LimitDown:         req.LimitDown,
		Suspended:         req.Suspended,
		Board:             req.Board,
		Confidence:        req.Confidence,
	}

	// 处理 data_snapshot
	if req.DataSnapshot != nil {
		if ds, ok := req.DataSnapshot["dataset_id"].(string); ok {
			entry.DataSnapshot.DatasetID = ds
		}
		if fs, ok := req.DataSnapshot["feature_set_id"].(string); ok {
			entry.DataSnapshot.FeatureSetID = fs
		}
		if rs, ok := req.DataSnapshot["rule_set_id"].(string); ok {
			entry.DataSnapshot.RuleSetID = rs
		}
		if dh, ok := req.DataSnapshot["data_hash"].(string); ok {
			entry.DataSnapshot.DataHash = dh
		}
		if ca, ok := req.DataSnapshot["captured_at"].(string); ok {
			if t, err := time.Parse("2006-01-02T15:04:05Z", ca); err == nil {
				entry.DataSnapshot.CapturedAt = t
			}
		}
	}

	// 处理 source
	if req.Source != nil {
		if rid, ok := req.Source["rule_id"].(string); ok {
			entry.Source.RuleID = rid
		}
		if rd, ok := req.Source["rule_desc"].(string); ok {
			entry.Source.RuleDesc = rd
		}
		if tb, ok := req.Source["triggered_by"].(string); ok {
			entry.Source.TriggeredBy = tb
		}
	}

	if err := s.ledger.AppendSignal(entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"signal": entry})
}

func (s *Server) handleForwardSignalGet(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	id := c.Param("id")
	entry, err := s.ledger.GetSignal(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"signal": entry})
}

func (s *Server) handleForwardSignalsList(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}

	var entries []ledger.SignalEntry

	if versionID := c.Query("paradigm_version_id"); versionID != "" {
		entries = s.ledger.ListByParadigm(versionID)
	} else if runID := c.Query("run_id"); runID != "" {
		entries = s.ledger.ListByRun(runID)
	} else if dateStr := c.Query("date"); dateStr != "" {
		if date, err := time.Parse("2006-01-02", dateStr); err == nil {
			entries = s.ledger.ListByDate(date)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "one of paradigm_version_id, run_id, or date is required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"signals": entries, "total": len(entries)})
}

// ============================================================================
// Equity Curve Handler
// ============================================================================

func (s *Server) handleForwardRunEquity(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")
	run, err := s.ledger.GetRun(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	constraints := trading.DefaultTradingConstraints()
	costModel := trading.DefaultCostModel()

	engine, err := ledger.NewPaperTradeEngine(s.ledger, runID, constraints, costModel, run.InitialCash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	curve := engine.GetEquityCurve()
	c.JSON(http.StatusOK, gin.H{
		"run":  run,
		"curve": curve,
	})
}

// ============================================================================
// Comparison Handler
// ============================================================================

type forwardRunCompareRequest struct {
	TheoreticalReturn   float64 `json:"theoretical_return"`
	TheoreticalMaxDD    float64 `json:"theoretical_max_drawdown"`
	TheoreticalSharpe   float64 `json:"theoretical_sharpe"`
	TheoreticalWinRate  float64 `json:"theoretical_win_rate"`
	TheoreticalSignals  int     `json:"theoretical_signals"`
	TheoreticalAnnualRet float64 `json:"theoretical_annualized_return"`
}

func (s *Server) handleForwardRunCompare(c *gin.Context) {
	if s.ledger == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ledger not initialized"})
		return
	}
	runID := c.Param("id")
	run, err := s.ledger.GetRun(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req forwardRunCompareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	theoretical := ledger.TheoreticalMetrics{
		TotalReturn:   req.TheoreticalReturn,
		AnnualizedRet: req.TheoreticalAnnualRet,
		MaxDrawdown:   req.TheoreticalMaxDD,
		SharpeRatio:   req.TheoreticalSharpe,
		WinRate:       req.TheoreticalWinRate,
		SignalCount:   req.TheoreticalSignals,
		IdealPnL:      req.TheoreticalReturn * run.InitialCash,
	}

	entries := s.ledger.ListByRun(runID)
	report := ledger.NewComparisonReport(run.ParadigmVersionID, runID, theoretical, run, entries)

	// 检查阈值 (默认 20% 收益差距, 10% 回撤差距)
	ok, warnings := report.ValidateGapThreshold(0.20, 0.10)

	c.JSON(http.StatusOK, gin.H{
		"report":   report,
		"pass":     ok,
		"warnings": warnings,
	})
}
