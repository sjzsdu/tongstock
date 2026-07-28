package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/history"
	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/stockpool"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"github.com/sjzsdu/tongstock/pkg/watchlist"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleHistoryList(c *gin.Context) {
	stocks, err := s.historyDB.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if stocks == nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}

	var data []gin.H
	for _, stock := range stocks {
		resolvedName := s.resolveDisplayName(stock.Code, stock.Name)
		data = append(data, gin.H{
			"code":        stock.Code,
			"name":        resolvedName,
			"analyzed_at": stock.AnalyzedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleHistoryAdd handles history add requests
func (s *Server) handleHistoryAdd(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	stock := history.HistoryStock{
		Code:       req.Code,
		Name:       s.resolveDisplayName(req.Code, req.Name),
		AnalyzedAt: time.Now(),
	}

	if err := s.historyDB.Upsert(stock); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "added"})
}

// handleHistoryDelete handles history delete requests
func (s *Server) handleHistoryDelete(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if err := s.historyDB.Delete(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// handleWatchlistList handles watchlist list requests
func (s *Server) handleWatchlistList(c *gin.Context) {
	group := c.Query("group")

	var stocks []watchlist.WatchlistStock
	var err error

	if group != "" {
		stocks, err = s.watchlistDB.GetByGroup(group)
	} else {
		stocks, err = s.watchlistDB.GetAll()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if stocks == nil {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}

	var data []gin.H
	for _, stock := range stocks {
		resolvedName := s.resolveDisplayName(stock.Code, stock.Name)
		data = append(data, gin.H{
			"code":       stock.Code,
			"name":       resolvedName,
			"group":      stock.Group,
			"note":       stock.Note,
			"added_at":   stock.AddedAt.Format(time.RFC3339),
			"updated_at": stock.UpdatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleWatchlistAdd handles watchlist add requests
func (s *Server) handleWatchlistAdd(c *gin.Context) {
	var req struct {
		Code  string `json:"code"`
		Name  string `json:"name"`
		Group string `json:"group"`
		Note  string `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	stock := watchlist.WatchlistStock{
		Code:    req.Code,
		Name:    s.resolveDisplayName(req.Code, req.Name),
		Group:   req.Group,
		Note:    req.Note,
		AddedAt: time.Now(),
	}

	if err := s.watchlistDB.Upsert(stock); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "added"})
}

// handleWatchlistDelete handles watchlist delete requests
func (s *Server) handleWatchlistDelete(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if err := s.watchlistDB.Delete(code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// handleWatchlistUpdateNote handles watchlist note update requests
func (s *Server) handleWatchlistUpdateNote(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.watchlistDB.UpdateNote(code, req.Note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// handleWatchlistUpdateGroup handles watchlist group update requests
func (s *Server) handleWatchlistUpdateGroup(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	var req struct {
		Group string `json:"group"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.watchlistDB.UpdateGroup(code, req.Group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// handleWatchlistGroups handles watchlist groups list requests
func (s *Server) handleWatchlistGroups(c *gin.Context) {
	groups, err := s.watchlistDB.GetGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	counts, err := s.watchlistDB.CountByGroup()
	if err != nil {
		counts = make(map[string]int)
	}

	data := make([]gin.H, 0)
	for _, g := range groups {
		data = append(data, gin.H{
			"name":  g,
			"count": counts[g],
		})
	}

	c.JSON(http.StatusOK, gin.H{"groups": data})
}

// handleStockpoolList handles stockpool list requests
func (s *Server) handleStockpoolList(c *gin.Context) {
	pools, err := s.stockpoolDB.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if pools == nil {
		pools = []stockpool.StockPool{}
	}
	c.JSON(http.StatusOK, gin.H{"pools": pools})
}

// handleStockpoolUpsert handles stockpool insert/update requests
func (s *Server) handleStockpoolUpsert(c *gin.Context) {
	var req struct {
		ID          string                      `json:"id"`
		Name        string                      `json:"name"`
		Description string                      `json:"description,omitempty"`
		Filters     []stockpool.StockPoolFilter `json:"filters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	pool := stockpool.StockPool{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Filters:     req.Filters,
	}

	if err := s.stockpoolDB.Upsert(pool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleStockpoolDelete handles stockpool delete requests
func (s *Server) handleStockpoolDelete(c *gin.Context) {
	id := c.Param("id")
	if err := s.stockpoolDB.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// normalizeCodeList 标准化代码列表
func normalizeCodeList(codes []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		result = append(result, code)
	}
	return result
}

// handleSyncDaily handles daily sync requests

func (s *Server) handleTradeCreate(c *gin.Context) {
	var req struct {
		Code   string  `json:"code" binding:"required"`
		Name   string  `json:"name"`
		Action string  `json:"action" binding:"required,oneof=buy sell"`
		Price  float64 `json:"price" binding:"required,min=0"`
		Signal string  `json:"signal"`
		Ktype  string  `json:"ktype"`
		Reason string  `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.tradingDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trading store not initialized"})
		return
	}

	ktype := req.Ktype
	if ktype == "" {
		ktype = "day"
	}

	trade := trading.Trade{
		Code:   req.Code,
		Name:   req.Name,
		Action: trading.TradeAction(req.Action),
		Price:  req.Price,
		Signal: req.Signal,
		Ktype:  ktype,
		Reason: req.Reason,
	}

	id, err := s.tradingDB.Create(trade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "code": req.Code, "action": req.Action})
}

func (s *Server) handleTradeList(c *gin.Context) {
	if s.tradingDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trading store not initialized"})
		return
	}

	codes := c.Query("codes")
	if codes != "" {
		codeList := strings.Split(codes, ",")
		for i := range codeList {
			codeList[i] = strings.TrimSpace(codeList[i])
		}
		trades, err := s.tradingDB.GetLatestByCodes(codeList)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, trades)
		return
	}

	trades, err := s.tradingDB.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"trades": trades})
}

func (s *Server) handleTradePositions(c *gin.Context) {
	if s.tradingDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trading store not initialized"})
		return
	}

	positions, err := s.tradingDB.GetAllPositions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"positions": positions})
}

func (s *Server) handleTradeDelete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}

	if s.tradingDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "trading store not initialized"})
		return
	}

	if err := s.tradingDB.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func parseMainFinanceMetricTables(content string) []financeMetricTable {
	return parseFinanceMetricTablesInSection(content, "【1.主要财务指标】", "【2.", []string{"年度对比", "最新季度"}, 2)
}

func parseProfitabilityFinanceMetricTables(content string) []financeMetricTable {
	return parseFinanceMetricTablesInSection(content, "【4.盈利能力指标】", "【5.", []string{"盈利年度对比", "盈利最新季度"}, 2)
}

func parseFinanceMetricTablesInSection(content, sectionTitle, nextSectionPrefix string, titles []string, maxTables int) []financeMetricTable {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), sectionTitle) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	tables := make([]financeMetricTable, 0, maxTables)
	for i := start; i < len(lines) && (maxTables <= 0 || len(tables) < maxTables); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, nextSectionPrefix) {
			break
		}
		if !strings.HasPrefix(line, "┌") {
			continue
		}
		rows := extractBoxTableRows(lines[i:])
		if table := buildFinanceMetricTable(rows, titleAt(titles, len(tables))); len(table.Periods) > 0 && len(table.Rows) > 0 {
			tables = append(tables, table)
		}
	}
	return tables
}

func titleAt(titles []string, index int) string {
	if index >= 0 && index < len(titles) && titles[index] != "" {
		return titles[index]
	}
	return "财务指标"
}

func buildFinanceMetricTable(rows [][]string, title string) financeMetricTable {
	if len(rows) < 2 || len(rows[0]) < 2 {
		return financeMetricTable{}
	}
	periods := make([]string, 0, len(rows[0])-1)
	for _, header := range rows[0][1:] {
		period := strings.TrimSpace(header)
		if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(period) {
			periods = append(periods, period)
		}
	}
	if len(periods) == 0 {
		return financeMetricTable{}
	}
	result := financeMetricTable{Title: title, Periods: periods, Rows: make([]financeMetricRow, 0, len(rows)-1)}
	for _, row := range mergeWrappedFinanceMetricRows(rows[1:]) {
		if len(row) < len(periods)+1 {
			continue
		}
		name := sanitizeFinanceMetricName(row[0])
		if name == "" || strings.Contains(name, "审计意见") {
			continue
		}
		values := make([]string, 0, len(periods))
		for _, value := range row[1 : len(periods)+1] {
			values = append(values, strings.TrimSpace(value))
		}
		result.Rows = append(result.Rows, financeMetricRow{Name: name, Values: values})
	}
	return result
}

func mergeWrappedFinanceMetricRows(rows [][]string) [][]string {
	merged := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		first := strings.TrimSpace(row[0])
		if first != "" && len(merged) > 0 && allEmptyCells(row[1:]) {
			prev := merged[len(merged)-1]
			prev[0] = strings.TrimSpace(prev[0] + first)
			merged[len(merged)-1] = prev
			continue
		}
		merged = append(merged, append([]string(nil), row...))
	}
	return merged
}

func allEmptyCells(cells []string) bool {
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func parseFinanceTrendRecords(content, mode string) ([]financeTrendRecord, []string) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	mainTables := parseMainFinanceMetricTables(content)
	if len(mainTables) > 0 {
		idx := 1
		if mode == "year" || len(mainTables) == 1 {
			idx = 0
		}
		if idx < len(mainTables) {
			if records, metrics := financeTrendRecordsFromMetricTable(mainTables[idx]); len(records) > 0 {
				profitTables := parseProfitabilityFinanceMetricTables(content)
				if idx < len(profitTables) {
					supplementalRecords, supplementalMetrics := financeTrendRecordsFromMetricTable(profitTables[idx])
					records, metrics = mergeFinanceTrendRecordsByPeriod(records, supplementalRecords, metrics, supplementalMetrics)
				}
				return records, metrics
			}
		}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	var yearRecords []financeTrendRecord
	var yearMetrics []string
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "近五年每股收益对比") {
			continue
		}
		if records, metrics := parseYearFinanceTable(lines[i+1:]); len(records) > 0 {
			yearRecords = records
			yearMetrics = metrics
			break
		}
	}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "最新财报") {
			continue
		}
		if records, metrics := parseQuarterFinanceTable(lines[i+1:]); len(records) > 0 {
			if mode == "quarter" {
				return records, metrics
			}
			if mode == "year" {
				return mergeYearFinanceRecords(aggregateQuarterFinanceRecords(records), yearRecords, metrics, yearMetrics)
			}
		}
	}
	if mode == "year" {
		return yearRecords, yearMetrics
	}
	return nil, nil
}

func financeTrendRecordsFromMetricTable(table financeMetricTable) ([]financeTrendRecord, []string) {
	if len(table.Periods) == 0 || len(table.Rows) == 0 {
		return nil, nil
	}
	records := make([]financeTrendRecord, len(table.Periods))
	for i, period := range table.Periods {
		records[i] = financeTrendRecord{Period: period, Year: parseYear(period), Quarter: quarterLabel(period), Label: quarterLabel(period)}
	}
	metricSeen := map[string]struct{}{}
	metrics := make([]string, 0, 7)
	for _, row := range table.Rows {
		assignFinanceMetricValues(records, row.Name, row.Values)
		for _, metric := range financeMetricKeysForName(row.Name) {
			if _, ok := metricSeen[metric]; ok {
				continue
			}
			metricSeen[metric] = struct{}{}
			metrics = append(metrics, metric)
		}
	}
	records = pruneEmptyFinanceRecords(records)
	sort.Slice(records, func(i, j int) bool { return records[i].Period < records[j].Period })
	return records, metrics
}

func mergeFinanceTrendRecordsByPeriod(base, supplemental []financeTrendRecord, baseMetrics, supplementalMetrics []string) ([]financeTrendRecord, []string) {
	if len(base) == 0 {
		return supplemental, supplementalMetrics
	}
	supplementalByPeriod := make(map[string]financeTrendRecord, len(supplemental))
	for _, record := range supplemental {
		supplementalByPeriod[record.Period] = record
	}
	merged := append([]financeTrendRecord(nil), base...)
	for i := range merged {
		other, ok := supplementalByPeriod[merged[i].Period]
		if !ok {
			continue
		}
		fillMissingFinanceTrendFields(&merged[i], other)
	}
	return merged, mergeMetricKeys(baseMetrics, supplementalMetrics)
}

func fillMissingFinanceTrendFields(dst *financeTrendRecord, src financeTrendRecord) {
	if dst.Revenue == nil {
		dst.Revenue = src.Revenue
	}
	if dst.NetProfit == nil {
		dst.NetProfit = src.NetProfit
	}
	if dst.GrossMargin == nil {
		dst.GrossMargin = src.GrossMargin
	}
	if dst.NetMargin == nil {
		dst.NetMargin = src.NetMargin
	}
	if dst.ROE == nil {
		dst.ROE = src.ROE
	}
	if dst.EPS == nil {
		dst.EPS = src.EPS
	}
	if dst.OperatingCashPS == nil {
		dst.OperatingCashPS = src.OperatingCashPS
	}
}

func mergeMetricKeys(groups ...[]string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, group := range groups {
		for _, metric := range group {
			if _, ok := seen[metric]; ok {
				continue
			}
			seen[metric] = struct{}{}
			result = append(result, metric)
		}
	}
	return result
}

func parseQuarterFinanceTable(lines []string) ([]financeTrendRecord, []string) {
	rows := extractBoxTableRows(lines)
	if len(rows) < 3 {
		return nil, nil
	}
	headers := rows[0]
	if len(headers) < 2 {
		return nil, nil
	}
	periods := make([]string, 0, len(headers)-1)
	for _, header := range headers[1:] {
		period := strings.TrimSpace(header)
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(period) {
			continue
		}
		periods = append(periods, period)
	}
	if len(periods) == 0 {
		return nil, nil
	}
	records := make([]financeTrendRecord, len(periods))
	for i, period := range periods {
		records[i] = financeTrendRecord{
			Period:  period,
			Year:    parseYear(period),
			Quarter: quarterLabel(period),
			Label:   quarterLabel(period),
		}
	}
	metrics := make([]string, 0, 5)
	metricSeen := map[string]struct{}{}
	for _, row := range mergeWrappedTableRows(rows[1:]) {
		if len(row) < len(periods)+1 {
			continue
		}
		name := sanitizeFinanceMetricName(row[0])
		values := row[1:]
		assignFinanceMetricValues(records, name, values)
		for _, metric := range financeMetricKeysForName(name) {
			if _, ok := metricSeen[metric]; ok {
				continue
			}
			metricSeen[metric] = struct{}{}
			metrics = append(metrics, metric)
		}
	}
	return pruneEmptyFinanceRecords(records), metrics
}

func parseYearFinanceTable(lines []string) ([]financeTrendRecord, []string) {
	rows := extractBoxTableRows(lines)
	if len(rows) < 3 {
		return nil, nil
	}
	headers := rows[0]
	if len(headers) < 2 {
		return nil, nil
	}
	labels := headers[1:]
	indices := map[string]int{}
	for idx, label := range labels {
		indices[strings.TrimSpace(label)] = idx + 1
	}
	if _, ok := indices["年度"]; !ok {
		return nil, nil
	}
	metrics := []string{"eps"}
	records := make([]financeTrendRecord, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) <= indices["年度"] {
			continue
		}
		year := parseYear(strings.TrimSpace(row[0]))
		if year == 0 {
			continue
		}
		record := financeTrendRecord{
			Period:  fmt.Sprintf("%04d-12-31", year),
			Year:    year,
			Quarter: "年度",
			Label:   fmt.Sprintf("%d年度", year),
			EPS:     parseOptionalFloat(cellAt(row, indices["年度"])),
		}
		records = append(records, record)
	}
	return records, metrics
}

func extractBoxTableRows(lines []string) [][]string {
	rows := make([][]string, 0)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(rows) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "└") && len(rows) > 0 {
			break
		}
		if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") {
			continue
		}
		if !strings.HasPrefix(line, "│") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		cells := parseBoxTableLine(line)
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	return rows
}

func parseBoxTableLine(line string) []string {
	parts := strings.Split(line, "│")
	cells := make([]string, 0, len(parts))
	for i := 1; i < len(parts)-1; i++ {
		cells = append(cells, strings.TrimSpace(parts[i]))
	}
	return cells
}

func mergeWrappedTableRows(rows [][]string) [][]string {
	merged := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		first := strings.TrimSpace(row[0])
		isContinuation := first != "" && !containsAny(first, []string{"每股", "营业", "利润", "毛利", "净利", "收益率", "净资产"})
		if isContinuation && len(merged) > 0 {
			prev := merged[len(merged)-1]
			prev[0] = strings.TrimSpace(prev[0] + first)
			merged[len(merged)-1] = prev
			continue
		}
		copied := append([]string(nil), row...)
		merged = append(merged, copied)
	}
	return merged
}

func sanitizeFinanceMetricName(name string) string {
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "\t", "")
	name = strings.ReplaceAll(name, "（", "(")
	name = strings.ReplaceAll(name, "）", ")")
	return name
}

func assignFinanceMetricValues(records []financeTrendRecord, name string, values []string) {
	for i := range records {
		if i >= len(values) {
			break
		}
		v := parseOptionalFloat(values[i])
		if v == nil {
			continue
		}
		isGrowthRate := strings.Contains(name, "增长率") || strings.Contains(name, "同比")
		switch {
		case !isGrowthRate && (strings.Contains(name, "营业收入") || strings.Contains(name, "营业总收") || strings.Contains(name, "总营收")):
			records[i].Revenue = v
		case isNetProfitMetricName(name):
			records[i].NetProfit = v
		case strings.Contains(name, "销售毛利率") || strings.Contains(name, "毛利率"):
			records[i].GrossMargin = v
		case strings.Contains(name, "销售净利率") || strings.Contains(name, "净利润率"):
			records[i].NetMargin = v
		case strings.Contains(name, "加权净资产收益率") || strings.Contains(name, "净资产收益率"):
			records[i].ROE = v
		case strings.Contains(name, "每股收益"):
			records[i].EPS = v
		case strings.Contains(name, "每股经营现金流"):
			records[i].OperatingCashPS = v
		}
	}
}

func financeMetricKeysForName(name string) []string {
	isGrowthRate := strings.Contains(name, "增长率") || strings.Contains(name, "同比")
	switch {
	case !isGrowthRate && (strings.Contains(name, "营业收入") || strings.Contains(name, "营业总收") || strings.Contains(name, "总营收")):
		return []string{"revenue"}
	case isNetProfitMetricName(name):
		return []string{"netProfit"}
	case strings.Contains(name, "销售毛利率") || strings.Contains(name, "毛利率"):
		return []string{"grossMargin"}
	case strings.Contains(name, "销售净利率") || strings.Contains(name, "净利润率"):
		return []string{"netMargin"}
	case strings.Contains(name, "加权净资产收益率") || strings.Contains(name, "净资产收益率"):
		return []string{"roe"}
	case strings.Contains(name, "每股收益"):
		return []string{"eps"}
	case strings.Contains(name, "每股经营现金流"):
		return []string{"operatingCashPerShare"}
	default:
		return nil
	}
}

func isNetProfitMetricName(name string) bool {
	if strings.Contains(name, "增长率") || strings.Contains(name, "现金含量") || strings.Contains(name, "净利率") || strings.Contains(name, "净资产") || strings.Contains(name, "总资产") {
		return false
	}
	return strings.Contains(name, "归属母公司净利润") || strings.Contains(name, "归母净利") || strings.HasPrefix(name, "净利润")
}

func aggregateQuarterFinanceRecords(records []financeTrendRecord) []financeTrendRecord {
	byYear := map[int]financeTrendRecord{}
	for _, record := range records {
		if record.Quarter != "Q4" {
			continue
		}
		record.Label = fmt.Sprintf("%d年度", record.Year)
		record.Quarter = "年度"
		byYear[record.Year] = record
	}
	years := make([]int, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Ints(years)
	result := make([]financeTrendRecord, 0, len(years))
	for _, year := range years {
		result = append(result, byYear[year])
	}
	return result
}

func mergeYearFinanceRecords(base, fallback []financeTrendRecord, baseMetrics, fallbackMetrics []string) ([]financeTrendRecord, []string) {
	fallbackByYear := make(map[int]financeTrendRecord, len(fallback))
	for _, record := range fallback {
		fallbackByYear[record.Year] = record
	}
	merged := make([]financeTrendRecord, 0, len(base))
	for _, record := range base {
		if fallbackRecord, ok := fallbackByYear[record.Year]; ok {
			if record.EPS == nil {
				record.EPS = fallbackRecord.EPS
			}
		}
		merged = append(merged, record)
	}
	if len(merged) == 0 {
		return fallback, fallbackMetrics
	}
	metricSeen := map[string]struct{}{}
	metrics := make([]string, 0, len(baseMetrics)+len(fallbackMetrics))
	for _, metric := range append(append([]string(nil), baseMetrics...), fallbackMetrics...) {
		if _, ok := metricSeen[metric]; ok {
			continue
		}
		metricSeen[metric] = struct{}{}
		metrics = append(metrics, metric)
	}
	return merged, metrics
}

func pruneEmptyFinanceRecords(records []financeTrendRecord) []financeTrendRecord {
	result := make([]financeTrendRecord, 0, len(records))
	for _, record := range records {
		if record.Revenue == nil && record.NetProfit == nil && record.GrossMargin == nil && record.NetMargin == nil && record.ROE == nil && record.EPS == nil && record.OperatingCashPS == nil {
			continue
		}
		result = append(result, record)
	}
	return result
}

func parseOptionalFloat(value string) *float64 {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if trimmed == "" || trimmed == "---" || trimmed == "--" || trimmed == "-" {
		return nil
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseYear(value string) int {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 4 {
		parsed, err := strconv.Atoi(trimmed[:4])
		if err == nil {
			return parsed
		}
	}
	return 0
}

func quarterLabel(period string) string {
	switch {
	case strings.HasSuffix(period, "03-31"):
		return "Q1"
	case strings.HasSuffix(period, "06-30"):
		return "Q2"
	case strings.HasSuffix(period, "09-30"):
		return "Q3"
	case strings.HasSuffix(period, "12-31"):
		return "Q4"
	default:
		return period
	}
}

func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// handleOvernightArbitrage handles overnight arbitrage strategy screening

func (s *Server) handleStockinfoList(c *gin.Context) {
	minMarketCapStr := c.Query("minMarketCap")
	maxMarketCapStr := c.Query("maxMarketCap")
	exchange := c.Query("exchange")

	var minMarketCap, maxMarketCap float64
	if minMarketCapStr != "" {
		fmt.Sscanf(minMarketCapStr, "%f", &minMarketCap)
	}
	if maxMarketCapStr != "" {
		fmt.Sscanf(maxMarketCapStr, "%f", &maxMarketCap)
	}

	var infos []stockinfo.StockInfo
	var err error

	if minMarketCap > 0 || maxMarketCap > 0 {
		infos, err = s.stockinfoDB.GetByMarketCap(minMarketCap, maxMarketCap)
	} else if exchange != "" {
		infos, err = s.stockinfoDB.GetByExchange(exchange)
	} else {
		infos, err = s.stockinfoDB.GetAll()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(infos),
		"infos": infos,
	})
}

// handleStockinfoGet returns stock info by code
func (s *Server) handleStockinfoGet(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	info, err := s.stockinfoDB.GetByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "stock not found"})
		return
	}

	c.JSON(http.StatusOK, info)
}

// handleStockinfoSync syncs stock info from TDX
func (s *Server) handleStockinfoSync(c *gin.Context) {
	var req struct {
		Force bool `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Force = false
	}

	startTime := time.Now()
	totalProcessed := 0
	successCount := 0
	failedCount := 0

	exchanges := []struct {
		name string
		ex   protocol.Exchange
	}{
		{"sz", protocol.ExchangeSZ},
		{"sh", protocol.ExchangeSH},
		{"bj", protocol.ExchangeBJ},
	}

	for _, item := range exchanges {
		codes, err := s.svc.FetchCodes(item.ex)
		if err != nil {
			log.Printf("获取 %s 代码列表失败: %v", item.name, err)
			continue
		}

		for _, code := range codes {
			if !isStockCode(code.Code, item.name) {
				continue
			}

			totalProcessed++

			// Skip if not force and data is fresh (updated in last 24 hours)
			if !req.Force {
				if exists, _ := s.stockinfoDB.Exists(code.Code); exists {
					staleCodes, _ := s.stockinfoDB.GetStale(24 * 60) // 24 hours
					isStale := false
					for _, staleCode := range staleCodes {
						if staleCode == code.Code {
							isStale = true
							break
						}
					}
					if !isStale {
						continue
					}
				}
			}

			fullCode := item.name + code.Code
			quotes, err := s.svc.GetQuote(fullCode)
			if err != nil {
				log.Printf("获取 %s 行情失败: %v", fullCode, err)
			}

			finance, err := s.svc.FetchFinance(fullCode)
			if err != nil {
				log.Printf("获取 %s 财务数据失败: %v", fullCode, err)
			}

			var quote *protocol.QuoteItem
			if len(quotes) > 0 {
				quote = quotes[0]
			}

			info := stockinfo.BuildFromQuoteAndFinance(item.name, code.Code, code.Name, quote, finance)
			if info != nil && info.MarketCap > 0 {
				if err := s.stockinfoDB.Upsert(*info); err != nil {
					failedCount++
				} else {
					successCount++
				}
			} else {
				failedCount++
			}
		}
	}

	duration := time.Since(startTime)

	c.JSON(http.StatusOK, gin.H{
		"total":      totalProcessed,
		"success":    successCount,
		"failed":     failedCount,
		"duration":   duration.String(),
		"updated_at": time.Now().Unix(),
	})
}

// handleStockinfoCount returns count of stock info records
func (s *Server) handleStockinfoCount(c *gin.Context) {
	count, err := s.stockinfoDB.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count": count,
	})
}
