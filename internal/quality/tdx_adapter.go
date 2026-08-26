package quality

import (
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx"
)

// TdxKlineAdapter 将 tdx.KlineStore 适配为质量门的 KlineDataFetcher。
type TdxKlineAdapter struct {
	store *tdx.KlineStore
}

// NewTdxKlineAdapter 创建基于 tdx.KlineStore 的适配器。
func NewTdxKlineAdapter(store *tdx.KlineStore) *TdxKlineAdapter {
	return &TdxKlineAdapter{store: store}
}

// GetKline 实现 KlineDataFetcher 接口，从数据库获取 K 线数据并转换为 quality.KlineRecord。
func (a *TdxKlineAdapter) GetKline(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error) {
	if a.store == nil {
		return nil, nil
	}
	klines, err := a.store.GetKline(code, ktype, startDate, endDate)
	if err != nil {
		return nil, err
	}
	records := make([]KlineRecord, 0, len(klines))
	for _, k := range klines {
		records = append(records, KlineRecord{
			Date:   k.Time,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
			Amount: k.Amount,
		})
	}
	return records, nil
}

// GetLatestDate 获取某只股票的最新 K 线日期。
func (a *TdxKlineAdapter) GetLatestDate(code string, ktype uint8) (time.Time, error) {
	dateStr, err := a.store.GetLatestDate(code, ktype)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("20060102", dateStr)
}

// DetectAndCleanCorrupted 检测并清理异常数据，返回清理的记录数。
func (a *TdxKlineAdapter) DetectAndCleanCorrupted(code string, ktype uint8) (int, error) {
	return a.store.DetectAndCleanCorruptedKlines(code, ktype)
}
