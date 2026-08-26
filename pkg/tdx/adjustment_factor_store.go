package tdx

import (
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// AdjustmentFactorStore 存储每个除权除息日对应的复权因子。
//
// 复权因子是"历史价格口径"的唯一来源:
//   - ForwardFactor (前复权因子): 把除权日之前的价格调整到与除权日之后同一口径
//     adjusted_price = raw_price * forward_factor
//   - BackwardFactor (后复权因子): 把除权日之后的价格调整到与除权日之前同一口径
//     adjusted_price = raw_price / backward_factor
//   - CumForward/CumBackward 为累积因子 (截至该除权日)，用于区间价格口径对齐。
//
// 因子的计算在 AdjustmentService 中完成，store 仅负责持久化。
type AdjustmentFactorStore struct {
	s   *storage.Storage
	mu  sync.RWMutex
	loc *time.Location
}

// AdjustmentFactor 单只股票单次除权除息事件对应的复权因子。
type AdjustmentFactor struct {
	Code        string    `json:"code"`
	Date        time.Time `json:"date"`
	PrevClose   float64   `json:"prev_close"`
	Forward     float64   `json:"forward_factor"`
	Backward    float64   `json:"backward_factor"`
	CumForward  float64   `json:"cum_forward"`
	CumBackward float64   `json:"cum_backward"`
	Reason      string    `json:"reason"` // ex_dividend / split / rights / ...
	CreatedAt   time.Time `json:"created_at"`
}

// NewAdjustmentFactorStore 创建存储。
func NewAdjustmentFactorStore(s *storage.Storage) (*AdjustmentFactorStore, error) {
	return &AdjustmentFactorStore{s: s, loc: time.Local}, nil
}

// Upsert 保存一批因子。
func (s *AdjustmentFactorStore) Upsert(items []*AdjustmentFactor) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(s.upsertSQL())
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, it := range items {
		dateStr := it.Date.In(s.loc).Format("2006-01-02")
		if _, err := stmt.Exec(
			it.Code,
			dateStr,
			it.PrevClose,
			it.Forward,
			it.Backward,
			it.CumForward,
			it.CumBackward,
			it.Reason,
			now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AdjustmentFactorStore) upsertSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `INSERT INTO adjustment_factor
			(code, date, prev_close, forward_factor, backward_factor,
			 cum_forward, cum_backward, reason, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (code, date) DO UPDATE SET
				prev_close=EXCLUDED.prev_close,
				forward_factor=EXCLUDED.forward_factor,
				backward_factor=EXCLUDED.backward_factor,
				cum_forward=EXCLUDED.cum_forward,
				cum_backward=EXCLUDED.cum_backward,
				reason=EXCLUDED.reason`
	case storage.MySQL:
		return `INSERT INTO adjustment_factor
			(code, date, prev_close, forward_factor, backward_factor,
			 cum_forward, cum_backward, reason, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
				prev_close=VALUES(prev_close),
				forward_factor=VALUES(forward_factor),
				backward_factor=VALUES(backward_factor),
				cum_forward=VALUES(cum_forward),
				cum_backward=VALUES(cum_backward),
				reason=VALUES(reason)`
	default: // SQLite
		return `INSERT INTO adjustment_factor
			(code, date, prev_close, forward_factor, backward_factor,
			 cum_forward, cum_backward, reason, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)
			ON CONFLICT(code, date) DO UPDATE SET
				prev_close=excluded.prev_close,
				forward_factor=excluded.forward_factor,
				backward_factor=excluded.backward_factor,
				cum_forward=excluded.cum_forward,
				cum_backward=excluded.cum_backward,
				reason=excluded.reason`
	}
}

// ListByCode 按日期升序列出所有因子。
func (s *AdjustmentFactorStore) ListByCode(code string) ([]*AdjustmentFactor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.s.DB().Query(fmt.Sprintf(`
		SELECT code, date, prev_close, forward_factor, backward_factor,
		       cum_forward, cum_backward, reason, created_at
		FROM adjustment_factor WHERE code = %s ORDER BY date ASC
	`, s.ph(1)), code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*AdjustmentFactor
	for rows.Next() {
		var it AdjustmentFactor
		var dateStr string
		var createdAt int64
		if err := rows.Scan(
			&it.Code, &dateStr, &it.PrevClose, &it.Forward, &it.Backward,
			&it.CumForward, &it.CumBackward, &it.Reason, &createdAt,
		); err != nil {
			return nil, err
		}
		if t, perr := time.ParseInLocation("2006-01-02", dateStr, s.loc); perr == nil {
			it.Date = t
		}
		if createdAt > 0 {
			it.CreatedAt = time.Unix(createdAt, 0)
		}
		list = append(list, &it)
	}
	return list, rows.Err()
}

// ph 占位符。
func (s *AdjustmentFactorStore) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Close 关闭。
func (s *AdjustmentFactorStore) Close() error { return nil }
