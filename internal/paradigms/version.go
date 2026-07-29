package paradigms

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// ============================================================================
// 版本管理
// ============================================================================

// SchemaVersion Schema 版本记录
type SchemaVersion struct {
	ID            string         `json:"id"`
	SchemaID      string         `json:"schema_id"`
	Version       int            `json:"version"`
	ParentVersion int            `json:"parent_version"`
	ChangeReason  string         `json:"change_reason"`
	Snapshot      *ParadigmSchema `json:"snapshot"` // 不可变快照
	CreatedAt     time.Time      `json:"created_at"`
	CreatedBy     string         `json:"created_by,omitempty"`
	Hash          string         `json:"hash"` // 内容哈希 (用于检测未变更)
}

// VersionStore 版本存储
type VersionStore struct {
	mu       sync.RWMutex
	versions map[string][]SchemaVersion // schema_id -> versions (按版本号排序)
	db       *storage.Storage
}

// NewVersionStore 创建版本存储
func NewVersionStore(db *storage.Storage) *VersionStore {
	return &VersionStore{
		versions: make(map[string][]SchemaVersion),
		db:       db,
	}
}

// SaveVersion 保存新版本
func (vs *VersionStore) SaveVersion(schema *ParadigmSchema, changeReason string, createdBy string) (*SchemaVersion, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 计算内容哈希
	hash := computeHash(schema)

	// 检查是否有未变更的版本
	existing := vs.findVersion(schema.ID, schema.Version-1)
	if existing != nil && existing.Hash == hash {
		return existing, nil // 内容未变更
	}

	// 创建版本记录
	version := SchemaVersion{
		ID:            fmt.Sprintf("%s:v%d", schema.ID, schema.Version),
		SchemaID:      schema.ID,
		Version:       schema.Version,
		ParentVersion: schema.ParentVersion,
		ChangeReason:  changeReason,
		Snapshot:      schema.DeepCopy(),
		CreatedAt:     time.Now(),
		CreatedBy:     createdBy,
		Hash:          hash,
	}

	// 保存到内存
	versions := vs.versions[schema.ID]
	vs.versions[schema.ID] = append(versions, version)

	// 持久化
	if vs.db != nil {
		if err := vs.saveVersionDB(&version); err != nil {
			return nil, fmt.Errorf("save version to db: %w", err)
		}
	}

	return &version, nil
}

// GetVersion 获取指定版本
func (vs *VersionStore) GetVersion(schemaID string, version int) (*SchemaVersion, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	v := vs.findVersion(schemaID, version)
	if v == nil {
		return nil, fmt.Errorf("version not found: %s v%d", schemaID, version)
	}
	return v, nil
}

// GetHistory 获取版本历史
func (vs *VersionStore) GetHistory(schemaID string) ([]SchemaVersion, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	versions := vs.versions[schemaID]
	if versions == nil {
		return nil, fmt.Errorf("schema not found: %s", schemaID)
	}
	return versions, nil
}

// GetLatestVersion 获取最新版本
func (vs *VersionStore) GetLatestVersion(schemaID string) (*SchemaVersion, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	versions := vs.versions[schemaID]
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions for schema: %s", schemaID)
	}
	return &versions[len(versions)-1], nil
}

// DiffVersions 比较两个版本的差异
func (vs *VersionStore) DiffVersions(schemaID string, v1, v2 int) (*VersionDiff, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	prev := vs.findVersion(schemaID, v1)
	curr := vs.findVersion(schemaID, v2)

	if prev == nil {
		return nil, fmt.Errorf("version v%d not found", v1)
	}
	if curr == nil {
		return nil, fmt.Errorf("version v%d not found", v2)
	}

	return diffSchemas(prev.Snapshot, curr.Snapshot), nil
}

// findVersion 查找版本 (内部方法, 不加锁)
func (vs *VersionStore) findVersion(schemaID string, version int) *SchemaVersion {
	versions := vs.versions[schemaID]
	for _, v := range versions {
		if v.Version == version {
			return &v
		}
	}
	return nil
}

// saveVersionDB 保存版本到数据库
func (vs *VersionStore) saveVersionDB(version *SchemaVersion) error {
	if vs.db == nil {
		return nil
	}

	data, err := json.Marshal(version)
	if err != nil {
		return err
	}

	switch vs.db.Dialect() {
	case storage.Postgres:
		_, err = vs.db.DB().Exec(`INSERT INTO schema_versions (id, schema_id, version, parent_version, change_reason, snapshot, hash, created_at, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT(id) DO UPDATE SET change_reason=$5, snapshot=$6, hash=$7`,
			version.ID, version.SchemaID, version.Version, version.ParentVersion,
			version.ChangeReason, string(data), version.Hash,
			version.CreatedAt, version.CreatedBy)
	default:
		_, err = vs.db.DB().Exec(`INSERT INTO schema_versions (id, schema_id, version, parent_version, change_reason, snapshot, hash, created_at, created_by)
			VALUES (?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET change_reason=excluded.change_reason, snapshot=excluded.snapshot, hash=excluded.hash`,
			version.ID, version.SchemaID, version.Version, version.ParentVersion,
			version.ChangeReason, string(data), version.Hash,
			version.CreatedAt, version.CreatedBy)
	}
	return err
}

// DeepCopy 深拷贝 Schema
func (s *ParadigmSchema) DeepCopy() *ParadigmSchema {
	deepCopy := &ParadigmSchema{
		ID:            s.ID,
		Name:          s.Name,
		Version:       s.Version,
		SchemaVersion: s.SchemaVersion,
		Description:   s.Description,
		HoldingPeriod: s.HoldingPeriod,
		MaxDrawdown:   s.MaxDrawdown,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
		ChangeReason:  s.ChangeReason,
		ParentVersion: s.ParentVersion,
	}

	// 深拷贝特征
	if s.Features != nil {
		deepCopy.Features = make([]FeatureDefinition, len(s.Features))
		copy(deepCopy.Features, s.Features)
		// 深拷贝每个特征的 params
		for i, f := range s.Features {
			if f.Params != nil {
				deepCopy.Features[i].Params = make(map[string]float64)
				for k, v := range f.Params {
					deepCopy.Features[i].Params[k] = v
				}
			}
			if f.Dependency != nil {
				deepCopy.Features[i].Dependency = make([]string, len(f.Dependency))
				copy(deepCopy.Features[i].Dependency, f.Dependency)
			}
		}
	}

	// 深拷贝上下文规则
	if s.ContextRules != nil {
		deepCopy.ContextRules = make([]ContextRule, len(s.ContextRules))
		copy(deepCopy.ContextRules, s.ContextRules)
		for i, cr := range s.ContextRules {
			if cr.Values != nil {
				deepCopy.ContextRules[i].Values = make([]string, len(cr.Values))
				copy(deepCopy.ContextRules[i].Values, cr.Values)
			}
		}
	}

	// 深拷贝规则
	if s.Rules != nil {
		deepCopy.Rules = make([]Rule, len(s.Rules))
		copy(deepCopy.Rules, s.Rules)
		for i, r := range s.Rules {
			if r.Thresholds != nil {
				deepCopy.Rules[i].Thresholds = make([]float64, len(r.Thresholds))
				copy(deepCopy.Rules[i].Thresholds, r.Thresholds)
			}
		}
	}

	return deepCopy
}

// VersionDiff 版本差异
type VersionDiff struct {
	AddedRules   []string `json:"added_rules"`
	RemovedRules []string `json:"removed_rules"`
	ChangedRules []string `json:"changed_rules"`
	AddedContext []string `json:"added_context"`
	RemovedCtx   []string `json:"removed_context"`
	FeatureChanges []string `json:"feature_changes"`
}

// diffSchemas 比较两个 Schema
func diffSchemas(old, new *ParadigmSchema) *VersionDiff {
	diff := &VersionDiff{}

	// 比较规则
	oldRules := make(map[string]*Rule)
	for i := range old.Rules {
		oldRules[old.Rules[i].ID] = &old.Rules[i]
	}

	newRules := make(map[string]*Rule)
	for i := range new.Rules {
		newRules[new.Rules[i].ID] = &new.Rules[i]
	}

	// 新增
	for id := range newRules {
		if _, exists := oldRules[id]; !exists {
			diff.AddedRules = append(diff.AddedRules, id)
		}
	}

	// 删除
	for id := range oldRules {
		if _, exists := newRules[id]; !exists {
			diff.RemovedRules = append(diff.RemovedRules, id)
		}
	}

	// 变更
	for id, oldRule := range oldRules {
		if newRule, exists := newRules[id]; exists {
			if !ruleEqual(oldRule, newRule) {
				diff.ChangedRules = append(diff.ChangedRules, id)
			}
		}
	}

	// 比较上下文
	oldCtx := make(map[ContextKey]bool)
	for _, cr := range old.ContextRules {
		oldCtx[cr.Key] = true
	}

	newCtx := make(map[ContextKey]bool)
	for _, cr := range new.ContextRules {
		newCtx[cr.Key] = true
	}

	for key := range newCtx {
		if !oldCtx[key] {
			diff.AddedContext = append(diff.AddedContext, string(key))
		}
	}

	for key := range oldCtx {
		if !newCtx[key] {
			diff.RemovedCtx = append(diff.RemovedCtx, string(key))
		}
	}

	return diff
}

// ruleEqual 比较两条规则是否相等
func ruleEqual(r1, r2 *Rule) bool {
	if r1.ID != r2.ID ||
		r1.Type != r2.Type ||
		r1.Side != r2.Side ||
		r1.FeatureName != r2.FeatureName ||
		r1.Operator != r2.Operator ||
		r1.Required != r2.Required ||
		r1.Weight != r2.Weight {
		return false
	}

	if len(r1.Thresholds) != len(r2.Thresholds) {
		return false
	}
	for i := range r1.Thresholds {
		if r1.Thresholds[i] != r2.Thresholds[i] {
			return false
		}
	}

	return true
}

// computeHash 计算 Schema 内容哈希 (简单版本)
func computeHash(schema *ParadigmSchema) string {
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	// 简单哈希 (实际应用中应使用 SHA-256)
	hash := 0
	for _, b := range data {
		hash = (hash * 31 + int(b)) & 0x7FFFFFFF
	}
	return fmt.Sprintf("%x", hash)
}
