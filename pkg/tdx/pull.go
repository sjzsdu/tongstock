package tdx

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type KlineStore struct {
	db  *sql.DB
	mu  sync.RWMutex
	loc *time.Location
}

var (
	klineStore     *KlineStore
	klineStoreOnce sync.Once
)

func GetKlineStore(dbPath string) (*KlineStore, error) {
	var err error
	klineStoreOnce.Do(func() {
		db, e := sql.Open("sqlite3", dbPath+"?cache=shared")
		if e != nil {
			err = e
			return
		}
		klineStore = &KlineStore{db: db, loc: time.Local}
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
	defer tx.Rollback()

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
		k.Time, _ = time.Parse("20060102", date)
		klines = append(klines, &k)
	}
	return klines, nil
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
	s.db.QueryRow(`SELECT COUNT(*) FROM kline`).Scan(&count)
	return fmt.Sprintf("KlineStore{count: %d}", count)
}
