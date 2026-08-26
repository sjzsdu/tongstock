package paradigm

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	if store == nil || store.s == nil {
		return fmt.Errorf("snapshot store is not initialized")
	}
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
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

	// 快照 ID 只允许写入一次。不可使用 REPLACE，否则已绑定实验的输入会静默改变。
	if _, err := tx.Exec(`INSERT INTO dataset_snapshot
		(id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		snapshot.ID, snapshot.Version,
		snapshot.DateRange.Start, snapshot.DateRange.End,
		string(universeJSON), snapshot.Market,
		string(snapshot.PriceAdjustment), snapshot.Description,
		snapshot.CreatedAt.Unix(), snapshot.ContentHash); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	// 写入数据源明细 (血缘追踪)
	for _, src := range snapshot.Sources {
		if _, err := tx.Exec(`INSERT INTO snapshot_data_source
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

// CreateKlineSnapshot 从当前数据库事务内读取真实 K 线并复制到不可变快照表。
// 后续实验只能读取 snapshot_kline_bar，不能重新读取可变的 kline 表。
func (store *DatasetSnapshotStore) CreateKlineSnapshot(snapshot *DatasetSnapshot, ktype uint8) error {
	if store == nil || store.s == nil {
		return fmt.Errorf("snapshot store is not initialized")
	}
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	if snapshot.ID == "" || snapshot.Version == "" {
		return fmt.Errorf("snapshot ID and version are required")
	}
	if len(snapshot.Universe) == 0 {
		return fmt.Errorf("snapshot universe is empty")
	}
	startDate, err := normalizeSnapshotDate(snapshot.DateRange.Start)
	if err != nil {
		return fmt.Errorf("invalid snapshot start date: %w", err)
	}
	endDate, err := normalizeSnapshotDate(snapshot.DateRange.End)
	if err != nil {
		return fmt.Errorf("invalid snapshot end date: %w", err)
	}
	if startDate > endDate {
		return fmt.Errorf("snapshot start date is after end date")
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}

	universe := append([]string(nil), snapshot.Universe...)
	for i := range universe {
		universe[i] = strings.TrimSpace(universe[i])
		if universe[i] == "" {
			return fmt.Errorf("snapshot universe contains empty stock code")
		}
	}
	sort.Strings(universe)
	for i, code := range universe {
		if i > 0 && code == universe[i-1] {
			return fmt.Errorf("snapshot universe contains duplicate stock code %q", code)
		}
	}
	snapshot.Universe = universe

	tx, err := store.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM dataset_snapshot WHERE id = ?`, snapshot.ID).Scan(&exists); err != nil {
		return fmt.Errorf("check snapshot ID: %w", err)
	}
	if exists > 0 {
		return fmt.Errorf("snapshot %q already exists and is immutable", snapshot.ID)
	}

	barsByCode := make(map[string][]SnapshotKlineBar, len(universe))
	manifests := make([]KlineSnapshotManifest, 0, len(universe))
	for _, code := range universe {
		bars, err := readLiveKlines(tx, code, ktype, startDate, endDate)
		if err != nil {
			return err
		}
		if len(bars) == 0 {
			return fmt.Errorf("no real K line data for %s in range %s-%s", code, startDate, endDate)
		}
		manifest := manifestForBars(code, ktype, bars)
		barsByCode[code] = bars
		manifests = append(manifests, manifest)
	}

	snapshot.KlineManifests = manifests
	snapshot.ContentHash = hashManifests(manifests)
	snapshot.Freeze()
	snapshot.Sources = replaceKlineSource(snapshot.Sources, DataSource{
		Type:            "kline",
		Version:         snapshot.ContentHash,
		AsOf:            snapshot.CreatedAt,
		SourceUpdatedAt: snapshot.CreatedAt,
		Hash:            snapshot.ContentHash,
	})

	universeJSON, err := json.Marshal(snapshot.Universe)
	if err != nil {
		return fmt.Errorf("marshal universe: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO dataset_snapshot
		(id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		snapshot.ID, snapshot.Version, snapshot.DateRange.Start, snapshot.DateRange.End,
		string(universeJSON), snapshot.Market, string(snapshot.PriceAdjustment),
		snapshot.Description, snapshot.CreatedAt.Unix(), snapshot.ContentHash); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	for _, src := range snapshot.Sources {
		if _, err := tx.Exec(`INSERT INTO snapshot_data_source
			(snapshot_id, source_type, source_version, as_of, source_updated_at, hash, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snapshot.ID, src.Type, src.Version, src.AsOf.Unix(),
			src.SourceUpdatedAt.Unix(), src.Hash, snapshot.CreatedAt.Unix()); err != nil {
			return fmt.Errorf("insert data source %s: %w", src.Type, err)
		}
	}

	for _, manifest := range manifests {
		if _, err := tx.Exec(`INSERT INTO snapshot_kline_manifest
			(snapshot_id, code, ktype, start_date, end_date, row_count, content_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snapshot.ID, manifest.Code, manifest.KType, manifest.StartDate,
			manifest.EndDate, manifest.RowCount, manifest.ContentHash); err != nil {
			return fmt.Errorf("insert K line manifest %s: %w", manifest.Code, err)
		}
		for _, bar := range barsByCode[manifest.Code] {
			if _, err := tx.Exec(`INSERT INTO snapshot_kline_bar
				(snapshot_id, code, ktype, date, open, high, low, close, volume, amount)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				snapshot.ID, bar.Code, bar.KType, bar.Date.Format("20060102"),
				bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount); err != nil {
				return fmt.Errorf("insert frozen K line %s %s: %w", bar.Code, bar.Date.Format("20060102"), err)
			}
		}
	}

	return tx.Commit()
}

// GetByID 按 ID 获取快照。
func (store *DatasetSnapshotStore) GetByID(id string) (*DatasetSnapshot, error) {
	row := store.s.DB().QueryRow(`SELECT id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen, content_hash FROM dataset_snapshot WHERE id = ?`, id)

	var snap DatasetSnapshot
	var universeJSON, priceAdj string
	var createdAt, frozen int64
	if err := row.Scan(&snap.ID, &snap.Version, &snap.DateRange.Start, &snap.DateRange.End, &universeJSON, &snap.Market, &priceAdj, &snap.Description, &createdAt, &frozen, &snap.ContentHash); err != nil {
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	snap.KlineManifests, err = store.getKlineManifests(id)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// GetFrozenKlines 读取并校验快照内冻结的 K 线内容。
func (store *DatasetSnapshotStore) GetFrozenKlines(snapshotID, code string, ktype uint8) ([]SnapshotKlineBar, error) {
	manifests, err := store.getKlineManifests(snapshotID)
	if err != nil {
		return nil, err
	}
	var expected *KlineSnapshotManifest
	for i := range manifests {
		if manifests[i].Code == code && manifests[i].KType == ktype {
			expected = &manifests[i]
			break
		}
	}
	if expected == nil {
		return nil, fmt.Errorf("snapshot %q has no K line manifest for %s type %d", snapshotID, code, ktype)
	}
	bars, err := readFrozenKlines(store.s.DB(), snapshotID, code, ktype)
	if err != nil {
		return nil, err
	}
	actual := manifestForBars(code, ktype, bars)
	if actual.RowCount != expected.RowCount || actual.StartDate != expected.StartDate ||
		actual.EndDate != expected.EndDate || actual.ContentHash != expected.ContentHash {
		return nil, fmt.Errorf("snapshot %q K line content hash mismatch for %s type %d", snapshotID, code, ktype)
	}
	return bars, nil
}

// VerifyContent 验证所有冻结 K 线分片及快照总哈希。
func (store *DatasetSnapshotStore) VerifyContent(snapshotID string) error {
	snapshot, err := store.GetByID(snapshotID)
	if err != nil {
		return err
	}
	if snapshot.ContentHash == "" || len(snapshot.KlineManifests) == 0 {
		return fmt.Errorf("snapshot %q has no frozen K line content", snapshotID)
	}
	for _, manifest := range snapshot.KlineManifests {
		if _, err := store.GetFrozenKlines(snapshotID, manifest.Code, manifest.KType); err != nil {
			return err
		}
	}
	if actual := hashManifests(snapshot.KlineManifests); actual != snapshot.ContentHash {
		return fmt.Errorf("snapshot %q manifest hash mismatch", snapshotID)
	}
	return nil
}

type rowQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func readLiveKlines(q rowQueryer, code string, ktype uint8, startDate, endDate string) ([]SnapshotKlineBar, error) {
	rows, err := q.Query(`SELECT date, open, high, low, close, volume, amount
		FROM kline
		WHERE code = ? AND ktype = ? AND REPLACE(date, '-', '') >= ? AND REPLACE(date, '-', '') <= ?
		ORDER BY REPLACE(date, '-', '')`, code, ktype, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("read real K line data for %s: %w", code, err)
	}
	defer rows.Close()
	return scanSnapshotBars(rows, code, ktype)
}

func readFrozenKlines(q rowQueryer, snapshotID, code string, ktype uint8) ([]SnapshotKlineBar, error) {
	rows, err := q.Query(`SELECT date, open, high, low, close, volume, amount
		FROM snapshot_kline_bar
		WHERE snapshot_id = ? AND code = ? AND ktype = ?
		ORDER BY date`, snapshotID, code, ktype)
	if err != nil {
		return nil, fmt.Errorf("read frozen K line data for %s: %w", code, err)
	}
	defer rows.Close()
	return scanSnapshotBars(rows, code, ktype)
}

func scanSnapshotBars(rows *sql.Rows, code string, ktype uint8) ([]SnapshotKlineBar, error) {
	var bars []SnapshotKlineBar
	for rows.Next() {
		var bar SnapshotKlineBar
		var date string
		bar.Code = code
		bar.KType = ktype
		if err := rows.Scan(&date, &bar.Open, &bar.High, &bar.Low, &bar.Close, &bar.Volume, &bar.Amount); err != nil {
			return nil, err
		}
		compact, err := normalizeSnapshotDate(date)
		if err != nil {
			return nil, fmt.Errorf("invalid K line date %q for %s: %w", date, code, err)
		}
		bar.Date, _ = time.Parse("20060102", compact)
		bars = append(bars, bar)
	}
	return bars, rows.Err()
}

func (store *DatasetSnapshotStore) getKlineManifests(snapshotID string) ([]KlineSnapshotManifest, error) {
	rows, err := store.s.DB().Query(`SELECT code, ktype, start_date, end_date, row_count, content_hash
		FROM snapshot_kline_manifest WHERE snapshot_id = ? ORDER BY code, ktype`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var manifests []KlineSnapshotManifest
	for rows.Next() {
		var manifest KlineSnapshotManifest
		if err := rows.Scan(&manifest.Code, &manifest.KType, &manifest.StartDate,
			&manifest.EndDate, &manifest.RowCount, &manifest.ContentHash); err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, rows.Err()
}

func manifestForBars(code string, ktype uint8, bars []SnapshotKlineBar) KlineSnapshotManifest {
	manifest := KlineSnapshotManifest{Code: code, KType: ktype, RowCount: len(bars)}
	if len(bars) == 0 {
		return manifest
	}
	manifest.StartDate = bars[0].Date.Format("20060102")
	manifest.EndDate = bars[len(bars)-1].Date.Format("20060102")
	hash := sha256.New()
	for _, bar := range bars {
		_, _ = fmt.Fprintf(hash, "%s|%d|%s|%.10g|%.10g|%.10g|%.10g|%.10g|%.10g\n",
			bar.Code, bar.KType, bar.Date.Format("20060102"), bar.Open, bar.High,
			bar.Low, bar.Close, bar.Volume, bar.Amount)
	}
	manifest.ContentHash = fmt.Sprintf("%x", hash.Sum(nil))
	return manifest
}

func hashManifests(manifests []KlineSnapshotManifest) string {
	sorted := append([]KlineSnapshotManifest(nil), manifests...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Code == sorted[j].Code {
			return sorted[i].KType < sorted[j].KType
		}
		return sorted[i].Code < sorted[j].Code
	})
	hash := sha256.New()
	for _, manifest := range sorted {
		_, _ = fmt.Fprintf(hash, "%s|%d|%s|%s|%d|%s\n",
			manifest.Code, manifest.KType, manifest.StartDate, manifest.EndDate,
			manifest.RowCount, manifest.ContentHash)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func normalizeSnapshotDate(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) != 8 {
		return "", fmt.Errorf("date must be YYYY-MM-DD or YYYYMMDD")
	}
	if _, err := time.Parse("20060102", value); err != nil {
		return "", err
	}
	return value, nil
}

func replaceKlineSource(sources []DataSource, kline DataSource) []DataSource {
	result := make([]DataSource, 0, len(sources)+1)
	for _, source := range sources {
		if source.Type != "kline" {
			result = append(result, source)
		}
	}
	return append(result, kline)
}

// List 列出所有快照 (按创建时间倒序)。
func (store *DatasetSnapshotStore) List(limit, offset int) ([]*DatasetSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := store.s.DB().Query(`SELECT id, version, date_range_start, date_range_end, universe, market, price_adjustment, description, created_at, frozen, content_hash FROM dataset_snapshot ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DatasetSnapshot
	for rows.Next() {
		var snap DatasetSnapshot
		var universeJSON, priceAdj string
		var createdAt, frozen int64
		if err := rows.Scan(&snap.ID, &snap.Version, &snap.DateRange.Start, &snap.DateRange.End, &universeJSON, &snap.Market, &priceAdj, &snap.Description, &createdAt, &frozen, &snap.ContentHash); err != nil {
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
