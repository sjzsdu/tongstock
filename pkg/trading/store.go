package trading

import (
	"fmt"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

type TradeAction string

const (
	TradeBuy  TradeAction = "buy"
	TradeSell TradeAction = "sell"
)

type Trade struct {
	ID        int64       `json:"id"`
	Code      string      `json:"code"`
	Name      string      `json:"name"`
	Action    TradeAction `json:"action"`
	Price     float64     `json:"price"`
	Signal    string      `json:"signal"`
	Ktype     string      `json:"ktype"`
	Reason    string      `json:"reason"`
	CreatedAt time.Time   `json:"created_at"`
}

type Store struct {
	s *storage.Storage
}

func New(s *storage.Storage) (*Store, error) {
	store := &Store{s: s}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) init() error {
	_, err := s.s.DB().Exec(s.createTableSQL())
	return err
}

func (s *Store) createTableSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `CREATE TABLE IF NOT EXISTS trades (
			id SERIAL PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			price FLOAT NOT NULL,
			signal TEXT NOT NULL DEFAULT '',
			ktype TEXT NOT NULL DEFAULT 'day',
			reason TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)`
	case storage.MySQL:
		return "CREATE TABLE IF NOT EXISTS trades (" +
			"id INTEGER AUTO_INCREMENT PRIMARY KEY," +
			"code VARCHAR(20) NOT NULL," +
			"name VARCHAR(100) NOT NULL DEFAULT ''," +
			"action VARCHAR(10) NOT NULL," +
			"price FLOAT NOT NULL," +
			"signal VARCHAR(100) NOT NULL DEFAULT ''," +
			"ktype VARCHAR(20) NOT NULL DEFAULT 'day'," +
			"reason TEXT NOT NULL DEFAULT ''," +
			"created_at BIGINT NOT NULL" +
			")"
	default:
		return `CREATE TABLE IF NOT EXISTS trades (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			price REAL NOT NULL,
			signal TEXT NOT NULL DEFAULT '',
			ktype TEXT NOT NULL DEFAULT 'day',
			reason TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`
	}
}

func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func (s *Store) Create(trade Trade) (int64, error) {
	now := time.Now().Unix()
	switch s.s.Dialect() {
	case storage.Postgres:
		var id int64
		err := s.s.DB().QueryRow(`
			INSERT INTO trades (code, name, action, price, signal, ktype, reason, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id
		`, trade.Code, trade.Name, trade.Action, trade.Price, trade.Signal, trade.Ktype, trade.Reason, now).Scan(&id)
		return id, err
	case storage.MySQL:
		result, err := s.s.DB().Exec(`
			INSERT INTO trades (code, name, action, price, signal, ktype, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, trade.Code, trade.Name, trade.Action, trade.Price, trade.Signal, trade.Ktype, trade.Reason, now)
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
	default:
		result, err := s.s.DB().Exec(`
			INSERT INTO trades (code, name, action, price, signal, ktype, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, trade.Code, trade.Name, trade.Action, trade.Price, trade.Signal, trade.Ktype, trade.Reason, now)
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
	}
}

func (s *Store) GetAll() ([]Trade, error) {
	rows, err := s.s.DB().Query(`SELECT id, code, name, action, price, signal, ktype, reason, created_at FROM trades ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []Trade
	for rows.Next() {
		var t Trade
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Action, &t.Price, &t.Signal, &t.Ktype, &t.Reason, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		trades = append(trades, t)
	}
	return trades, rows.Err()
}

func (s *Store) GetByCode(code string) ([]Trade, error) {
	rows, err := s.s.DB().Query(`SELECT id, code, name, action, price, signal, ktype, reason, created_at FROM trades WHERE code = ? ORDER BY created_at DESC`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []Trade
	for rows.Next() {
		var t Trade
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Action, &t.Price, &t.Signal, &t.Ktype, &t.Reason, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		trades = append(trades, t)
	}
	return trades, rows.Err()
}

func (s *Store) Delete(id int64) error {
	_, err := s.s.DB().Exec(`DELETE FROM trades WHERE id = ?`, id)
	return err
}

func (s *Store) GetLatestByCodes(codes []string) (map[string]Trade, error) {
	if len(codes) == 0 {
		return map[string]Trade{}, nil
	}

	placeholders := make([]string, len(codes))
	args := make([]interface{}, len(codes))
	for i, code := range codes {
		placeholders[i] = s.ph(i + 1)
		args[i] = code
	}

	query := fmt.Sprintf(`SELECT id, code, name, action, price, signal, ktype, reason, created_at FROM trades WHERE code IN (%s) ORDER BY created_at DESC`,
		strings.Join(placeholders, ","))

	rows, err := s.s.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	latest := make(map[string]Trade)
	for rows.Next() {
		var t Trade
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Action, &t.Price, &t.Signal, &t.Ktype, &t.Reason, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		if _, exists := latest[t.Code]; !exists {
			latest[t.Code] = t
		}
	}
	return latest, rows.Err()
}

func (s *Store) GetCurrentPosition(code string) (*Trade, error) {
	trades, err := s.GetByCode(code)
	if err != nil {
		return nil, err
	}

	position := &Trade{}
	for _, t := range trades {
		if t.Action == TradeBuy {
			*position = t
			break
		} else if t.Action == TradeSell {
			position = &Trade{}
		}
	}

	if position.ID == 0 {
		return nil, nil
	}
	return position, nil
}

func (s *Store) GetAllPositions() ([]Trade, error) {
	trades, err := s.GetAll()
	if err != nil {
		return nil, err
	}

	// GetAll returns trades ordered by created_at DESC (newest first).
	// For position calculation we need chronological order (oldest first)
	// so that sells correctly cancel prior buys.
	positions := make(map[string]Trade)
	for i := len(trades) - 1; i >= 0; i-- {
		t := trades[i]
		if t.Action == TradeBuy {
			positions[t.Code] = t
		} else if t.Action == TradeSell {
			delete(positions, t.Code)
		}
	}

	result := make([]Trade, 0, len(positions))
	for _, p := range positions {
		result = append(result, p)
	}
	return result, nil
}
