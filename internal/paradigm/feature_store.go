package paradigm

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// FeatureStore 特征存储, 持久化 FeatureSpec/FeatureSetSpec/FeatureValue。
type FeatureStore struct {
	s *storage.Storage
}

// NewFeatureStore 创建特征存储实例。
func NewFeatureStore(s *storage.Storage) *FeatureStore {
	return &FeatureStore{s: s}
}

// SaveSpec 保存特征规格 (版本化)。
func (store *FeatureStore) SaveSpec(spec *FeatureSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("feature ID is required")
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now()
	}

	paramsJSON, _ := json.Marshal(spec.DefaultParams)
	depsJSON, _ := json.Marshal(spec.Dependencies)
	dataReqJSON, _ := json.Marshal(spec.DataRequired)

	_, err := store.s.DB().Exec(`INSERT OR REPLACE INTO feature_spec
		(id, version, name, category, description, default_params, window, min_samples,
		 dependencies, timing, data_required, formula_hash, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.ID, spec.Version, spec.Name, string(spec.Category), spec.Description,
		string(paramsJSON), spec.Window, spec.MinSamples,
		string(depsJSON), string(spec.Timing), string(dataReqJSON),
		spec.FormulaHash, spec.Status, spec.CreatedAt.Unix())
	return err
}

// GetSpec 获取特征规格 (指定版本)。
func (store *FeatureStore) GetSpec(id string, version int) (*FeatureSpec, error) {
	row := store.s.DB().QueryRow(`SELECT id, version, name, category, description, default_params,
		window, min_samples, dependencies, timing, data_required, formula_hash, status, created_at
		FROM feature_spec WHERE id = ? AND version = ?`, id, version)

	return scanSpec(row)
}

// GetLatestSpec 获取最新版本的特征规格。
func (store *FeatureStore) GetLatestSpec(id string) (*FeatureSpec, error) {
	row := store.s.DB().QueryRow(`SELECT id, version, name, category, description, default_params,
		window, min_samples, dependencies, timing, data_required, formula_hash, status, created_at
		FROM feature_spec WHERE id = ? ORDER BY version DESC LIMIT 1`, id)

	return scanSpec(row)
}

func scanSpec(row interface{ Scan(dest ...any) error }) (*FeatureSpec, error) {
	var spec FeatureSpec
	var category, timing, status, paramsJSON, depsJSON, dataReqJSON string
	var createdAt int64

	if err := row.Scan(&spec.ID, &spec.Version, &spec.Name, &category, &spec.Description,
		&paramsJSON, &spec.Window, &spec.MinSamples, &depsJSON, &timing, &dataReqJSON,
		&spec.FormulaHash, &status, &createdAt); err != nil {
		return nil, err
	}

	spec.Category = FeatureCategory(category)
	spec.Timing = ComputationTiming(timing)
	spec.Status = status
	spec.CreatedAt = time.Unix(createdAt, 0)

	json.Unmarshal([]byte(paramsJSON), &spec.DefaultParams)
	json.Unmarshal([]byte(depsJSON), &spec.Dependencies)
	json.Unmarshal([]byte(dataReqJSON), &spec.DataRequired)

	if spec.DefaultParams == nil {
		spec.DefaultParams = make(map[string]interface{})
	}

	return &spec, nil
}

// ListSpecs 列出所有特征规格 (可按分类过滤)。
func (store *FeatureStore) ListSpecs(category FeatureCategory, activeOnly bool) ([]*FeatureSpec, error) {
	query := `SELECT id, version, name, category, description, default_params,
		window, min_samples, dependencies, timing, data_required, formula_hash, status, created_at
		FROM feature_spec WHERE 1=1`
	args := []interface{}{}

	if category != "" {
		query += " AND category = ?"
		args = append(args, string(category))
	}
	if activeOnly {
		query += " AND status = 'active'"
	}

	// 每个 ID 取最新版本
	query += " ORDER BY id, version DESC"
	rows, err := store.s.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var results []*FeatureSpec
	for rows.Next() {
		spec, err := scanSpec(rows)
		if err != nil {
			return nil, err
		}
		if !seen[spec.ID] {
			seen[spec.ID] = true
			results = append(results, spec)
		}
	}
	return results, rows.Err()
}

// SaveFeatureSetSpec 保存特征集合规格。
func (store *FeatureStore) SaveFeatureSetSpec(fs *FeatureSetSpec) error {
	if fs.CreatedAt.IsZero() {
		fs.CreatedAt = time.Now()
	}

	featuresJSON, _ := json.Marshal(fs.Features)

	_, err := store.s.DB().Exec(`INSERT OR REPLACE INTO feature_set_spec
		(id, version, name, description, features, category, price_req, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		fs.ID, fs.Version, fs.Name, fs.Description,
		string(featuresJSON), string(fs.Category), string(fs.PriceReq),
		fs.CreatedAt.Unix())
	return err
}

// GetFeatureSetSpec 获取特征集合规格。
func (store *FeatureStore) GetFeatureSetSpec(id string, version int) (*FeatureSetSpec, error) {
	var fs FeatureSetSpec
	var featuresJSON, category, priceReq string
	var createdAt int64

	query := `SELECT id, version, name, description, features, category, price_req, created_at
		FROM feature_set_spec WHERE id = ?`
	args := []interface{}{id}
	if version > 0 {
		query += " AND version = ?"
		args = append(args, version)
	} else {
		query += " ORDER BY version DESC LIMIT 1"
	}

	err := store.s.DB().QueryRow(query, args...).Scan(
		&fs.ID, &fs.Version, &fs.Name, &fs.Description,
		&featuresJSON, &category, &priceReq, &createdAt)
	if err != nil {
		return nil, err
	}

	fs.Category = FeatureCategory(category)
	fs.PriceReq = PriceAdjustment(priceReq)
	fs.CreatedAt = time.Unix(createdAt, 0)
	json.Unmarshal([]byte(featuresJSON), &fs.Features)

	return &fs, nil
}

// SaveFeatureValue 保存特征计算结果。
func (store *FeatureStore) SaveFeatureValue(val *FeatureValue) error {
	_, err := store.s.DB().Exec(`INSERT INTO feature_value
		(feature_id, feature_version, stock_code, date, value, source_data, computed_at, as_of, leak_checked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		val.FeatureID, val.Version, val.StockCode,
		val.Date.Format("2006-01-02"), val.Value,
		val.SourceData, val.ComputedAt.Unix(), val.AsOf.Unix(),
		boolToInt(val.LeakChecked))
	return err
}

// GetFeatureValues 获取指定股票/日期范围的特征值。
func (store *FeatureStore) GetFeatureValues(stockCode, featureID string, startDate, endDate time.Time) ([]*FeatureValue, error) {
	rows, err := store.s.DB().Query(`SELECT feature_id, feature_version, stock_code, date, value,
		source_data, computed_at, as_of, leak_checked
		FROM feature_value
		WHERE stock_code = ? AND feature_id = ? AND date >= ? AND date <= ?
		ORDER BY date`,
		stockCode, featureID,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*FeatureValue
	for rows.Next() {
		var v FeatureValue
		var dateStr, sourceData string
		var version, leakChecked int
		var computedAt, asOf int64

		if err := rows.Scan(&v.FeatureID, &version, &v.StockCode, &dateStr, &v.Value,
			&sourceData, &computedAt, &asOf, &leakChecked); err != nil {
			return nil, err
		}

		v.Version = version
		v.Date, _ = time.Parse("2006-01-02", dateStr)
		v.SourceData = sourceData
		v.ComputedAt = time.Unix(computedAt, 0)
		v.AsOf = time.Unix(asOf, 0)
		v.LeakChecked = leakChecked == 1
		results = append(results, &v)
	}
	return results, rows.Err()
}

// CountSpecs 返回特征规格总数 (按 ID 去重)。
func (store *FeatureStore) CountSpecs() (int, error) {
	var count int
	err := store.s.DB().QueryRow(`SELECT COUNT(DISTINCT id) FROM feature_spec`).Scan(&count)
	return count, err
}

// CountAllVersions 返回所有特征版本总数。
func (store *FeatureStore) CountAllVersions() (int, error) {
	var count int
	err := store.s.DB().QueryRow(`SELECT COUNT(*) FROM feature_spec`).Scan(&count)
	return count, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
