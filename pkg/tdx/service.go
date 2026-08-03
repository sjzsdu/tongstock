package tdx

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/cache"
	"github.com/sjzsdu/tongstock/pkg/storage"
	protocol "github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type SyncMode string

const (
	SyncModeAuto        SyncMode = "auto"
	SyncModeFull        SyncMode = "full"
	SyncModeIncremental SyncMode = "incremental"
)

type KlineSyncResult struct {
	Code   string          `json:"code"`
	Mode   SyncMode        `json:"mode"`
	Status string          `json:"status"`
	Count  int             `json:"count"`
	State  *KlineSyncState `json:"state,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type KlineBatchSyncResult struct {
	Total   int               `json:"total"`
	Success int               `json:"success"`
	Failed  int               `json:"failed"`
	Results []KlineSyncResult `json:"results"`
}

// Service wraps an Executor + local stores for cached data access. All
// TDX protocol operations go through the Executor so that the pool (or
// single client) remains the sole owner of connection lifecycle and
// concurrency.
type Service struct {
	executor     Executor
	ownsExecutor bool
	ownsStorage  bool
	storage      *storage.Storage
	codes        *CodeStore
	klines       *KlineStore
	workdays     *WorkdayStore
	xdxr         *XdXrStore
	finance      *FinanceStore
	company      *CompanyStore
	block        *BlockStore
	// Client exposes the underlying *Client for backwards-compatible call
	// sites in handlers.go. New code should use ExecDo or typed Fetch/Sync
	// methods instead of touching Client directly — Client may be nil when
	// the Service is backed by a Pool (use ExecDo to borrow a Client).
	Client *Client
}

// NewService creates a new Service with a shared storage instance using a
// single *Client wrapped in a singleExecutor for backwards compatibility.
// New code should prefer NewServiceWithExecutor to pass a Pool or
// singleExecutor explicitly.
func NewService(client *Client, s *storage.Storage) (*Service, error) {
	if client == nil {
		return nil, errors.New("nil client")
	}
	svc, err := NewServiceWithExecutor(newSingleExecutor(client), s)
	if err != nil {
		return nil, err
	}
	svc.Client = client
	svc.ownsExecutor = true
	return svc, nil
}

// NewOwnedService is the CLI composition helper. Unlike NewService, Close
// also releases the supplied Storage. Server code must keep using
// NewServiceWithExecutor because App owns its shared Storage.
func NewOwnedService(client *Client, s *storage.Storage) (*Service, error) {
	svc, err := NewService(client, s)
	if err != nil {
		return nil, err
	}
	svc.ownsStorage = true
	return svc, nil
}

// NewServiceWithExecutor creates a Service backed by the given Executor
// (Pool or singleExecutor). The Executor owns the underlying connection(s)
// and must outlive the Service. If the executor is a singleExecutor its
// underlying Client is also exposed via Service.Client for backwards
// compatibility; Pool-backed executors leave Client nil and callers must
// use ExecDo to borrow a Client.
func NewServiceWithExecutor(exec Executor, s *storage.Storage) (*Service, error) {
	if exec == nil {
		return nil, errors.New("nil executor")
	}
	if s == nil {
		return nil, errors.New("nil storage")
	}

	svc := &Service{executor: exec, storage: s}
	if se, ok := exec.(*singleExecutor); ok {
		svc.Client = se.client
	}

	sharedCache, err := cache.NewSQLiteCacheWithDB(s.DB())
	if err != nil {
		return nil, err
	}
	codes, err := NewCodeStore(sharedCache)
	if err != nil {
		return nil, err
	}
	svc.codes = codes

	klines, err := NewKlineStore(s)
	if err != nil {
		_ = codes.Close()
		return nil, err
	}
	svc.klines = klines

	w, err := NewWorkdayStore(s)
	if err != nil {
		_ = klines.Close()
		_ = codes.Close()
		return nil, err
	}
	svc.workdays = w

	svc.xdxr = &XdXrStore{cache: codes.cache, ttl: xdxrTTL}
	svc.finance = &FinanceStore{cache: codes.cache, ttl: financeTTL}
	svc.company = &CompanyStore{cache: codes.cache, ttl: companyTTL}
	svc.block = &BlockStore{cache: codes.cache, ttl: blockTTL}

	return svc, nil
}

// Close closes the service along with all internal stores. Services created
// by NewService also close their internally-created single executor; callers
// of NewServiceWithExecutor retain executor ownership.
func (s *Service) Close() error {
	var errs []error
	if s.codes != nil {
		if err := s.codes.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.klines != nil {
		if err := s.klines.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.ownsExecutor && s.executor != nil {
		if err := s.executor.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.workdays != nil {
		if err := s.workdays.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.ownsStorage && s.storage != nil {
		if err := s.storage.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// GetSyncState returns the sync state for a given code and kline type.
func (s *Service) GetSyncState(code string, ktype uint8) (*KlineSyncState, error) {
	if s.klines == nil {
		return nil, errors.New("kline store not initialized")
	}
	return s.klines.GetSyncState(code, ktype)
}

func (s *Service) withClient(fn func(c *Client) error) error {
	return s.withClientContext(context.Background(), fn)
}

func (s *Service) withClientContext(ctx context.Context, fn func(c *Client) error) error {
	if s.executor == nil {
		return errors.New("executor not initialized")
	}
	return s.executor.DoContext(ctx, fn)
}

// ExecDo exposes the executor's Do method for handlers that need to call
// TDX methods that are not wrapped by typed Service methods. It is the only
// sanctioned way to obtain a *Client inside the HTTP layer.
func (s *Service) ExecDo(fn func(c *Client) error) error {
	return s.withClient(fn)
}

// ExecDoContext is the context-aware form of ExecDo. Cancellation applies
// while waiting for an executor slot; protocol calls still use Client-level
// deadlines for in-flight I/O.
func (s *Service) ExecDoContext(ctx context.Context, fn func(c *Client) error) error {
	return s.withClientContext(ctx, fn)
}

// FetchCodes tries to load codes from cache first, then fetches from the Client if needed.
func (s *Service) FetchCodes(exchange protocol.Exchange) ([]*protocol.CodeItem, error) {
	if s.codes != nil {
		if codes, err := s.codes.GetCodes(exchange); err == nil && codes != nil && len(codes) > 0 {
			return codes, nil
		}
	}
	var items []*protocol.CodeItem
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetCode(exchange)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.codes != nil {
		_ = s.codes.SaveCodes(items, exchange)
	}
	return items, nil
}

func (s *Service) FetchXdXr(code string) ([]*protocol.XdXrItem, error) {
	if s.xdxr != nil {
		if items, err := s.xdxr.Get(code); err == nil && items != nil {
			return items, nil
		}
	}
	var items []*protocol.XdXrItem
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetXdXrInfo(code)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.xdxr != nil {
		_ = s.xdxr.Save(code, items)
	}
	return items, nil
}

func (s *Service) FetchFinance(code string) (*protocol.FinanceInfo, error) {
	if s.finance != nil {
		if info, err := s.finance.Get(code); err == nil && info != nil {
			return info, nil
		}
	}
	var info *protocol.FinanceInfo
	err := s.withClient(func(c *Client) error {
		var e error
		info, e = c.GetFinanceInfo(code)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.finance != nil {
		_ = s.finance.Save(code, info)
	}
	return info, nil
}

func (s *Service) FetchCompanyCategory(code string) ([]*protocol.CompanyCategoryItem, error) {
	if s.company != nil {
		if items, err := s.company.GetCategory(code); err == nil && items != nil {
			return items, nil
		}
	}
	return s.RefreshCompanyCategory(code)
}

func (s *Service) RefreshCompanyCategory(code string) ([]*protocol.CompanyCategoryItem, error) {
	var items []*protocol.CompanyCategoryItem
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetCompanyInfoCategory(code)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.company != nil {
		_ = s.company.SaveCategory(code, items)
	}
	return items, nil
}

func (s *Service) FetchCompanyContent(code, filename string, start, length uint32) (string, error) {
	if s.company != nil && start == 0 && length == 0 {
		if content, err := s.company.GetContent(code, filename); err == nil && content != "" {
			return content, nil
		}
	}
	var content string
	err := s.withClient(func(c *Client) error {
		var e error
		content, e = c.GetCompanyInfoContentAll(code, filename, start, length)
		return e
	})
	if err != nil {
		return "", err
	}
	if s.company != nil && start == 0 && length == 0 {
		_ = s.company.SaveContent(code, filename, content)
	}
	return content, nil
}

func (s *Service) FetchBlock(blockFile string) ([]*protocol.BlockItem, error) {
	if s.block != nil {
		if items, err := s.block.Get(blockFile); err == nil && items != nil {
			return items, nil
		}
	}
	var items []*protocol.BlockItem
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetBlockInfoAll(blockFile)
		return e
	})
	if err != nil {
		return nil, err
	}
	if s.block != nil {
		_ = s.block.Save(blockFile, items)
	}
	return items, nil
}

func (s *Service) FetchKlineAll(code string, ktype uint8) ([]*protocol.Kline, error) {
	if !isDailyKline(ktype) {
		if ktype == protocol.TypeKlineMinute {
			var klines []*protocol.Kline
			err := s.withClient(func(c *Client) error {
				var e error
				klines, e = c.GetKline(code, ktype, 0, 800)
				return e
			})
			if err != nil {
				return s.FetchKlineAll(code, protocol.TypeKlineMinute2)
			}
			return klines, nil
		}
		var klines []*protocol.Kline
		err := s.withClient(func(c *Client) error {
			var e error
			klines, e = c.GetKlineAll(code, ktype)
			return e
		})
		return klines, err
	}

	latest, err := s.klines.GetLatestDate(code, ktype)
	if err != nil && !errors.Is(err, ErrKlineNotFound) {
		var klines []*protocol.Kline
		e := s.withClient(func(c *Client) error {
			var ee error
			klines, ee = c.GetKlineAll(code, ktype)
			return ee
		})
		return klines, e
	}

	if errors.Is(err, ErrKlineNotFound) || latest == "" {
		return s.fetchAndSaveKlineAll(code, ktype)
	}

	now := marketNow()
	today := now.Format("20060102")
	expected := lastCompleteTradingDate(now)

	if latest >= expected && !isDuringTradingHours(now) {
		return s.klines.GetKline(code, ktype, "", "")
	}

	if latest == today && isDuringTradingHours(now) {
		return s.refreshTodayKline(code, ktype)
	}

	return s.fetchIncrementalKline(code, ktype, latest)
}

func (s *Service) SyncDailyKline(code string, mode SyncMode) KlineSyncResult {
	if mode == "" {
		mode = SyncModeAuto
	}
	ktype := ParseKlineType("day")
	result := KlineSyncResult{Code: code, Mode: mode, Status: "ok"}

	var klines []*protocol.Kline
	var err error
	switch mode {
	case SyncModeFull:
		klines, err = s.fetchAndSaveKlineAll(code, ktype)
	case SyncModeIncremental:
		latest, latestErr := s.klines.GetLatestDate(code, ktype)
		if errors.Is(latestErr, ErrKlineNotFound) || latest == "" {
			klines, err = s.fetchAndSaveKlineAll(code, ktype)
		} else if latestErr != nil {
			err = latestErr
		} else {
			klines, err = s.fetchIncrementalKline(code, ktype, latest)
		}
	case SyncModeAuto:
		klines, err = s.FetchKlineAll(code, ktype)
	default:
		err = fmt.Errorf("unsupported sync mode: %s", mode)
	}

	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		_ = s.klines.UpdateSyncState(code, ktype, result.Status, result.Error)
		result.State, _ = s.klines.GetSyncState(code, ktype)
		return result
	}
	result.Count = len(klines)
	_ = s.klines.UpdateSyncState(code, ktype, result.Status, "")
	result.State, _ = s.klines.GetSyncState(code, ktype)
	return result
}

func (s *Service) SyncDailyKlines(codes []string, mode SyncMode, concurrency int) KlineBatchSyncResult {
	if concurrency <= 0 {
		concurrency = 3
	}
	out := KlineBatchSyncResult{Total: len(codes), Results: make([]KlineSyncResult, len(codes))}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, code := range codes {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c string) {
			defer wg.Done()
			defer func() { <-sem }()
			out.Results[idx] = s.SyncDailyKline(c, mode)
		}(i, code)
	}
	wg.Wait()
	for _, r := range out.Results {
		if r.Status == "ok" {
			out.Success++
		} else {
			out.Failed++
		}
	}
	return out
}

func (s *Service) fetchAndSaveKlineAll(code string, ktype uint8) ([]*protocol.Kline, error) {
	var klines []*protocol.Kline
	err := s.withClient(func(c *Client) error {
		var e error
		klines, e = c.GetKlineAll(code, ktype)
		return e
	})
	if err != nil {
		return nil, err
	}
	klines = FilterValidKlines(klines)
	if err := s.klines.ReplaceKlines(code, ktype, klines); err != nil {
		return nil, err
	}
	return klines, nil
}

func (s *Service) refreshTodayKline(code string, ktype uint8) ([]*protocol.Kline, error) {
	var fresh []*protocol.Kline
	err := s.withClient(func(c *Client) error {
		var e error
		fresh, e = c.GetKline(code, ktype, 0, 1)
		return e
	})
	if err != nil {
		return s.klines.GetKline(code, ktype, "", "")
	}
	if len(fresh) > 0 {
		fresh = FilterValidKlines(fresh)
		_ = s.klines.SaveKline(code, ktype, fresh)
	}
	return s.klines.GetKline(code, ktype, "", "")
}

func (s *Service) fetchIncrementalKline(code string, ktype uint8, latest string) ([]*protocol.Kline, error) {
	var klines []*protocol.Kline
	err := s.withClient(func(c *Client) error {
		var e error
		klines, e = c.GetKlineUntil(code, ktype, func(k *protocol.Kline) bool {
			return k.Time.Format("20060102") < latest
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	klines = FilterValidKlines(klines)
	if len(klines) > 0 {
		_ = s.klines.SaveKline(code, ktype, klines)
	}
	return s.klines.GetKline(code, ktype, "", "")
}

func FilterValidKlines(klines []*protocol.Kline) []*protocol.Kline {
	if len(klines) == 0 {
		return klines
	}
	now := time.Now()
	maxDate := now.AddDate(0, 0, 1)
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)

	filtered := make([]*protocol.Kline, 0, len(klines))
	var lastClose float64
	for _, k := range klines {
		if k.Time.Before(minDate) || k.Time.After(maxDate) {
			continue
		}
		if validateKline(k) != "" {
			continue
		}
		if lastClose > 0 {
			ratio := k.Close / lastClose
			if ratio > 3.0 || ratio < 0.33 {
				continue
			}
		}
		lastClose = k.Close
		filtered = append(filtered, k)
	}
	return filtered
}

func (s *Service) CleanAndRefetchKlines(code string, ktype uint8) ([]*protocol.Kline, error) {
	deleted, err := s.klines.DetectAndCleanCorruptedKlines(code, ktype)
	if err != nil {
		return nil, fmt.Errorf("清理异常数据失败: %w", err)
	}
	if deleted == 0 {
		return s.klines.GetKline(code, ktype, "", "")
	}

	log.Printf("[kline] 清理了 %d 条异常数据，重新获取 %s 的K线数据", deleted, code)

	var klines []*protocol.Kline
	err = s.withClient(func(c *Client) error {
		var e error
		klines, e = c.GetKlineAll(code, ktype)
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("重新获取K线数据失败: %w", err)
	}

	klines = FilterValidKlines(klines)
	if err := s.klines.ReplaceKlines(code, ktype, klines); err != nil {
		return nil, fmt.Errorf("保存K线数据失败: %w", err)
	}

	return klines, nil
}

func (s *Service) FetchKline(code string, ktype uint8, start, count uint16) ([]*protocol.Kline, error) {
	var klines []*protocol.Kline
	err := s.withClient(func(c *Client) error {
		var e error
		klines, e = c.GetKline(code, ktype, start, count)
		return e
	})
	if err != nil && ktype == protocol.TypeKlineMinute {
		err = s.withClient(func(c *Client) error {
			var e error
			klines, e = c.GetKline(code, protocol.TypeKlineMinute2, start, count)
			return e
		})
	}
	return klines, err
}

// GetQuote fetches real-time quotes for one or more stock codes.
func (s *Service) GetQuote(codes ...string) ([]*protocol.QuoteItem, error) {
	var items []*protocol.QuoteItem
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetQuote(codes...)
		return e
	})
	return items, err
}

// FetchKlineUpstream bypasses local read models and stops paging once the
// earliest requested date is reached. TDX pages are offset from the newest
// bar, so newer bars may be transferred, but older history is not fetched.
// It is intended only for the StockDataService Provider.
func (s *Service) FetchKlineUpstream(ctx context.Context, code string, ktype uint8, earliest time.Time) ([]*protocol.Kline, error) {
	var items []*protocol.Kline
	err := s.withClientContext(ctx, func(c *Client) error {
		var e error
		items, e = c.GetKlineUntil(code, ktype, func(item *protocol.Kline) bool {
			return !earliest.IsZero() && !item.Time.After(earliest)
		})
		return e
	})
	return items, err
}

func (s *Service) FetchQuoteUpstream(ctx context.Context, codes ...string) ([]*protocol.QuoteItem, error) {
	var items []*protocol.QuoteItem
	err := s.withClientContext(ctx, func(c *Client) error {
		var e error
		items, e = c.GetQuote(codes...)
		return e
	})
	return items, err
}

// FetchFinanceUpstream bypasses legacy cache state for StockDataService sync.
func (s *Service) FetchFinanceUpstream(ctx context.Context, code string) (*protocol.FinanceInfo, error) {
	var item *protocol.FinanceInfo
	err := s.withClientContext(ctx, func(c *Client) error {
		var e error
		item, e = c.GetFinanceInfo(code)
		return e
	})
	return item, err
}

// GetIndexBars fetches index K-line bars.
func (s *Service) GetIndexBars(code string, ktype uint8, start, count uint16) ([]*protocol.IndexBar, error) {
	var items []*protocol.IndexBar
	err := s.withClient(func(c *Client) error {
		var e error
		items, e = c.GetIndexBars(code, ktype, start, count)
		return e
	})
	return items, err
}

// GetMinute fetches real-time minute data for a stock.
func (s *Service) GetMinute(code string) (*protocol.MinuteResp, error) {
	var result *protocol.MinuteResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetMinute(code)
		return e
	})
	return result, err
}

// GetHistoryMinute fetches historical minute data for a stock.
func (s *Service) GetHistoryMinute(date, code string) (*protocol.MinuteResp, error) {
	var result *protocol.MinuteResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetHistoryMinute(date, code)
		return e
	})
	return result, err
}

// GetMinuteTradeAll fetches real-time trade data for a stock.
func (s *Service) GetMinuteTradeAll(code string) (*protocol.TradeResp, error) {
	var result *protocol.TradeResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetMinuteTradeAll(code)
		return e
	})
	return result, err
}

// GetMinuteTrade fetches a window of real-time trade data.
func (s *Service) GetMinuteTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
	var result *protocol.TradeResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetMinuteTrade(code, start, count)
		return e
	})
	return result, err
}

// GetHistoryMinuteTrade fetches historical trade data for a stock.
func (s *Service) GetHistoryMinuteTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
	var result *protocol.TradeResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetHistoryMinuteTrade(date, code, start, count)
		return e
	})
	return result, err
}

// GetCallAuction fetches call auction data for a stock.
func (s *Service) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	var result *protocol.CallAuctionResp
	err := s.withClient(func(c *Client) error {
		var e error
		result, e = c.GetCallAuction(code)
		return e
	})
	return result, err
}

// GetSecurityCount returns the number of securities in an exchange.
func (s *Service) GetSecurityCount(exchange protocol.Exchange) (int, error) {
	var count int
	err := s.withClient(func(c *Client) error {
		var e error
		count, e = c.GetSecurityCount(exchange)
		return e
	})
	return count, err
}

func (s *Service) EnsureWorkday() error {
	if s.workdays == nil {
		return errors.New("workday store not initialized")
	}
	if _, err := s.workdays.GetLastWorkday(); err == nil {
		return nil
	}
	return s.withClient(func(c *Client) error {
		return s.workdays.UpdateFromKline(c, "999999")
	})
}

func ParseKlineType(s string) uint8 {
	switch s {
	case "1m", "minute":
		return 7
	case "5m":
		return 0
	case "15m":
		return 1
	case "30m":
		return 2
	case "60m":
		return 3
	case "day":
		return 9
	case "week":
		return 5
	case "month":
		return 6
	case "quarter":
		return 10
	case "year":
		return 11
	default:
		return 9
	}
}
