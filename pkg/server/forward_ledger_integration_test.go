package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/trading"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func TestForwardLedgerPersistsRealMarketAndAccountAcrossRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "forward-ledger.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bars := []struct {
		date                   string
		open, high, low, close float64
	}{
		{"20250102", 9.8, 10.2, 9.7, 10.0},
		{"20250103", 10.2, 10.4, 10.0, 10.3},
		{"20250106", 9.6, 9.8, 9.4, 9.5},
		{"20250107", 9.0, 9.2, 8.9, 9.0},
	}
	for _, bar := range bars {
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES ('600000', 9, ?, ?, ?, ?, ?, 100000, 1000000)`,
			bar.date, bar.open, bar.high, bar.low, bar.close); err != nil {
			t.Fatal(err)
		}
	}

	persistent, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	run, err := persistent.NewForwardRun(
		"pv-real", start, 100_000,
		trading.DefaultTradingConstraints(), trading.DefaultCostModel(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Append signals directly via the ledger Go API (the manual HTTP injection
	// endpoint has been deleted; signals are now appended by the deterministic
	// execution path, not by users).
	buyMarket, buySnap := fetchKlineBar(t, store, "600000", "20250102")
	buyEntry := ledger.SignalEntry{
		ID:                "sig-buy",
		RunID:             run.ID,
		ParadigmVersionID: "pv-real",
		StockCode:         "600000",
		Direction:         "buy",
		SignalDate:        start,
		Price:             buyMarket.Close,
		PreClose:          buyMarket.PreClose,
		LimitUp:           buyMarket.LimitUp,
		LimitDown:         buyMarket.LimitDown,
		Suspended:         buyMarket.Suspended,
		Board:             buyMarket.Board,
		Market:            buyMarket,
		DataSnapshot:      buySnap,
		Source:            ledger.SignalSource{RuleID: "real-rule", TriggeredBy: "close breakout"},
	}
	if err := persistent.AppendSignal(buyEntry); err != nil {
		t.Fatalf("append buy: %v", err)
	}

	sellMarket, sellSnap := fetchKlineBar(t, store, "600000", "20250106")
	sellEntry := ledger.SignalEntry{
		ID:                "sig-sell",
		RunID:             run.ID,
		ParadigmVersionID: "pv-real",
		StockCode:         "600000",
		Direction:         "sell",
		SignalDate:        time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		Price:             sellMarket.Close,
		PreClose:          sellMarket.PreClose,
		LimitUp:           sellMarket.LimitUp,
		LimitDown:         sellMarket.LimitDown,
		Suspended:         sellMarket.Suspended,
		Board:             sellMarket.Board,
		Market:            sellMarket,
		DataSnapshot:      sellSnap,
		Source:            ledger.SignalSource{RuleID: "real-rule"},
	}
	if err := persistent.AppendSignal(sellEntry); err != nil {
		t.Fatalf("append sell: %v", err)
	}

	router := forwardTestRouter(store, persistent)

	response := forwardJSONRequest(t, router, http.MethodPost,
		"/api/forward/runs/"+run.ID+"/execute", `{"signal_id":"sig-buy"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("execute buy: status=%d body=%s", response.Code, response.Body.String())
	}
	buySignal, err := persistent.GetSignal("sig-buy")
	if err != nil || buySignal.Execution == nil || buySignal.Execution.Status != "filled" {
		t.Fatalf("buy was not recorded: signal=%+v err=%v", buySignal, err)
	}
	if buySignal.Execution.Market.Open != 10.2 ||
		!buySignal.Execution.Market.Date.Equal(time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("execution did not use next real bar: %+v", buySignal.Execution.Market)
	}
	staleLedger, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	staleRouter := forwardTestRouter(store, staleLedger)
	staleResponse := forwardJSONRequest(t, staleRouter, http.MethodPost,
		"/api/forward/runs/"+run.ID+"/execute", `{"signal_id":"sig-buy"}`)
	if staleResponse.Code == http.StatusOK {
		t.Fatalf("stale service overwrote an already recorded execution: %s", staleResponse.Body.String())
	}
	cashAfterBuy, err := persistent.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cashAfterBuy.FinalCash >= cashAfterBuy.InitialCash || len(cashAfterBuy.Positions) != 1 {
		t.Fatalf("buy account state not recorded: %+v", cashAfterBuy)
	}

	// Simulate a fresh service process: reconstruct both ledger and engine from SQLite.
	reloaded, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	router = forwardTestRouter(store, reloaded)
	response = forwardJSONRequest(t, router, http.MethodPost,
		"/api/forward/runs/"+run.ID+"/execute", `{"signal_id":"sig-sell"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("execute sell after restart: status=%d body=%s", response.Code, response.Body.String())
	}

	sellSignal, err := reloaded.GetSignal("sig-sell")
	if err != nil || sellSignal.Execution == nil || sellSignal.Execution.Status != "filled" {
		t.Fatalf("sell was not recorded: signal=%+v err=%v", sellSignal, err)
	}
	buyExec := buySignal.Execution
	sellExec := sellSignal.Execution
	wantGross := (sellExec.ExecPrice - buyExec.ExecPrice) * float64(sellExec.ExecQty)
	wantNet := wantGross - buyExec.Fee - sellExec.Fee
	if math.Abs(sellExec.GrossPnL-wantGross) > 1e-9 ||
		math.Abs(sellExec.PnL-wantNet) > 1e-9 {
		t.Fatalf("cost-basis PnL mismatch: got gross=%f net=%f want gross=%f net=%f",
			sellExec.GrossPnL, sellExec.PnL, wantGross, wantNet)
	}

	finalRun, err := reloaded.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalRun.FinalCash == run.InitialCash || len(finalRun.Positions) != 0 {
		t.Fatalf("restart reset cash or failed to close position: %+v", finalRun)
	}
	if len(finalRun.EquityCurve) != 3 || finalRun.MaxDrawdown <= 0 {
		t.Fatalf("equity/drawdown not persisted: curve=%+v drawdown=%f",
			finalRun.EquityCurve, finalRun.MaxDrawdown)
	}

	again, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := again.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedRun.SignalCount != 2 || persistedRun.ExecutedCount != 2 ||
		len(again.ListByRun(run.ID)) != 2 ||
		math.Abs(persistedRun.MaxDrawdown-finalRun.MaxDrawdown) > 1e-12 {
		t.Fatalf("second restart lost ledger state: %+v", persistedRun)
	}
}

// This opt-in test copies four actual daily bars from the user's read-only
// TongStock database into an isolated ledger database. No market row is mocked
// or synthesized, and the source database is never migrated or modified.
func TestForwardLedgerAgainstRealDatabase(t *testing.T) {
	path := os.Getenv("TONGSTOCK_REAL_DB")
	if path == "" {
		t.Skip("set TONGSTOCK_REAL_DB to run against the local real-data database")
	}
	source, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	var code string
	if err := source.QueryRow(`SELECT code FROM kline
		WHERE ktype=9 AND length(code)=6 AND code<>'999999'
			AND open>0 AND high>0 AND low>0 AND close>0 AND volume>0
			AND length(REPLACE(date, '-', ''))=8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		GROUP BY code HAVING COUNT(*)>=4
		ORDER BY MAX(REPLACE(date, '-', '')) DESC, COUNT(*) DESC LIMIT 1`).Scan(&code); err != nil {
		t.Fatal(err)
	}
	rows, err := source.Query(`SELECT date, open, high, low, close, volume, amount FROM (
		SELECT date, open, high, low, close, volume, amount FROM kline
		WHERE code=? AND ktype=9 AND open>0 AND high>0 AND low>0 AND close>0 AND volume>0
			AND length(REPLACE(date, '-', ''))=8
			AND REPLACE(date, '-', '') BETWEEN '19900101' AND '20991231'
		ORDER BY REPLACE(date, '-', '') DESC LIMIT 4)
		ORDER BY REPLACE(date, '-', '')`, code)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type realBar struct {
		date                   string
		open, high, low, close float64
		volume, amount         float64
	}
	var bars []realBar
	for rows.Next() {
		var bar realBar
		if err := rows.Scan(&bar.date, &bar.open, &bar.high, &bar.low, &bar.close,
			&bar.volume, &bar.amount); err != nil {
			t.Fatal(err)
		}
		bars = append(bars, bar)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(bars) != 4 {
		t.Fatalf("real bar count=%d, want 4", len(bars))
	}

	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "real-forward.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, bar := range bars {
		if _, err := store.DB().Exec(`INSERT INTO kline
			(code, ktype, date, open, high, low, close, volume, amount)
			VALUES (?, 9, ?, ?, ?, ?, ?, ?, ?)`, code, bar.date, bar.open,
			bar.high, bar.low, bar.close, bar.volume, bar.amount); err != nil {
			t.Fatal(err)
		}
	}
	parseDate := func(raw string) time.Time {
		value, err := time.Parse("20060102", strings.ReplaceAll(raw, "-", ""))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	signalLedger, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := signalLedger.NewForwardRun("pv-real-db", parseDate(bars[0].date),
		100_000, trading.DefaultTradingConstraints(), trading.DefaultCostModel())
	if err != nil {
		t.Fatal(err)
	}

	appendSignal := func(id, direction, date string) {
		market, snap := fetchKlineBar(t, store, code, strings.ReplaceAll(date, "-", ""))
		entry := ledger.SignalEntry{
			ID:                id,
			RunID:             run.ID,
			ParadigmVersionID: "pv-real-db",
			StockCode:         code,
			Direction:         direction,
			SignalDate:        parseDate(date),
			Price:             market.Close,
			PreClose:          market.PreClose,
			LimitUp:           market.LimitUp,
			LimitDown:         market.LimitDown,
			Suspended:         market.Suspended,
			Board:             market.Board,
			Market:            market,
			DataSnapshot:      snap,
			Source:            ledger.SignalSource{RuleID: "real-rule"},
		}
		if err := signalLedger.AppendSignal(entry); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
	appendSignal("real-buy", "buy", bars[0].date)
	appendSignal("real-sell", "sell", bars[2].date)

	router := forwardTestRouter(store, signalLedger)
	response := forwardJSONRequest(t, router, http.MethodPost,
		"/api/forward/runs/"+run.ID+"/execute", `{"signal_id":"real-buy"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("execute real buy: status=%d body=%s", response.Code, response.Body.String())
	}
	reloaded, err := ledger.NewSQLiteSignalLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	router = forwardTestRouter(store, reloaded)
	response = forwardJSONRequest(t, router, http.MethodPost,
		"/api/forward/runs/"+run.ID+"/execute", `{"signal_id":"real-sell"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("execute real sell after restart: status=%d body=%s", response.Code, response.Body.String())
	}
	sell, err := reloaded.GetSignal("real-sell")
	if err != nil || sell.Execution == nil {
		t.Fatalf("load real sell: signal=%+v err=%v", sell, err)
	}
	if sell.Execution.Status != "filled" || sell.Execution.ExecQty <= 0 {
		t.Fatalf("real sell did not fill: %+v", sell.Execution)
	}
	if !sell.Execution.Market.Date.Equal(parseDate(bars[3].date)) ||
		math.Abs(sell.Execution.Market.Open-bars[3].open) > 1e-9 {
		t.Fatalf("real execution bar mismatch: got=%+v source=%+v", sell.Execution.Market, bars[3])
	}
	t.Logf("verified forward ledger code=%s signal=%s/%s execution=%s/%s net_pnl=%.2f",
		code, bars[0].date, bars[2].date, bars[1].date, bars[3].date, sell.Execution.PnL)
}

func forwardTestRouter(store *storage.Storage, signalLedger *ledger.SignalLedger) *gin.Engine {
	router := gin.New()
	NewServer(Dependencies{Storage: store, Ledger: signalLedger}).SetupRoutes(router)
	return router
}

func forwardJSONRequest(
	t *testing.T,
	router http.Handler,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// fetchKlineBar reads a real daily K-line bar from the test database and
// builds an ExecutionMarket with limit-up/down and suspension state, plus
// the data hash required by the ledger's integrity check.
func fetchKlineBar(t *testing.T, store *storage.Storage, code, dateStr string) (ledger.ExecutionMarket, ledger.DataSnapshot) {
	t.Helper()
	var open, high, low, close, volume, amount float64
	var dateRaw string
	if err := store.DB().QueryRow(`SELECT date, open, high, low, close, volume, amount
		FROM kline WHERE code=? AND ktype=9 AND REPLACE(date, '-', '')=?
		LIMIT 1`, code, dateStr).
		Scan(&dateRaw, &open, &high, &low, &close, &volume, &amount); err != nil {
		t.Fatalf("fetch kline bar %s %s: %v", code, dateStr, err)
	}
	var previousClose float64
	_ = store.DB().QueryRow(`SELECT close FROM kline
		WHERE code=? AND ktype=9 AND REPLACE(date, '-', '')<?
		ORDER BY REPLACE(date, '-', '') DESC LIMIT 1`,
		code, dateStr).Scan(&previousClose)
	if previousClose <= 0 {
		previousClose = close
	}
	signalDate, err := time.Parse("20060102", strings.ReplaceAll(dateRaw, "-", ""))
	if err != nil {
		t.Fatalf("parse signal date %s: %v", dateRaw, err)
	}
	board := backtest.BoardForCode(code)
	limitUp, limitDown := trading.CalculateLimits(previousClose, board)
	market := ledger.ExecutionMarket{
		Date:      signalDate,
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		PreClose:  previousClose,
		Volume:    volume,
		Amount:    amount,
		LimitUp:   limitUp,
		LimitDown: limitDown,
		Suspended: volume <= 0 || open <= 0 || high <= 0 || low <= 0 || close <= 0,
		Board:     string(board),
	}
	payload, _ := json.Marshal(struct {
		Code string `json:"code"`
		Bar  struct {
			Date                   string  `json:"date"`
			Open, High, Low, Close float64 `json:"open,high,low,close"`
			Volume, Amount         float64 `json:"volume,amount"`
		} `json:"bar"`
	}{Code: code, Bar: struct {
		Date                   string  `json:"date"`
		Open, High, Low, Close float64 `json:"open,high,low,close"`
		Volume, Amount         float64 `json:"volume,amount"`
	}{Date: dateRaw, Open: open, High: high, Low: low, Close: close, Volume: volume, Amount: amount}})
	sum := sha256.Sum256(payload)
	snapshot := ledger.DataSnapshot{
		DatasetID:  "kline:" + code + ":9:" + dateStr + ":" + dateStr,
		DataHash:   hex.EncodeToString(sum[:]),
		CapturedAt: time.Now().UTC(),
	}
	return market, snapshot
}
