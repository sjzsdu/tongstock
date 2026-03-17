package tdx

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type CodeStore struct {
	db   *sql.DB
	mu   sync.RWMutex
	date time.Time
}

var (
	store     *CodeStore
	storeOnce sync.Once
)

func GetCodeStore(dbPath string) (*CodeStore, error) {
	var err error
	storeOnce.Do(func() {
		db, e := sql.Open("sqlite3", dbPath+"?cache=shared")
		if e != nil {
			err = e
			return
		}
		store = &CodeStore{db: db, date: time.Now()}
		err = store.init()
	})
	return store, err
}

func (s *CodeStore) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS codes (
			code TEXT PRIMARY KEY,
			name TEXT,
			exchange TEXT,
			updated_at INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_exchange ON codes(exchange);
	`)
	return err
}

func (s *CodeStore) SaveCodes(codes []*protocol.CodeItem, exchange protocol.Exchange) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO codes (code, name, exchange, updated_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, c := range codes {
		_, err := stmt.Exec(c.Code, c.Name, exchange.String(), now)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.date = time.Now()
	return nil
}

func (s *CodeStore) GetCodes(exchange protocol.Exchange) ([]*protocol.CodeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT code, name FROM codes
		WHERE exchange = ?
		ORDER BY code
	`, exchange.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*protocol.CodeItem
	for rows.Next() {
		var c protocol.CodeItem
		if err := rows.Scan(&c.Code, &c.Name); err != nil {
			return nil, err
		}
		items = append(items, &c)
	}
	return items, nil
}

func (s *CodeStore) GetCode(code string) (*protocol.CodeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var c protocol.CodeItem
	err := s.db.QueryRow(`
		SELECT code, name FROM codes
		WHERE code = ?
	`, code).Scan(&c.Code, &c.Name)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *CodeStore) NeedUpdate(exchange protocol.Exchange, maxAge time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.date) > maxAge
}

func (s *CodeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *CodeStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("CodeStore{date: %s}", s.date.Format("2006-01-02"))
}
