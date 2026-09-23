package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/agentservice"
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

	sumDays        int
	sumLimit       int
	sumJSON        bool
	sumAllMentions bool
	sumDate        string
	sumModel       string
	sumRaw         bool
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

var newsSummarizeCmd = &cobra.Command{
	Use:   "summarize [code]",
	Short: "把个股指定日期的各渠道新闻交给 AI 简报员，输出一份简报",
	Long: `把某只股票在指定日期（或最近 N 天）内、来自各渠道（财联社/东方财富/雪球/巨潮资讯等）的
资讯交给 AI 简报员（news-summarize agent），输出一份简报，而不是原始资讯列表。

示例：
  tongstock news summarize 001216                  # 今天
  tongstock news summarize 001216 --date 2026-09-21  # 指定日期
  tongstock news summarize 001216 --days 7           # 最近 7 天
  tongstock news summarize 001216 --raw              # 不调 AI，输出原始聚合（调试用）

默认路径依赖 agent.enabled=true；一致性语义与其他命令相同：
require_fresh / allow_stale / cache_only。`,
	Args: cobra.ExactArgs(1),
	RunE: runNewsSummarize,
}

func init() {
	newsCmd.AddCommand(newsQueryCmd)
	newsCmd.AddCommand(newsFetchCmd)
	newsCmd.AddCommand(newsSummarizeCmd)

	newsQueryCmd.Flags().IntVarP(&newsLimit, "limit", "n", 20, "返回条数")
	newsQueryCmd.Flags().IntVarP(&newsDays, "days", "d", 0, "只返回最近 N 天（0 表示不限）")
	newsQueryCmd.Flags().StringVarP(&newsType, "type", "t", "", "资讯类型: 新闻/研报/快讯/公告/其他")
	newsQueryCmd.Flags().BoolVar(&newsJSON, "json", false, "以 JSON 输出")
	newsQueryCmd.Flags().BoolVar(&newsAllMentions, "all-mentions", false, "包含仅在正文中被提及的弱关联资讯")
	newsQueryCmd.Flags().Float64Var(&newsMinConf, "min-confidence", 0, "关联置信度下限 [0,1]，0 表示用默认值 0.5")

	newsFetchCmd.Flags().BoolVar(&newsFetchGlobal, "global", false, "抓取全局快讯流而非指定个股")

	newsSummarizeCmd.Flags().StringVar(&sumDate, "date", "", "指定日期 YYYY-MM-DD（默认今天，与 --days 互斥）")
	newsSummarizeCmd.Flags().IntVarP(&sumDays, "days", "d", 1, "回溯最近 N 天（默认 1 即当天）")
	newsSummarizeCmd.Flags().IntVarP(&sumLimit, "limit", "n", 200, "最多汇总的条数")
	newsSummarizeCmd.Flags().BoolVar(&sumJSON, "json", false, "以 JSON 输出")
	newsSummarizeCmd.Flags().BoolVar(&sumAllMentions, "all-mentions", true, "包含仅在正文中被提及的弱关联资讯")
	newsSummarizeCmd.Flags().BoolVar(&sumRaw, "raw", false, "跳过 AI，输出原始聚合摘要（调试用）")
	newsSummarizeCmd.Flags().StringVar(&sumModel, "model", "", "可选 AI 模型覆盖")
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

func runNewsSummarize(cmd *cobra.Command, args []string) error {
	code := strings.TrimSpace(args[0])
	if len(code) != 6 {
		return fmt.Errorf("股票代码必须为 6 位数字: %q", args[0])
	}
	if strings.TrimSpace(sumDate) != "" && cmd.Flags().Changed("days") {
		return fmt.Errorf("--date 与 --days 不能同时使用")
	}

	now := time.Now()
	since, until, err := newsWindow(sumDate, sumDays, now)
	if err != nil {
		return err
	}

	svc, cleanup, err := dialNewsService()
	if err != nil {
		return err
	}
	defer cleanup()

	// 抓取窗口略宽于统计窗口，避免边界条目被截断。
	fetchDays := int(now.Sub(since).Hours()/24) + 2
	if fetchDays < 1 {
		fetchDays = 1
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 120*time.Second)
	result, err := svc.StockNews(ctx, newsfeed.StockNewsRequest{
		Code:            code,
		Mode:            cliNewsMode(),
		ForceRefresh:    forceRefresh,
		Limit:           sumLimit,
		Days:            fetchDays,
		IncludeMentions: sumAllMentions,
	})
	cancel()
	if err != nil {
		return err
	}

	result.Items = filterNewsWindow(result.Items, since, until)
	window := newsWindowLabel(since, until)

	if sumRaw {
		digest := newsfeed.BuildNewsDigest(result)
		if sumJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(digest)
		}
		printNewsDigest(digest, window)
		return nil
	}

	if len(result.Items) == 0 {
		if sumJSON {
			return encodeNewsBriefing(newsBriefingOutput{Code: code, Since: since, Until: until})
		}
		fmt.Printf("%s 在 %s 窗口内没有相关资讯\n", code, window)
		return nil
	}

	prompt := buildNewsBriefingPrompt(code, since, until, result)
	summary, err := runNewsBriefing(cmd, prompt)
	if err != nil {
		return err
	}
	if sumJSON {
		return encodeNewsBriefing(newsBriefingOutput{
			Code:    code,
			Since:   since,
			Until:   until,
			Total:   len(result.Items),
			Sources: newsDigestSources(result.Items),
			Summary: summary,
		})
	}
	fmt.Println(strings.TrimSpace(summary))
	return nil
}

// newsWindow 计算资讯统计窗口。指定 --date 时为该自然日；否则回溯 days 天（默认当天）。
func newsWindow(dateFlag string, days int, now time.Time) (time.Time, time.Time, error) {
	if d := strings.TrimSpace(dateFlag); d != "" {
		day, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--date 必须为 YYYY-MM-DD 格式: %q", dateFlag)
		}
		return day, day.AddDate(0, 0, 1), nil
	}
	if days < 1 {
		days = 1
	}
	y, m, dd := now.In(time.Local).Date()
	start := time.Date(y, m, dd, 0, 0, 0, 0, time.Local).AddDate(0, 0, -(days - 1))
	return start, now.Add(time.Minute), nil
}

// newsWindowLabel 把窗口渲染为一行标签。
func newsWindowLabel(since, until time.Time) string {
	return fmt.Sprintf("%s ~ %s", since.Format("2006-01-02 15:04"), until.Format("2006-01-02 15:04"))
}

// filterNewsWindow 只保留 [since, until) 内的资讯，并按时间倒序排列。
func filterNewsWindow(items []newsfeed.NewsSummary, since, until time.Time) []newsfeed.NewsSummary {
	out := make([]newsfeed.NewsSummary, 0, len(items))
	for _, it := range items {
		if it.PublishTime.Before(since) || !it.PublishTime.Before(until) {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].PublishTime.After(out[j].PublishTime)
	})
	return out
}

// newsDigestSources 返回去重且排序的渠道列表。
func newsDigestSources(items []newsfeed.NewsSummary) []newsfeed.SourceType {
	seen := map[newsfeed.SourceType]bool{}
	out := make([]newsfeed.SourceType, 0)
	for _, it := range items {
		if !seen[it.Source] {
			seen[it.Source] = true
			out = append(out, it.Source)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// newsBriefingOutput AI 简报的 JSON 结构。
type newsBriefingOutput struct {
	Code    string                `json:"code"`
	Since   time.Time             `json:"since"`
	Until   time.Time             `json:"until"`
	Total   int                   `json:"total"`
	Sources []newsfeed.SourceType `json:"sources"`
	Summary string                `json:"summary,omitempty"`
}

func encodeNewsBriefing(v newsBriefingOutput) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// maxBriefingItems 单次喂给简报 agent 的资讯条数上限，超出部分只在 prompt 中注明。
const maxBriefingItems = 150

// buildNewsBriefingPrompt 把窗口内的资讯拼成给简报 agent 的一次性输入。
func buildNewsBriefingPrompt(code string, since, until time.Time, res *newsfeed.StockNewsResult) string {
	var b strings.Builder
	b.WriteString("请为下列个股资讯生成一份中文简报。\n\n")
	fmt.Fprintf(&b, "股票代码：%s\n", code)
	fmt.Fprintf(&b, "统计窗口：%s ~ %s（本地时间）\n", since.Format("2006-01-02 15:04"), until.Format("2006-01-02 15:04"))
	sources := newsDigestSources(res.Items)
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, string(s))
	}
	fmt.Fprintf(&b, "资讯条数：%d 条（渠道：%s）\n", len(res.Items), strings.Join(names, "、"))
	if len(res.Degraded) > 0 {
		degraded := make([]string, 0, len(res.Degraded))
		for _, d := range res.Degraded {
			degraded = append(degraded, fmt.Sprintf("%s（%s）", d.Source, d.Error))
		}
		fmt.Fprintf(&b, "数据源降级：%s\n", strings.Join(degraded, "；"))
	}

	shown := res.Items
	truncated := 0
	if len(shown) > maxBriefingItems {
		truncated = len(shown) - maxBriefingItems
		shown = shown[:maxBriefingItems]
	}
	b.WriteString("\n资讯列表（时间倒序）：\n")
	for i, it := range shown {
		fmt.Fprintf(&b, "%d. [%s] [%s/%s] %s\n", i+1, it.PublishTime.Format("01-02 15:04"), it.Source, it.NewsType, it.Title)
		if s := strings.TrimSpace(it.Summary); s != "" {
			fmt.Fprintf(&b, "   摘要：%s\n", truncate(s, 150))
		}
	}
	if truncated > 0 {
		fmt.Fprintf(&b, "\n（另有 %d 条更早的资讯未列出）\n", truncated)
	}
	return b.String()
}

// runNewsBriefing 调用内置的 news-summarize agent，把 prompt 加工成简报。
// 运行时、模型和 agent 定义全部来自统一的 agent 服务，与 Server 共用配置。
func runNewsBriefing(cmd *cobra.Command, prompt string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	svc, err := agentservice.Open(cfg.Agent, agentservice.Options{})
	if err != nil {
		if errors.Is(err, agentservice.ErrDisabled) {
			return "", fmt.Errorf("%w（或改用 --raw 查看原始聚合）", err)
		}
		return "", err
	}
	defer svc.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), 180*time.Second)
	defer cancel()
	return svc.Run(ctx, agentservice.Request{
		Agent:   "news-summarize",
		Session: "news-summarize",
		Model:   firstNonEmptyResearch(sumModel, cfg.Agent.Model),
		Prompt:  prompt,
	})
}

// printNewsDigest 输出人类可读的跨渠道综合摘要（--raw 调试路径）。
func printNewsDigest(d *newsfeed.NewsDigest, window string) {
	fmt.Printf("%s 资讯综合摘要（%s，共 %d 条，状态 %s）\n", d.Code, window, d.Total, d.Status)
	if d.Total == 0 {
		if d.Message != "" {
			fmt.Printf("  %s\n", d.Message)
		}
		for _, deg := range d.Degraded {
			fmt.Printf("  数据源 %s 不可用: %s\n", deg.Source, deg.Error)
		}
		return
	}
	fmt.Printf("时间范围: %s ~ %s  生成于 %s\n\n",
		d.From.Format("01-02 15:04"), d.To.Format("01-02 15:04"), d.GeneratedAt.Format("2006-01-02 15:04"))

	fmt.Println("渠道分布:")
	fmt.Printf("  %s\n", joinCounts(d.SourceCounts))
	fmt.Println("类型分布:")
	fmt.Printf("  %s\n\n", joinCounts(d.TypeCounts))

	if len(d.Groups) > 0 {
		fmt.Printf("跨渠道事件（%d 组，同一事件的多渠道报道已合并）:\n", len(d.Groups))
		for i, g := range d.Groups {
			fmt.Printf("%2d. [%d 条/%d 渠道] %s\n", i+1, g.Count, len(g.Sources), g.Title)
			fmt.Printf("    渠道: %s  时间: %s ~ %s\n",
				joinSources(g.Sources), g.From.Format("01-02 15:04"), g.To.Format("01-02 15:04"))
			for _, it := range g.Items {
				fmt.Printf("    - %s [%s/%s] %s\n", it.PublishTime.Format("01-02 15:04"), it.Source, it.NewsType, truncate(it.Title, 60))
			}
		}
		fmt.Println()
	}

	fmt.Printf("完整时间线（%d 条）:\n", d.Total)
	for i, it := range d.Items {
		fmt.Printf("%3d. %s [%s/%s] %s\n", i+1, it.PublishTime.Format("01-02 15:04"), it.Source, it.NewsType, truncate(it.Title, 70))
		if s := strings.TrimSpace(it.Summary); s != "" {
			fmt.Printf("     %s\n", truncate(s, 100))
		}
		if it.URL != "" {
			fmt.Printf("     %s\n", it.URL)
		}
	}
	if d.WeakCount > 0 {
		fmt.Printf("\n另有 %d 条仅在正文中提及该股的资讯未包含在内（--all-mentions=false 时显示）\n", d.WeakCount)
	}
	for _, deg := range d.Degraded {
		fmt.Printf("数据源 %s 不可用: %s\n", deg.Source, deg.Error)
	}
}

// joinCounts 把计数 map 渲染为 "k1 n1 | k2 n2"（按数量降序）。
func joinCounts(m map[string]int) string {
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s %d", p.key, p.count))
	}
	return strings.Join(parts, " | ")
}

func joinSources(ss []newsfeed.SourceType) string {
	parts := make([]string, 0, len(ss))
	for _, s := range ss {
		parts = append(parts, string(s))
	}
	return strings.Join(parts, "/")
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
