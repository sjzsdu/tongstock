package tdx

import (
	"database/sql"
	"errors"

	protocol "github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

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
	if s.company != nil {
		if content, err := s.company.GetContent(code, filename); err == nil && content != "" {
			return content, nil
		}
	}
	content, err := s.Client.GetCompanyInfoContent(code, filename, start, length)
	if err != nil {
		return "", err
	}
	if s.company != nil {
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
		return s.Client.GetKlineAll(code, ktype)
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

func (s *Service) fetchAndSaveKlineAll(code string, ktype uint8) ([]*protocol.Kline, error) {
	klines, err := s.Client.GetKlineAll(code, ktype)
	if err != nil {
		return nil, err
	}
	_ = s.klines.SaveKline(code, ktype, klines)
	return klines, nil
}

func (s *Service) refreshTodayKline(code string, ktype uint8) ([]*protocol.Kline, error) {
	fresh, err := s.Client.GetKline(code, ktype, 0, 1)
	if err != nil {
		return s.klines.GetKline(code, ktype, "", "")
	}
	if len(fresh) > 0 {
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
	if len(klines) > 0 {
		_ = s.klines.SaveKline(code, ktype, klines)
	}
	return s.klines.GetKline(code, ktype, "", "")
}

// FetchKline passes through to the Client for non-cached real-time data.
func (s *Service) FetchKline(code string, ktype uint8, start, count uint16) ([]*protocol.Kline, error) {
	return s.Client.GetKline(code, ktype, start, count)
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
