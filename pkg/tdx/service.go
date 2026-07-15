package tdx

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

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
// For data that benefits from local caching (codes, klines, workdays),
// use Service methods. For real-time data (quotes, minutes, trades), access Client directly via svc.Client.
type Service struct {
	Client   *Client // Public: for direct protocol calls
	codes    *CodeStore
	klines   *KlineStore
	workdays *Workday
	xdxr     *XdXrStore
	finance  *FinanceStore
	company  *CompanyStore
	block    *BlockStore
}

// NewService creates a new Service instance wrapping an already-connected Client.
// It initializes the singleton stores for codes, klines and workdays.
func NewService(client *Client) (*Service, error) {
	if client == nil {
		return nil, errors.New("nil client")
	}
	svc := &Service{Client: client}
	// Codes store
	codes, err := GetCodeStore("")
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	svc.codes = codes
	// Kline store
	klines, err := GetKlineStore("")
	if err != nil {
		_ = codes.Close()
		_ = client.Close()
		return nil, err
	}
	svc.klines = klines
	// Workday store
	w, err := GetWorkday("")
	if err != nil {
		_ = klines.Close()
		_ = codes.Close()
		_ = client.Close()
		return nil, err
	}
	svc.workdays = w
	// Cache-backed stores that reuse CodeStore's cache backend
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
	if s.xdxr != nil {
		if err := s.xdxr.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.finance != nil {
		if err := s.finance.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.company != nil {
		if err := s.company.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.block != nil {
		if err := s.block.Close(); err != nil {
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
	// Try cache first
	if s.codes != nil {
		if codes, err := s.codes.GetCodes(exchange); err == nil && codes != nil && len(codes) > 0 {
			return codes, nil
		}
	}
	// Fallback to remote
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
	if err != nil && err != sql.ErrNoRows {
		return s.Client.GetKlineAll(code, ktype)
	}

	if err == sql.ErrNoRows || latest == "" {
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
		if latestErr == sql.ErrNoRows || latest == "" {
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
	klines = filterValidKlines(klines)
	_ = s.klines.SaveKline(code, ktype, klines)
	return klines, nil
}

func (s *Service) refreshTodayKline(code string, ktype uint8) ([]*protocol.Kline, error) {
	fresh, err := s.Client.GetKline(code, ktype, 0, 1)
	if err != nil {
		return s.klines.GetKline(code, ktype, "", "")
	}
	if len(fresh) > 0 {
		fresh = filterValidKlines(fresh)
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
	klines = filterValidKlines(klines)
	if len(klines) > 0 {
		_ = s.klines.SaveKline(code, ktype, klines)
	}
	return s.klines.GetKline(code, ktype, "", "")
}

func filterValidKlines(klines []*protocol.Kline) []*protocol.Kline {
	if len(klines) == 0 {
		return klines
	}
	filtered := make([]*protocol.Kline, 0, len(klines))
	for _, k := range klines {
		if isValidKlineStoreRecord(k) {
			filtered = append(filtered, k)
		}
	}
	return filtered
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
// This is a package-level helper used by CLI and Server.
func ParseKlineType(s string) uint8 {
	switch s {
	case "1m", "minute":
		return 7 // TypeKlineMinute
	case "5m":
		return 0 // TypeKline5Minute
	case "15m":
		return 1 // TypeKline15Minute
	case "30m":
		return 2 // TypeKline30Minute
	case "60m":
		return 3 // TypeKline60Minute
	case "day":
		return 9 // TypeKlineDay
	case "week":
		return 5 // TypeKlineWeek
	case "month":
		return 6 // TypeKlineMonth
	case "quarter":
		return 10 // TypeKlineQuarter
	case "year":
		return 11 // TypeKlineYear
	default:
		return 9 // TypeKlineDay as default
	}
}
