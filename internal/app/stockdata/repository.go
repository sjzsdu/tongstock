package stockdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type SQLiteRepository struct {
	storage *storage.Storage
}

// SQLiteTradingCalendar uses the persisted TDX workday table when the
// requested day falls inside its known coverage. Outside that coverage it
// safely falls back to weekdays so an empty/partially initialized calendar
// does not make the application unusable.
type SQLiteTradingCalendar struct {
	storage *storage.Storage
	mu      sync.RWMutex
	loaded  bool
	days    map[string]struct{}
	first   string
	last    string
}

func NewSQLiteTradingCalendar(s *storage.Storage) (*SQLiteTradingCalendar, error) {
	if s == nil || s.Dialect() != storage.SQLite {
		return nil, errors.New("trading calendar requires sqlite storage")
	}
	return &SQLiteTradingCalendar{storage: s}, nil
}

func (c *SQLiteTradingCalendar) IsTradingDay(ctx context.Context, _ string, day time.Time) (bool, error) {
	if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		return false, nil
	}
	raw := day.Format("20060102")
	if err := c.load(ctx); err != nil {
		return false, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.first != "" && raw >= c.first && raw <= c.last {
		_, exists := c.days[raw]
		return exists, nil
	}
	return true, nil
}

func (c *SQLiteTradingCalendar) load(ctx context.Context) error {
	c.mu.RLock()
	loaded := c.loaded
	c.mu.RUnlock()
	if loaded {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return nil
	}
	days, err := loadTradingDays(ctx, c.storage.DB(), `SELECT date FROM workday ORDER BY date`)
	if err != nil {
		return err
	}
	// Older databases may not have materialized workday. Derive the exchange
	// calendar from the union of validated daily stock bars instead of treating
	// every weekday (including public holidays) as a trading day.
	if len(days) == 0 {
		days, err = loadTradingDays(ctx, c.storage.DB(), `SELECT DISTINCT date FROM kline
			WHERE ktype IN (4,9) AND code <> '999999'
				AND length(date)=8 AND date BETWEEN '19900101' AND strftime('%Y%m%d','now','+1 day')
			ORDER BY date`)
		if err != nil {
			return err
		}
	}
	c.days = make(map[string]struct{}, len(days))
	for _, value := range days {
		c.days[value] = struct{}{}
	}
	if len(days) > 0 {
		c.first, c.last = days[0], days[len(days)-1]
	}
	c.loaded = true
	return nil
}

func loadTradingDays(ctx context.Context, db *sql.DB, query string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		days = append(days, value)
	}
	return days, rows.Err()
}

func NewSQLiteRepository(s *storage.Storage) (*SQLiteRepository, error) {
	if s == nil || s.Dialect() != storage.SQLite {
		return nil, errors.New("stock data repository requires sqlite storage")
	}
	return &SQLiteRepository{storage: s}, nil
}

func (r *SQLiteRepository) InspectCoverage(ctx context.Context, spec DataSpec) (Coverage, error) {
	switch spec.Type {
	case DataKline:
		rows, err := r.storage.DB().QueryContext(ctx,
			`SELECT date FROM kline WHERE code = ? AND ktype = ? ORDER BY date`, spec.Code, spec.KType)
		if err != nil {
			return Coverage{}, err
		}
		defer rows.Close()
		var points []time.Time
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				return Coverage{}, err
			}
			point, err := time.ParseInLocation("20060102", raw, time.Local)
			if err == nil {
				points = append(points, point)
			}
		}
		coverage := Coverage{Exists: len(points) > 0, Points: points}
		if len(points) > 0 {
			coverage.Start, coverage.End = points[0], points[len(points)-1]
		}
		state, _ := r.syncState(ctx, syncKey(spec))
		if state.lastSync == 0 && state.status == "" {
			state, _ = r.legacyKlineSyncState(ctx, spec.Code, spec.KType)
		}
		mergeState(&coverage, state)
		return coverage, rows.Err()
	case DataQuote, DataFinance:
		table := "quote_snapshot"
		if spec.Type == DataFinance {
			table = "finance_snapshot"
		}
		var source, updated int64
		err := r.storage.DB().QueryRowContext(ctx,
			`SELECT source_updated_at, updated_at FROM `+table+` WHERE code = ?`, spec.Code).
			Scan(&source, &updated)
		if errors.Is(err, sql.ErrNoRows) {
			return Coverage{}, nil
		}
		if err != nil {
			return Coverage{}, err
		}
		coverage := Coverage{
			Exists: true, SourceUpdatedAt: time.Unix(source, 0), LastSyncAt: time.Unix(updated, 0),
		}
		state, _ := r.syncState(ctx, syncKey(spec))
		mergeState(&coverage, state)
		return coverage, nil
	default:
		return Coverage{}, fmt.Errorf("unsupported data type %q", spec.Type)
	}
}

func (r *SQLiteRepository) Query(ctx context.Context, spec DataSpec) (Dataset, error) {
	switch spec.Type {
	case DataKline:
		query := `SELECT date, open, high, low, close, volume, amount FROM kline WHERE code = ? AND ktype = ?`
		args := []any{spec.Code, spec.KType}
		if !spec.Start.IsZero() {
			query += ` AND date >= ?`
			args = append(args, spec.Start.Format("20060102"))
		}
		if !spec.End.IsZero() {
			query += ` AND date <= ?`
			args = append(args, spec.End.Format("20060102"))
		}
		rows, err := r.storage.DB().QueryContext(ctx, query+` ORDER BY date`, args...)
		if err != nil {
			return Dataset{}, err
		}
		defer rows.Close()
		var result []*protocol.Kline
		for rows.Next() {
			var item protocol.Kline
			var raw string
			if err := rows.Scan(&raw, &item.Open, &item.High, &item.Low, &item.Close, &item.Volume, &item.Amount); err != nil {
				return Dataset{}, err
			}
			item.Time, err = time.ParseInLocation("20060102", raw, time.Local)
			if err != nil {
				return Dataset{}, fmt.Errorf("invalid stored kline date %q: %w", raw, err)
			}
			if err := validateKlineRecord(&item, time.Now()); err != nil {
				return Dataset{}, fmt.Errorf("invalid stored kline %s: %w", raw, err)
			}
			result = append(result, &item)
		}
		if err := rows.Err(); err != nil {
			return Dataset{}, err
		}
		if len(result) == 0 {
			return Dataset{}, sql.ErrNoRows
		}
		return Dataset{Klines: result}, nil
	case DataQuote:
		var raw string
		if err := r.storage.DB().QueryRowContext(ctx, `SELECT payload FROM quote_snapshot WHERE code = ?`, spec.Code).Scan(&raw); err != nil {
			return Dataset{}, err
		}
		var item protocol.QuoteItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return Dataset{}, err
		}
		return Dataset{Quote: &item}, nil
	case DataFinance:
		var raw string
		if err := r.storage.DB().QueryRowContext(ctx, `SELECT payload FROM finance_snapshot WHERE code = ?`, spec.Code).Scan(&raw); err != nil {
			return Dataset{}, err
		}
		var item protocol.FinanceInfo
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return Dataset{}, err
		}
		return Dataset{Finance: &item}, nil
	default:
		return Dataset{}, fmt.Errorf("unsupported data type %q", spec.Type)
	}
}

func (r *SQLiteRepository) SaveSynced(ctx context.Context, spec DataSpec, dataset Dataset, metadata SyncMetadata) error {
	tx, err := r.storage.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	rowsWritten := 0
	coverageStart, coverageEnd := "", ""

	switch spec.Type {
	case DataKline:
		for index, item := range dataset.Klines {
			if err := validateKlineRecord(item, time.Now()); err != nil {
				return fmt.Errorf("invalid kline at index %d: %w", index, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO kline
				(code, ktype, date, open, high, low, close, volume, amount)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(code, ktype, date) DO UPDATE SET
				open=excluded.open, high=excluded.high, low=excluded.low, close=excluded.close,
				volume=excluded.volume, amount=excluded.amount`,
				spec.Code, spec.KType, item.Time.Format("20060102"), item.Open, item.High,
				item.Low, item.Close, item.Volume, item.Amount); err != nil {
				return err
			}
			rowsWritten++
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MIN(date), ''), COALESCE(MAX(date), '') FROM kline WHERE code = ? AND ktype = ?`,
			spec.Code, spec.KType).Scan(&coverageStart, &coverageEnd); err != nil {
			return err
		}
	case DataQuote:
		if dataset.Quote == nil {
			return errors.New("empty quote dataset")
		}
		raw, err := json.Marshal(dataset.Quote)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO quote_snapshot(code, payload, source_updated_at, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(code) DO UPDATE SET payload=excluded.payload,
			source_updated_at=excluded.source_updated_at, updated_at=excluded.updated_at`,
			spec.Code, string(raw), metadata.SourceUpdatedAt.Unix(), now); err != nil {
			return err
		}
		rowsWritten = 1
	case DataFinance:
		if dataset.Finance == nil {
			return errors.New("empty finance dataset")
		}
		raw, err := json.Marshal(dataset.Finance)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO finance_snapshot(code, payload, source_updated_at, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(code) DO UPDATE SET payload=excluded.payload,
			source_updated_at=excluded.source_updated_at, updated_at=excluded.updated_at`,
			spec.Code, string(raw), metadata.SourceUpdatedAt.Unix(), now); err != nil {
			return err
		}
		rowsWritten = 1
	default:
		return fmt.Errorf("unsupported data type %q", spec.Type)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO data_sync_state
		(sync_key, data_type, market, code, granularity, range_start, range_end,
		coverage_start, coverage_end, source_updated_at, last_sync_at, status, quality,
		error_code, error_message, rows_written)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ok', ?, '', '', ?)
		ON CONFLICT(sync_key) DO UPDATE SET range_start=excluded.range_start,
		range_end=excluded.range_end, coverage_start=excluded.coverage_start,
		coverage_end=excluded.coverage_end, source_updated_at=excluded.source_updated_at,
		last_sync_at=excluded.last_sync_at, status='ok', quality=excluded.quality,
		error_code='', error_message='', rows_written=excluded.rows_written`,
		syncKey(spec), spec.Type, spec.Market, spec.Code, spec.Granularity,
		formatDay(spec.Start), formatDay(spec.End), coverageStart, coverageEnd,
		metadata.SourceUpdatedAt.Unix(), now, metadata.Quality, rowsWritten)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type persistedState struct {
	sourceUpdated int64
	lastSync      int64
	status        string
	quality       string
}

func (r *SQLiteRepository) syncState(ctx context.Context, key string) (persistedState, error) {
	var state persistedState
	err := r.storage.DB().QueryRowContext(ctx, `SELECT source_updated_at, last_sync_at, status, quality
		FROM data_sync_state WHERE sync_key = ?`, key).
		Scan(&state.sourceUpdated, &state.lastSync, &state.status, &state.quality)
	return state, err
}

func (r *SQLiteRepository) legacyKlineSyncState(ctx context.Context, code string, ktype uint8) (persistedState, error) {
	var state persistedState
	err := r.storage.DB().QueryRowContext(ctx, `SELECT last_sync_at, status
		FROM kline_sync_state WHERE code = ? AND ktype = ?`, code, ktype).
		Scan(&state.lastSync, &state.status)
	return state, err
}

func mergeState(coverage *Coverage, state persistedState) {
	if coverage.SourceUpdatedAt.IsZero() && state.sourceUpdated > 0 {
		coverage.SourceUpdatedAt = time.Unix(state.sourceUpdated, 0)
	}
	if state.lastSync > 0 {
		coverage.LastSyncAt = time.Unix(state.lastSync, 0)
	}
	coverage.Status, coverage.Quality = state.status, state.quality
}

func syncKey(spec DataSpec) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d", spec.Type, spec.Market, spec.Code, spec.Granularity, spec.KType)
}

func formatDay(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("20060102")
}
