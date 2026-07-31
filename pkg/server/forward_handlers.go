package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/backtest"
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
	Executed int                       `json:"executed"`
	Rejected int                       `json:"rejected"`
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

	constraints, costModel := forwardExecutionConfig(run)

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
		if entry.RunID != runID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "signal does not belong to forward run"})
			return
		}
		market, err := s.captureForwardExecutionMarket(entry)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		exec, err := engine.ExecuteSignal(entry, market)
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
	loadMarket := func(entry ledger.SignalEntry) (ledger.ExecutionMarket, error) {
		return s.captureForwardExecutionMarket(entry)
	}
	if req.FromDate != "" && req.ToDate != "" {
		from, ferr := time.Parse("2006-01-02", req.FromDate)
		to, terr := time.Parse("2006-01-02", req.ToDate)
		if ferr != nil || terr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
			return
		}
		executed, rejected, err = engine.ExecuteByDate(from, to, loadMarket)
	} else {
		executed, rejected, err = engine.ExecuteAllPending(loadMarket)
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
	ID                string              `json:"id"`
	ParadigmVersionID string              `json:"paradigm_version_id"`
	StockCode         string              `json:"stock_code"`
	Direction         string              `json:"direction"`
	SignalDate        string              `json:"signal_date"`
	Confidence        float64             `json:"confidence"`
	Source            ledger.SignalSource `json:"source"`
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
	run, err := s.ledger.GetRun(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if run.Status != "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "forward run is not active"})
		return
	}
	if req.ParadigmVersionID == "" {
		req.ParadigmVersionID = run.ParadigmVersionID
	}
	if req.ParadigmVersionID != run.ParadigmVersionID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paradigm_version_id does not match run"})
		return
	}
	req.StockCode = strings.TrimSpace(req.StockCode)
	if req.StockCode == "" || (req.Direction != "buy" && req.Direction != "sell") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stock_code and direction (buy/sell) are required"})
		return
	}

	signalDate, err := time.Parse("2006-01-02", req.SignalDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signal_date"})
		return
	}

	if existing, getErr := s.ledger.GetSignal(req.ID); getErr == nil {
		if signalRequestMatches(existing, req, runID, signalDate) {
			c.JSON(http.StatusOK, gin.H{"signal": existing})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "signal ID already exists with different immutable content"})
		return
	}

	market, dataHash, err := s.captureForwardSignalMarket(req.StockCode, signalDate)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	entry := ledger.SignalEntry{
		ID:                req.ID,
		RunID:             runID,
		ParadigmVersionID: req.ParadigmVersionID,
		StockCode:         req.StockCode,
		Direction:         req.Direction,
		SignalDate:        signalDate,
		Price:             market.Close,
		PreClose:          market.PreClose,
		LimitUp:           market.LimitUp,
		LimitDown:         market.LimitDown,
		Suspended:         market.Suspended,
		Board:             market.Board,
		Market:            market,
		Confidence:        req.Confidence,
		DataSnapshot: ledger.DataSnapshot{
			DatasetID: fmt.Sprintf("kline:%s:9:%s:%s", req.StockCode,
				signalDate.Format("20060102"), signalDate.Format("20060102")),
			RuleSetID: req.ParadigmVersionID, DataHash: dataHash, CapturedAt: time.Now().UTC(),
		},
		Source: req.Source,
	}

	if err := s.ledger.AppendSignal(entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	persisted, err := s.ledger.GetSignal(entry.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"signal": persisted})
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

	c.JSON(http.StatusOK, gin.H{
		"run":   run,
		"curve": run.EquityCurve,
	})
}

func forwardExecutionConfig(run *ledger.ForwardRun) (trading.TradingConstraints, trading.CostModel) {
	constraints := trading.DefaultTradingConstraints()
	constraints.EnableT1 = run.ConstraintsSnapshot.EnableT1
	constraints.EnablePriceLimit = run.ConstraintsSnapshot.EnablePriceLimit
	constraints.EnableSuspension = run.ConstraintsSnapshot.EnableSuspension
	constraints.Board = trading.Board(run.ConstraintsSnapshot.Board)
	constraints.MinTradeUnit = run.ConstraintsSnapshot.MinTradeUnit
	cost := trading.DefaultCostModel()
	cost.CommissionRate = run.ConstraintsSnapshot.CommissionRate
	cost.MinCommission = run.ConstraintsSnapshot.MinCommission
	cost.SlippageBps = run.ConstraintsSnapshot.SlippageBps
	cost.StampDutyRate = run.ConstraintsSnapshot.StampDutyRate
	cost.TransferFeeRate = run.ConstraintsSnapshot.TransferFeeRate
	return constraints, cost
}

func signalRequestMatches(
	existing ledger.SignalEntry,
	req forwardSignalAppendRequest,
	runID string,
	signalDate time.Time,
) bool {
	left, _ := json.Marshal(existing.Source)
	right, _ := json.Marshal(req.Source)
	return existing.RunID == runID &&
		existing.ParadigmVersionID == req.ParadigmVersionID &&
		existing.StockCode == req.StockCode &&
		existing.Direction == req.Direction &&
		existing.SignalDate.Equal(signalDate) &&
		existing.Confidence == req.Confidence &&
		string(left) == string(right)
}

func (s *Server) captureForwardSignalMarket(
	code string,
	signalDate time.Time,
) (ledger.ExecutionMarket, string, error) {
	if s.storage == nil {
		return ledger.ExecutionMarket{}, "", fmt.Errorf("server storage is required for real market capture")
	}
	type row struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	var signal row
	if err := s.storage.DB().QueryRow(`SELECT date, open, high, low, close, volume, amount
		FROM kline WHERE code=? AND ktype=9 AND REPLACE(date, '-', '')=?
		LIMIT 1`, code, signalDate.Format("20060102")).
		Scan(&signal.date, &signal.open, &signal.high, &signal.low, &signal.close,
			&signal.volume, &signal.amount); err != nil {
		return ledger.ExecutionMarket{}, "", fmt.Errorf("real signal-date K-line unavailable for %s on %s: %w",
			code, signalDate.Format("2006-01-02"), err)
	}
	var previousClose float64
	_ = s.storage.DB().QueryRow(`SELECT close FROM kline
		WHERE code=? AND ktype=9 AND REPLACE(date, '-', '')<?
		ORDER BY REPLACE(date, '-', '') DESC LIMIT 1`,
		code, signalDate.Format("20060102")).Scan(&previousClose)
	if previousClose <= 0 {
		previousClose = signal.close
	}
	board := backtest.BoardForCode(code)
	limitUp, limitDown := trading.CalculateLimits(previousClose, board)
	market := ledger.ExecutionMarket{
		Date: signalDate, Open: signal.open, High: signal.high, Low: signal.low,
		Close: signal.close, PreClose: previousClose, Volume: signal.volume,
		Amount: signal.amount, LimitUp: limitUp, LimitDown: limitDown,
		Suspended: signal.volume <= 0 || signal.open <= 0 || signal.high <= 0 ||
			signal.low <= 0 || signal.close <= 0,
		Board: string(board),
	}
	payload, _ := json.Marshal(struct {
		Code   string `json:"code"`
		Signal row    `json:"signal"`
	}{Code: code, Signal: signal})
	sum := sha256.Sum256(payload)
	return market, hex.EncodeToString(sum[:]), nil
}

func (s *Server) captureForwardExecutionMarket(entry ledger.SignalEntry) (ledger.ExecutionMarket, error) {
	if s.storage == nil {
		return ledger.ExecutionMarket{}, fmt.Errorf("server storage is required for real market capture")
	}
	code, signalDate := entry.StockCode, entry.SignalDate
	type row struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	signalClose := entry.Price
	if signalClose <= 0 {
		return ledger.ExecutionMarket{}, fmt.Errorf("frozen signal close is invalid for %s on %s",
			code, signalDate.Format("2006-01-02"))
	}
	var execution row
	if err := s.storage.DB().QueryRow(`SELECT date, open, high, low, close, volume, amount
		FROM kline WHERE code=? AND ktype=9 AND REPLACE(date, '-', '')>?
		ORDER BY REPLACE(date, '-', '') LIMIT 1`, code, signalDate.Format("20060102")).
		Scan(&execution.date, &execution.open, &execution.high, &execution.low,
			&execution.close, &execution.volume, &execution.amount); err != nil {
		return ledger.ExecutionMarket{}, fmt.Errorf("next real trading bar unavailable for %s after %s: %w",
			code, signalDate.Format("2006-01-02"), err)
	}
	executionDate, err := time.Parse("20060102", strings.ReplaceAll(execution.date, "-", ""))
	if err != nil {
		return ledger.ExecutionMarket{}, fmt.Errorf("invalid persisted execution date: %w", err)
	}
	board := backtest.BoardForCode(code)
	limitUp, limitDown := trading.CalculateLimits(signalClose, board)
	market := ledger.ExecutionMarket{
		Date: executionDate, Open: execution.open, High: execution.high,
		Low: execution.low, Close: execution.close, PreClose: signalClose,
		Volume: execution.volume, Amount: execution.amount,
		LimitUp: limitUp, LimitDown: limitDown,
		Suspended: execution.volume <= 0 || execution.open <= 0 ||
			execution.high <= 0 || execution.low <= 0 || execution.close <= 0,
		Board: string(board),
	}
	return market, nil
}

// ============================================================================
// Comparison Handler
// ============================================================================

type forwardRunCompareRequest struct {
	TheoreticalReturn    float64 `json:"theoretical_return"`
	TheoreticalMaxDD     float64 `json:"theoretical_max_drawdown"`
	TheoreticalSharpe    float64 `json:"theoretical_sharpe"`
	TheoreticalWinRate   float64 `json:"theoretical_win_rate"`
	TheoreticalSignals   int     `json:"theoretical_signals"`
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
