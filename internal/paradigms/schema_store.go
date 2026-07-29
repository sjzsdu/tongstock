package paradigms

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// ============================================================================
// Schema 存储 (与现有 Paradigm Store 集成)
// ============================================================================

// SchemaStore Schema 存储
type SchemaStore struct {
	mu           sync.RWMutex
	schemas      map[string]*ParadigmSchema // schema_id -> latest
	versionStore *VersionStore
	validator    *Validator
	db           *storage.Storage
}

// NewSchemaStore 创建 Schema 存储
func NewSchemaStore(db *storage.Storage) *SchemaStore {
	return &SchemaStore{
		schemas:      make(map[string]*ParadigmSchema),
		versionStore: NewVersionStore(db),
		validator:    NewValidator(),
		db:           db,
	}
}

// Save 保存 Schema (自动验证和版本管理)
func (ss *SchemaStore) Save(schema *ParadigmSchema) error {
	if schema == nil {
		return fmt.Errorf("schema cannot be nil")
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// 1. 验证 Schema
	errors := ss.validator.ValidateSchema(schema)
	for _, e := range errors {
		if e.Level == "error" {
			return fmt.Errorf("schema validation failed: %s", e)
		}
	}

	// 2. 检查是否为更新操作 (已有版本)
	existing, exists := ss.schemas[schema.ID]
	if exists {
		// 确认父版本
		if schema.ParentVersion == 0 {
			schema.ParentVersion = existing.Version
		}

		// 版本号递增
		if schema.Version <= existing.Version {
			schema.Version = existing.Version + 1
		}
	}

	// 3. 更新时间
	now := time.Now()
	schema.UpdatedAt = now
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = now
	}

	// 4. 保存版本
	changeReason := schema.ChangeReason
	if changeReason == "" {
		if exists {
			changeReason = "update"
		} else {
			changeReason = "initial creation"
		}
	}

	_, err := ss.versionStore.SaveVersion(schema, changeReason, "")
	if err != nil {
		return fmt.Errorf("save version: %w", err)
	}

	// 5. 保存最新版本
	ss.schemas[schema.ID] = schema

	// 6. 持久化到数据库
	if err := ss.saveDB(schema); err != nil {
		log.Printf("warn: save schema to db failed: %v", err)
	}

	return nil
}

// Get 获取最新版本的 Schema
func (ss *SchemaStore) Get(id string) (*ParadigmSchema, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	schema, ok := ss.schemas[id]
	if !ok {
		return nil, fmt.Errorf("schema not found: %s", id)
	}
	return schema, nil
}

// GetVersion 获取指定版本的 Schema
func (ss *SchemaStore) GetVersion(schemaID string, version int) (*ParadigmSchema, error) {
	sv, err := ss.versionStore.GetVersion(schemaID, version)
	if err != nil {
		return nil, err
	}
	if sv.Snapshot == nil {
		return nil, fmt.Errorf("version snapshot is empty")
	}
	return sv.Snapshot, nil
}

// GetHistory 获取版本历史
func (ss *SchemaStore) GetHistory(schemaID string) ([]SchemaVersion, error) {
	return ss.versionStore.GetHistory(schemaID)
}

// List 列出所有 Schema
func (ss *SchemaStore) List() []*ParadigmSchema {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	schemas := make([]*ParadigmSchema, 0, len(ss.schemas))
	for _, s := range ss.schemas {
		schemas = append(schemas, s)
	}
	return schemas
}

// Delete 删除 Schema
func (ss *SchemaStore) Delete(id string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if _, ok := ss.schemas[id]; !ok {
		return fmt.Errorf("schema not found: %s", id)
	}

	delete(ss.schemas, id)

	// 删除所有版本
	delete(ss.versionStore.versions, id)

	// 从数据库删除
	if err := ss.deleteDB(id); err != nil {
		log.Printf("warn: delete schema from db failed: %v", err)
	}

	return nil
}

// Validate 验证 Schema (不保存)
func (ss *SchemaStore) Validate(schema *ParadigmSchema) ([]ValidationError, error) {
	return ss.validator.ValidateSchema(schema), nil
}

// NewVersion 创建新版本 (基于现有 Schema 的修改)
func (ss *SchemaStore) NewVersion(id string, modifiedSchema *ParadigmSchema, changeReason string) (*ParadigmSchema, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	existing, ok := ss.schemas[id]
	if !ok {
		return nil, fmt.Errorf("schema not found: %s", id)
	}

	// 基于现有版本创建新版本
	newSchema := existing.CreateVersion(changeReason)

	// 应用修改
	if modifiedSchema != nil {
		// 保留版本信息
		newSchema.Version = existing.Version + 1
		newSchema.ParentVersion = existing.Version
		newSchema.ChangeReason = changeReason

		// 应用修改的字段
		if modifiedSchema.Name != "" {
			newSchema.Name = modifiedSchema.Name
		}
		if modifiedSchema.Description != "" {
			newSchema.Description = modifiedSchema.Description
		}
		if modifiedSchema.HoldingPeriod != "" {
			newSchema.HoldingPeriod = modifiedSchema.HoldingPeriod
		}
		if modifiedSchema.MaxDrawdown > 0 {
			newSchema.MaxDrawdown = modifiedSchema.MaxDrawdown
		}
		if len(modifiedSchema.Features) > 0 {
			newSchema.Features = modifiedSchema.Features
		}
		if len(modifiedSchema.ContextRules) > 0 {
			newSchema.ContextRules = modifiedSchema.ContextRules
		}
		if len(modifiedSchema.Rules) > 0 {
			newSchema.Rules = modifiedSchema.Rules
		}
	}

	// 验证
	errors := ss.validator.ValidateSchema(newSchema)
	for _, e := range errors {
		if e.Level == "error" {
			return nil, fmt.Errorf("new version validation failed: %s", e)
		}
	}

	// 保存版本
	_, err := ss.versionStore.SaveVersion(newSchema, changeReason, "")
	if err != nil {
		return nil, fmt.Errorf("save version: %w", err)
	}

	// 更新最新版本
	ss.schemas[id] = newSchema

	// 持久化
	if err := ss.saveDB(newSchema); err != nil {
		log.Printf("warn: save schema to db failed: %v", err)
	}

	return newSchema, nil
}

// GetValidator 获取验证器
func (ss *SchemaStore) GetValidator() *Validator {
	return ss.validator
}

// saveDB 保存到数据库
func (ss *SchemaStore) saveDB(schema *ParadigmSchema) error {
	if ss.db == nil {
		return nil
	}

	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	switch ss.db.Dialect() {
	case storage.Postgres:
		_, err = ss.db.DB().Exec(`INSERT INTO paradigm_schemas (id, version, data, updated_at)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT(id) DO UPDATE SET version=$2, data=$3, updated_at=$4`,
			schema.ID, schema.Version, string(data), time.Now())
	default:
		_, err = ss.db.DB().Exec(`INSERT INTO paradigm_schemas (id, version, data, updated_at)
			VALUES (?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET version=excluded.version, data=excluded.data, updated_at=excluded.updated_at`,
			schema.ID, schema.Version, string(data), time.Now())
	}
	return err
}

// deleteDB 从数据库删除
func (ss *SchemaStore) deleteDB(id string) error {
	if ss.db == nil {
		return nil
	}

	placeholder := "?"
	if ss.db.Dialect() == storage.Postgres {
		placeholder = "$1"
	}

	_, err := ss.db.DB().Exec("DELETE FROM paradigm_schemas WHERE id = "+placeholder, id)
	return err
}

// loadAllDB 从数据库加载所有 Schema
func (ss *SchemaStore) loadAllDB() error {
	if ss.db == nil {
		return nil
	}

	rows, err := ss.db.DB().Query(`SELECT data FROM paradigm_schemas`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			log.Printf("warn: scan schema row failed: %v", err)
			continue
		}

		var schema ParadigmSchema
		if err := json.Unmarshal([]byte(raw), &schema); err != nil {
			log.Printf("warn: parse schema from db failed: %v", err)
			continue
		}

		ss.schemas[schema.ID] = &schema
	}

	return nil
}

// CompareSchemas 比较两个 Schema
func (ss *SchemaStore) CompareSchemas(schemaID string, v1, v2 int) (*VersionDiff, error) {
	return ss.versionStore.DiffVersions(schemaID, v1, v2)
}

// GetVersionStore 获取版本存储
func (ss *SchemaStore) GetVersionStore() *VersionStore {
	return ss.versionStore
}
