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
