package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
)

func TestSplitMonitoringSeriesKeepsRecentWindowAsForward(t *testing.T) {
	points := make([]monitoringPoint, 0, 400)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 400; i++ {
		points = append(points, monitoringPoint{
			Date: base.AddDate(0, 0, i),
			Ret:  float64(i) / 10000,
		})
	}

	split := splitMonitoringSeries(points, 60)

	if len(split.Forward) != monitoringMinForwardPoints+40 {
		t.Fatalf("forward points=%d, want 60", len(split.Forward))
	}
	if len(split.ForwardDates) != len(split.Forward) {
		t.Fatalf("forward dates=%d, want %d", len(split.ForwardDates), len(split.Forward))
	}
	if len(split.Baseline) != monitoringBaselineMaxPoints {
		t.Fatalf("baseline points=%d, want %d", len(split.Baseline), monitoringBaselineMaxPoints)
	}
	// 前向窗口必须是序列末尾, 基准窗口紧挨其前
	if got := split.Forward[0]; got != float64(340)/10000 {
		t.Fatalf("first forward return=%v, want %v", got, float64(340)/10000)
	}
	if got := split.Baseline[len(split.Baseline)-1]; got != float64(339)/10000 {
		t.Fatalf("last baseline return=%v, want %v", got, float64(339)/10000)
	}
	if !split.ForwardDates[0].Equal(points[340].Date) {
		t.Fatalf("forward start=%v, want %v", split.ForwardDates[0], points[340].Date)
	}
}

func TestSplitMonitoringSeriesRefusesThinSeries(t *testing.T) {
	points := make([]monitoringPoint, 0, 50)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 50; i++ {
		points = append(points, monitoringPoint{Date: base.AddDate(0, 0, i), Ret: 0.001})
	}

	split := splitMonitoringSeries(points, 60)
	if len(split.Forward) != 0 || len(split.Baseline) != 0 {
		t.Fatalf("thin series must not produce a split: %+v", split)
	}
}

func TestBuildMonitoringInputReportsMissingSources(t *testing.T) {
	s := NewServer(Dependencies{})

	_, status, err := s.buildMonitoringInput(context.Background())
	if err == nil {
		t.Fatal("expected error when no observation source exists")
	}
	if !strings.Contains(err.Error(), "真实观测") {
		t.Fatalf("error should explain real observations are missing: %v", err)
	}
	if status.Source != "none" {
		t.Fatalf("source=%q, want none", status.Source)
	}

	joined := strings.Join(status.Notes, " | ")
	for _, want := range []string{"前向", "交易", "自选"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("notes should mention %s: %s", want, joined)
		}
	}
}

func TestMonitoringRefreshRefusesToInventMissingObservations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewServer(Dependencies{})
	router := gin.New()
	router.Use(RequestID(), ErrorEnvelopeMiddleware(), Recovery())
	api.registerMonitoringRoutes(&router.RouterGroup)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/monitoring/report/refresh", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var body struct {
		Available bool                  `json:"available"`
		Error     APIError              `json:"error"`
		Input     MonitoringInputStatus `json:"input"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Available {
		t.Fatalf("empty inputs must not report available: %s", response.Body.String())
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code=%q, want not_found (fail-closed envelope)", body.Error.Code)
	}
	if body.Input.Source != "none" || len(body.Input.Notes) == 0 {
		t.Fatalf("diagnostics must be returned to the UI: %+v", body.Input)
	}
}

// TestMonitoringReportBuildsFromWatchlistProxy 覆盖成功路径：
// 没有前向运行和真实交易时，用自选股的真实日线构造等权组合收益，
// 切出前向/基准两个窗口后生成完整报告。
func TestMonitoringReportBuildsFromWatchlistProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store, err := storage.New(storage.Config{
		Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "monitoring.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	watchlistStore, err := watchlist.New(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, stock := range []watchlist.WatchlistStock{
		{Code: "600000", Name: "浦发银行"},
		{Code: "000001", Name: "平安银行"},
	} {
		if err := watchlistStore.Upsert(stock); err != nil {
			t.Fatal(err)
		}
	}

	service, err := stockdata.NewService(
		contractRepository{
			coverage: stockdata.Coverage{Exists: true, SourceUpdatedAt: time.Now()},
			dataset:  stockdata.Dataset{Klines: monitoringFixtureKlines(400)},
		},
		contractProvider{},
		contractFreshnessPolicy{},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	api := NewServer(Dependencies{UnifiedData: service, Watchlist: watchlistStore})
	router := gin.New()
	api.registerMonitoringRoutes(&router.RouterGroup)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/monitoring/report/refresh", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var body monitoringReportResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Input.Source != "watchlist_proxy" {
		t.Fatalf("source=%q, want watchlist_proxy (notes: %v)", body.Input.Source, body.Input.Notes)
	}
	if body.Input.ForwardCount != 60 || body.Input.BaselineCount != 180 {
		t.Fatalf("forward=%d baseline=%d, want 60/180", body.Input.ForwardCount, body.Input.BaselineCount)
	}
	if len(body.Input.Universe) != 2 || body.Input.PositionCount != 2 {
		t.Fatalf("universe=%v positions=%d, want 2 watchlist codes", body.Input.Universe, body.Input.PositionCount)
	}
	if body.Report.DriftSummary.TotalDetections == 0 || body.Report.DecaySummary.TotalDetections == 0 {
		t.Fatalf("expected drift and decay detections: drift=%d decay=%d",
			body.Report.DriftSummary.TotalDetections, body.Report.DecaySummary.TotalDetections)
	}
	if len(body.Report.ConcentrationResults) != 3 {
		t.Fatalf("concentration results=%d, want stock/industry/position", len(body.Report.ConcentrationResults))
	}
	if body.Report.HealthScore < 0 || body.Report.HealthScore > 100 {
		t.Fatalf("health score out of range: %v", body.Report.HealthScore)
	}
	if body.Report.Source != "watchlist_proxy" {
		t.Fatalf("report source=%q, want watchlist_proxy", body.Report.Source)
	}
}

// monitoringFixtureKlines 生成最近 calendarDays 个日历日内的工作日日线，
// 收益按固定序列波动，保证漂移/衰减检测有足够样本。
func monitoringFixtureKlines(calendarDays int) []*protocol.Kline {
	end := time.Now()
	start := end.AddDate(0, 0, -calendarDays)

	var klines []*protocol.Kline
	close := 10.0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		next := close * (1 + 0.012*float64(len(klines)%7-3)/3.0)
		klines = append(klines, &protocol.Kline{
			Time: day, Open: close, High: next * 1.01, Low: next * 0.99,
			Close: next, Volume: 1_000_000, Amount: 10_000_000,
		})
		close = next
	}
	return klines
}
