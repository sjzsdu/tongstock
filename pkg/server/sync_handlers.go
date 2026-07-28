package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

func (s *Server) handleSyncDaily(c *gin.Context) {
	var req struct {
		Codes       []string `json:"codes"`
		Mode        string   `json:"mode"`
		Concurrency int      `json:"concurrency"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize code list
	codes := normalizeCodeList(req.Codes)
	if len(codes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	mode := tdx.SyncMode(req.Mode)
	if mode == "" {
		mode = tdx.SyncModeAuto
	}

	// Check service availability first
	if s.svc == nil {
		log.Printf("[sync] 服务不可用: s.svc is nil")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务不可用: s.svc is nil"})
		return
	}

	result := s.svc.SyncDailyKlines(codes, mode, req.Concurrency)
	c.JSON(http.StatusOK, result)
}

// handleSyncState returns sync state for a given code without triggering a sync.
func (s *Server) handleSyncState(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ktypeStr := c.DefaultQuery("ktype", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	if s.stockData != nil {
		coverage, decision, err := s.stockData.InspectFreshness(c.Request.Context(), stockdata.DataSpec{
			Type: stockdata.DataKline, Market: marketForCode(code), Code: code,
			Granularity: ktypeStr, KType: ktype,
		})
		if err == nil {
			status := coverage.Status
			if status == "" && coverage.Exists {
				status = "ok"
			}
			freshness := "stale"
			if !coverage.Exists {
				freshness = "empty"
			} else if decision.Fresh {
				freshness = "fresh"
			} else if !coverage.LastSyncAt.IsZero() && time.Since(coverage.LastSyncAt) > 24*time.Hour {
				freshness = "outdated"
			}
			c.JSON(http.StatusOK, gin.H{
				"code": code, "ktype": ktype, "status": status,
				"first_date": formatSyncDate(coverage.Start), "last_date": formatSyncDate(coverage.End),
				"row_count": len(coverage.Points), "last_sync_at": formatSyncTime(coverage.LastSyncAt),
				"freshness": freshness, "stale_reason": decision.Reason,
			})
			return
		}
	}

	state, err := s.svc.GetSyncState(code, ktype)
	if err != nil {
		// No sync state record found — return empty state
		c.JSON(http.StatusOK, gin.H{
			"code":   code,
			"ktype":  ktype,
			"status": "unknown",
		})
		return
	}

	c.JSON(http.StatusOK, state)
}

func formatSyncDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("20060102")
}

func formatSyncTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

// handleSyncFreshness handles requests to get sync freshness for multiple codes
func (s *Server) handleSyncFreshness(c *gin.Context) {
	codesStr := strings.TrimSpace(c.Query("codes"))
	if codesStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "codes is required"})
		return
	}

	codes := strings.Split(codesStr, ",")
	ktype := tdx.ParseKlineType("day")

	type FreshnessResult struct {
		Code        string `json:"code"`
		Status      string `json:"status"`
		LastDate    string `json:"last_date,omitempty"`
		LastSyncAt  string `json:"last_sync_at,omitempty"`
		RowCount    int    `json:"row_count,omitempty"`
		Freshness   string `json:"freshness"` // fresh, stale, outdated, unknown
		StaleReason string `json:"stale_reason,omitempty"`
		Error       string `json:"error,omitempty"`
	}

	results := make([]FreshnessResult, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}

		state, err := s.svc.GetSyncState(code, ktype)
		if err != nil {
			results = append(results, FreshnessResult{
				Code:      code,
				Status:    "unknown",
				Freshness: "unknown",
			})
			continue
		}

		freshness, staleReason := evaluateFreshness(state)
		results = append(results, FreshnessResult{
			Code:        code,
			Status:      state.Status,
			LastDate:    state.LastDate,
			LastSyncAt:  state.LastSyncAt.Format(time.RFC3339),
			RowCount:    state.RowCount,
			Freshness:   freshness,
			StaleReason: staleReason,
			Error:       state.Error,
		})
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// evaluateFreshness evaluates the freshness of kline data based on sync state
func evaluateFreshness(state *tdx.KlineSyncState) (string, string) {
	if state.Status == "failed" || state.Error != "" {
		return "failed", state.Error
	}

	if state.LastDate == "" || state.RowCount == 0 {
		return "empty", "无数据"
	}

	now := time.Now()
	lastSync := state.LastSyncAt

	// Check if data is from before today (non-trading days handled)
	today := now.Format("20060102")
	if state.LastDate < today {
		// Check if last sync was recent
		syncAge := now.Sub(lastSync)
		if syncAge > 24*time.Hour {
			return "outdated", fmt.Sprintf("数据截止至 %s，超过24小时未同步", state.LastDate)
		}
		return "stale", fmt.Sprintf("数据截止至 %s", state.LastDate)
	}

	// Data is current
	return "fresh", ""
}

// handleCleanKlines handles requests to clean corrupted kline data and re-fetch.
func (s *Server) handleCleanKlines(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	// Clean corrupted data and re-fetch
	klines, err := s.svc.CleanAndRefetchKlines(code, ktype)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"count":   len(klines),
		"message": "数据清理完成并已重新获取",
	})
}

// handleIndicatorSettings handles indicator settings requests
func (s *Server) handleIndicatorSettings(c *gin.Context) {
	config, err := param.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, indicatorConfigToPayload(config))
}

// handleSaveIndicatorSettings handles save indicator settings requests
func (s *Server) handleSaveIndicatorSettings(c *gin.Context) {
	var payload indicatorParamPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	normalizeIndicatorPayload(&payload)
	if err := validateIndicatorPayload(payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := param.SaveConfig(payloadToIndicatorConfig(payload)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cfg, err := param.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok", "config": indicatorConfigToPayload(cfg)})
}

// indicatorParamPayload 是指标参数字段负载结构
type indicatorParamPayload struct {
	Defaults   indicatorCategoryPayload            `json:"defaults" yaml:"defaults"`
	Categories map[string]indicatorCategoryPayload `json:"categories,omitempty" yaml:"categories,omitempty"`
	Overrides  map[string]indicatorCategoryPayload `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	Path       string                              `json:"path,omitempty"`
}

// indicatorCategoryPayload 是指标分类参数字段负载
type indicatorCategoryPayload struct {
	MA   []int          `json:"ma,omitempty" yaml:"ma,omitempty"`
	MACD *ta.MACDConfig `json:"macd,omitempty" yaml:"macd,omitempty"`
	KDJ  *ta.KDJConfig  `json:"kdj,omitempty" yaml:"kdj,omitempty"`
	BOLL *ta.BOLLConfig `json:"boll,omitempty" yaml:"boll,omitempty"`
	RSI  []int          `json:"rsi,omitempty" yaml:"rsi,omitempty"`
}

// indicatorCategoryToPayload 将 CategoryParams 转换为 indicatorCategoryPayload
func indicatorCategoryToPayload(src param.CategoryParams) indicatorCategoryPayload {
	return indicatorCategoryPayload{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

// indicatorConfigToPayload 将 ParamConfig 转换为 indicatorParamPayload
func indicatorConfigToPayload(cfg *param.ParamConfig) indicatorParamPayload {
	payload := indicatorParamPayload{
		Defaults: indicatorCategoryToPayload(cfg.Defaults),
	}
	if len(cfg.Categories) > 0 {
		payload.Categories = make(map[string]indicatorCategoryPayload, len(cfg.Categories))
		for key, value := range cfg.Categories {
			payload.Categories[key] = indicatorCategoryToPayload(value)
		}
	}
	if len(cfg.Overrides) > 0 {
		payload.Overrides = make(map[string]indicatorCategoryPayload, len(cfg.Overrides))
		for key, value := range cfg.Overrides {
			payload.Overrides[key] = indicatorCategoryToPayload(value)
		}
	}
	return payload
}

// payloadCategoryToParam 将 indicatorCategoryPayload 转换为 CategoryParams
func payloadCategoryToParam(src indicatorCategoryPayload) param.CategoryParams {
	return param.CategoryParams{
		MA:   append([]int(nil), src.MA...),
		MACD: cloneMACDConfig(src.MACD),
		KDJ:  cloneKDJConfig(src.KDJ),
		BOLL: cloneBOLLConfig(src.BOLL),
		RSI:  append([]int(nil), src.RSI...),
	}
}

// payloadToIndicatorConfig 将 indicatorParamPayload 转换为 ParamConfig
func payloadToIndicatorConfig(payload indicatorParamPayload) *param.ParamConfig {
	cfg := &param.ParamConfig{
		Defaults: payloadCategoryToParam(payload.Defaults),
	}
	if len(payload.Categories) > 0 {
		cfg.Categories = make(map[string]param.CategoryParams, len(payload.Categories))
		for key, value := range payload.Categories {
			cfg.Categories[key] = payloadCategoryToParam(value)
		}
	}
	if len(payload.Overrides) > 0 {
		cfg.Overrides = make(map[string]param.CategoryParams, len(payload.Overrides))
		for key, value := range payload.Overrides {
			cfg.Overrides[key] = payloadCategoryToParam(value)
		}
	}
	return cfg
}

func cloneMACDConfig(src *ta.MACDConfig) *ta.MACDConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneKDJConfig(src *ta.KDJConfig) *ta.KDJConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneBOLLConfig(src *ta.BOLLConfig) *ta.BOLLConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func normalizeIndicatorPayload(payload *indicatorParamPayload) {
	payload.Defaults.MA = normalizePeriods(payload.Defaults.MA)
	payload.Defaults.RSI = normalizePeriods(payload.Defaults.RSI)
	if payload.Categories == nil {
		payload.Categories = map[string]indicatorCategoryPayload{}
	}
	for key, value := range payload.Categories {
		value.MA = normalizePeriods(value.MA)
		value.RSI = normalizePeriods(value.RSI)
		payload.Categories[key] = value
	}
	if payload.Overrides == nil {
		payload.Overrides = map[string]indicatorCategoryPayload{}
	}
	for key, value := range payload.Overrides {
		value.MA = normalizePeriods(value.MA)
		value.RSI = normalizePeriods(value.RSI)
		payload.Overrides[key] = value
	}
}

func normalizePeriods(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func validateIndicatorCategory(name string, payload indicatorCategoryPayload, allowEmpty bool) error {
	if len(payload.MA) > 0 {
		for _, value := range payload.MA {
			if value <= 0 {
				return fmt.Errorf("%s 的 ma 周期必须大于 0", name)
			}
		}
	}
	if payload.MACD != nil {
		if payload.MACD.Fast <= 0 || payload.MACD.Slow <= 0 || payload.MACD.Signal <= 0 {
			return fmt.Errorf("%s 的 MACD 参数必须大于 0", name)
		}
	}
	if payload.KDJ != nil {
		if payload.KDJ.N <= 0 || payload.KDJ.M1 <= 0 || payload.KDJ.M2 <= 0 {
			return fmt.Errorf("%s 的 KDJ 参数必须大于 0", name)
		}
	}
	if payload.BOLL != nil {
		if payload.BOLL.N <= 0 || payload.BOLL.K <= 0 {
			return fmt.Errorf("%s 的 BOLL 参数必须大于 0", name)
		}
	}
	if len(payload.RSI) > 0 {
		for _, value := range payload.RSI {
			if value <= 0 {
				return fmt.Errorf("%s 的 RSI 周期必须大于 0", name)
			}
		}
	}
	if allowEmpty {
		return nil
	}
	if len(payload.MA) == 0 && payload.MACD == nil && payload.KDJ == nil && payload.BOLL == nil && len(payload.RSI) == 0 {
		return fmt.Errorf("%s 至少需要一个指标配置", name)
	}
	return nil
}

func validateIndicatorPayload(payload indicatorParamPayload) error {
	if err := validateIndicatorCategory("默认参数", payload.Defaults, false); err != nil {
		return err
	}
	for key, value := range payload.Categories {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("分类名称不能为空")
		}
		if err := validateIndicatorCategory(fmt.Sprintf("分类 %s", key), value, true); err != nil {
			return err
		}
	}
	for key, value := range payload.Overrides {
		if matched, _ := regexp.MatchString(`^\d{6}$`, key); !matched {
			return fmt.Errorf("个股覆盖代码 %s 非法", key)
		}
		if err := validateIndicatorCategory(fmt.Sprintf("个股 %s", key), value, true); err != nil {
			return err
		}
	}
	return nil
}

// Finance metric types
type financeTrendRecord struct {
	Period          string   `json:"period"`
	Year            int      `json:"year"`
	Quarter         string   `json:"quarter"`
	Label           string   `json:"label"`
	Revenue         *float64 `json:"revenue,omitempty"`
	NetProfit       *float64 `json:"netProfit,omitempty"`
	GrossMargin     *float64 `json:"grossMargin,omitempty"`
	NetMargin       *float64 `json:"netMargin,omitempty"`
	ROE             *float64 `json:"roe,omitempty"`
	EPS             *float64 `json:"eps,omitempty"`
	OperatingCashPS *float64 `json:"operatingCashPerShare,omitempty"`
}

type financeTrendsResponse struct {
	Code      string               `json:"code"`
	Mode      string               `json:"mode"`
	Metrics   []string             `json:"metrics"`
	Records   []financeTrendRecord `json:"records"`
	Available []string             `json:"available"`
}

type financeMetricTableResponse struct {
	Code   string               `json:"code"`
	Tables []financeMetricTable `json:"tables"`
}

type financeMetricTable struct {
	Title   string             `json:"title"`
	Periods []string           `json:"periods"`
	Rows    []financeMetricRow `json:"rows"`
}

type financeMetricRow struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// handleStockCompare handles stock vs block comparison requests
