package quality

import (
	"time"
)

// KlineDataFetcher 是获取 K 线数据的抽象接口，
// 由上层（如 tdx.KlineStore）实现，用于将真实数据源接入质量门。
type KlineDataFetcher interface {
	GetKline(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error)
}

// QualityDataSource 封装了数据质量门所需的全部数据源。
type QualityDataSource struct {
	Fetcher     KlineDataFetcher
	StockMeta   StockMetaProvider
	TradingDays TradingDayProvider
}

// StockMetaProvider 股票元数据查询接口。
type StockMetaProvider interface {
	GetStockName(code string) string
	ListStockCodes() []string
}

// TradingDayProvider 交易日历接口。
type TradingDayProvider interface {
	GetExpectedTradingDays(start, end time.Time) []time.Time
}

// KlineDataFetcherFunc 函数式适配器。
type KlineDataFetcherFunc func(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error)

func (f KlineDataFetcherFunc) GetKline(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error) {
	return f(code, ktype, startDate, endDate)
}

// FetchKlineData 将一组股票代码的 K 线数据填入 EvaluateOptions。
func (ds *QualityDataSource) FetchKlineData(codes []string, ktype uint8, startDate, endDate string, opts *EvaluateOptions) error {
	if ds == nil || ds.Fetcher == nil {
		return nil
	}
	if opts.KlineData == nil {
		opts.KlineData = make(map[string][]KlineRecord, len(codes))
	}
	for _, code := range codes {
		records, err := ds.Fetcher.GetKline(code, ktype, startDate, endDate)
		if err != nil {
			continue
		}
		opts.KlineData[code] = records
	}
	return nil
}

// FetchKlineDataForCode 获取单只股票的 K 线数据。
func (ds *QualityDataSource) FetchKlineDataForCode(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error) {
	if ds == nil || ds.Fetcher == nil {
		return nil, nil
	}
	return ds.Fetcher.GetKline(code, ktype, startDate, endDate)
}
