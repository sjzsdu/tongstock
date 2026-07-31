package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signal"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
)

var (
	indicatorCode   string
	indicatorType   string
	indicatorAll    bool
	indicatorCount  int
	indicatorConfig string
	indicatorJSON   bool
	indicatorDays   int
)

var indicatorCmd = &cobra.Command{
	Use:   "indicator",
	Short: "查询技术指标",
	RunE:  runIndicator,
}

func init() {
	indicatorCmd.Flags().StringVarP(&indicatorCode, "code", "c", "", "股票代码")
	indicatorCmd.Flags().StringVarP(&indicatorType, "type", "t", "day", "K线类型")
	indicatorCmd.Flags().BoolVarP(&indicatorAll, "all", "a", false, "获取全部历史K线")
	indicatorCmd.Flags().IntVarP(&indicatorCount, "count", "n", 250, "K线数量")
	indicatorCmd.Flags().StringVarP(&indicatorConfig, "config", "", "", "参数配置文件路径")
	indicatorCmd.Flags().BoolVarP(&indicatorJSON, "json", "j", false, "JSON格式输出")
	indicatorCmd.Flags().IntVarP(&indicatorDays, "days", "d", 1, "JSON输出时返回的历史天数")
	_ = indicatorCmd.MarkFlagRequired("code")
}

func runIndicator(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(indicatorType)
	service, cleanup, err := dialStockData(cmd.Context())
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer cleanup()
	spec := stockdata.DataSpec{
		Type: stockdata.DataKline, Market: cliMarketForCode(indicatorCode),
		Code: indicatorCode, Granularity: indicatorType, KType: ktype,
	}
	data, err := service.Query(cmd.Context(), cliDataRequest(spec))
	if err != nil {
		return fmt.Errorf("获取K线失败: %w", cliDataError(err, spec))
	}
	klines := data.Klines
	if !indicatorAll && indicatorCount > 0 && len(klines) > indicatorCount {
		klines = klines[len(klines)-indicatorCount:]
	}

	inputs := make([]ta.KlineInput, len(klines))
	for i, k := range klines {
		inputs[i] = ta.KlineInput{Time: k.Time, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume, Amount: k.Amount}
	}

	if indicatorConfig != "" {
		_ = param.Init(indicatorConfig)
	} else {
		_ = param.AutoInit()
	}
	category := param.DetectCategory(indicatorCode)
	cfg := param.Resolve(indicatorCode, category)

	result := ta.Calculate(inputs, cfg)
	signals := signal.Detect(indicatorCode, inputs, result, nil)

	if indicatorJSON {
		return outputIndicatorJSON(indicatorCode, inputs, result, signals)
	}

	return outputIndicatorTable(indicatorCode, string(category), inputs, result, signals)
}

func outputIndicatorJSON(code string, inputs []ta.KlineInput, result *ta.IndicatorResult, signals []signal.Signal) error {
	n := len(inputs)
	if n == 0 {
		return fmt.Errorf("无数据")
	}

	stockName := code
	quotes, err := func() ([]*protocol.QuoteItem, error) {
		svc, err := dialService()
		if err != nil {
			return nil, err
		}
		defer svc.Close()
		return svc.GetQuote(code)
	}()
	if err == nil && len(quotes) > 0 {
		stockName = quotes[0].Name
	}

	days := indicatorDays
	if days < 1 {
		days = 1
	}
	if days > n {
		days = n
	}

	startIdx := n - days

	if days == 1 {
		last := inputs[n-1]
		var change, changePct float64
		if n > 1 {
			change = last.Close - inputs[n-2].Close
			if inputs[n-2].Close > 0 {
				changePct = change / inputs[n-2].Close * 100
			}
		}

		trend := calcTrend(result, n-1)
		macdSignal := calcMACDSignal(result, n-1)
		kdjSignal := calcKDJSignal(result, n-1)
		rsiSignal := calcRSISignal(result, n-1)
		bollSignal, bollPosition := calcBOLLSignal(result, inputs[n-1], n-1)

		jsonOutput := map[string]interface{}{
			"code":      code,
			"name":      stockName,
			"timestamp": last.Time.Format("2006-01-02"),
			"price": map[string]interface{}{
				"current":    last.Close,
				"change":     change,
				"change_pct": changePct,
			},
			"ma":      buildMAData(result, n-1, trend),
			"macd":    buildMACDData(result, n-1, macdSignal),
			"kdj":     buildKDJData(result, n-1, kdjSignal),
			"rsi":     buildRSIData(result, n-1, rsiSignal),
			"boll":    buildBOLLData(result, n-1, bollSignal, bollPosition),
			"volume":  buildVolumeData(result),
			"signals": buildSignals(macdSignal, kdjSignal, trend),
			"summary": buildSummary(trend),
		}

		output, err := json.MarshalIndent(jsonOutput, "", "  ")
		if err != nil {
			return fmt.Errorf("JSON序列化失败: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	var history []map[string]interface{}
	for i := startIdx; i < n; i++ {
		dayData := inputs[i]
		var change, changePct float64
		if i > 0 {
			change = dayData.Close - inputs[i-1].Close
			if inputs[i-1].Close > 0 {
				changePct = change / inputs[i-1].Close * 100
			}
		}

		trend := calcTrend(result, i)
		macdSignal := calcMACDSignal(result, i)
		kdjSignal := calcKDJSignal(result, i)
		rsiSignal := calcRSISignal(result, i)
		bollSignal, bollPosition := calcBOLLSignal(result, dayData, i)

		history = append(history, map[string]interface{}{
			"timestamp": dayData.Time.Format("2006-01-02"),
			"price": map[string]interface{}{
				"current":    dayData.Close,
				"change":     change,
				"change_pct": changePct,
			},
			"ma":      buildMAData(result, i, trend),
			"macd":    buildMACDData(result, i, macdSignal),
			"kdj":     buildKDJData(result, i, kdjSignal),
			"rsi":     buildRSIData(result, i, rsiSignal),
			"boll":    buildBOLLData(result, i, bollSignal, bollPosition),
			"signals": buildSignals(macdSignal, kdjSignal, trend),
		})
	}

	latestTrend := calcTrend(result, n-1)
	jsonOutput := map[string]interface{}{
		"code":    code,
		"name":    stockName,
		"days":    days,
		"count":   len(history),
		"history": history,
		"summary": buildSummary(latestTrend),
	}

	output, err := json.MarshalIndent(jsonOutput, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}
	fmt.Println(string(output))
	return nil
}

func calcTrend(result *ta.IndicatorResult, idx int) string {
	if ma5, ok := result.MA["5"]; ok {
		if ma20, ok2 := result.MA["20"]; ok2 && idx >= 0 {
			if ma5[idx] > ma20[idx] {
				return "bullish"
			} else if ma5[idx] < ma20[idx] {
				return "bearish"
			}
		}
	}
	return "neutral"
}

func calcMACDSignal(result *ta.IndicatorResult, idx int) string {
	if result.MACD != nil && idx >= 0 {
		if result.MACD.DIF[idx] > result.MACD.DEA[idx] {
			return "golden_cross"
		} else if result.MACD.DIF[idx] < result.MACD.DEA[idx] {
			return "death_cross"
		}
	}
	return "neutral"
}

func calcKDJSignal(result *ta.IndicatorResult, idx int) string {
	if result.KDJ != nil && idx >= 0 {
		if result.KDJ.J[idx] > 100 {
			return "overbought"
		} else if result.KDJ.J[idx] < 0 {
			return "oversold"
		}
	}
	return "neutral"
}

func calcRSISignal(result *ta.IndicatorResult, idx int) string {
	if rsi6, ok := result.RSI["6"]; ok && idx >= 0 {
		if rsi6[idx] > 80 {
			return "overbought"
		} else if rsi6[idx] < 20 {
			return "oversold"
		}
	}
	return "neutral"
}

func calcBOLLSignal(result *ta.IndicatorResult, day ta.KlineInput, idx int) (string, float64) {
	signal := "normal"
	position := 0.0
	if result.BOLL != nil && idx >= 0 {
		upper := result.BOLL.Upper[idx]
		lower := result.BOLL.Lower[idx]
		if upper > lower {
			position = (day.Close - lower) / (upper - lower)
		}
		if day.Close > upper {
			signal = "break_upper"
		} else if day.Close < lower {
			signal = "break_lower"
		}
	}
	return signal, position
}

func buildMAData(result *ta.IndicatorResult, idx int, trend string) map[string]interface{} {
	m := map[string]interface{}{"trend": trend}
	for _, p := range []string{"5", "10", "20", "60", "120"} {
		if v, ok := result.MA[p]; ok && idx >= 0 && idx < len(v) {
			m["ma"+p] = v[idx]
		}
	}
	return m
}

func buildMACDData(result *ta.IndicatorResult, idx int, signal string) map[string]interface{} {
	if result.MACD == nil || idx < 0 || idx >= len(result.MACD.DIF) {
		return nil
	}
	return map[string]interface{}{
		"dif":    result.MACD.DIF[idx],
		"dea":    result.MACD.DEA[idx],
		"hist":   result.MACD.Hist[idx],
		"signal": signal,
	}
}

func buildKDJData(result *ta.IndicatorResult, idx int, signal string) map[string]interface{} {
	if result.KDJ == nil || idx < 0 || idx >= len(result.KDJ.K) {
		return nil
	}
	return map[string]interface{}{
		"k":      result.KDJ.K[idx],
		"d":      result.KDJ.D[idx],
		"j":      result.KDJ.J[idx],
		"signal": signal,
	}
}

func buildRSIData(result *ta.IndicatorResult, idx int, signal string) map[string]interface{} {
	if len(result.RSI) == 0 || idx < 0 {
		return nil
	}
	m := map[string]interface{}{"signal": signal}
	for p, v := range result.RSI {
		if idx < len(v) {
			m["rsi"+p] = v[idx]
		}
	}
	return m
}

func buildBOLLData(result *ta.IndicatorResult, idx int, signal string, position float64) map[string]interface{} {
	if result.BOLL == nil || idx < 0 || idx >= len(result.BOLL.Upper) {
		return nil
	}
	return map[string]interface{}{
		"upper":    result.BOLL.Upper[idx],
		"middle":   result.BOLL.Middle[idx],
		"lower":    result.BOLL.Lower[idx],
		"position": position,
		"signal":   signal,
	}
}

func buildVolumeData(result *ta.IndicatorResult) map[string]interface{} {
	if result.VolumeRatio == nil {
		return nil
	}
	return map[string]interface{}{
		"current": result.VolumeRatio.Current,
		"avg5":    result.VolumeRatio.Avg5,
		"ratio":   result.VolumeRatio.Ratio,
		"signal":  result.VolumeRatio.Signal,
	}
}

func buildSignals(macdSignal, kdjSignal, trend string) []string {
	var s []string
	if macdSignal == "golden_cross" {
		s = append(s, "golden_cross")
	}
	if macdSignal == "death_cross" {
		s = append(s, "death_cross")
	}
	if kdjSignal == "overbought" {
		s = append(s, "overbought")
	}
	if kdjSignal == "oversold" {
		s = append(s, "oversold")
	}
	if trend == "bullish" {
		s = append(s, "多头排列")
	}
	if trend == "bearish" {
		s = append(s, "空头排列")
	}
	return s
}

func buildSummary(trend string) map[string]interface{} {
	signal := "持有"
	if trend == "bullish" {
		signal = "买入"
	} else if trend == "bearish" {
		signal = "卖出"
	}
	strength := 50
	if trend == "bullish" {
		strength = 70
	} else if trend == "bearish" {
		strength = 30
	}
	return map[string]interface{}{
		"trend":    trend,
		"signal":   signal,
		"strength": strength,
	}
}

func outputIndicatorTable(code, category string, inputs []ta.KlineInput, result *ta.IndicatorResult, signals []signal.Signal) error {
	n := len(inputs)
	if n == 0 {
		return fmt.Errorf("无数据")
	}

	fmt.Printf("\n%s 技术指标 (分类: %s)\n", code, category)
	fmt.Println(strings.Repeat("=", 100))

	header := "%-12s %-8s %-8s %-8s %-8s %-8s %-8s"
	headerArgs := []interface{}{"日期", "收盘", "MA5", "MA10", "MA20", "MA60", "MA120"}

	if result.MACD != nil {
		header += " %-8s %-8s %-8s"
		headerArgs = append(headerArgs, "DIF", "DEA", "HIST")
	}
	if result.KDJ != nil {
		header += " %-8s %-8s %-8s"
		headerArgs = append(headerArgs, "K", "D", "J")
	}
	if len(result.RSI) > 0 {
		header += " %-8s %-8s %-8s"
		headerArgs = append(headerArgs, "RSI6", "RSI12", "RSI24")
	}
	if result.BOLL != nil {
		header += " %-8s %-8s %-8s"
		headerArgs = append(headerArgs, "UPPER", "MID", "LOWER")
	}
	if result.VolumeRatio != nil {
		header += " %-8s"
		headerArgs = append(headerArgs, "量比")
	}

	fmt.Printf(header+"\n", headerArgs...)
	fmt.Println(strings.Repeat("-", 120))

	start := 0
	if n > 20 {
		start = n - 20
	}
	for i := start; i < n; i++ {
		ma60 := 0.0
		ma120 := 0.0
		if v, ok := result.MA["60"]; ok {
			ma60 = v[i]
		}
		if v, ok := result.MA["120"]; ok {
			ma120 = v[i]
		}

		row := fmt.Sprintf("%-12s %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f",
			inputs[i].Time.Format("2006-01-02"), inputs[i].Close,
			result.MA["5"][i], result.MA["10"][i], result.MA["20"][i], ma60, ma120)

		if result.MACD != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.MACD.DIF[i], result.MACD.DEA[i], result.MACD.Hist[i])
		}
		if result.KDJ != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.KDJ.K[i], result.KDJ.D[i], result.KDJ.J[i])
		}
		if len(result.RSI) > 0 {
			rsi6 := 0.0
			rsi12 := 0.0
			rsi24 := 0.0
			if v, ok := result.RSI["6"]; ok {
				rsi6 = v[i]
			}
			if v, ok := result.RSI["12"]; ok {
				rsi12 = v[i]
			}
			if v, ok := result.RSI["24"]; ok {
				rsi24 = v[i]
			}
			row += fmt.Sprintf(" %-8.1f %-8.1f %-8.1f", rsi6, rsi12, rsi24)
		}
		if result.BOLL != nil {
			row += fmt.Sprintf(" %-8.2f %-8.2f %-8.2f", result.BOLL.Upper[i], result.BOLL.Middle[i], result.BOLL.Lower[i])
		}
		if result.VolumeRatio != nil && i == n-1 {
			row += fmt.Sprintf(" %-8.2f", result.VolumeRatio.Ratio)
		} else if result.VolumeRatio != nil {
			row += fmt.Sprintf(" %-8s", "-")
		}
		fmt.Println(row)
	}

	if result.VolumeRatio != nil {
		fmt.Printf("\n量比: %.2f (5日均量: %.0f, 信号: %s)\n",
			result.VolumeRatio.Ratio, result.VolumeRatio.Avg5, result.VolumeRatio.Signal)
	}

	if len(signals) > 0 {
		fmt.Printf("\n最新信号:\n")
		fmt.Println(strings.Repeat("-", 60))
		recentSignals := signals
		if len(signals) > 10 {
			recentSignals = signals[len(signals)-10:]
		}
		for _, s := range recentSignals {
			fmt.Printf("  [%s] %s %s (%s) 强度: %.2f\n",
				s.Date.Format("2006-01-02"), s.Indicator, s.Type, s.Details, s.Strength)
		}
	}

	return nil
}

var (
	screenType   string
	screenCodes  string
	screenFile   string
	screenSignal string
	screenPool   int
)

var screenCmd = &cobra.Command{
	Use:   "screen",
	Short: "批量筛选股票信号",
	RunE:  runScreen,
}

func init() {
	screenCmd.Flags().StringVarP(&screenType, "type", "t", "day", "K线类型")
	screenCmd.Flags().StringVarP(&screenCodes, "codes", "c", "", "逗号分隔的股票代码列表")
	screenCmd.Flags().StringVarP(&screenFile, "file", "f", "", "股票代码文件路径（每行一个代码）")
	screenCmd.Flags().StringVarP(&screenSignal, "signal", "s", "", "筛选信号类型: golden_cross/death_cross/overbought/oversold")
	screenCmd.Flags().IntVarP(&screenPool, "pool", "p", 10, "并发池大小")
}

func runScreen(cmd *cobra.Command, args []string) error {
	ktype := tdx.ParseKlineType(screenType)

	// Parse code list
	var codeList []string
	if screenCodes != "" {
		for _, c := range strings.Split(screenCodes, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				codeList = append(codeList, c)
			}
		}
	} else if screenFile != "" {
		data, err := os.ReadFile(screenFile)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				codeList = append(codeList, line)
			}
		}
	} else {
		return fmt.Errorf("请指定 --codes 或 --file 参数")
	}

	if len(codeList) == 0 {
		return fmt.Errorf("没有有效的股票代码")
	}

	service, cleanup, err := dialStockData(cmd.Context())
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer cleanup()

	_ = param.AutoInit()

	type screenResult struct {
		Code    string
		Klines  []ta.KlineInput
		Ind     *ta.IndicatorResult
		Signals []signal.Signal
		Err     error
	}

	results := make([]screenResult, len(codeList))
	sem := make(chan struct{}, screenPool)
	var wg sync.WaitGroup

	for i, code := range codeList {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c string) {
			defer wg.Done()
			defer func() { <-sem }()

			spec := stockdata.DataSpec{
				Type: stockdata.DataKline, Market: cliMarketForCode(c),
				Code: c, Granularity: screenType, KType: ktype,
			}
			data, err := service.Query(cmd.Context(), cliDataRequest(spec))
			if err != nil {
				results[idx] = screenResult{Code: c, Err: cliDataError(err, spec)}
				return
			}
			klines := data.Klines
			if len(klines) > 250 {
				klines = klines[len(klines)-250:]
			}

			inputs := make([]ta.KlineInput, len(klines))
			for j, k := range klines {
				inputs[j] = ta.KlineInput{Time: k.Time, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume, Amount: k.Amount}
			}

			category := param.DetectCategory(c)
			cfg := param.Resolve(c, category)
			ind := ta.Calculate(inputs, cfg)
			sigs := signal.Detect(c, inputs, ind, nil)

			results[idx] = screenResult{Code: c, Klines: inputs, Ind: ind, Signals: sigs}
		}(i, code)
	}
	wg.Wait()

	// Filter by signal type if specified
	var filtered []screenResult
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		if screenSignal == "" {
			filtered = append(filtered, r)
			continue
		}
		for _, s := range r.Signals {
			match := false
			switch screenSignal {
			case "golden_cross":
				match = s.Type == signal.SignalGoldenCross
			case "death_cross":
				match = s.Type == signal.SignalDeathCross
			case "overbought":
				match = s.Type == signal.SignalOverbought
			case "oversold":
				match = s.Type == signal.SignalOversold
			}
			if match {
				filtered = append(filtered, r)
				break
			}
		}
	}

	// Output results
	fmt.Printf("\n筛选结果 (%d/%d 只股票)\n", len(filtered), len(codeList))
	fmt.Println(strings.Repeat("=", 100))
	header := fmt.Sprintf("%-8s %-10s %-8s %-8s %-8s %-8s %-8s %-8s %-8s 信号",
		"代码", "日期", "收盘", "MA5", "MA10", "MA20", "DIF", "K", "J")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)+20))

	for _, r := range filtered {
		n := len(r.Klines)
		if n == 0 {
			continue
		}
		last := r.Klines[n-1]
		ma5 := r.Ind.MA["5"][n-1]
		ma10 := r.Ind.MA["10"][n-1]
		ma20 := r.Ind.MA["20"][n-1]
		dif := 0.0
		kVal := 0.0
		jVal := 0.0
		if r.Ind.MACD != nil {
			dif = r.Ind.MACD.DIF[n-1]
		}
		if r.Ind.KDJ != nil {
			kVal = r.Ind.KDJ.K[n-1]
			jVal = r.Ind.KDJ.J[n-1]
		}

		var sigStrs []string
		for _, s := range r.Signals {
			if n > 0 && s.Date.Equal(r.Klines[n-1].Time) {
				sigStrs = append(sigStrs, fmt.Sprintf("%s%s", s.Indicator, s.Type))
			}
		}
		sigStr := strings.Join(sigStrs, ", ")
		if sigStr == "" {
			sigStr = "-"
		}

		fmt.Printf("%-8s %-10s %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %-8.2f %s\n",
			r.Code, last.Time.Format("2006-01-02"), last.Close, ma5, ma10, ma20, dif, kVal, jVal, sigStr)
	}

	return nil
}
