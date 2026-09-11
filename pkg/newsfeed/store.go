package newsfeed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// 错误定义
var (
	ErrStoreNotSet   = errors.New("store not set")
	ErrNewsNotFound  = errors.New("news not found")
	ErrEventNotFound = errors.New("event not found")
	ErrDuplicateNews = errors.New("duplicate news")
)

// Store 存储层接口
type Store interface {
	// SaveNews 保存新闻列表
	SaveNews(ctx context.Context, news []*NewsItem) error

	// GetNewsByID 根据ID获取新闻
	GetNewsByID(ctx context.Context, id string) (*NewsItem, error)

	// FilterNews 按条件筛选新闻
	FilterNews(ctx context.Context, filter FeedFilter) (*FeedResult, error)

	// SaveHotEvent 保存热点事件
	SaveHotEvent(ctx context.Context, event *HotEvent) error

	// GetHotEvents 获取热点事件列表
	GetHotEvents(ctx context.Context, filter HotEventFilter) (*EventResult, error)

	// GetHotEventDetail 获取热点事件详情
	GetHotEventDetail(ctx context.Context, eventID string) (*HotEvent, error)

	// DeleteExpiredNews 删除过期新闻
	DeleteExpiredNews(ctx context.Context, days int) (int, error)

	// Close 关闭数据库连接
	Close() error
}

// SQLiteStore SQLite存储实现
type SQLiteStore struct {
	db      *sql.DB
	owner   *storage.Storage
	matcher *EntityMatcher
}

// SetEntityMatcher 注入实体识别器，用于在保存时补充推测关联。
// 不注入则只保留数据源自带的关联——宁可关联少，也不凭空猜测。
func (s *SQLiteStore) SetEntityMatcher(m *EntityMatcher) {
	s.matcher = m
}

// NewSQLiteStore creates a standalone compatibility store. Application code
// should use NewStoreWithStorage so Newsfeed shares the App-owned connection.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	owner, err := storage.New(storage.Config{Driver: "sqlite3", DSN: dsn})
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{db: owner.DB(), owner: owner}, nil
}

// NewStoreWithStorage creates a non-owning Newsfeed store backed by the shared
// App storage. Close is intentionally a no-op for this variant.
func NewStoreWithStorage(s *storage.Storage) (*SQLiteStore, error) {
	if s == nil {
		return nil, errors.New("nil storage")
	}
	if s.Dialect() != storage.SQLite {
		return nil, errors.New("newsfeed requires sqlite storage")
	}
	return &SQLiteStore{db: s.DB()}, nil
}

// SaveNews 保存新闻列表
func (s *SQLiteStore) SaveNews(ctx context.Context, news []*NewsItem) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range news {
		if item.ID == "" {
			item.ID = generateID()
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now()
		}
		item.UpdatedAt = time.Now()

		// 先解析关联再落库，保证 related_stocks 与 news_stock_ref 一致。
		refs := s.resolveRefs(item)
		if len(refs) > 0 {
			item.RelatedStocks = make([]string, 0, len(refs))
			for _, r := range refs {
				item.RelatedStocks = append(item.RelatedStocks, r.Code)
			}
		}

		tagsJSON, _ := json.Marshal(item.Tags)
		stocksJSON, _ := json.Marshal(item.RelatedStocks)

		_, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO news_items 
			(id, source, news_type, title, summary, content, publish_time, hot_score, tags, related_stocks, url, original_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			item.ID,
			item.Source,
			item.NewsType,
			item.Title,
			item.Summary,
			item.Content,
			item.PublishTime.Format(time.RFC3339),
			item.HotScore,
			string(tagsJSON),
			string(stocksJSON),
			item.URL,
			item.OriginalID,
			item.CreatedAt.Format(time.RFC3339),
			item.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return err
		}

		if err := s.saveRefsWith(ctx, tx, item, refs); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// saveRefsWith 写入新闻与股票的关联
func (s *SQLiteStore) saveRefsWith(ctx context.Context, tx *sql.Tx, item *NewsItem, refs []StockRef) error {
	if len(refs) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM news_stock_ref WHERE news_id = ?`, item.ID); err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO news_stock_ref (news_id, code, match_type, confidence, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, item.ID, ref.Code, ref.MatchType, ref.Confidence, time.Now().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// resolveRefs 合并数据源自带关联与实体识别结果。同一代码保留置信度更高的来源。
func (s *SQLiteStore) resolveRefs(item *NewsItem) []StockRef {
	best := make(map[string]StockRef)
	for _, r := range item.StockRefs {
		if r.Code == "" {
			continue
		}
		if prev, ok := best[r.Code]; ok && prev.Confidence >= r.Confidence {
			continue
		}
		best[r.Code] = r
	}
	if s.matcher != nil && !s.matcher.Empty() {
		// 标题命中与正文命中必须分开评估：正文里出现股票简称的情况远多于
		// 真正「关于这只股票」的文章。混在一起会让榜单类文章拿到和头条
		// 新闻一样的置信度，相关性过滤随之失效。
		add := func(refs []StockRef) {
			for _, r := range refs {
				if prev, ok := best[r.Code]; ok && prev.Confidence >= r.Confidence {
					continue
				}
				best[r.Code] = r
			}
		}
		add(s.matcher.Match(item.Title))
		body := s.matcher.Match(item.Summary + " " + item.Content)
		for i := range body {
			body[i].Confidence *= bodyMatchPenalty
		}
		add(body)
	}
	out := make([]StockRef, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// bodyMatchPenalty 是正文命中相对标题命中的置信度折扣。
// 取 0.4 使「正文代码命中」落在 0.4、「正文简称命中」落在 0.36，
// 明确低于默认阈值 0.5，不会卡在边界上；需要时可用 --all-mentions 找回。
const bodyMatchPenalty = 0.4

// loadRefs 读取某条新闻的关联
func (s *SQLiteStore) loadRefs(ctx context.Context, newsID string) ([]StockRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT code, match_type, confidence FROM news_stock_ref WHERE news_id = ? ORDER BY confidence DESC, code`, newsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []StockRef
	for rows.Next() {
		var r StockRef
		if err := rows.Scan(&r.Code, &r.MatchType, &r.Confidence); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// GetNewsByID 根据ID获取新闻
func (s *SQLiteStore) GetNewsByID(ctx context.Context, id string) (*NewsItem, error) {
	var item NewsItem
	var tagsJSON, stocksJSON string
	var publishTimeStr, createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, source, news_type, title, summary, content, publish_time, hot_score, tags, related_stocks, url, original_id, created_at, updated_at
		FROM news_items WHERE id = ?
	`, id).Scan(
		&item.ID,
		&item.Source,
		&item.NewsType,
		&item.Title,
		&item.Summary,
		&item.Content,
		&publishTimeStr,
		&item.HotScore,
		&tagsJSON,
		&stocksJSON,
		&item.URL,
		&item.OriginalID,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNewsNotFound
		}
		return nil, err
	}

	item.PublishTime, _ = time.Parse(time.RFC3339, publishTimeStr)
	item.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	json.Unmarshal([]byte(tagsJSON), &item.Tags)
	json.Unmarshal([]byte(stocksJSON), &item.RelatedStocks)

	return &item, nil
}

// FilterNews 按条件筛选新闻
func (s *SQLiteStore) FilterNews(ctx context.Context, filter FeedFilter) (*FeedResult, error) {
	// 条件单独拼进 where，列表与总数共用同一套，避免两者口径不一致。
	where := ""
	args := []interface{}{}

	// 按来源筛选
	if len(filter.Sources) > 0 {
		placeholders := make([]string, len(filter.Sources))
		for i, source := range filter.Sources {
			placeholders[i] = "?"
			args = append(args, source)
		}
		where += fmt.Sprintf(" AND source IN (%s)", joinStrings(placeholders))
	}

	// 按新闻类型筛选
	if len(filter.NewsTypes) > 0 {
		placeholders := make([]string, len(filter.NewsTypes))
		for i, nt := range filter.NewsTypes {
			placeholders[i] = "?"
			args = append(args, nt)
		}
		where += fmt.Sprintf(" AND news_type IN (%s)", joinStrings(placeholders))
	}

	// 按关联股票筛选。必须走 news_stock_ref 关联表，不能对 related_stocks
	// 这个 JSON 文本列做 LIKE —— 后者既用不上索引，也会误命中。
	if len(filter.RelatedStocks) > 0 {
		placeholders := make([]string, len(filter.RelatedStocks))
		for i, code := range filter.RelatedStocks {
			placeholders[i] = "?"
			args = append(args, code)
		}
		where += fmt.Sprintf(
			" AND EXISTS (SELECT 1 FROM news_stock_ref r WHERE r.news_id = news_items.id AND r.code IN (%s)",
			joinStrings(placeholders),
		)
		if filter.MinConfidence > 0 {
			where += " AND r.confidence >= ?"
			args = append(args, filter.MinConfidence)
		}
		where += ")"
	}

	// 按时间范围筛选
	if filter.StartTime != nil {
		where += " AND publish_time >= ?"
		args = append(args, filter.StartTime.Format(time.RFC3339))
	}
	if filter.EndTime != nil {
		where += " AND publish_time <= ?"
		args = append(args, filter.EndTime.Format(time.RFC3339))
	}

	// 按热度筛选
	if filter.HotScoreMin > 0 {
		where += " AND hot_score >= ?"
		args = append(args, filter.HotScoreMin)
	}

	sortBy := filter.SortBy
	if sortBy == "" {
		sortBy = "time"
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageNum <= 0 {
		filter.PageNum = 1
	}
	offset := (filter.PageNum - 1) * filter.PageSize

	// 总数与列表口径一致：去掉排序后复用同一套条件，只是不分页。
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM news_items WHERE 1=1`+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	order := " ORDER BY publish_time DESC"
	if sortBy == "hot" {
		order = " ORDER BY hot_score DESC, publish_time DESC"
	}
	query := `SELECT id, source, news_type, title, summary, publish_time, hot_score, tags, related_stocks, url FROM news_items WHERE 1=1` +
		where + order + " LIMIT ? OFFSET ?"
	args = append(args, filter.PageSize, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 显式初始化为空切片：返回 null 会让调用方多一层判空，
	// 列表接口应始终是数组。
	items := make([]NewsSummary, 0)
	for rows.Next() {
		var item NewsSummary
		var tagsJSON, stocksJSON, publishTimeStr string
		err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.NewsType,
			&item.Title,
			&item.Summary,
			&publishTimeStr,
			&item.HotScore,
			&tagsJSON,
			&stocksJSON,
			&item.URL,
		)
		if err != nil {
			return nil, err
		}
		item.PublishTime, _ = time.Parse(time.RFC3339, publishTimeStr)
		json.Unmarshal([]byte(tagsJSON), &item.Tags)
		json.Unmarshal([]byte(stocksJSON), &item.RelatedStocks)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	attachRefs(ctx, s.db, items)

	return &FeedResult{
		Total:    total,
		Items:    items,
		PageNum:  filter.PageNum,
		PageSize: filter.PageSize,
	}, nil
}

// attachRefs 批量读取并挂载股票关联，避免逐条查询。
func attachRefs(ctx context.Context, db *sql.DB, items []NewsSummary) {
	if len(items) == 0 {
		return
	}
	ids := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	index := make(map[string]int, len(items))
	for i, it := range items {
		ids = append(ids, it.ID)
		placeholders = append(placeholders, "?")
		index[it.ID] = i
	}
	rows, err := db.QueryContext(ctx,
		`SELECT news_id, code, match_type, confidence FROM news_stock_ref WHERE news_id IN (`+
			joinStrings(placeholders)+`) ORDER BY confidence DESC, code`, ids...)
	if err != nil {
		return // 关联读取失败不应让整个查询失败
	}
	defer rows.Close()
	for rows.Next() {
		var newsID string
		var r StockRef
		if err := rows.Scan(&newsID, &r.Code, &r.MatchType, &r.Confidence); err != nil {
			return
		}
		if i, ok := index[newsID]; ok {
			items[i].StockRefs = append(items[i].StockRefs, r)
		}
	}
}

func (s *SQLiteStore) SaveHotEvent(ctx context.Context, event *HotEvent) error {
	if event.ID == "" {
		event.ID = generateID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	event.UpdatedAt = time.Now()

	keywordsJSON, _ := json.Marshal(event.Keywords)
	stocksJSON, _ := json.Marshal(event.RelatedStocks)
	sourceCountsJSON, _ := json.Marshal(event.SourceCounts)
	newsItemIDsJSON, _ := json.Marshal(event.NewsItemIDs)

	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO hot_events 
		(id, title, keywords, related_stocks, hot_index, source_counts, news_item_ids, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID,
		event.Title,
		string(keywordsJSON),
		string(stocksJSON),
		event.HotIndex,
		string(sourceCountsJSON),
		string(newsItemIDsJSON),
		event.Status,
		event.CreatedAt.Format(time.RFC3339),
		event.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

// GetHotEvents 获取热点事件列表
func (s *SQLiteStore) GetHotEvents(ctx context.Context, filter HotEventFilter) (*EventResult, error) {
	query := `SELECT id, title, keywords, related_stocks, hot_index, source_counts, news_item_ids, status, updated_at FROM hot_events WHERE 1=1`
	args := []interface{}{}

	// 按最低热度筛选
	if filter.MinHotIndex > 0 {
		query += " AND hot_index >= ?"
		args = append(args, filter.MinHotIndex)
	}

	// 按状态筛选
	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, status := range filter.Status {
			placeholders[i] = "?"
			args = append(args, status)
		}
		query += fmt.Sprintf(" AND status IN (%s)", joinStrings(placeholders))
	}

	// 排序
	query += " ORDER BY hot_index DESC, updated_at DESC"

	// 限制数量
	if filter.Limit <= 0 {
		filter.Limit = 10
	}
	query += " LIMIT ?"
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []EventSummary
	for rows.Next() {
		var item EventSummary
		var keywordsJSON, stocksJSON, sourceCountsJSON, newsItemIDsJSON, updatedAtStr string
		err := rows.Scan(
			&item.ID,
			&item.Title,
			&keywordsJSON,
			&stocksJSON,
			&item.HotIndex,
			&sourceCountsJSON,
			&newsItemIDsJSON,
			&item.Status,
			&updatedAtStr,
		)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		json.Unmarshal([]byte(keywordsJSON), &item.Keywords)
		json.Unmarshal([]byte(stocksJSON), &item.RelatedStocks)
		json.Unmarshal([]byte(sourceCountsJSON), &item.SourceCounts)
		var newsIDs []string
		json.Unmarshal([]byte(newsItemIDsJSON), &newsIDs)
		item.NewsCount = len(newsIDs)
		items = append(items, item)
	}

	return &EventResult{
		Total: len(items),
		Items: items,
	}, nil
}

// GetHotEventDetail 获取热点事件详情
func (s *SQLiteStore) GetHotEventDetail(ctx context.Context, eventID string) (*HotEvent, error) {
	var event HotEvent
	var keywordsJSON, stocksJSON, sourceCountsJSON, newsItemIDsJSON string
	var createdAtStr, updatedAtStr string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, keywords, related_stocks, hot_index, source_counts, news_item_ids, status, created_at, updated_at
		FROM hot_events WHERE id = ?
	`, eventID).Scan(
		&event.ID,
		&event.Title,
		&keywordsJSON,
		&stocksJSON,
		&event.HotIndex,
		&sourceCountsJSON,
		&newsItemIDsJSON,
		&event.Status,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	event.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	event.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	json.Unmarshal([]byte(keywordsJSON), &event.Keywords)
	json.Unmarshal([]byte(stocksJSON), &event.RelatedStocks)
	json.Unmarshal([]byte(sourceCountsJSON), &event.SourceCounts)
	json.Unmarshal([]byte(newsItemIDsJSON), &event.NewsItemIDs)

	return &event, nil
}

// DeleteExpiredNews 删除过期新闻
func (s *SQLiteStore) DeleteExpiredNews(ctx context.Context, days int) (int, error) {
	expireTime := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM news_items WHERE publish_time < ?
	`, expireTime.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// Close 关闭数据库连接
func (s *SQLiteStore) Close() error {
	if s.owner != nil {
		return s.owner.Close()
	}
	return nil
}

// generateID 生成唯一ID
func generateID() string {
	return fmt.Sprintf("news_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// joinStrings 连接字符串
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += "," + strs[i]
	}
	return result
}
