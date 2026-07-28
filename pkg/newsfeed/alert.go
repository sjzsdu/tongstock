package newsfeed

import (
	"context"
	"fmt"
	"time"
)

// AlertLevel 推送级别
type AlertLevel string

const (
	AlertLevelCritical  AlertLevel = "critical"  // 紧急（重大利好/利空）
	AlertLevelImportant AlertLevel = "important" // 重要（关联资讯）
	AlertLevelNormal    AlertLevel = "normal"    // 普通（日常资讯）
)

// AlertType 预警类型
type AlertType string

const (
	AlertTypePositive AlertType = "positive" // 正面消息
	AlertTypeNegative AlertType = "negative" // 负面消息
	AlertTypeNeutral  AlertType = "neutral"  // 中性消息
)

// AlertRule 推送规则
type AlertRule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Level       AlertLevel `json:"level"`
	StockCodes  []string   `json:"stockCodes"`  // 关注的股票代码，为空表示全部
	MinHotScore int        `json:"minHotScore"` // 最低热度阈值
	Sentiment   AlertType  `json:"sentiment"`   // 情绪类型过滤，neutral表示不过滤
	Enabled     bool       `json:"enabled"`
	LastTrigger time.Time  `json:"lastTrigger"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// AlertRecord 推送记录
type AlertRecord struct {
	ID          string     `json:"id"`
	RuleID      string     `json:"ruleID"`
	NewsID      string     `json:"newsID"`
	StockCode   string     `json:"stockCode"`
	Level       AlertLevel `json:"level"`
	Type        AlertType  `json:"type"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Source      SourceType `json:"source"`
	Read        bool       `json:"read"`
	TriggerTime time.Time  `json:"triggerTime"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// AlertService 预警服务
type AlertService struct {
	store        *SQLiteStore
	sentimentSvc *SentimentService
	rules        []*AlertRule
	watchlist    []string // 关注列表
}

// NewAlertService 创建预警服务
func NewAlertService(store *SQLiteStore, sentimentSvc *SentimentService) *AlertService {
	return &AlertService{
		store:        store,
		sentimentSvc: sentimentSvc,
		rules:        []*AlertRule{},
		watchlist:    []string{},
	}
}

// SetWatchlist 设置关注列表
func (a *AlertService) SetWatchlist(codes []string) {
	a.watchlist = codes
}

// AddRule 添加推送规则
func (a *AlertService) AddRule(rule *AlertRule) {
	rule.ID = generateID()
	rule.CreatedAt = time.Now()
	rule.LastTrigger = time.Time{}
	a.rules = append(a.rules, rule)
}

// RemoveRule 移除推送规则
func (a *AlertService) RemoveRule(ruleID string) {
	for i, rule := range a.rules {
		if rule.ID == ruleID {
			a.rules = append(a.rules[:i], a.rules[i+1:]...)
			return
		}
	}
}

// GetRules 获取所有规则
func (a *AlertService) GetRules() []*AlertRule {
	return a.rules
}

// GetRule 获取规则
func (a *AlertService) GetRule(ruleID string) *AlertRule {
	for _, rule := range a.rules {
		if rule.ID == ruleID {
			return rule
		}
	}
	return nil
}

// UpdateRule 更新规则
func (a *AlertService) UpdateRule(ruleID string, updates *AlertRule) bool {
	for _, rule := range a.rules {
		if rule.ID == ruleID {
			if updates.Name != "" {
				rule.Name = updates.Name
			}
			if updates.Level != "" {
				rule.Level = updates.Level
			}
			if len(updates.StockCodes) > 0 {
				rule.StockCodes = updates.StockCodes
			}
			if updates.MinHotScore > 0 {
				rule.MinHotScore = updates.MinHotScore
			}
			if updates.Sentiment != "" {
				rule.Sentiment = updates.Sentiment
			}
			rule.Enabled = updates.Enabled
			return true
		}
	}
	return false
}

// CheckNews 检查新闻是否触发预警
func (a *AlertService) CheckNews(ctx context.Context, news *NewsItem) ([]*AlertRecord, error) {
	var records []*AlertRecord

	// 分析情绪
	sentiment := a.sentimentSvc.analyzer.AnalyzeNews(news)

	// 确定预警类型
	alertType := AlertTypeNeutral
	if sentiment.Type == SentimentPositive {
		alertType = AlertTypePositive
	} else if sentiment.Type == SentimentNegative {
		alertType = AlertTypeNegative
	}

	// 确定预警级别
	alertLevel := a.determineAlertLevel(news, sentiment)

	// 检查是否匹配关注列表
	matchesWatchlist := a.matchesWatchlist(news)

	// 如果匹配关注列表且满足条件，生成预警记录
	if matchesWatchlist {
		// 检查是否匹配规则
		for _, rule := range a.rules {
			if !rule.Enabled {
				continue
			}

			// 检查股票代码匹配
			if !a.matchesStockCodes(news, rule.StockCodes) {
				continue
			}

			// 检查热度阈值
			if news.HotScore < rule.MinHotScore {
				continue
			}

			// 检查情绪类型
			if rule.Sentiment != AlertTypeNeutral && rule.Sentiment != alertType {
				continue
			}

			// 使用规则级别或自动确定的级别
			level := rule.Level
			if level == "" {
				level = alertLevel
			}

			record := &AlertRecord{
				ID:          generateID(),
				RuleID:      rule.ID,
				NewsID:      news.ID,
				StockCode:   news.RelatedStocks[0], // 取第一个关联股票
				Level:       level,
				Type:        alertType,
				Title:       news.Title,
				Summary:     news.Summary,
				Source:      news.Source,
				Read:        false,
				TriggerTime: time.Now(),
				CreatedAt:   time.Now(),
			}

			// 保存记录
			if err := a.saveAlertRecord(ctx, record); err != nil {
				return nil, err
			}

			records = append(records, record)

			// 更新规则最后触发时间
			rule.LastTrigger = time.Now()
		}

		// 如果没有匹配的规则，但新闻满足条件，生成默认预警
		if len(records) == 0 && alertLevel != AlertLevelNormal {
			record := &AlertRecord{
				ID:          generateID(),
				RuleID:      "",
				NewsID:      news.ID,
				StockCode:   news.RelatedStocks[0],
				Level:       alertLevel,
				Type:        alertType,
				Title:       news.Title,
				Summary:     news.Summary,
				Source:      news.Source,
				Read:        false,
				TriggerTime: time.Now(),
				CreatedAt:   time.Now(),
			}

			if err := a.saveAlertRecord(ctx, record); err != nil {
				return nil, err
			}

			records = append(records, record)
		}
	}

	return records, nil
}

// determineAlertLevel 根据新闻内容确定预警级别
func (a *AlertService) determineAlertLevel(news *NewsItem, sentiment SentimentResult) AlertLevel {
	// 紧急级别：热度非常高且情绪强烈
	if news.HotScore > 1000 && sentiment.Confidence > 0.7 {
		if sentiment.Score > 0.5 || sentiment.Score < -0.5 {
			return AlertLevelCritical
		}
	}

	// 重要级别：热度较高或情绪较强烈
	if news.HotScore > 500 || sentiment.Confidence > 0.5 {
		return AlertLevelImportant
	}

	return AlertLevelNormal
}

// matchesWatchlist 检查新闻是否匹配关注列表
func (a *AlertService) matchesWatchlist(news *NewsItem) bool {
	if len(a.watchlist) == 0 {
		return true // 关注列表为空，匹配所有
	}

	for _, stock := range news.RelatedStocks {
		for _, watched := range a.watchlist {
			if stock == watched {
				return true
			}
		}
	}

	return false
}

// matchesStockCodes 检查新闻是否匹配规则中的股票代码
func (a *AlertService) matchesStockCodes(news *NewsItem, stockCodes []string) bool {
	if len(stockCodes) == 0 {
		return true // 股票代码为空，匹配所有
	}

	for _, stock := range news.RelatedStocks {
		for _, code := range stockCodes {
			if stock == code {
				return true
			}
		}
	}

	return false
}

// saveAlertRecord 保存预警记录到存储
func (a *AlertService) saveAlertRecord(ctx context.Context, record *AlertRecord) error {
	_, err := a.store.db.ExecContext(ctx, `
		INSERT INTO alert_records (id, rule_id, news_id, stock_code, level, type, title, summary, source, read, trigger_time, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.RuleID, record.NewsID, record.StockCode,
		record.Level, record.Type, record.Title, record.Summary,
		record.Source, record.Read, record.TriggerTime.Format(time.RFC3339),
		record.CreatedAt.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("保存预警记录失败: %w", err)
	}

	return nil
}

// GetAlertRecords 获取预警记录
func (a *AlertService) GetAlertRecords(ctx context.Context, limit int, read bool) ([]*AlertRecord, error) {
	query := `
		SELECT id, rule_id, news_id, stock_code, level, type, title, summary, source, read, trigger_time, created_at
		FROM alert_records
		WHERE read = ?
		ORDER BY trigger_time DESC
		LIMIT ?
	`

	rows, err := a.store.db.QueryContext(ctx, query, read, limit)
	if err != nil {
		return nil, fmt.Errorf("查询预警记录失败: %w", err)
	}
	defer rows.Close()

	var records []*AlertRecord
	for rows.Next() {
		var record AlertRecord
		var triggerTimeStr, createdAtStr string
		if err := rows.Scan(
			&record.ID, &record.RuleID, &record.NewsID, &record.StockCode,
			&record.Level, &record.Type, &record.Title, &record.Summary,
			&record.Source, &record.Read, &triggerTimeStr, &createdAtStr); err != nil {
			return nil, err
		}

		record.TriggerTime, _ = time.Parse(time.RFC3339, triggerTimeStr)
		record.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)

		records = append(records, &record)
	}

	return records, nil
}

// MarkAlertAsRead 标记预警为已读
func (a *AlertService) MarkAlertAsRead(ctx context.Context, alertID string) error {
	_, err := a.store.db.ExecContext(ctx, `
		UPDATE alert_records SET read = true WHERE id = ?
	`, alertID)

	if err != nil {
		return fmt.Errorf("标记预警已读失败: %w", err)
	}

	return nil
}

// MarkAllAlertsAsRead 标记所有预警为已读
func (a *AlertService) MarkAllAlertsAsRead(ctx context.Context) error {
	_, err := a.store.db.ExecContext(ctx, `
		UPDATE alert_records SET read = true WHERE read = false
	`)

	if err != nil {
		return fmt.Errorf("标记所有预警已读失败: %w", err)
	}

	return nil
}

// GetUnreadCount 获取未读预警数量
func (a *AlertService) GetUnreadCount(ctx context.Context) (int, error) {
	var count int
	err := a.store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM alert_records WHERE read = false
	`).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("查询未读数量失败: %w", err)
	}

	return count, nil
}

// CleanOldAlerts 清理旧的预警记录
func (a *AlertService) CleanOldAlerts(ctx context.Context, days int) error {
	expireTime := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	_, err := a.store.db.ExecContext(ctx, `
		DELETE FROM alert_records WHERE created_at < ?
	`, expireTime.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("清理旧预警记录失败: %w", err)
	}

	return nil
}
