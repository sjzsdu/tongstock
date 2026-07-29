package paradigm

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// DatasetSnapshotStore 数据快照存储, 提供创建/查询/不可变约束。
type DatasetSnapshotStore struct {
	s *storage.Storage
}

// NewDatasetSnapshotStore 创建快照存储实例。
func NewDatasetSnapshotStore(s *storage.Storage) *DatasetSnapshotStore {
	return &DatasetSnapshotStore{s: s}
}

// Create 创建并冻结一个数据快照。创建后不可修改。
func (store *DatasetSnapshotStore) Create(snapshot *DatasetSnapshot) error {
	if snapshot.ID == "" {
		return fmt.Errorf("snapshot ID is required")
	}
	if snapshot.Version == "" {
		return fmt.Errorf("snapshot version is required")
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}

	// 先冻结
	snapshot.Freeze()

	universeJSON, err := json.Marshal(snapshot.Universe)
	if err != nil {
		return fmt.Errorf("marshal universe: %w", err)
	}

	tx, err := store.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 写入快照主记录
	if _, err := tx.Exec(`INSERT OR REPLACE INTO dataset_snapshot
		(id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		snapshot.ID, snapshot.Version,
		snapshot.DateRange.Start, snapshot.DateRange.End,
		string(universeJSON), snapshot.Market,
		string(snapshot.PriceAdjustment), snapshot.Description,
		snapshot.CreatedAt.Unix()); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	// 写入数据源明细 (血缘追踪)
	for _, src := range snapshot.Sources {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO snapshot_data_source
			(snapshot_id, source_type, source_version, as_of, source_updated_at, hash, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snapshot.ID, src.Type, src.Version,
			src.AsOf.Unix(), src.SourceUpdatedAt.Unix(),
			src.Hash, time.Now().Unix()); err != nil {
			return fmt.Errorf("insert data source %s: %w", src.Type, err)
		}
	}

	return tx.Commit()
}

// GetByID 按 ID 获取快照。
func (store *DatasetSnapshotStore) GetByID(id string) (*DatasetSnapshot, error) {
	row := store.s.DB().QueryRow(`SELECT id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen FROM dataset_snapshot WHERE id = ?`, id)

	var snap DatasetSnapshot
	var universeJSON, priceAdj string
	var createdAt, frozen int64
	if err := row.Scan(&snap.ID, &snap.Version, &snap.DateRange.Start, &snap.DateRange.End, &universeJSON, &snap.Market, &priceAdj, &snap.Description, &createdAt, &frozen); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(universeJSON), &snap.Universe); err != nil {
		return nil, fmt.Errorf("unmarshal universe: %w", err)
	}

	snap.PriceAdjustment = PriceAdjustment(priceAdj)
	snap.CreatedAt = time.Unix(createdAt, 0)
	snap.Frozen = frozen == 1

	// 加载数据源明细
	rows, err := store.s.DB().Query(`SELECT source_type, source_version, as_of, source_updated_at, hash FROM snapshot_data_source WHERE snapshot_id = ? ORDER BY source_type`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var src DataSource
		var asOf, srcUpdatedAt int64
		if err := rows.Scan(&src.Type, &src.Version, &asOf, &srcUpdatedAt, &src.Hash); err != nil {
			return nil, err
		}
		src.AsOf = time.Unix(asOf, 0)
		src.SourceUpdatedAt = time.Unix(srcUpdatedAt, 0)
		snap.Sources = append(snap.Sources, src)
	}

	return &snap, rows.Err()
}

// List 列出所有快照 (按创建时间倒序)。
func (store *DatasetSnapshotStore) List(limit, offset int) ([]*DatasetSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := store.s.DB().Query(`SELECT id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen FROM dataset_snapshot ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DatasetSnapshot
	for rows.Next() {
		var snap DatasetSnapshot
		var universeJSON, priceAdj string
		var createdAt, frozen int64
		if err := rows.Scan(&snap.ID, &snap.Version, &snap.DateRange.Start, &snap.DateRange.End, &universeJSON, &snap.Market, &priceAdj, &snap.Description, &createdAt, &frozen); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(universeJSON), &snap.Universe); err != nil {
			return nil, err
		}
		snap.PriceAdjustment = PriceAdjustment(priceAdj)
		snap.CreatedAt = time.Unix(createdAt, 0)
		snap.Frozen = frozen == 1
		results = append(results, &snap)
	}
	return results, rows.Err()
}

// BindExperiment 为实验绑定不可变快照 ID 列表。
// 绑定后, 实验在重跑时必须使用这些快照的数据, 不能静默读取更新后的数据.
func (store *DatasetSnapshotStore) BindExperiment(experimentID string, snapshotIDs []string) error {
	if experimentID == "" {
		return fmt.Errorf("experiment ID is required")
	}

	tx, err := store.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, sid := range snapshotIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO experiment_snapshot_binding(experiment_id, snapshot_id, bound_at) VALUES (?, ?, ?)`,
			experimentID, sid, time.Now().Unix()); err != nil {
			return fmt.Errorf("bind snapshot %s: %w", sid, err)
		}
	}

	return tx.Commit()
}

// GetBoundSnapshots 获取实验绑定的所有快照 ID。
func (store *DatasetSnapshotStore) GetBoundSnapshots(experimentID string) ([]string, error) {
	rows, err := store.s.DB().Query(`SELECT snapshot_id FROM experiment_snapshot_binding WHERE experiment_id = ? ORDER BY snapshot_id`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// VerifyBinding 验证实验是否绑定了指定快照 ID。
// 如果实验有绑定记录, 必须完全匹配 (不允许静默替换为更新版本).
func (store *DatasetSnapshotStore) VerifyBinding(experimentID string, snapshotID string) error {
	bound, err := store.GetBoundSnapshots(experimentID)
	if err != nil {
		return err
	}
	if len(bound) == 0 {
		// 没有绑定记录, 允许首次绑定
		return nil
	}
	for _, id := range bound {
		if id == snapshotID {
			return nil
		}
	}
	return fmt.Errorf("experiment %s is bound to snapshots %v, cannot use %s without explicit rebind", experimentID, bound, snapshotID)
}

// Count 返回快照总数。
func (store *DatasetSnapshotStore) Count() (int, error) {
	var count int
	err := store.s.DB().QueryRow(`SELECT COUNT(*) FROM dataset_snapshot`).Scan(&count)
	return count, err
}
