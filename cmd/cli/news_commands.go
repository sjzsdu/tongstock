package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/newsfeed"
	"github.com/sjzsdu/tongstock/pkg/newsfeed/sources"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/spf13/cobra"
)

var (
	newsLimit       int
	newsDays        int
	newsType        string
	newsJSON        bool
	newsAllMentions bool
	newsMinConf     float64
	newsFetchGlobal bool
)

var newsCmd = &cobra.Command{
	Use:   "news",
	Short: "查询个股新闻资讯",
}

var newsQueryCmd = &cobra.Command{
	Use:   "query [code]",
	Short: "查询某只股票的最新资讯",
	Long: `查询某只股票的最新资讯，数据来自东方财富（新闻/研报）与财联社（快讯）。

默认只显示「标题命中股票」或「数据源原生关联」的强相关资讯；
加 --all-mentions 可包含仅在正文中被提及的内容。

一致性遵循与其他命令相同的语义：
  require_fresh（默认）  必要时抓取，源全部失败则报错
  allow_stale            源失败时返回库内已有数据
  cache_only             只读库，不访问网络`,
	Args: cobra.ExactArgs(1),
	RunE: runNewsQuery,
}

var newsFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "手动抓取资讯入库",
	RunE:  runNewsFetch,
}

func init() {
	newsCmd.AddCommand(newsQueryCmd)
	newsCmd.AddCommand(newsFetchCmd)

	newsQueryCmd.Flags().IntVarP(&newsLimit, "limit", "n", 20, "返回条数")
	newsQueryCmd.Flags().IntVarP(&newsDays, "days", "d", 0, "只返回最近 N 天（0 表示不限）")
	newsQueryCmd.Flags().StringVarP(&newsType, "type", "t", "", "资讯类型: 新闻/研报/快讯/公告/其他")
	newsQueryCmd.Flags().BoolVar(&newsJSON, "json", false, "以 JSON 输出")
	newsQueryCmd.Flags().BoolVar(&newsAllMentions, "all-mentions", false, "包含仅在正文中被提及的弱关联资讯")
	newsQueryCmd.Flags().Float64Var(&newsMinConf, "min-confidence", 0, "关联置信度下限 [0,1]，0 表示用默认值 0.5")

	newsFetchCmd.Flags().BoolVar(&newsFetchGlobal, "global", false, "抓取全局快讯流而非指定个股")
}

// dialNewsService 构造个股资讯服务。与行情服务一样走同一份 SQLite。
func dialNewsService() (*newsfeed.Service, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	store, err := storage.New(storage.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = store.Close() }
	if err := store.Migrate(); err != nil {
		cleanup()
		return nil, nil, err
	}
	nfStore, err := newsfeed.NewStoreWithStorage(store)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	svc, err := newsfeed.NewService(nfStore, sources.NewAllSources(), newsfeed.NewDBDirectory(store.DB()))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return svc, cleanup, nil
}

// cliNewsMode 把 CLI 的一致性参数映射到资讯服务
func cliNewsMode() newsfeed.ConsistencyMode {
	switch strings.ToLower(strings.TrimSpace(dataConsistency)) {
	case "allow_stale":
		return newsfeed.NewsAllowStale
	case "cache_only":
		return newsfeed.NewsCacheOnly
	default:
		return newsfeed.NewsRequireFresh
	}
}

// parseNewsType 接受中文类型名与英文别名
func parseNewsType(v string) (newsfeed.NewsType, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", false
	case "新闻", "news":
		return newsfeed.NewsTypeOther, true
	case "研报", "报告", "report", "research":
		return newsfeed.NewsTypeReport, true
	case "快讯", "flash":
		return newsfeed.NewsTypeFlash, true
	case "公告", "announce", "announcement":
		return newsfeed.NewsTypeAnnouncement, true
	case "讨论", "discussion":
		return newsfeed.NewsTypeDiscussion, true
	}
	return "", false
}

func runNewsQuery(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])
	if len(code) != 6 {
		return fmt.Errorf("股票代码必须为 6 位数字: %q", args[0])
	}
	if newsMinConf < 0 || newsMinConf > 1 {
		return fmt.Errorf("--min-confidence 必须在 [0,1] 之间")
	}

	svc, cleanup, err := dialNewsService()
	if err != nil {
		return err
	}
	defer cleanup()

	req := newsfeed.StockNewsRequest{
		Code:            code,
		Mode:            cliNewsMode(),
		ForceRefresh:    forceRefresh,
		Limit:           newsLimit,
		Days:            newsDays,
		IncludeMentions: newsAllMentions,
		MinConfidence:   newsMinConf,
	}
	if t, ok := parseNewsType(newsType); ok && t != "" {
		req.NewsTypes = []newsfeed.NewsType{t}
	} else if newsType != "" {
		return fmt.Errorf("未知的资讯类型: %s（可选: 新闻/研报/快讯/公告/讨论）", newsType)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	result, err := svc.StockNews(ctx, req)
	if err != nil {
		return err
	}

	if newsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	printStockNews(code, result)
	return nil
}

func printStockNews(code string, r *newsfeed.StockNewsResult) {
	if len(r.Items) == 0 {
		fmt.Printf("%s: 没有可用的资讯\n", code)
		if r.Message != "" {
			fmt.Printf("  %s\n", r.Message)
		}
		for _, d := range r.Degraded {
			fmt.Printf("  数据源 %s 不可用: %s\n", d.Source, d.Error)
		}
		return
	}

	statusLabel := map[string]string{
		"ok":                "",
		"stale":             "（部分数据源不可用，展示库内已有数据）",
		"insufficient_data": "（数据不足）",
	}[r.Status]

	fmt.Printf("%s 最新资讯 %s\n", code, statusLabel)
	if r.Status == "stale" {
		for _, d := range r.Degraded {
			fmt.Printf("  降级数据源: %s — %s\n", d.Source, d.Error)
		}
	}
	fmt.Println()

	for i, it := range r.Items {
		marker := ""
		if len(it.StockRefs) > 0 {
			top := it.StockRefs[0]
			switch top.MatchType {
			case newsfeed.MatchNative:
				marker = "原生"
			case newsfeed.MatchCodeHit:
				marker = "代码命中"
			case newsfeed.MatchNameHit:
				if top.Confidence >= 0.9 {
					marker = "标题命中"
				} else {
					marker = "正文提及"
				}
			}
		}
		fmt.Printf("%2d. [%s] [%s/%s] %s\n",
			i+1, it.PublishTime.Format("01-02 15:04"), it.Source, it.NewsType, truncate(it.Title, 60))
		fmt.Printf("    %s  关联=%s(%.2f)\n", it.URL, marker, confOf(it))
	}
	if r.WeakCount > 0 {
		fmt.Printf("\n另有 %d 条仅在正文中提及该股，已按置信度阈值过滤（--all-mentions 查看）\n", r.WeakCount)
	}
}

func confOf(it newsfeed.NewsSummary) float64 {
	if len(it.StockRefs) == 0 {
		return 0
	}
	return it.StockRefs[0].Confidence
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func runNewsFetch(cmd *cobra.Command, args []string) error {
	svc, cleanup, err := dialNewsService()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(cmd.Context(), 120*time.Second)
	defer cancel()

	if !newsFetchGlobal && len(args) > 0 {
		code := strings.TrimSpace(args[0])
		res, err := svc.StockNews(ctx, newsfeed.StockNewsRequest{
			Code: code, Mode: newsfeed.NewsRequireFresh, ForceRefresh: true, Limit: 1,
		})
		if err != nil {
			return err
		}
		fmt.Printf("%s: 抓取完成，库内共 %d 条，另有 %d 条弱关联\n", code, len(res.Items), res.WeakCount)
		return nil
	}

	n, degraded := svc.GlobalSync(ctx)
	fmt.Printf("全局快讯抓取完成，入库 %d 条\n", n)
	for _, d := range degraded {
		fmt.Printf("  降级数据源: %s — %s\n", d.Source, d.Error)
	}
	return nil
}
