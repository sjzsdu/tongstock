package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/paradigms"
)

func TestHandleParadigmCreateSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	store, err := paradigms.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(Dependencies{})
	s.SetParadigmStore(store)
	s.SetupParadigmRoutes(&router.RouterGroup)

	payload := paradigmCreateRequest{
		Name:      "测试多头突破",
		Side:      "buy",
		StockCode: "600519",
		StockName: "贵州茅台",
		Logic:     "多头排列突破关键阻力位",
		Rationale: "趋势延续",
		BuyConditions: []paradigms.Condition{
			{Indicator: "MA5", Operator: "gt", Value: "MA20"},
		},
		SellConditions: paradigms.SellConditions{
			TakeProfit: []paradigms.Condition{
				{Indicator: "RSI14", Operator: "gt", Value: "70"},
			},
		},
		Confirmations: []string{"成交量放大"},
		Invalidations: []string{"跌破 MA60"},
		Expectation: paradigms.Expectation{
			HoldingPeriod:  "10日",
			ExpectedReturn: "10%",
			RiskReward:     "2:1",
			Confidence:     0.75,
		},
		Tags: []string{"trend", "breakout"},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/paradigm/hypothesis", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp paradigmCreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Valid {
		t.Fatalf("expected valid=true, got errors: %v", resp.Errors)
	}

	if resp.Paradigm == nil {
		t.Fatal("expected paradigm to be set")
	}

	if resp.Paradigm.ID == "" {
		t.Error("paradigm ID should be set")
	}
	if resp.Paradigm.Name != "测试多头突破" {
		t.Errorf("paradigm name = %q, want %q", resp.Paradigm.Name, "测试多头突破")
	}
	if resp.Paradigm.StockCode != "600519" {
		t.Errorf("stock code = %q, want %q", resp.Paradigm.StockCode, "600519")
	}
	if len(resp.Paradigm.Invalid) != 1 {
		t.Errorf("invalidations count = %d, want 1", len(resp.Paradigm.Invalid))
	}
	if resp.Paradigm.ReviewStatus != "pending" {
		t.Errorf("review status = %q, want %q", resp.Paradigm.ReviewStatus, "pending")
	}
}

func TestHandleParadigmCreateMissingFalsifiability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	store, err := paradigms.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(Dependencies{})
	s.SetParadigmStore(store)
	s.SetupParadigmRoutes(&router.RouterGroup)

	payload := paradigmCreateRequest{
		Name:      "无失效条件假设",
		Side:      "buy",
		StockCode: "000001",
		BuyConditions: []paradigms.Condition{
			{Indicator: "MA5", Operator: "gt", Value: "MA20"},
		},
		Invalidations: nil, // 关键: 缺少可证伪条件
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/paradigm/hypothesis", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp paradigmCreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Valid {
		t.Error("expected valid=false when missing falsifiability")
	}

	hasFalsifiabilityErr := false
	for _, e := range resp.Errors {
		if e == "缺少可证伪条件 (否定条件)，假设不能进入实验" {
			hasFalsifiabilityErr = true
			break
		}
	}
	if !hasFalsifiabilityErr {
		t.Errorf("expected falsifiability error, got: %v", resp.Errors)
	}
}

func TestHandleParadigmCreateMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	store, err := paradigms.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(Dependencies{})
	s.SetParadigmStore(store)
	s.SetupParadigmRoutes(&router.RouterGroup)

	// 缺少名称、方向、证券代码、买入条件、失效条件
	payload := paradigmCreateRequest{}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/paradigm/hypothesis", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp paradigmCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Valid {
		t.Error("expected valid=false")
	}

	expectedErrs := map[string]bool{
		"假设名称不能为空":                false,
		"方向必须为 buy 或 sell":        false,
		"必须选择目标证券":                false,
		"至少需要一个买入触发条件":            false,
		"缺少可证伪条件 (否定条件)，假设不能进入实验": false,
	}

	for _, e := range resp.Errors {
		if _, ok := expectedErrs[e]; ok {
			expectedErrs[e] = true
		}
	}

	for msg, found := range expectedErrs {
		if !found {
			t.Errorf("missing expected error: %s", msg)
		}
	}
}

func TestHandleParadigmCreateInvalidSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	store, err := paradigms.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(Dependencies{})
	s.SetParadigmStore(store)
	s.SetupParadigmRoutes(&router.RouterGroup)

	payload := paradigmCreateRequest{
		Name:      "测试",
		Side:      "hold", // 非法方向
		StockCode: "000001",
		BuyConditions: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "10"},
		},
		Invalidations: []string{"失效"},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/paradigm/hypothesis", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp paradigmCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Valid {
		t.Error("expected valid=false")
	}
}

func TestHandleParadigmCreatePersistsToStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	store, err := paradigms.NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(Dependencies{})
	s.SetParadigmStore(store)
	s.SetupParadigmRoutes(&router.RouterGroup)

	payload := paradigmCreateRequest{
		Name:      "持久化测试",
		Side:      "buy",
		StockCode: "000001",
		BuyConditions: []paradigms.Condition{
			{Indicator: "close", Operator: "gt", Value: "10"},
		},
		Invalidations: []string{"跌破 5 日均线"},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/paradigm/hypothesis", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp paradigmCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// 验证已经持久化到 store
	got, err := store.Get(resp.Paradigm.ID)
	if err != nil {
		t.Fatalf("paradigm not found in store: %v", err)
	}

	if got.Name != "持久化测试" {
		t.Errorf("got name = %q, want %q", got.Name, "持久化测试")
	}
	if len(got.Invalid) != 1 || got.Invalid[0] != "跌破 5 日均线" {
		t.Errorf("invalidations mismatch: %v", got.Invalid)
	}
}
