package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/stockinfo"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
	"github.com/spf13/cobra"
)

var (
	stockinfoJSON     bool
	stockinfoExchange string
	stockinfoForce    bool
)

// stockinfoCmd 获取个股基础信息
var stockinfoCmd = &cobra.Command{
	Use:   "stockinfo [code]",
	Short: "查看个股基础信息（来自本地 stockinfo 库）",
	Long: `获取指定股票的基础信息，包括价格、市值、财务指标等。

示例:
  tongstock stockinfo 000001
  tongstock stockinfo 600519 --json
  tongstock stockinfo --exchange sh  # 列出上交所所有股票
  tongstock stockinfo sync           # 从 TDX 同步数据`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStockinfo,
}

func init() {
	stockinfoCmd.Flags().BoolVar(&stockinfoJSON, "json", false, "以 JSON 输出")
	stockinfoCmd.Flags().StringVar(&stockinfoExchange, "exchange", "", "按交易所筛选 (sh/sz/bj)")
	stockinfoCmd.AddCommand(stockinfoSyncCmd)
}

func runStockinfo(cmd *cobra.Command, args []string) error {
	code := ""
	if len(args) > 0 {
		code = strings.TrimSpace(args[0])
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer store.Close()

	infoStore, err := stockinfo.New(store)
	if err != nil {
		return fmt.Errorf("初始化 stockinfo 存储失败: %w", err)
	}

	// 列出所有股票
	if code == "" && stockinfoExchange == "" {
		infos, err := infoStore.GetAll()
		if err != nil {
			return fmt.Errorf("获取股票列表失败: %w", err)
		}

		if stockinfoJSON {
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(map[string]interface{}{
				"total": len(infos),
				"infos": infos,
			})
		}

		fmt.Printf("股票列表 (共 %d 只):\n", len(infos))
		for i, info := range infos {
			if i >= 50 {
				fmt.Printf("  ... 仅显示前 50 只\n")
				break
			}
			fmt.Printf("  %s %s  价格: %.2f  涨跌: %.2f%%\n", info.Code, info.Name, info.Price, info.ChangePct)
		}
		return nil
	}

	// 按交易所筛选
	if stockinfoExchange != "" {
		infos, err := infoStore.GetByExchange(stockinfoExchange)
		if err != nil {
			return fmt.Errorf("获取 %s 交易所股票失败: %w", stockinfoExchange, err)
		}

		if stockinfoJSON {
			enc := json.NewEncoder(os.Stdout)
			return enc.Encode(map[string]interface{}{
				"exchange": stockinfoExchange,
				"total":    len(infos),
				"infos":    infos,
			})
		}

		fmt.Printf("%s 交易所股票 (共 %d 只):\n", strings.ToUpper(stockinfoExchange), len(infos))
		for i, info := range infos {
			if i >= 50 {
				fmt.Printf("  ... 仅显示前 50 只\n")
				break
			}
			fmt.Printf("  %s %s  价格: %.2f  涨跌: %.2f%%\n", info.Code, info.Name, info.Price, info.ChangePct)
		}
		return nil
	}

	// 查询单只股票
	info, err := infoStore.GetByCode(code)
	if err != nil {
		return fmt.Errorf("获取 %s 信息失败: %w\n提示: 请先运行 tongstock stockinfo sync 同步数据", code, err)
	}

	if stockinfoJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(info)
	}

	fmt.Printf("股票基础信息 (%s %s):\n", info.Code, info.Name)
	fmt.Printf("  交易所:     %s\n", info.Exchange)
	fmt.Printf("  价格:       %.2f\n", info.Price)
	fmt.Printf("  涨跌幅:     %.2f%%\n", info.ChangePct)
	fmt.Printf("  成交量:     %.0f 手\n", info.Volume)
	fmt.Printf("  成交额:     %.2f 万元\n", info.Amount)
	fmt.Printf("  换手率:     %.2f%%\n", info.TurnoverRate)
	fmt.Printf("  流通股本:   %.2f 万股\n", info.LiuTongGuBen)
	fmt.Printf("  总股本:     %.2f 万股\n", info.ZongGuBen)
	fmt.Printf("  流通市值:   %.2f 亿元\n", info.MarketCap)
	fmt.Printf("  总市值:     %.2f 亿元\n", info.TotalMarketCap)
	fmt.Printf("  净资产:     %.2f 万元\n", info.JingZiChan)
	fmt.Printf("  净利润:     %.2f 万元\n", info.JingLiRun)
	fmt.Printf("  每股净资产: %.2f 元\n", info.MeiGuJingZiChan)
	if info.StFlag != 0 {
		fmt.Printf("  ST 标记:    是\n")
	}
	return nil
}

// stockinfoSyncCmd 从 TDX 同步股票基础信息
var stockinfoSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "从 TDX 同步股票基础信息到本地数据库",
	Long: `从通达信服务器拉取最新的股票行情和财务数据，写入本地 stockinfo 库。

示例:
  tongstock stockinfo sync           # 同步所有交易所
  tongstock stockinfo sync --force   # 强制全量同步（忽略24小时新鲜窗口）`,
	RunE: runStockinfoSync,
}

func init() {
	stockinfoSyncCmd.Flags().BoolVar(&stockinfoForce, "force", false, "强制全量同步")
}

func runStockinfoSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer store.Close()

	infoStore, err := stockinfo.New(store)
	if err != nil {
		return fmt.Errorf("初始化 stockinfo 存储失败: %w", err)
	}

	svc, err := dialService()
	if err != nil {
		return err
	}

	type exchangeInfo struct {
		name string
		ex   protocol.Exchange
	}
	exchanges := []exchangeInfo{
		{"sz", protocol.ExchangeSZ},
		{"sh", protocol.ExchangeSH},
		{"bj", protocol.ExchangeBJ},
	}

	startTime := time.Now()
	totalProcessed := 0
	successCount := 0
	skippedCount := 0
	failedCount := 0

	for _, item := range exchanges {
		fmt.Printf("获取 %s 代码列表...\n", strings.ToUpper(item.name))
		codes, err := svc.FetchCodes(item.ex)
		if err != nil {
			fmt.Printf("  获取 %s 代码列表失败: %v\n", item.name, err)
			continue
		}

		for _, code := range codes {
			if !isStockCodeCLI(code.Code, item.name) {
				continue
			}

			totalProcessed++

			// Skip if not force and data is fresh (updated in last 24 hours)
			if !stockinfoForce {
				exists, _ := infoStore.Exists(code.Code)
				if exists {
					staleCodes, _ := infoStore.GetStale(24 * 60) // 24 hours
					isStale := false
					for _, staleCode := range staleCodes {
						if staleCode == code.Code {
							isStale = true
							break
						}
					}
					if !isStale {
						skippedCount++
						continue
					}
				}
			}

			fullCode := item.name + code.Code
			quotes, err := svc.GetQuote(fullCode)
			if err != nil {
				failedCount++
				continue
			}

			finance, err := svc.FetchFinance(fullCode)
			if err != nil {
				// Finance might not be available, continue without it
			}

			var quote *protocol.QuoteItem
			if len(quotes) > 0 {
				quote = quotes[0]
			}

			info := stockinfo.BuildFromQuoteAndFinance(item.name, code.Code, code.Name, quote, finance)
			if info != nil && info.MarketCap > 0 {
				if err := infoStore.Upsert(*info); err != nil {
					failedCount++
				} else {
					successCount++
				}
			} else {
				failedCount++
			}

			// Progress output
			if totalProcessed%100 == 0 {
				fmt.Printf("  已处理 %d 只 (成功: %d, 跳过: %d, 失败: %d)\n", totalProcessed, successCount, skippedCount, failedCount)
			}
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n同步完成! 耗时: %s\n", elapsed.Round(time.Second))
	fmt.Printf("  总处理: %d 只\n", totalProcessed)
	fmt.Printf("  成功:   %d 只\n", successCount)
	fmt.Printf("  跳过:   %d 只 (数据新鲜)\n", skippedCount)
	fmt.Printf("  失败:   %d 只\n", failedCount)

	return nil
}

// isStockCodeCLI 判断是否为股票代码（排除指数、基金等）
func isStockCodeCLI(code string, exchange string) bool {
	if len(code) != 6 {
		return false
	}
	switch code[:3] {
	case "000", "001", "002", "003": // 深交所主板、中小板
		return exchange == "sz"
	case "300", "301": // 深交所创业板
		return exchange == "sz"
	case "600", "601", "603", "605": // 上交所主板
		return exchange == "sh"
	case "688", "689": // 上交所科创板
		return exchange == "sh"
	case "800", "801", "802", "803", "804", "805", "806", "807", "808", "809",
		"810", "811", "812", "813", "814", "815", "816", "817", "818", "819",
		"820", "821", "822", "823", "824", "825", "826", "827", "828", "829",
		"830", "831", "832", "833", "834", "835", "836", "837", "838", "839",
		"870", "871", "872", "873", "874", "875", "876", "877", "878", "879": // 北交所
		return exchange == "bj"
	}
	return false
}
