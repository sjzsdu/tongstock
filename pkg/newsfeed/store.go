package newsfeed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	db *sql.DB
}

// NewSQLiteStore 创建SQLite存储
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// 创建表
	if err := createTables(db); err != nil {
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

// createTables 创建数据库表
func createTables(db *sql.DB) error {
	newsTable := `
	CREATE TABLE IF NOT EXISTS news_items (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		news_type TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT,
		content TEXT,
		publish_time DATETIME NOT NULL,
		hot_score INTEGER DEFAULT 0,
		tags TEXT,
		related_stocks TEXT,
		url TEXT,
		original_id TEXT UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_news_source ON news_items(source);
	CREATE INDEX IF NOT EXISTS idx_news_publish_time ON news_items(publish_time);
	CREATE INDEX IF NOT EXISTS idx_news_hot_score ON news_items(hot_score);
	CREATE INDEX IF NOT EXISTS idx_news_related_stocks ON news_items(related_stocks);
	`

	eventTable := `
	CREATE TABLE IF NOT EXISTS hot_events (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		keywords TEXT,
		related_stocks TEXT,
		hot_index INTEGER DEFAULT 0,
		source_counts TEXT,
		news_item_ids TEXT,
		status TEXT DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_event_hot_index ON hot_events(hot_index);
	CREATE INDEX IF NOT EXISTS idx_event_status ON hot_events(status);
	CREATE INDEX IF NOT EXISTS idx_event_updated_at ON hot_events(updated_at);
	`

	if _, err := db.Exec(newsTable); err != nil {
		return err
	}

	if _, err := db.Exec(eventTable); err != nil {
		return err
	}

	return nil
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
	}

	return tx.Commit()
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
	query := `SELECT id, source, news_type, title, summary, publish_time, hot_score, tags, related_stocks, url FROM news_items WHERE 1=1`
	args := []interface{}{}

	// 按来源筛选
	if len(filter.Sources) > 0 {
		placeholders := make([]string, len(filter.Sources))
		for i, source := range filter.Sources {
			placeholders[i] = "?"
			args = append(args, source)
		}
		query += fmt.Sprintf(" AND source IN (%s)", joinStrings(placeholders))
	}

	// 按新闻类型筛选
	if len(filter.NewsTypes) > 0 {
		placeholders := make([]string, len(filter.NewsTypes))
		for i, nt := range filter.NewsTypes {
			placeholders[i] = "?"
			args = append(args, nt)
		}
		query += fmt.Sprintf(" AND news_type IN (%s)", joinStrings(placeholders))
	}

	// 按时间范围筛选
	if filter.StartTime != nil {
		query += " AND publish_time >= ?"
		args = append(args, filter.StartTime.Format(time.RFC3339))
	}
	if filter.EndTime != nil {
		query += " AND publish_time <= ?"
		args = append(args, filter.EndTime.Format(time.RFC3339))
	}

	// 按热度筛选
	if filter.HotScoreMin > 0 {
		query += " AND hot_score >= ?"
		args = append(args, filter.HotScoreMin)
	}

	// 排序
	sortBy := filter.SortBy
	if sortBy == "" {
		sortBy = "time"
	}
	if sortBy == "hot" {
		query += " ORDER BY hot_score DESC, publish_time DESC"
	} else {
		query += " ORDER BY publish_time DESC"
	}

	// 分页
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageNum <= 0 {
		filter.PageNum = 1
	}
	offset := (filter.PageNum - 1) * filter.PageSize
	query += " LIMIT ? OFFSET ?"
	args = append(args, filter.PageSize, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NewsSummary
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

	// 获取总数
	countQuery := `SELECT COUNT(*) FROM news_items WHERE 1=1`
	countArgs := args[:len(args)-2] // 移除 LIMIT 和 OFFSET 参数
	var total int
	err = s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, err
	}

	return &FeedResult{
		Total:    total,
		Items:    items,
		PageNum:  filter.PageNum,
		PageSize: filter.PageSize,
	}, nil
}

// SaveHotEvent 保存热点事件
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
	if s.db != nil {
		return s.db.Close()
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
