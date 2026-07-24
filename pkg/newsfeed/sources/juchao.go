package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/newsfeed"
)

// JuChaoSource 巨潮资讯数据源
type JuChaoSource struct {
	baseURL string
}

// NewJuChaoSource 创建巨潮资讯数据源
func NewJuChaoSource() *JuChaoSource {
	return &JuChaoSource{
		baseURL: "http://www.cninfo.com.cn",
	}
}

// Name 返回数据源名称
func (s *JuChaoSource) Name() newsfeed.SourceType {
	return newsfeed.SourceJuChao
}

// RefreshInterval 返回建议的刷新间隔
func (s *JuChaoSource) RefreshInterval() time.Duration {
	return 10 * time.Minute
}

// HealthCheck 检查数据源健康状态
func (s *JuChaoSource) HealthCheck(ctx context.Context) bool {
	_, err := httpGet(ctx, s.baseURL, nil)
	return err == nil
}

// Fetch 获取最新新闻列表
func (s *JuChaoSource) Fetch(ctx context.Context) ([]*newsfeed.NewsItem, error) {
	url := "http://www.cninfo.com.cn/new/hisAnnouncement/query?pageNum=1&pageSize=15&column=szse&tabName=fulltext&searchkey=&secid=&category=&trade=&seDate=&sortName=&sortType=&limit=&showTitle=false"
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result juChaoResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data), nil
}

// FetchByStock 获取指定股票相关的新闻
func (s *JuChaoSource) FetchByStock(ctx context.Context, code string) ([]*newsfeed.NewsItem, error) {
	// 巨潮资讯需要完整的市场代码格式
	var market string
	if len(code) == 6 {
		if code[0] == '6' {
			market = "sh"
		} else {
			market = "sz"
		}
	} else {
		return []*newsfeed.NewsItem{}, nil
	}

	url := fmt.Sprintf("http://www.cninfo.com.cn/new/hisAnnouncement/query?pageNum=1&pageSize=15&column=%s&tabName=fulltext&searchkey=&secid=%s,%s&category=&trade=&seDate=&sortName=&sortType=&limit=&showTitle=false",
		market, market, code)
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result juChaoResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data), nil
}

// FetchByKeyword 获取指定关键词相关的新闻
func (s *JuChaoSource) FetchByKeyword(ctx context.Context, keyword string) ([]*newsfeed.NewsItem, error) {
	url := fmt.Sprintf("http://www.cninfo.com.cn/new/hisAnnouncement/query?pageNum=1&pageSize=15&column=&tabName=fulltext&searchkey=%s&secid=&category=&trade=&seDate=&sortName=&sortType=&limit=&showTitle=false",
		keyword)
	data, err := httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	var result juChaoResponse
	if err := parseJSON(data, &result); err != nil {
		return nil, err
	}

	return s.parseNewsItems(result.Data), nil
}

// parseNewsItems 解析新闻列表
func (s *JuChaoSource) parseNewsItems(items []juChaoItem) []*newsfeed.NewsItem {
	var news []*newsfeed.NewsItem
	for _, item := range items {
		newsItem := &newsfeed.NewsItem{
			ID:          generateID(),
			Source:      newsfeed.SourceJuChao,
			NewsType:    newsfeed.NewsTypeAnnouncement,
			Title:       cleanText(item.Title),
			Summary:     cleanText(item.Title),
			Content:     "", // 巨潮资讯详情需要单独请求
			PublishTime: parseTime(item.AnnouncementTime, []string{"2006-01-02 15:04:05"}),
			HotScore:    0,
			Tags:        []string{item.Category},
			RelatedStocks: item.SecCodes,
			URL:         s.baseURL + "/new/AnnouncementDetail?announcementId=" + item.AnnouncementID,
			OriginalID:  fmt.Sprintf("jc_%s", item.AnnouncementID),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		news = append(news, newsItem)
	}
	return news
}

// juChaoResponse 巨潮资讯API响应结构
type juChaoResponse struct {
	Data []juChaoItem `json:"announcements"`
}

type juChaoItem struct {
	AnnouncementID   string   `json:"announcementId"`
	Title            string   `json:"announcementTitle"`
	AnnouncementTime string   `json:"announcementTime"`
	Category         string   `json:"category"`
	SecCodes         []string `json:"secCodes"`
	SecNames         []string `json:"secNames"`
}
