package newsfeed

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HotTopicStock 是热门股票榜单中的一行：某只股票在指定日期被多少条
// 新闻以多强的关联提及，以及由此折算出的热度分。
type HotTopicStock struct {
	Code string `json:"code"`
	Name string `json:"name,omitempty"`
	// Mentions 是该股当日被关联（强关联，见 HotTopicsRequest.MinConfidence）的新闻条数。
	Mentions int `json:"mentions"`
	// TitleHits 是其中标题命中或代码命中的条数。远高于正文提及时，
	// 说明新闻是「关于这只股票」而不是顺带一提。
	TitleHits int `json:"titleHits"`
	// NativeHits 是数据源原生标注关联的条数（如财联社 stock_list）。
	NativeHits int `json:"nativeHits"`
	// AvgHotScore 是关联新闻的平均热度（0-100，无热度数据时为 0）。
	AvgHotScore float64 `json:"avgHotScore"`
	// Sources 是提及该股的数据源列表，按名称去重排序。
	Sources []string `json:"sources"`
	// HotScore 是综合热度分：提及数、标题/原生命中、平均热度与来源多样性
	// 的加权和。仅用于排序展示，不构成投资建议。
	HotScore int `json:"hotScore"`
	// Headlines 是该股当日最热的几条代表新闻，便于快速浏览。
	Headlines []HotTopicHeadline `json:"headlines,omitempty"`
}

// HotTopicHeadline 是热门股票的一条代表新闻。
type HotTopicHeadline struct {
	NewsID      string     `json:"newsId"`
	Title       string     `json:"title"`
	Source      SourceType `json:"source"`
	PublishTime time.Time  `json:"publishTime"`
	URL         string     `json:"url,omitempty"`
}

// HotTopicResult 是指定日期的热门股票榜单结果。
type HotTopicResult struct {
	// Date 是请求统计的日期（YYYY-MM-DD，本地时区）。
	Date string `json:"date"`
	// TradingDate 是实际统计用的交易日。请求日期为非交易日时自动回溯
	// 到最近的一个交易日，避免周末/节假日返回空榜。
	TradingDate string `json:"tradingDate"`
	// Fallback 为真表示请求日期不是交易日，已自动回退到 TradingDate。
	Fallback bool            `json:"fallback"`
	Status   string          `json:"status"` // ok | stale | insufficient_data
	Items    []HotTopicStock `json:"items"`
	AsOf     time.Time       `json:"asOf"`
	// SyncedCount 是本次为填充该日期实际抓取入库的新闻条数（0 表示命中
	// 新鲜窗口或 cache_only，没有重新抓取）。
	SyncedCount int `json:"syncedCount"`
	// Degraded 列出本次失败的数据源。为空表示全部源成功。
	Degraded []SourceDegradation `json:"degraded,omitempty"`
	Message  string              `json:"message,omitempty"`
}

// HotTopicsRequest 热门股票榜单查询请求。
type HotTopicsRequest struct {
	// Date 是目标日期，支持 2006-01-02 与 20060102 两种写法；
	// 为空表示今天（服务本地时区）。
	Date string
	// Top 返回榜单条数，<=0 时取 DefaultTopicTop。
	Top int
	// Mode 一致性模式。cache_only 只读库；require_fresh 与 allow_stale
	// 在新鲜窗口外会触发全局同步，同步失败时的行为与个股资讯一致。
	Mode ConsistencyMode
	// ForceRefresh 跳过新鲜窗口强制重新同步。
	ForceRefresh bool
	// IncludeWeekend 为真时不再回溯交易日，严格按请求日期统计。
	IncludeWeekend bool
	// MinConfidence 覆盖默认的强关联阈值；<=0 时用 DefaultMinConfidence。
	MinConfidence float64
}

// DefaultTopicTop 是热门榜单的默认条数。
const DefaultTopicTop = 10

// topicMinConfidenceFloor 是「计入榜单」的最低关联置信度。榜单是聚合视图，
// 阈值放得太低会被榜单类文章的正文偶然提及淹没。
const topicMinConfidenceFloor = 0.5

// HotStockTopics 查询指定日期的热门股票。非 cache_only 且新鲜窗口外时
// 先做一次全局同步（与后台定时同步同一条路径），再从库内聚合。
func (s *Service) HotStockTopics(ctx context.Context, req HotTopicsRequest) (*HotTopicResult, error) {
	mode := req.Mode
	if mode == "" {
		mode = NewsRequireFresh
	}

	target, err := parseTopicDate(req.Date, s.now)
	if err != nil {
		return nil, err
	}

	// 交易日回溯：周末与节假日没有盘面，空榜只会让人误以为功能坏了。
	tradingDate := target
	fallback := false
	if !req.IncludeWeekend {
		if td := s.latestTradingDay(target); !td.Equal(target) {
			tradingDate = td
			fallback = true
		}
	}

	var (
		synced   int
		degraded []SourceDegradation
	)
	if mode != NewsCacheOnly && (req.ForceRefresh || !s.isFresh("global")) {
		if s.matcher == nil {
			if err := s.RefreshEntities(ctx); err != nil {
				return nil, err
			}
		}
		synced, degraded = s.GlobalSync(ctx)
		// 全军覆没且要求新鲜数据 —— 与个股资讯同一契约，明确报错。
		if synced == 0 && len(degraded) > 0 && mode == NewsRequireFresh {
			return nil, fmt.Errorf("%w: 全局快讯同步失败", ErrFetchUnavailable)
		}
	}

	items, asOf, err := s.aggregateTopics(ctx, tradingDate, req.Top, req.MinConfidence)
	if err != nil {
		return nil, err
	}

	out := &HotTopicResult{
		Date:        target.Format("2006-01-02"),
		TradingDate: tradingDate.Format("2006-01-02"),
		Fallback:    fallback,
		Status:      "ok",
		Items:       items,
		AsOf:        asOf,
		SyncedCount: synced,
		Degraded:    degraded,
	}
	if out.Items == nil {
		out.Items = []HotTopicStock{}
	}
	out.Message = topicMessage(out, req.Date)
	switch {
	case len(items) == 0 && degraded != nil:
		out.Status = "stale"
	case len(items) == 0:
		out.Status = "insufficient_data"
	}
	return out, nil
}

// topicMessage 组装结果附言。优先级：交易日回退 > 数据源降级 > 空榜说明。
func topicMessage(out *HotTopicResult, requested string) string {
	if out.Fallback && requested != "" {
		return fmt.Sprintf("%s 不是交易日，已回溯到最近交易日 %s", out.Date, out.TradingDate)
	}
	if len(out.Degraded) > 0 {
		return "部分数据源不可用，榜单基于库内已有数据"
	}
	if len(out.Items) == 0 {
		if out.SyncedCount == 0 {
			return "库内没有该日期的新闻关联数据；可稍后重试或先执行 news fetch --global"
		}
		return "数据源本次未返回该日期的内容"
	}
	return ""
}

// topicAcc 是聚合过程中的中间累加结构。
type topicAcc struct {
	item    HotTopicStock
	firstAt time.Time
}

// aggregateTopics 从库内按日期窗口聚合热门股票。
func (s *Service) aggregateTopics(ctx context.Context, day time.Time, top int, minConf float64) ([]HotTopicStock, time.Time, error) {
	if top <= 0 {
		top = DefaultTopicTop
	}
	if minConf <= 0 {
		minConf = DefaultMinConfidence
	}
	if minConf < topicMinConfidenceFloor {
		minConf = topicMinConfidenceFloor
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.Add(24 * time.Hour)

	// 只取强关联，日期窗口过滤与计数直接在 SQL 完成，避免把全天新闻
	// 拉进内存。(news_id, code) 在落库时已去重，COUNT(*) 即提及条数。
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT r.code,
		       COUNT(*) AS mentions,
		       SUM(CASE WHEN r.match_type IN ('native', 'code_hit') OR r.confidence >= 0.9 THEN 1 ELSE 0 END) AS title_hits,
		       SUM(CASE WHEN r.match_type = 'native' THEN 1 ELSE 0 END) AS native_hits,
		       AVG(COALESCE(n.hot_score, 0)) AS avg_hot,
		       MIN(n.publish_time) AS first_seen
		FROM news_stock_ref r
		JOIN news_items n ON n.id = r.news_id
		WHERE r.confidence >= ?
		  AND n.publish_time >= ? AND n.publish_time < ?
		GROUP BY r.code
	`, minConf, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()

	accs := make(map[string]*topicAcc)
	for rows.Next() {
		var (
			code    string
			ment    int
			titleH  int
			nativeH int
			avgHot  float64
			firstS  string
		)
		if err := rows.Scan(&code, &ment, &titleH, &nativeH, &avgHot, &firstS); err != nil {
			return nil, time.Time{}, err
		}
		firstAt, _ := time.Parse(time.RFC3339, firstS)
		accs[code] = &topicAcc{
			item: HotTopicStock{
				Code:        code,
				Mentions:    ment,
				TitleHits:   titleH,
				NativeHits:  nativeH,
				AvgHotScore: avgHot,
			},
			firstAt: firstAt,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	if len(accs) == 0 {
		return nil, time.Time{}, nil
	}

	// 来源多样性与代表新闻逐 code 补齐。候选数通常是几十的量级，
	// 全量算完分数再截断，保证排序稳定可解释。
	codes := make([]string, 0, len(accs))
	for code := range accs {
		codes = append(codes, code)
	}
	kept := make([]string, 0, len(codes))
	for _, code := range codes {
		name, known := s.topicNameOf(code)
		// 榜单只收股票：名录可用时，名录里没有的代码（常见如 399xxx 指数）
		// 不入榜，否则「创业板指大涨」会盖过真正的个股热点。
		if s.directory != nil && !known {
			continue
		}
		a := accs[code]
		if err := s.fillTopicSources(ctx, code, start, end, minConf, a); err != nil {
			return nil, time.Time{}, err
		}
		if err := s.fillTopicHeadlines(ctx, code, start, end, minConf, a); err != nil {
			return nil, time.Time{}, err
		}
		a.item.Name = name
		a.item.HotScore = topicHotScore(a.item)
		kept = append(kept, code)
	}

	sort.Slice(kept, func(i, j int) bool {
		return topicLess(accs[kept[i]].item, accs[kept[j]].item)
	})
	if len(kept) > top {
		kept = kept[:top]
	}
	out := make([]HotTopicStock, 0, len(kept))
	var asOf time.Time
	for _, code := range kept {
		out = append(out, accs[code].item)
		if accs[code].firstAt.After(asOf) {
			asOf = accs[code].firstAt
		}
	}
	return out, asOf, nil
}

// fillTopicSources 统计一只股票当日的来源多样性。
func (s *Service) fillTopicSources(ctx context.Context, code string, start, end time.Time, minConf float64, a *topicAcc) error {
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT DISTINCT n.source
		FROM news_stock_ref r
		JOIN news_items n ON n.id = r.news_id
		WHERE r.code = ? AND r.confidence >= ?
		  AND n.publish_time >= ? AND n.publish_time < ?
	`, code, minConf, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return err
	}
	defer rows.Close()
	sources := []string{}
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return err
		}
		sources = append(sources, src)
	}
	a.item.Sources = sources
	return rows.Err()
}

// fillTopicHeadlines 取一只股票当日最热的几条代表新闻。
func (s *Service) fillTopicHeadlines(ctx context.Context, code string, start, end time.Time, minConf float64, a *topicAcc) error {
	rows, err := s.store.db.QueryContext(ctx, `
		SELECT n.id, n.title, n.source, n.publish_time, n.url
		FROM news_stock_ref r
		JOIN news_items n ON n.id = r.news_id
		WHERE r.code = ? AND r.confidence >= ?
		  AND n.publish_time >= ? AND n.publish_time < ?
		ORDER BY n.hot_score DESC, n.publish_time DESC
		LIMIT 3
	`, code, minConf, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return err
	}
	defer rows.Close()
	headlines := []HotTopicHeadline{}
	for rows.Next() {
		var (
			id, title, urlStr string
			src, pubS         string
		)
		if err := rows.Scan(&id, &title, &src, &pubS, &urlStr); err != nil {
			return err
		}
		pub, _ := time.Parse(time.RFC3339, pubS)
		headlines = append(headlines, HotTopicHeadline{
			NewsID:      id,
			Title:       title,
			Source:      SourceType(src),
			PublishTime: pub,
			URL:         urlStr,
		})
	}
	a.item.Headlines = headlines
	return rows.Err()
}

// topicHotScore 折算综合热度分。
func topicHotScore(st HotTopicStock) int {
	score := st.Mentions*10 + st.TitleHits*5 + st.NativeHits*8
	score += int(st.AvgHotScore)
	score += len(st.Sources) * 5
	if score < 0 {
		score = 0
	}
	return score
}

// topicLess 排序规则：热度降序，其次提及数，再按代码保证稳定。
func topicLess(a, b HotTopicStock) bool {
	if a.HotScore != b.HotScore {
		return a.HotScore > b.HotScore
	}
	if a.Mentions != b.Mentions {
		return a.Mentions > b.Mentions
	}
	return a.Code < b.Code
}

// topicNameOf 解析代码 → 简称；known 表示名录中是否存在该代码。
// 解析不到就留空，绝不猜测。
func (s *Service) topicNameOf(code string) (string, bool) {
	if s.directory == nil {
		return "", true
	}
	return s.directory.NameOf(context.Background(), code)
}

// latestTradingDay 返回 target 或其之前最近的一个交易日。
//
// 交易日历来自 TDX 维护的 workday 表（date 列为 20060102 格式）。
// 库内日历未物化或不含 target 时，退化为按周判断：周末回退到周五，
// 与 stockdata.SQLiteTradingCalendar 的降级策略一致。
func (s *Service) latestTradingDay(target time.Time) time.Time {
	var last string
	err := s.store.db.QueryRow(`
		SELECT date FROM workday
		WHERE unix <= ?
		ORDER BY unix DESC
		LIMIT 1
	`, target.Unix()).Scan(&last)
	if err == nil {
		if day, perr := time.ParseInLocation("20060102", strings.TrimSpace(last), target.Location()); perr == nil {
			return day
		}
	}
	// 日历不可用：只修正周末，无法识别节假日。
	switch target.Weekday() {
	case time.Saturday:
		return target.AddDate(0, 0, -1)
	case time.Sunday:
		return target.AddDate(0, 0, -2)
	}
	return target
}

// parseTopicDate 解析用户输入的日期。空串表示今天。
func parseTopicDate(v string, now func() time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		n := now()
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()), nil
	}
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("日期格式无效: %q（支持 2006-01-02 或 20060102）", v)
}
