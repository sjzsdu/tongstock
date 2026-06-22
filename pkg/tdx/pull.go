package tdx

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type KlineStore struct {
	db  *sql.DB
	mu  sync.RWMutex
	loc *time.Location
}

type KlineSyncState struct {
	Code       string    `json:"code"`
	KType      uint8     `json:"ktype"`
	FirstDate  string    `json:"first_date,omitempty"`
	LastDate   string    `json:"last_date,omitempty"`
	RowCount   int       `json:"row_count"`
	LastSyncAt time.Time `json:"last_sync_at"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

var (
	klineStore     *KlineStore
	klineStoreOnce sync.Once
)

func GetKlineStore(dbPath string) (*KlineStore, error) {
	var err error
	klineStoreOnce.Do(func() {
		database, e := openDatabase(dbPath)
		if e != nil {
			err = e
			return
		}
		klineStore = &KlineStore{db: database, loc: time.Local}
		err = klineStore.init()
	})
	return klineStore, err
}

func (s *KlineStore) init() error {
	_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS kline (
			code TEXT,
			ktype INTEGER,
			date TEXT,
			open REAL,
			high REAL,
			low REAL,
			close REAL,
			volume REAL,
			amount REAL,
			PRIMARY KEY (code, ktype, date)
		);
			CREATE INDEX IF NOT EXISTS idx_code_ktype ON kline(code, ktype);
			CREATE INDEX IF NOT EXISTS idx_date ON kline(date);
			CREATE TABLE IF NOT EXISTS kline_sync_state (
				code TEXT,
				ktype INTEGER,
				first_date TEXT NOT NULL DEFAULT '',
				last_date TEXT NOT NULL DEFAULT '',
				row_count INTEGER NOT NULL DEFAULT 0,
				last_sync_at INTEGER NOT NULL DEFAULT 0,
				status TEXT NOT NULL DEFAULT '',
				error TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (code, ktype)
			);
		`)
	return err
}

func (s *KlineStore) SaveKline(code string, ktype uint8, klines []*protocol.Kline) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO kline (code, ktype, date, open, high, low, close, volume, amount)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range klines {
		date := k.Time.Format("20060102")
		_, err := stmt.Exec(code, ktype, date, k.Open, k.High, k.Low, k.Close, k.Volume, k.Amount)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *KlineStore) GetKline(code string, ktype uint8, startDate, endDate string) ([]*protocol.Kline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT date, open, high, low, close, volume, amount FROM kline WHERE code = ? AND ktype = ?`
	args := []interface{}{code, ktype}

	if startDate != "" {
		query += " AND date >= ?"
		args = append(args, startDate)
	}
	if endDate != "" {
		query += " AND date <= ?"
		args = append(args, endDate)
	}
	query += " ORDER BY date"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var klines []*protocol.Kline
	for rows.Next() {
		var k protocol.Kline
		var date string
		if err := rows.Scan(&date, &k.Open, &k.High, &k.Low, &k.Close, &k.Volume, &k.Amount); err != nil {
			return nil, err
		}
		parsedTime, err := parseKlineStoreDate(date, s.loc)
		if err != nil {
			return nil, err
		}
		k.Time = parsedTime
		klines = append(klines, &k)
	}
	return klines, nil
}

func parseKlineStoreDate(date string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, date, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid kline date %q", date)
}

// GetLatestDate returns the latest date string for a given code and ktype.
// Returns empty string and sql.ErrNoRows if no data exists.
func (s *KlineStore) GetLatestDate(code string, ktype uint8) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var date string
	err := s.db.QueryRow(
		`SELECT date FROM kline WHERE code = ? AND ktype = ? ORDER BY REPLACE(date, '-', '') DESC LIMIT 1`,
		code, ktype,
	).Scan(&date)
	if err != nil {
		return "", err
	}
	parsed, err := parseKlineStoreDate(date, s.loc)
	if err != nil {
		return "", err
	}
	return parsed.Format("20060102"), nil
}

func (s *KlineStore) UpdateSyncState(code string, ktype uint8, status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstDate, lastDate string
	var rowCount int
	_ = s.db.QueryRow(`SELECT COALESCE(MIN(date), ''), COALESCE(MAX(date), ''), COUNT(*) FROM kline WHERE code = ? AND ktype = ?`, code, ktype).
		Scan(&firstDate, &lastDate, &rowCount)
	_, err := s.db.Exec(`
		INSERT INTO kline_sync_state (code, ktype, first_date, last_date, row_count, last_sync_at, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(code, ktype) DO UPDATE SET
			first_date = excluded.first_date,
			last_date = excluded.last_date,
			row_count = excluded.row_count,
			last_sync_at = excluded.last_sync_at,
			status = excluded.status,
			error = excluded.error
	`, code, ktype, firstDate, lastDate, rowCount, time.Now().Unix(), status, errMsg)
	return err
}

func (s *KlineStore) GetSyncState(code string, ktype uint8) (*KlineSyncState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var state KlineSyncState
	var lastSyncAt int64
	err := s.db.QueryRow(`
		SELECT code, ktype, first_date, last_date, row_count, last_sync_at, status, error
		FROM kline_sync_state WHERE code = ? AND ktype = ?
	`, code, ktype).Scan(&state.Code, &state.KType, &state.FirstDate, &state.LastDate, &state.RowCount, &lastSyncAt, &state.Status, &state.Error)
	if err != nil {
		return nil, err
	}
	if lastSyncAt > 0 {
		state.LastSyncAt = time.Unix(lastSyncAt, 0)
	}
	return &state, nil
}

type PullKlineOption struct {
	PoolSize   int
	BatchSize  int
	OnProgress func(current, total int, code string)
	OnError    func(code string, err error)
}

func (s *KlineStore) PullKline(client *Client, codes []*protocol.CodeItem, ktype uint8, opt *PullKlineOption) error {
	if opt == nil {
		opt = &PullKlineOption{
			PoolSize:  5,
			BatchSize: 800,
		}
	}

	if opt.PoolSize <= 0 {
		opt.PoolSize = 5
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = 800
	}

	total := len(codes)
	var wg sync.WaitGroup
	sem := make(chan struct{}, opt.PoolSize)
	mu := sync.Mutex{}
	completed := 0

	for _, code := range codes {
		wg.Add(1)
		sem <- struct{}{}

		go func(code *protocol.CodeItem) {
			defer wg.Done()
			defer func() { <-sem }()

			klines, err := client.GetKlineAll(code.Code, ktype)
			if err != nil {
				if opt.OnError != nil {
					opt.OnError(code.Code, err)
				}
				return
			}

			if err := s.SaveKline(code.Code, ktype, klines); err != nil {
				if opt.OnError != nil {
					opt.OnError(code.Code, err)
				}
				return
			}

			mu.Lock()
			completed++
			progress := completed
			mu.Unlock()

			if opt.OnProgress != nil {
				opt.OnProgress(progress, total, code.Code)
			}
		}(code)
	}

	wg.Wait()
	return nil
}

func (s *KlineStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *KlineStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM kline`).Scan(&count)
	return fmt.Sprintf("KlineStore{count: %d}", count)
}
