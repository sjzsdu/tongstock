package tdx

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

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

// Service wraps Client + local stores for cached data access.
type Service struct {
	Client   *Client
	storage  *storage.Storage
	codes    *CodeStore
	klines   *KlineStore
	workdays *WorkdayStore
	xdxr     *XdXrStore
	finance  *FinanceStore
	company  *CompanyStore
	block    *BlockStore
}

// NewService creates a new Service with a shared storage instance.
func NewService(client *Client, s *storage.Storage) (*Service, error) {
	if client == nil {
		return nil, errors.New("nil client")
	}
	if s == nil {
		return nil, errors.New("nil storage")
	}

	svc := &Service{Client: client, storage: s}

	// Codes store
	codes, err := GetCodeStore("")
	if err != nil {
		return nil, err
	}
	svc.codes = codes

	// Kline store
	klines, err := NewKlineStore(s)
	if err != nil {
		_ = codes.Close()
		return nil, err
	}
	svc.klines = klines

	// Workday store
	w, err := NewWorkdayStore(s)
	if err != nil {
		_ = klines.Close()
		_ = codes.Close()
		return nil, err
	}
	svc.workdays = w

	// Cache-backed stores
	svc.xdxr = &XdXrStore{cache: codes.cache, ttl: xdxrTTL}
	svc.finance = &FinanceStore{cache: codes.cache, ttl: financeTTL}
	svc.company = &CompanyStore{cache: codes.cache, ttl: companyTTL}
	svc.block = &BlockStore{cache: codes.cache, ttl: blockTTL}

	return svc, nil
}

// Close closes the service along with all internal stores and the client.
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
	if s.workdays != nil {
		if err := s.workdays.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Client != nil {
		if err := s.Client.Close(); err != nil {
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

// FetchCodes tries to load codes from cache first, then fetches from the Client if needed.
func (s *Service) FetchCodes(exchange protocol.Exchange) ([]*protocol.CodeItem, error) {
	if s.codes != nil {
		if codes, err := s.codes.GetCodes(exchange); err == nil && codes != nil && len(codes) > 0 {
			return codes, nil
		}
	}
	items, err := s.Client.GetCode(exchange)
	if err != nil {
		return nil, err
	}
	if s.codes != nil {
		_ = s.codes.SaveCodes(items, exchange)
	}
	return items, nil
}

// FetchXdXr caches or fetches XdXr data
func (s *Service) FetchXdXr(code string) ([]*protocol.XdXrItem, error) {
	if s.xdxr != nil {
		if items, err := s.xdxr.Get(code); err == nil && items != nil {
			return items, nil
		}
	}
	items, err := s.Client.GetXdXrInfo(code)
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
	info, err := s.Client.GetFinanceInfo(code)
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
	items, err := s.Client.GetCompanyInfoCategory(code)
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
	content, err := s.Client.GetCompanyInfoContentAll(code, filename, start, length)
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
	items, err := s.Client.GetBlockInfoAll(blockFile)
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
			klines, err := s.Client.GetKline(code, ktype, 0, 800)
			if err != nil {
				return s.Client.GetKline(code, protocol.TypeKlineMinute2, 0, 800)
			}
			return klines, nil
		}
		klines, err := s.Client.GetKlineAll(code, ktype)
		return klines, err
	}

	latest, err := s.klines.GetLatestDate(code, ktype)
	if err != nil && !errors.Is(err, ErrKlineNotFound) {
		return s.Client.GetKlineAll(code, ktype)
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
	klines, err := s.Client.GetKlineAll(code, ktype)
	if err != nil {
		return nil, err
	}
	klines = FilterValidKlines(klines)
	_ = s.klines.SaveKline(code, ktype, klines)
	return klines, nil
}

func (s *Service) refreshTodayKline(code string, ktype uint8) ([]*protocol.Kline, error) {
	fresh, err := s.Client.GetKline(code, ktype, 0, 1)
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
	klines, err := s.Client.GetKlineUntil(code, ktype, func(k *protocol.Kline) bool {
		return k.Time.Format("20060102") < latest
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
	// 日期范围检查
	now := time.Now()
	maxDate := now.AddDate(0, 0, 1)
	minDate := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)

	filtered := make([]*protocol.Kline, 0, len(klines))
	var lastClose float64
	for _, k := range klines {
		// 过滤日期异常的数据
		if k.Time.Before(minDate) || k.Time.After(maxDate) {
			continue
		}
		if validateKline(k) != "" {
			continue
		}
		// 检查与前一条有效 K 线的价格变化
		if lastClose > 0 {
			ratio := k.Close / lastClose
			if ratio > 3.0 || ratio < 0.33 {
				continue // 跳过异常跳变
			}
		}
		lastClose = k.Close
		filtered = append(filtered, k)
	}
	return filtered
}

// CleanAndRefetchKlines removes corrupted klines from DB and re-fetches from TDX server.
func (s *Service) CleanAndRefetchKlines(code string, ktype uint8) ([]*protocol.Kline, error) {
	deleted, err := s.klines.DetectAndCleanCorruptedKlines(code, ktype)
	if err != nil {
		return nil, fmt.Errorf("清理异常数据失败: %w", err)
	}
	if deleted == 0 {
		return s.klines.GetKline(code, ktype, "", "")
	}

	log.Printf("[kline] 清理了 %d 条异常数据，重新获取 %s 的K线数据", deleted, code)

	klines, err := s.Client.GetKlineAll(code, ktype)
	if err != nil {
		return nil, fmt.Errorf("重新获取K线数据失败: %w", err)
	}

	klines = FilterValidKlines(klines)
	if err := s.klines.SaveKline(code, ktype, klines); err != nil {
		return nil, fmt.Errorf("保存K线数据失败: %w", err)
	}

	return klines, nil
}

// FetchKline passes through to the Client for non-cached real-time data.
func (s *Service) FetchKline(code string, ktype uint8, start, count uint16) ([]*protocol.Kline, error) {
	klines, err := s.Client.GetKline(code, ktype, start, count)
	if err != nil && ktype == protocol.TypeKlineMinute {
		return s.Client.GetKline(code, protocol.TypeKlineMinute2, start, count)
	}
	return klines, err
}

// EnsureWorkday makes sure there is workday data available.
func (s *Service) EnsureWorkday() error {
	if s.workdays == nil {
		return errors.New("workday store not initialized")
	}
	if _, err := s.workdays.GetLastWorkday(); err == nil {
		return nil
	}
	return s.workdays.UpdateFromKline(s.Client, "999999")
}

// ParseKlineType converts a human-friendly kline type string to the protocol uint8 constant.
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
