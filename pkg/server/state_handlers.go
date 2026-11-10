package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// 单股状态序列的超时窗口。FetchKlineAll 是 DB-first：本地没有水位时会去拉全历史，
// 单股首次调用可能十几秒，所以这里必须设上限，超时就把可操作的提示返回给前端，
// 而不是让请求一直挂着把连接拖死。
const (
	stateSeriesDefaultTimeout = 25 * time.Second
	stateSeriesMinTimeout     = 2 * time.Second
	stateSeriesMaxTimeout     = 120 * time.Second
	// stateSeriesSlowFetch 超过这个耗时就带上「首次拉全历史较慢」的提示。
	stateSeriesSlowFetch = 3 * time.Second
)

// stateSeriesItem 是逐日的一条状态带：当天四维状态 + 当日收盘价。
type stateSeriesItem struct {
	Date string `json:"date"`
	// Close 是当日未复权收盘价，供「状态 × 股价」对照视图使用。
	Close float64 `json:"close"`
	// States 是四维状态（键为 cross/osc/boll/ma，值为 buy/sell/neutral/insufficient_data）。
	States map[string]string `json:"states"`
	// Insufficient 只列出当天算不出来的维度（其余维度不必再传布尔值）。
	Insufficient []string `json:"insufficient,omitempty"`
	// Reasons 是判定依据（含具体数值），用来解释「为什么这天是这个态」。
	Reasons map[string]string `json:"reasons,omitempty"`
	// AdjustVersion 是这条状态带对应的复权口径。
	AdjustVersion string `json:"adjust_version"`
}

// handleStockStateSeries 返回单股逐日四维状态时序（现算，不落库）。
//
// GET /api/stock/:code/state-series?from=&to=&adjust=raw&type=day&include_reasons=1
func (s *Server) handleStockStateSeries(c *gin.Context) {
	code, ok := s.resolveStockCodeOrRespond(c, c.Param("code"))
	if !ok {
		return
	}

	// 本期只支持未复权：前/后复权会随新的除权事件改写历史价，历史状态随之漂移，
	// 回溯结果不可复现。要开口径必须先评审，不能在这里悄悄放开。
	adjust := strings.ToLower(strings.TrimSpace(c.DefaultQuery("adjust", signalstate.AdjustRaw)))
	if adjust != signalstate.AdjustRaw {
		WriteError(c, http.StatusBadRequest, "unsupported_adjust",
			"状态序列本期只支持未复权口径 adjust=raw（复权会改写历史状态，需单独评审）")
		return
	}

	ktypeStr := c.DefaultQuery("type", "day")
	ktype := tdx.ParseKlineType(ktypeStr)

	from, to, err := parseStateSeriesRange(c.Query("from"), c.Query("to"))
	if err != nil {
		WriteError(c, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	includeReasons := truthyQuery(c.Query("include_reasons"))

	if s.svc == nil {
		WriteError(c, http.StatusServiceUnavailable, "service_unavailable", "行情服务未就绪，暂时无法计算状态序列")
		return
	}

	timeout := stateSeriesTimeout(c.Query("timeout_ms"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	started := time.Now()
	klines, err := s.loadStateSeriesKlines(ctx, code, ktype)
	if err != nil {
		writeStateSeriesError(c, err, timeout)
		return
	}
	fetchCost := time.Since(started)

	klines = tdx.FilterValidKlines(klines)
	if len(klines) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":       code,
			"type":       ktypeStr,
			"adjust":     signalstate.AdjustRaw,
			"dimensions": signalstate.Metadata(),
			"items":      []stateSeriesItem{},
			"meta": gin.H{
				"bar_count": 0,
				"rows":      0,
				"fetch_ms":  fetchCost.Milliseconds(),
			},
			"note": "本地没有该股票的日线数据（或全部被过滤），请先同步日线（POST /api/sync/daily）后重试",
		})
		return
	}

	inputs := make([]ta.KlineInput, 0, len(klines))
	for _, k := range klines {
		inputs = append(inputs, ta.KlineInput{
			Time:   k.Time,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
			Amount: k.Amount,
		})
	}

	def := param.ResolveStateDefinition(code)
	vectors := signalstate.ComputeSeries(code, ktype, inputs, def)

	items := make([]stateSeriesItem, 0, len(vectors))
	for _, v := range vectors {
		if !withinStateRange(v.Date.Format("2006-01-02"), from, to) {
			continue
		}
		items = append(items, buildStateSeriesItem(v, includeReasons))
	}

	response := gin.H{
		"code":       code,
		"type":       ktypeStr,
		"adjust":     signalstate.AdjustRaw,
		"dimensions": signalstate.Metadata(),
		"definition": def,
		"warmup":     warmupBars(def),
		"items":      items,
		"meta": gin.H{
			"bar_count":    len(inputs),
			"rows":         len(items),
			"first_date":   firstStateDate(items),
			"last_date":    lastStateDate(items),
			"fetch_ms":     fetchCost.Milliseconds(),
			"calc_version": signalstate.CalcVersion,
			"dims":         len(signalstate.Dims()),
		},
	}
	// latest 始终带判定依据：前端图例要看「最新一天为什么是这个态」。
	if len(vectors) > 0 {
		response["latest"] = buildStateSeriesItem(vectors[len(vectors)-1], true)
	}
	if fetchCost > stateSeriesSlowFetch {
		response["note"] = "本次取本地日线较慢（首次调用会拉全历史），已成功；后续调用走本地库会明显更快"
	}
	// 进度提示：让前端能显示「这次算了多少根、花了多久」。
	c.Header("X-State-Series-Bars", strconv.Itoa(len(inputs)))
	c.Header("X-State-Series-Fetch-Ms", strconv.FormatInt(fetchCost.Milliseconds(), 10))
	c.JSON(http.StatusOK, response)
}

// loadStateSeriesKlines 在 ctx 截止前取回该股票的日线。
//
// FetchKlineAll 不支持 context，所以放到 goroutine 里并给结果缓冲一位：超时后
// 调用方立刻返回，goroutine 自己跑完把结果丢进缓冲 channel 后被回收，不会泄漏。
func (s *Server) loadStateSeriesKlines(ctx context.Context, code string, ktype uint8) ([]*protocol.Kline, error) {
	type fetchResult struct {
		klines []*protocol.Kline
		err    error
	}
	done := make(chan fetchResult, 1)
	go func() {
		klines, err := s.svc.FetchKlineAll(code, ktype)
		done <- fetchResult{klines: klines, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.klines, result.err
	}
}

func writeStateSeriesError(c *gin.Context, err error, timeout time.Duration) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		WriteError(c, http.StatusGatewayTimeout, "upstream_timeout",
			"取日线超时（本地无水位时会拉全历史）。请先同步该股日线（POST /api/sync/daily）再重试；本次等待上限 "+timeout.String())
		return
	}
	log.Printf("state-series: fetch klines failed: %v", err)
	WriteError(c, http.StatusServiceUnavailable, "kline_unavailable", "取该股日线失败，请稍后重试或先同步日线")
}

// buildStateSeriesItem 把内核的状态向量转成接口 DTO；字段顺序固定按 Dims()。
func buildStateSeriesItem(v signalstate.StateVector, includeReasons bool) stateSeriesItem {
	states := make(map[string]string, len(signalstate.Dims()))
	reasons := make(map[string]string, len(signalstate.Dims()))
	insufficient := make([]string, 0, len(signalstate.Dims()))
	for _, dim := range signalstate.Dims() {
		state := v.State(dim)
		states[string(dim)] = string(state)
		if state == signalstate.StateInsufficient {
			insufficient = append(insufficient, string(dim))
		}
		if includeReasons {
			reasons[string(dim)] = v.Reasons[dim]
		}
	}
	item := stateSeriesItem{
		Date:          v.Date.Format("2006-01-02"),
		Close:         v.Close,
		States:        states,
		AdjustVersion: v.AdjustVersion,
	}
	if len(insufficient) > 0 {
		item.Insufficient = insufficient
	}
	if includeReasons {
		item.Reasons = reasons
	}
	return item
}

// warmupBars 把每个维度的热身门槛返回给前端做图例提示。
func warmupBars(def signalstate.Definition) map[string]int {
	out := make(map[string]int, len(signalstate.Dims()))
	for _, dim := range signalstate.Dims() {
		out[string(dim)] = def.MinBarsFor(dim)
	}
	return out
}

func parseStateSeriesRange(fromRaw, toRaw string) (string, string, error) {
	from, err := normalizeStateDate(fromRaw)
	if err != nil {
		return "", "", err
	}
	to, err := normalizeStateDate(toRaw)
	if err != nil {
		return "", "", err
	}
	if from != "" && to != "" && from > to {
		return "", "", errors.New("from 不能晚于 to")
	}
	return from, to, nil
}

// normalizeStateDate 接受 2006-01-02 与 20060102 两种写法，统一成 2006-01-02。
func normalizeStateDate(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return "", errors.New("日期格式非法，需为 2006-01-02 或 20060102")
}

func withinStateRange(date, from, to string) bool {
	if from != "" && date < from {
		return false
	}
	if to != "" && date > to {
		return false
	}
	return true
}

func truthyQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// stateSeriesTimeout 解析 timeout_ms，并夹在允许区间内。
func stateSeriesTimeout(raw string) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return stateSeriesDefaultTimeout
	}
	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		return stateSeriesDefaultTimeout
	}
	timeout := time.Duration(ms) * time.Millisecond
	if timeout < stateSeriesMinTimeout {
		return stateSeriesMinTimeout
	}
	if timeout > stateSeriesMaxTimeout {
		return stateSeriesMaxTimeout
	}
	return timeout
}

func firstStateDate(items []stateSeriesItem) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].Date
}

func lastStateDate(items []stateSeriesItem) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].Date
}
