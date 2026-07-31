package server

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	pinyin "github.com/mozillazg/go-pinyin"
	"github.com/sjzsdu/tongstock/internal/ai_tools"
	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/internal/experiment"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/monitoring"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/paradigms"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Server holds the service and stores for the HTTP server
type Server struct {
	svc                   *tdx.Service
	stockData             *stockdata.Service
	historyDB             *history.Store
	watchlistDB           *watchlist.Store
	tradingDB             *trading.Store
	stockpoolDB           *stockpool.Store
	stockinfoDB           *stockinfo.Store
	stockSearchIndexCache stockSearchIndexCache
	agentState            *AgentState
	agentListFunc         func() ([]EmbeddedAgent, error)
	ledger                *ledger.SignalLedger
	paradigmStore         *paradigms.Store
	paradigmSnapshots     *paradigm.DatasetSnapshotStore
	experimentRegistry    *experiment.SQLiteRegistry
	researchTools         *ai_tools.ToolRegistry
	storage               *storage.Storage
	monitoringMu          sync.RWMutex
	monitoringEngine      *monitoring.MonitorEngine
	monitoringReport      *monitoring.MonitorReport
	paradigmAlertMu       sync.RWMutex
	paradigmAlertCache    []paradigmAlert
	paradigmAlertLastScan time.Time
	backgroundWG          sync.WaitGroup
	newsfeedHandler       *NewsfeedHandler
	diagnostics           DiagnosticsProvider
}

const (
	stockSearchDefaultLimit = 10
	stockSearchMaxLimit     = 20
	stockSearchIndexTTL     = 10 * time.Minute
)

func isStockCode(code string, exchange string) bool {
	if len(code) != 6 {
		return false
	}
	// Check prefix for valid A-share stock codes
	switch code[:3] {
	case "001", "002", "003": // Shenzhen: SME board, new main board
		return exchange == "" || exchange == "sz"
	case "300", "301": // Shenzhen: ChiNext
		return exchange == "" || exchange == "sz"
	case "600", "601", "603", "605": // Shanghai: main board
		return exchange == "" || exchange == "sh"
	case "688", "689": // Shanghai: STAR Market
		return exchange == "" || exchange == "sh"
	case "000": // Shenzhen: main board (excluding indices)
		// 000xxx codes are special: in Shanghai they are indices, in Shenzhen they are stocks
		if exchange == "sh" {
			return false // 000xxx in Shanghai are indices
		}
		// In Shenzhen exchange, all 000xxx codes are valid stocks
		// e.g., 000001 is 平安银行, 000002 is 万科A
		return exchange == "" || exchange == "sz"
	case "800", "801", "802", "803", "804", "805", "806", "807", "808", "809",
		"810", "811", "812", "813", "814", "815", "816", "817", "818", "819",
		"820", "821", "822", "823", "824", "825", "826", "827", "828", "829",
		"830", "831", "832", "833", "834", "835", "836", "837", "838", "839",
		"840", "841", "842", "843", "844", "845", "846", "847", "848", "849",
		"850", "851", "852", "853", "854", "855", "856", "857", "858", "859",
		"860", "861", "862", "863", "864", "865", "866", "867", "868", "869",
		"870", "871", "872", "873", "874", "875", "876", "877", "878", "879",
		"880", "881", "882", "883", "884", "885", "886", "887", "888", "889",
		"890", "891", "892", "893", "894", "895", "896", "897", "898", "899": // Beijing exchange
		return exchange == "" || exchange == "bj"
	}
	return false
}

// Dependencies is the explicit composition boundary for the HTTP adapter.
// Adding a module no longer requires growing a positional constructor.
type Dependencies struct {
	StockData   *tdx.Service
	UnifiedData *stockdata.Service
	History     *history.Store
	Watchlist   *watchlist.Store
	Trading     *trading.Store
	StockPool   *stockpool.Store
	StockInfo   *stockinfo.Store
	Newsfeed    *NewsfeedHandler
	Diagnostics DiagnosticsProvider
	Storage     *storage.Storage
	Ledger      *ledger.SignalLedger
}

// NewServer creates a transport adapter from explicitly composed modules.
func NewServer(deps Dependencies) *Server {
	s := &Server{
		svc:                   deps.StockData,
		stockData:             deps.UnifiedData,
		historyDB:             deps.History,
		watchlistDB:           deps.Watchlist,
		tradingDB:             deps.Trading,
		stockpoolDB:           deps.StockPool,
		stockinfoDB:           deps.StockInfo,
		stockSearchIndexCache: stockSearchIndexCache{},
		ledger:                deps.Ledger,
		newsfeedHandler:       deps.Newsfeed,
		diagnostics:           deps.Diagnostics,
		storage:               deps.Storage,
		monitoringEngine:      monitoring.NewMonitorEngine(monitoring.NewDefaultMonitorConfig()),
	}
	if deps.Storage != nil {
		s.paradigmSnapshots = paradigm.NewDatasetSnapshotStore(deps.Storage)
		s.experimentRegistry, _ = experiment.NewSQLiteRegistry(deps.Storage)
		s.researchTools = ai_tools.NewToolRegistry()
		_ = s.researchTools.Register(&verifiedResearchEvidenceTool{server: s})
	}
	if s.ledger == nil {
		s.ledger = ledger.NewSignalLedger()
	}
	return s
}

func (s *Server) SetChatStore(store *ChatStore) {
	if s.agentState == nil {
		s.agentState = &AgentState{}
	}
	s.agentState.chatStore = store
}

// SetParadigmStore sets the paradigm store on the server instance.
func (s *Server) SetParadigmStore(store *paradigms.Store) {
	s.paradigmStore = store
}

// SetLedger sets the signal ledger on the server instance.
func (s *Server) SetLedger(ledger *ledger.SignalLedger) {
	s.ledger = ledger
}

// SetAgentLister registers an agent list function on the server instance.
func (s *Server) SetAgentLister(fn func() ([]EmbeddedAgent, error)) {
	s.agentListFunc = fn
}

// Close releases Server-owned optional runtimes. Shared Store connections are
// owned by App and are deliberately not closed here.
func (s *Server) Close() error {
	if s.agentState != nil {
		s.agentState.mu.Lock()
		defer s.agentState.mu.Unlock()
		if s.agentState.runner != nil {
			s.agentState.runner.Close()
			s.agentState.runner = nil
		}
	}
	return nil
}

// WaitForBackgroundTasks waits for context-controlled workers started by the
// Server. The caller must cancel their parent context before calling it.
func (s *Server) WaitForBackgroundTasks() {
	s.backgroundWG.Wait()
}

// Stock search types
type stockSearchMatch struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	MatchType string `json:"matchType"`
}

type stockSearchIndexResponse struct {
	UpdatedAt int64                   `json:"updatedAt"`
	Total     int                     `json:"total"`
	Items     []stockSearchIndexEntry `json:"items"`
}

type stockSearchIndexEntry struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	NameNorm string `json:"nameNorm"`
	Pinyin   string `json:"pinyin"`
	Initials string `json:"initials"`
}

type stockSearchResponse struct {
	Query    string             `json:"query"`
	Total    int                `json:"total"`
	Exact    bool               `json:"exact"`
	Resolved bool               `json:"resolved"`
	Matches  []stockSearchMatch `json:"matches"`
}

type stockSearchErrorResponse struct {
	Error   string             `json:"error"`
	Query   string             `json:"query"`
	Total   int                `json:"total"`
	Matches []stockSearchMatch `json:"matches"`
}

type stockSearchIndexItem struct {
	Code       string
	Name       string
	Exchange   string
	NameNorm   string
	PinyinNorm string
	Initials   string
}

type scoredStockMatch struct {
	stockSearchMatch
	Score int
}

type stockSearchIndexCache struct {
	sync.RWMutex
	builtAt time.Time
	items   []stockSearchIndexItem
}

// withRetry is retained while handlers migrate to typed Service methods. The
// Service owns executor borrowing and retry/reconnect semantics, so this
// wrapper must not borrow a second Client around a typed Service call.

func withRetry[T any](s *Server, fn func() (T, error)) (T, error) {
	return fn()
}

// Stock search helper functions
func (s *Server) resolveStockCodeOrRespond(c *gin.Context, raw string) (string, bool) {
	query := strings.TrimSpace(raw)
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 code 参数"})
		return "", false
	}

	// Fast path: 6-digit numeric code is always treated as a direct stock code.
	// Skip search index — let the TDX data fetch validate existence.
	if code := normalizeStockCodeQuery(query); code != "" {
		return code, true
	}

	matches, resolved, _, err := s.searchStockMatches(query, stockSearchDefaultLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", false
	}
	if len(matches) == 0 {
		c.JSON(http.StatusNotFound, stockSearchErrorResponse{Error: "未找到匹配股票", Query: query, Total: 0, Matches: []stockSearchMatch{}})
		return "", false
	}
	if !resolved {
		c.JSON(http.StatusConflict, stockSearchErrorResponse{Error: "找到多个匹配股票，请先选择具体个股", Query: query, Total: len(matches), Matches: matches})
		return "", false
	}
	return matches[0].Code, true
}

func (s *Server) searchStockMatches(query string, limit int) ([]stockSearchMatch, bool, bool, error) {
	if limit <= 0 {
		limit = stockSearchDefaultLimit
	}
	if limit > stockSearchMaxLimit {
		limit = stockSearchMaxLimit
	}

	items, err := s.getStockSearchIndex()
	if err != nil {
		return nil, false, false, err
	}

	normalizedQuery := normalizeStockSearchText(query)
	normalizedCode := normalizeStockCodeQuery(query)
	if normalizedQuery == "" && normalizedCode == "" {
		return []stockSearchMatch{}, false, false, nil
	}

	matches := make([]scoredStockMatch, 0, limit)
	var exactCodeMatch *scoredStockMatch
	for _, item := range items {
		score, matchType, ok := s.scoreStockMatch(item, normalizedQuery, normalizedCode)
		if !ok {
			continue
		}
		m := scoredStockMatch{stockSearchMatch: stockSearchMatch{Code: item.Code, Name: item.Name, Exchange: item.Exchange, MatchType: matchType}, Score: score}
		matches = append(matches, m)
		if matchType == "exact_code" && exactCodeMatch == nil {
			exactCodeMatch = &m
		}
	}

	// If there's an exact code match, return it directly as resolved
	if exactCodeMatch != nil {
		result := []stockSearchMatch{exactCodeMatch.stockSearchMatch}
		return result, true, true, nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Code != matches[j].Code {
			return matches[i].Code < matches[j].Code
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}

	result := make([]stockSearchMatch, len(matches))
	for i, match := range matches {
		result[i] = match.stockSearchMatch
	}
	exact := len(result) == 1 && strings.HasPrefix(result[0].MatchType, "exact_")
	resolved := len(result) == 1
	return result, resolved, exact, nil
}

func (s *Server) scoreStockMatch(item stockSearchIndexItem, normalizedQuery, normalizedCode string) (int, string, bool) {
	if normalizedCode != "" {
		switch {
		case item.Code == normalizedCode:
			return 1000, "exact_code", true
		case strings.HasPrefix(item.Code, normalizedCode):
			return 900, "prefix_code", true
		case strings.Contains(item.Code, normalizedCode):
			return 760, "contains_code", true
		}
	}
	if normalizedQuery == "" {
		return 0, "", false
	}
	if item.NameNorm == normalizedQuery {
		return 980, "exact_name", true
	}
	if item.PinyinNorm == normalizedQuery {
		return 970, "exact_pinyin", true
	}
	if item.Initials == normalizedQuery {
		return 960, "exact_initials", true
	}
	if strings.HasPrefix(item.NameNorm, normalizedQuery) {
		return 880, "prefix_name", true
	}
	if strings.HasPrefix(item.PinyinNorm, normalizedQuery) {
		return 870, "prefix_pinyin", true
	}
	if strings.HasPrefix(item.Initials, normalizedQuery) {
		return 860, "prefix_initials", true
	}
	if strings.Contains(item.NameNorm, normalizedQuery) {
		return 780, "contains_name", true
	}
	if strings.Contains(item.PinyinNorm, normalizedQuery) {
		return 770, "contains_pinyin", true
	}
	if strings.Contains(item.Initials, normalizedQuery) {
		return 765, "contains_initials", true
	}
	return 0, "", false
}

func (s *Server) getStockSearchIndex() ([]stockSearchIndexItem, error) {
	s.stockSearchIndexCache.RLock()
	if time.Since(s.stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(s.stockSearchIndexCache.items) > 0 {
		items := s.stockSearchIndexCache.items
		s.stockSearchIndexCache.RUnlock()
		return items, nil
	}
	s.stockSearchIndexCache.RUnlock()

	s.stockSearchIndexCache.Lock()
	defer s.stockSearchIndexCache.Unlock()
	if time.Since(s.stockSearchIndexCache.builtAt) < stockSearchIndexTTL && len(s.stockSearchIndexCache.items) > 0 {
		return s.stockSearchIndexCache.items, nil
	}

	svc := s.svc
	if svc == nil {
		return nil, fmt.Errorf("服务未初始化")
	}
	sources := []struct {
		exchange protocol.Exchange
		label    string
	}{{protocol.ExchangeSH, "上交所"}, {protocol.ExchangeSZ, "深交所"}, {protocol.ExchangeBJ, "北交所"}}

	items := make([]stockSearchIndexItem, 0, 6000)
	for _, source := range sources {
		codes, err := svc.FetchCodes(source.exchange)
		if err != nil {
			return nil, err
		}
		var exchangeCode string
		switch source.exchange {
		case protocol.ExchangeSH:
			exchangeCode = "sh"
		case protocol.ExchangeSZ:
			exchangeCode = "sz"
		case protocol.ExchangeBJ:
			exchangeCode = "bj"
		}
		for _, code := range codes {
			if !isStockCode(code.Code, exchangeCode) {
				continue
			}
			item := stockSearchIndexItem{Code: code.Code, Name: code.Name, Exchange: source.label}
			item.NameNorm = normalizeStockSearchText(item.Name)
			item.PinyinNorm, item.Initials = buildStockPinyinKeys(item.Name)
			items = append(items, item)
		}
	}
	s.stockSearchIndexCache.items = items
	s.stockSearchIndexCache.builtAt = time.Now()
	return items, nil
}

func buildStockPinyinKeys(name string) (string, string) {
	baseArgs := pinyin.NewArgs()
	baseArgs.Fallback = func(r rune, _ pinyin.Args) []string { return []string{string(r)} }
	full := normalizeStockSearchText(strings.Join(pinyin.LazyPinyin(name, baseArgs), ""))

	initialArgs := pinyin.NewArgs()
	initialArgs.Style = pinyin.FirstLetter
	initialArgs.Fallback = func(r rune, _ pinyin.Args) []string { return []string{string(r)} }
	initials := normalizeStockSearchText(strings.Join(pinyin.LazyPinyin(name, initialArgs), ""))
	return full, initials
}

func normalizeStockSearchText(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	input = strings.ReplaceAll(input, " ", "")
	input = strings.ReplaceAll(input, "-", "")
	input = strings.ReplaceAll(input, "_", "")
	return input
}

func normalizeStockCodeQuery(input string) string {
	input = normalizeStockSearchText(input)
	if len(input) == 8 {
		prefix := input[:2]
		if prefix == "sh" || prefix == "sz" || prefix == "bj" {
			input = input[2:]
		}
	}
	if len(input) != 6 {
		return ""
	}
	for _, ch := range input {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return input
}

func (s *Server) setupHealthRoutes(r *gin.Engine) {
	live := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "tongstock"})
	}
	r.GET("/health", live)
	r.GET("/health/live", live)
	r.GET("/health/ready", func(c *gin.Context) {
		if s.diagnostics == nil {
			c.JSON(http.StatusServiceUnavailable, Diagnostics{
				Status: "unavailable", Service: "tongstock",
				Modules:   map[string]ModuleHealth{"app": {Status: "unavailable", Message: "diagnostics not configured"}},
				CheckedAt: time.Now(),
			})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 750*time.Millisecond)
		defer cancel()
		result := s.diagnostics.Diagnostics(ctx)
		status := http.StatusOK
		if result.Status == "unavailable" {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, result)
	})
	r.GET("/health/diagnostics", func(c *gin.Context) {
		if s.diagnostics == nil {
			c.JSON(http.StatusOK, Diagnostics{
				Status: "degraded", Service: "tongstock",
				Modules:   map[string]ModuleHealth{"app": {Status: "degraded", Message: "diagnostics not configured"}},
				CheckedAt: time.Now(),
			})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 750*time.Millisecond)
		defer cancel()
		c.JSON(http.StatusOK, s.diagnostics.Diagnostics(ctx))
	})
	r.GET("/health/data-sync", func(c *gin.Context) {
		if s.stockData == nil {
			c.JSON(http.StatusOK, gin.H{"decisions": []stockdata.DecisionDiagnostic{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"decisions": s.stockData.Diagnostics()})
	})
}

// handleQuote handles quote requests
