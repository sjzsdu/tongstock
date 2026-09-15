package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
)

var (
	klineCode  string
	klineType  string
	klineCount int
	klineAll   bool
	klineJSON  bool
)

var klineCmd = &cobra.Command{
	Use:   "kline",
	Short: "查询K线数据",
	RunE:  runKline,
}

func init() {
	klineCmd.Flags().StringVarP(&klineCode, "code", "c", "", "股票代码")
	klineCmd.Flags().StringVarP(&klineType, "type", "t", "day", "K线类型: 1m/minute, 5m, 15m, 30m, 60m, day, week, month, quarter, year")
	klineCmd.Flags().IntVarP(&klineCount, "count", "n", 500, "K线数量")
	klineCmd.Flags().BoolVarP(&klineAll, "all", "a", false, "获取全部历史K线")
	klineCmd.Flags().BoolVarP(&klineJSON, "json", "j", false, "JSON格式输出")
	_ = klineCmd.MarkFlagRequired("code")
}

func runKline(cmd *cobra.Command, _ []string) error {
	granularity, ktype, err := validateKlineOptions(klineCode, klineType, klineCount, klineAll)
	if err != nil {
		return err
	}

	items, metadata, err := queryKlines(cmd, klineCode, granularity, ktype)
	if err != nil {
		return err
	}

	items = newestKlines(items, klineCount, klineAll)
	if klineJSON {
		return outputKlineJSON(cmd.OutOrStdout(), klineCode, granularity, ktype, items, metadata)
	}
	return outputKlineTable(cmd.OutOrStdout(), ktype, items)
}

// queryKlines follows the same split as the HTTP handler: daily bars use the
// DB-first consistency service, while intraday and aggregate periods use the
// upstream TDX API because the current SQLite read model stores day precision.
func queryKlines(cmd *cobra.Command, code, granularity string, ktype uint8) ([]*protocol.Kline, stockdata.ResultMetadata, error) {
	if ktype == tdx.ParseKlineType("day") {
		service, cleanup, err := dialStockData(cmd.Context())
		if err != nil {
			return nil, stockdata.ResultMetadata{}, fmt.Errorf("连接服务器失败: %w", err)
		}
		defer cleanup()
		spec := stockdata.DataSpec{
			Type: stockdata.DataKline, Market: cliMarketForCode(code), Code: code,
			Granularity: granularity, KType: ktype,
		}
		result, err := service.Query(cmd.Context(), cliDataRequest(spec))
		if err != nil {
			return nil, stockdata.ResultMetadata{}, fmt.Errorf("获取K线失败: %w", cliDataError(err, spec))
		}
		return result.Klines, result.Metadata, nil
	}

	service, err := dialService()
	if err != nil {
		return nil, stockdata.ResultMetadata{}, fmt.Errorf("连接服务器失败: %w", err)
	}
	defer service.Close()
	items, err := service.FetchKlineAll(code, ktype)
	if err != nil {
		return nil, stockdata.ResultMetadata{}, fmt.Errorf("获取K线失败: %w", err)
	}
	return items, stockdata.ResultMetadata{
		AsOf: time.Now(), Freshness: "fresh", Reason: "direct_upstream", SyncStatus: "upstream",
	}, nil
}

func validateKlineOptions(code, granularity string, count int, all bool) (string, uint8, error) {
	if strings.TrimSpace(code) == "" {
		return "", 0, fmt.Errorf("--code 不能为空")
	}
	granularity = strings.ToLower(strings.TrimSpace(granularity))
	validTypes := map[string]struct{}{
		"1m": {}, "minute": {}, "5m": {}, "15m": {}, "30m": {}, "60m": {},
		"day": {}, "week": {}, "month": {}, "quarter": {}, "year": {},
	}
	if _, ok := validTypes[granularity]; !ok {
		return "", 0, fmt.Errorf("无效的 --type %q: 支持 1m/minute, 5m, 15m, 30m, 60m, day, week, month, quarter, year", granularity)
	}
	if !all && count <= 0 {
		return "", 0, fmt.Errorf("--count 必须大于 0（使用 --all 可忽略数量）")
	}
	return granularity, tdx.ParseKlineType(granularity), nil
}

func newestKlines(items []*protocol.Kline, count int, all bool) []*protocol.Kline {
	ordered := make([]*protocol.Kline, 0, len(items))
	for _, item := range items {
		if item != nil {
			ordered = append(ordered, item)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Time.Before(ordered[j].Time)
	})
	if !all && len(ordered) > count {
		ordered = ordered[len(ordered)-count:]
	}
	return ordered
}

type klineJSONItem struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Amount float64 `json:"amount"`
}

type klineJSONOutput struct {
	Code     string                   `json:"code"`
	Type     string                   `json:"type"`
	Items    []klineJSONItem          `json:"items"`
	Metadata stockdata.ResultMetadata `json:"metadata"`
}

func outputKlineJSON(w io.Writer, code, granularity string, ktype uint8, items []*protocol.Kline, metadata stockdata.ResultMetadata) error {
	output := klineJSONOutput{
		Code: code, Type: granularity, Items: make([]klineJSONItem, 0, len(items)), Metadata: metadata,
	}
	for _, item := range items {
		output.Items = append(output.Items, klineJSONItem{
			Time: formatKlineTime(item.Time, ktype), Open: item.Open, High: item.High,
			Low: item.Low, Close: item.Close, Volume: item.Volume, Amount: item.Amount,
		})
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}
	return nil
}

func outputKlineTable(w io.Writer, ktype uint8, items []*protocol.Kline) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "TIME\tOPEN\tHIGH\tLOW\tCLOSE\tVOLUME\tAMOUNT"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(tw, "%s\t%.3f\t%.3f\t%.3f\t%.3f\t%.2f\t%.2f\n",
			formatKlineTime(item.Time, ktype), item.Open, item.High, item.Low, item.Close, item.Volume, item.Amount); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatKlineTime(value time.Time, ktype uint8) string {
	switch ktype {
	case 7, 0, 1, 2, 3:
		return value.Format("2006-01-02 15:04:05")
	default:
		return value.Format("2006-01-02")
	}
}
