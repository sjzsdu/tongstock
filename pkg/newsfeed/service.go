package newsfeed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 一致性模式，语义与 internal/app/stockdata 保持一致。
type ConsistencyMode string

const (
	NewsRequireFresh ConsistencyMode = "require_fresh" // 必要时同步，失败返回错误
	NewsAllowStale   ConsistencyMode = "allow_stale"   // 同步失败时允许返回库内旧数据
	NewsCacheOnly    ConsistencyMode = "cache_only"    // 只读库，不访问网络
)

// DefaultFreshnessWindow 是个股资讯的默认新鲜度窗口。
// 超过该时长未成功抓取则认为过期，需要重新同步。
const DefaultFreshnessWindow = 30 * time.Minute

// DefaultMinConfidence 是默认的关联置信度下限。
// 东财检索是全文匹配，会带回大量「正文里恰好提到这只股票」的榜单类文章。
// 默认只保留标题命中或数据源原生关联，避免噪声盖过真正相关的资讯。
const DefaultMinConfidence = 0.5

var (
	// ErrInsufficientData 表示没有足够的真实数据支撑结果。
	ErrInsufficientData = errors.New("insufficient news data")
	// ErrFetchUnavailable 表示在 require_fresh 下无法获取新数据。
	ErrFetchUnavailable = errors.New("news source unavailable")
)

// StockDirectory 提供股票名录，用于实体识别与代码→简称解析。
// 由外部注入，避免 newsfeed 反向依赖具体数据源包。
type StockDirectory interface {
	ListEntities(ctx context.Context) ([]StockEntity, error)
	NameOf(ctx context.Context, code string) (string, bool)
}

// SourceDegradation 描述某个数据源本次的降级情况。
// 源失败必须显式暴露，而不是被静默吞掉。
type SourceDegradation struct {
	Source SourceType `json:"source"`
	Error  string     `json:"error"`
}

// Service 个股资讯应用服务：DB-first + 一致性契约。
//
// 流程遵循与行情数据相同的模式：先看库与水位是否新鲜，不新鲜且一致性允许
// 时才去抓取，抓取后事务写库并重新读库返回。没有数据时明确报告
// insufficient_data，绝不用无关新闻或空列表蒙混。
type Service struct {
	store           *SQLiteStore
	feeds           []Feed
	directory       StockDirectory
	matcher         *EntityMatcher
	freshnessWindow time.Duration
	now             func() time.Time
}

// NameResolverAware 由需要「代码 → 简称」才能检索的数据源实现。
// 东财的检索接口按关键词工作，用简称检索的召回质量远高于用 6 位代码。
type NameResolverAware interface {
	SetNameResolver(fn func(code string) string)
}

// NewService 创建个股资讯服务。feeds 与 directory 由调用方注入。
func NewService(store *SQLiteStore, feeds []Feed, dir StockDirectory) (*Service, error) {
	if store == nil {
		return nil, errors.New("newsfeed store is required")
	}
	svc := &Service{
		store:           store,
		feeds:           feeds,
		directory:       dir,
		freshnessWindow: DefaultFreshnessWindow,
		now:             time.Now,
	}
	if dir != nil {
		for _, f := range feeds {
			if aware, ok := f.(NameResolverAware); ok {
				aware.SetNameResolver(func(code string) string {
					name, _ := dir.NameOf(context.Background(), code)
					return name
				})
			}
		}
	}
	return svc, nil
}

// SetFreshnessWindow 覆盖新鲜度窗口
func (s *Service) SetFreshnessWindow(d time.Duration) {
	if d > 0 {
		s.freshnessWindow = d
	}
}

// RefreshEntities 从股票名录重建实体识别器。
// 名录为空时 matcher 为空，此时不会产生任何推测关联。
func (s *Service) RefreshEntities(ctx context.Context) error {
	if s.directory == nil {
		return nil
	}
	entities, err := s.directory.ListEntities(ctx)
	if err != nil {
		return err
	}
	s.matcher = NewEntityMatcher(entities)
	s.store.SetEntityMatcher(s.matcher)
	return nil
}

// StockNewsRequest 个股资讯查询请求
type StockNewsRequest struct {
	Code          string
	Mode          ConsistencyMode
	ForceRefresh  bool
	Limit         int
	Days          int
	NewsTypes     []NewsType
	MinConfidence float64
	// IncludeMentions 为真时不过滤「仅正文提及」的弱关联。
	IncludeMentions bool
}

// StockNewsResult 个股资讯查询结果
type StockNewsResult struct {
	Code   string        `json:"code"`
	Status string        `json:"status"` // ok | stale | insufficient_data
	Items  []NewsSummary `json:"items"`
	AsOf   time.Time     `json:"asOf"`
	// Degraded 列出本次失败的数据源。为空表示全部源成功。
	Degraded []SourceDegradation `json:"degraded,omitempty"`
	Message  string              `json:"message,omitempty"`
	// WeakCount 是被置信度阈值过滤掉的「仅正文提及」条数。
	// 明确报出来，避免用户以为系统漏抓了。
	WeakCount int `json:"weakCount"`
}

// StockNews 查询某只股票的最新资讯。
func (s *Service) StockNews(ctx context.Context, req StockNewsRequest) (*StockNewsResult, error) {
	code := strings.TrimSpace(req.Code)
	if len(code) != 6 {
		return nil, fmt.Errorf("股票代码无效: %q", req.Code)
	}
	mode := req.Mode
	if mode == "" {
		mode = NewsRequireFresh
	}

	// 只有需要抓取时才加载名录（抓取路径要用它做检索与识别）。
	if mode != NewsCacheOnly && s.matcher == nil {
		if err := s.RefreshEntities(ctx); err != nil {
			return nil, err
		}
	}

	fresh := !req.ForceRefresh && s.isFresh(code)
	if !fresh && mode != NewsCacheOnly {
		degraded, fetchErr := s.sync(ctx, code)
		if fetchErr != nil {
			// 全军覆没且要求新鲜数据 —— 明确报错，不返回陈旧内容。
			if mode == NewsRequireFresh {
				return nil, fmt.Errorf("%w: %v", ErrFetchUnavailable, fetchErr)
			}
			return s.fromStore(ctx, req, degraded, "数据源不可用，返回库内数据")
		}
		return s.fromStore(ctx, req, degraded, "")
	}

	if !fresh && mode == NewsCacheOnly {
		return s.fromStore(ctx, req, nil, "")
	}
	return s.fromStore(ctx, req, nil, "")
}

// isFresh 判断该股票的资讯是否仍在新鲜窗口内
func (s *Service) isFresh(code string) bool {
	var last string
	err := s.store.db.QueryRow(
		`SELECT last_success_at FROM news_sync_state WHERE scope = ? AND source = ?`, code, "*").Scan(&last)
	if err != nil {
		return false
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return false
	}
	return s.now().Sub(t) < s.freshnessWindow
}

// sync 从所有数据源抓取并落库，返回失败源的降级信息。
func (s *Service) sync(ctx context.Context, code string) ([]SourceDegradation, error) {
	var degraded []SourceDegradation
	var lastErr error
	total := 0

	for _, f := range s.feeds {
		if f == nil {
			continue
		}
		items, err := f.FetchByStock(ctx, code)
		if err != nil {
			degraded = append(degraded, SourceDegradation{Source: f.Name(), Error: err.Error()})
			lastErr = err
			continue
		}
		if len(items) == 0 {
			continue
		}
		if err := s.store.SaveNews(ctx, items); err != nil {
			degraded = append(degraded, SourceDegradation{Source: f.Name(), Error: err.Error()})
			lastErr = err
			continue
		}
		total += len(items)
	}

	s.markSync(code, total, lastErr)

	// 所有源都失败才算失败；部分失败只是降级。
	if len(degraded) > 0 && total == 0 {
		return degraded, lastErr
	}
	return degraded, nil
}

func (s *Service) markSync(code string, count int, err error) {
	now := s.now().Format(time.RFC3339)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	_, _ = s.store.db.Exec(`
		INSERT INTO news_sync_state (scope, source, last_fetch_at, last_success_at, last_item_count, last_error, updated_at)
		VALUES (?, '*', ?, ?, ?, ?, ?)
		ON CONFLICT(scope, source) DO UPDATE SET
			last_fetch_at = excluded.last_fetch_at,
			last_item_count = excluded.last_item_count,
			last_error = excluded.last_error,
			last_success_at = CASE WHEN excluded.last_error = '' THEN excluded.last_fetch_at ELSE last_success_at END,
			updated_at = excluded.updated_at
	`, code, now, now, count, errMsg, now)
}

func (s *Service) fromStore(ctx context.Context, req StockNewsRequest, degraded []SourceDegradation, msg string) (*StockNewsResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	minConfidence := req.MinConfidence
	if minConfidence <= 0 && !req.IncludeMentions {
		minConfidence = DefaultMinConfidence
	}
	filter := FeedFilter{
		RelatedStocks: []string{strings.TrimSpace(req.Code)},
		NewsTypes:     req.NewsTypes,
		MinConfidence: minConfidence,
		PageSize:      limit,
		PageNum:       1,
		SortBy:        "time",
	}
	if req.Days > 0 {
		since := s.now().AddDate(0, 0, -req.Days)
		filter.StartTime = &since
	}

	res, err := s.store.FilterNews(ctx, filter)
	if err != nil {
		return nil, err
	}

	if res.Items == nil {
		res.Items = []NewsSummary{}
	}
	out := &StockNewsResult{
		Code:     strings.TrimSpace(req.Code),
		Items:    res.Items,
		Degraded: degraded,
		Message:  msg,
	}
	if minConfidence > 0 {
		weakFilter := filter
		weakFilter.MinConfidence = 0
		if weak, werr := s.store.FilterNews(ctx, weakFilter); werr == nil && weak.Total > res.Total {
			out.WeakCount = weak.Total - res.Total
		}
	}

	switch {
	case len(res.Items) == 0:
		out.Status = "insufficient_data"
		out.Message = "库中没有该股票的资讯，且本次未获取到新数据"
	case degraded != nil || msg != "":
		out.Status = "stale"
	default:
		out.Status = "ok"
	}
	if len(res.Items) > 0 {
		out.AsOf = res.Items[0].PublishTime
	}
	return out, nil
}

// GlobalSync 抓取全局快讯（不针对个股），供后台定时同步使用。
func (s *Service) GlobalSync(ctx context.Context) (int, []SourceDegradation) {
	if s.matcher == nil {
		_ = s.RefreshEntities(ctx)
	}
	var degraded []SourceDegradation
	total := 0
	for _, f := range s.feeds {
		if f == nil {
			continue
		}
		items, err := f.Fetch(ctx)
		if err != nil {
			degraded = append(degraded, SourceDegradation{Source: f.Name(), Error: err.Error()})
			continue
		}
		if len(items) == 0 {
			continue
		}
		if err := s.store.SaveNews(ctx, items); err != nil {
			degraded = append(degraded, SourceDegradation{Source: f.Name(), Error: err.Error()})
			continue
		}
		total += len(items)
	}
	s.markSync("global", total, nil)
	return total, degraded
}

// LoadStockEntitiesFromDB 从 stockinfo 表读取股票名录。
// 这是一个便捷实现，供没有独立名录服务的调用方使用。
func LoadStockEntitiesFromDB(ctx context.Context, db *sql.DB) ([]StockEntity, error) {
	rows, err := db.QueryContext(ctx, `SELECT code, name FROM stockinfo WHERE name <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StockEntity
	for rows.Next() {
		var e StockEntity
		if err := rows.Scan(&e.Code, &e.Name); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// dbDirectory 是基于 SQL 的股票名录实现
type dbDirectory struct {
	db *sql.DB
}

// NewDBDirectory 用数据库连接构造股票名录
func NewDBDirectory(db *sql.DB) StockDirectory { return &dbDirectory{db: db} }

func (d *dbDirectory) ListEntities(ctx context.Context) ([]StockEntity, error) {
	return LoadStockEntitiesFromDB(ctx, d.db)
}

func (d *dbDirectory) NameOf(ctx context.Context, code string) (string, bool) {
	var name string
	if err := d.db.QueryRowContext(ctx, `SELECT name FROM stockinfo WHERE code = ?`, code).Scan(&name); err != nil {
		return "", false
	}
	return name, true
}

// SortDegradations 稳定排序降级信息，便于测试与展示
func SortDegradations(in []SourceDegradation) {
	sort.Slice(in, func(i, j int) bool { return string(in[i].Source) < string(in[j].Source) })
}
