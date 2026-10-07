package factorlab

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// MethodIDFactorCombo 是因子组合通道的虚拟方法标识：不属于方法库，
// 只用于前端把「这个产出是谁给的」展示清楚。
const MethodIDFactorCombo = "factor-combo"

// Run 是因子通道一次持久化产出（TopN 观察名单）。
type PickRun struct {
	// RunID 是日期驱动的幂等键（pick-<截面日期>）：同截面日重跑直接更新。
	RunID string `json:"run_id"`
	// SnapshotID / SnapshotDateEnd 溯源数据来源与新鲜度。
	SnapshotID      string `json:"snapshot_id"`
	SnapshotDateEnd string `json:"snapshot_date_end,omitempty"`
	// AsOf 是截面日期。
	AsOf string `json:"as_of"`
	// StaleDays 是快照数据截止距生成时刻的自然日数（新鲜度如实透出）。
	StaleDays int `json:"stale_days"`
	// FactorsSnapshot 是本轮显著因子评估的快照（供追溯打分构成）。
	FactorsSnapshot []FactorEval `json:"factors_snapshot,omitempty"`
	// Note 是结论的人话总结。
	Note string `json:"note,omitempty"`
	// Picks 是 TopN 观察名单（含贡献分解）。
	Picks []TopPick `json:"picks"`
	// CreatedAt / UpdatedAt 是 Unix 毫秒时间。
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// TopPickResult 是一次 TopN 落库的结果（幂等更新返回同一 run_id）。
type TopPickResult struct {
	RunID     string `json:"run_id"`
	AsOf      string `json:"as_of"`
	TopK      int    `json:"top_k"`
	Created   bool   `json:"created"`
	Updated   bool   `json:"updated"`
	StaleDays int    `json:"stale_days"`
	Note      string `json:"note,omitempty"`
}

// PickStore 是因子通道 TopN 产出仓库（SQLite）。
type PickStore interface {
	SavePickRun(ctx context.Context, run *PickRun) error
	GetLatestPickRun(ctx context.Context) (*PickRun, error)
}

// SQLitePickStore 是 PickStore 的 SQLite 实现。
type SQLitePickStore struct {
	db *sql.DB
}

// NewSQLitePickStore 复用主 SQLite 连接。
func NewSQLitePickStore(db *sql.DB) (*SQLitePickStore, error) {
	if db == nil {
		return nil, fmt.Errorf("factor pick store requires storage")
	}
	return &SQLitePickStore{db: db}, nil
}

// factorPickRunJSON 序列化整个 PickRun（列里同时存 trace 字段供查询）。
func factorPickRunJSON(r *PickRun) ([]byte, error) {
	return json.Marshal(r)
}

// SavePickRun 保存一次 TopN 产出：run_id 唯一（pick-<截面日期>），
// 同 id 重写（updated=1），否则新建（created=1）。
func (s *SQLitePickStore) SavePickRun(ctx context.Context, run *PickRun) error {
	if run == nil || run.RunID == "" || run.AsOf == "" {
		return fmt.Errorf("complete factor pick run is required")
	}
	raw, err := factorPickRunJSON(run)
	if err != nil {
		return fmt.Errorf("marshal factor pick run: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO factor_pick_run (run_id,snapshot_id,snapshot_date_end,as_of,stale_days,pick_count,pick_json,created_at_ns,updated_at_ns)
VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT(run_id) DO UPDATE SET
	snapshot_id=excluded.snapshot_id, snapshot_date_end=excluded.snapshot_date_end,
	stale_days=excluded.stale_days, pick_count=excluded.pick_count, pick_json=excluded.pick_json,
	updated_at_ns=excluded.updated_at_ns
`, run.RunID, run.SnapshotID, run.SnapshotDateEnd, run.AsOf, run.StaleDays, len(run.Picks), raw, run.CreatedAt, run.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save factor pick run: %w", err)
	}
	if rows, rerr := res.RowsAffected(); rerr == nil && rows > 1 {
		// SQLite upsert 受影响行数可能 >1（存储引擎差异），防御仅在真实数据改变时更新幂等提示。
		return nil
	}
	return nil
}

// GetLatestPickRun 返回最近一次 TopN 产出（无产出返回 nil,nil）。
func (s *SQLitePickStore) GetLatestPickRun(ctx context.Context) (*PickRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT pick_json, updated_at_ns FROM factor_pick_run ORDER BY as_of DESC, updated_at_ns DESC LIMIT 1`)
	var raw string
	var updatedAt int64
	if err := row.Scan(&raw, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load latest factor pick run: %w", err)
	}
	var run PickRun
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		return nil, fmt.Errorf("unmarshal factor pick run: %w", err)
	}
	return &run, nil
}
