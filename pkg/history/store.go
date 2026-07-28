package history

import (
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// HistoryStock 历史记录
type HistoryStock struct {
	Code       string    `json:"code"`
	Name       string    `json:"name,omitempty"`
	AnalyzedAt time.Time `json:"analyzed_at"`
}

// Store 历史记录存储
type Store struct {
	s *storage.Storage
}

// New 创建存储实例
func New(s *storage.Storage) (*Store, error) {
	return &Store{s: s}, nil
}

// GetAll 获取所有历史记录
func (s *Store) GetAll() ([]HistoryStock, error) {
	rows, err := s.s.DB().Query(`SELECT code, name, analyzed_at FROM history_stocks ORDER BY analyzed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stocks []HistoryStock
	for rows.Next() {
		var stock HistoryStock
		var ts int64
		if err := rows.Scan(&stock.Code, &stock.Name, &ts); err != nil {
			return nil, err
		}
		stock.AnalyzedAt = time.Unix(ts, 0)
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

// Upsert 插入或更新记录
func (s *Store) Upsert(stock HistoryStock) error {
	if stock.Code == "" {
		return fmt.Errorf("code is required")
	}

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES ($1, $2, $3)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN $2 <> '' THEN $2 ELSE history_stocks.name END,
				analyzed_at = $3
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = CASE WHEN VALUES(name) <> '' THEN VALUES(name) ELSE name END,
				analyzed_at = VALUES(analyzed_at)
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(`
			INSERT INTO history_stocks (code, name, analyzed_at) VALUES (?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE history_stocks.name END,
				analyzed_at = excluded.analyzed_at
		`, stock.Code, stock.Name, stock.AnalyzedAt.Unix())
		return err
	}
}

// ph 返回占位符 ? 或 $N
func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Delete 删除指定记录
func (s *Store) Delete(code string) error {
	query := fmt.Sprintf(`DELETE FROM history_stocks WHERE code = %s`, s.ph(1))
	_, err := s.s.DB().Exec(query, code)
	return err
}
