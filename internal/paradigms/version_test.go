package paradigms

import (
	"testing"
	"time"
)

// ============================================================================
// Version 测试
// ============================================================================

func TestNewVersionStore(t *testing.T) {
	vs := NewVersionStore(nil)
	if vs == nil {
		t.Fatal("NewVersionStore should return a non-nil store")
	}
}

func TestVersionStore_SaveAndGet(t *testing.T) {
	vs := NewVersionStore(nil)
	schema := createValidSchema()

	// 保存版本
	version, err := vs.SaveVersion(schema, "initial creation", "test-user")
	if err != nil {
		t.Fatalf("SaveVersion failed: %v", err)
	}

	if version.Version != 1 {
		t.Errorf("expected version 1, got %d", version.Version)
	}
	if version.SchemaID != "test-001" {
		t.Errorf("expected schema ID test-001, got %s", version.SchemaID)
	}
	if version.Snapshot == nil {
		t.Error("snapshot should not be nil")
	}

	// 获取版本
	got, err := vs.GetVersion("test-001", 1)
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("expected version 1, got %d", got.Version)
	}
}

func TestVersionStore_GetNonExistent(t *testing.T) {
	vs := NewVersionStore(nil)

	_, err := vs.GetVersion("nonexistent", 1)
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestVersionStore_History(t *testing.T) {
	vs := NewVersionStore(nil)
	schema := createValidSchema()

	// 保存多个版本
	_, err := vs.SaveVersion(schema, "v1", "user1")
	if err != nil {
		t.Fatal(err)
	}

	// 创建新版本
	time.Sleep(10 * time.Millisecond)
	schema2 := schema.CreateVersion("v2")
	_, err = vs.SaveVersion(schema2, "v2", "user1")
	if err != nil {
		t.Fatal(err)
	}

	// 获取历史
	history, err := vs.GetHistory("test-001")
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("expected 2 versions, got %d", len(history))
	}
}

func TestVersionStore_GetLatest(t *testing.T) {
	vs := NewVersionStore(nil)
	schema := createValidSchema()

	// 保存版本
	_, err := vs.SaveVersion(schema, "v1", "user1")
	if err != nil {
		t.Fatal(err)
	}

	// 获取最新版本
	latest, err := vs.GetLatestVersion("test-001")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 1 {
		t.Errorf("expected version 1, got %d", latest.Version)
	}
}

func TestVersionStore_GetLatestEmpty(t *testing.T) {
	vs := NewVersionStore(nil)

	_, err := vs.GetLatestVersion("nonexistent")
	if err == nil {
		t.Error("expected error for empty versions")
	}
}

func TestVersionStore_Diff(t *testing.T) {
	vs := NewVersionStore(nil)
	schema := createValidSchema()

	// 保存版本
	_, err := vs.SaveVersion(schema, "v1", "user1")
	if err != nil {
		t.Fatal(err)
	}

	// 创建新版本 (修改阈值)
	time.Sleep(10 * time.Millisecond)
	schema2 := schema.CreateVersion("v2")
	schema2.Rules[0].Thresholds[0] = 15.0 // 修改阈值
	_, err = vs.SaveVersion(schema2, "v2", "user1")
	if err != nil {
		t.Fatal(err)
	}

	// 比较版本
	diff, err := vs.DiffVersions("test-001", 1, 2)
	if err != nil {
		t.Fatalf("DiffVersions failed: %v", err)
	}

	// 规则应该在 changed 列表中
	if len(diff.ChangedRules) == 0 {
		t.Error("expected changed rules")
	}
}

func TestVersionStore_DiffNonExistent(t *testing.T) {
	vs := NewVersionStore(nil)

	_, err := vs.DiffVersions("nonexistent", 1, 2)
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestSchemaDeepCopy(t *testing.T) {
	schema := createValidSchema()

	// 深拷贝
	copy := schema.DeepCopy()

	// 修改副本不影响原
	copy.Rules[0].Thresholds[0] = 20.0
	if schema.Rules[0].Thresholds[0] == 20.0 {
		t.Error("DeepCopy should create independent copy")
	}

	// 修改特征参数
	copy.Features[0].Params["period"] = 10
	if schema.Features[0].Params["period"] == 10 {
		t.Error("DeepCopy should copy feature params")
	}

	// 修改上下文规则
	copy.ContextRules[0].Values = append(copy.ContextRules[0].Values, "new_value")
	if len(schema.ContextRules[0].Values) != 2 {
		t.Error("DeepCopy should copy context values")
	}
}

func TestVersionDiff(t *testing.T) {
	v1 := createValidSchema()
	v2 := v1.DeepCopy()
	v2.Rules[0].Thresholds[0] = 20.0  // 修改阈值
	v2.Rules = append(v2.Rules, Rule{ // 添加新规则
		ID:          "new-rule",
		Type:        TypeConfirmation,
		Side:        SideBuy,
		FeatureName: "price.volume",
		Operator:    OpGreaterThan,
		Thresholds:  []float64{1000000.0},
		Required:    false,
		Weight:      0.5,
	})

	diff := diffSchemas(v1, v2)

	// 应该有新增规则
	if len(diff.AddedRules) == 0 {
		t.Error("expected added rules")
	}
	// 应该有变更规则
	if len(diff.ChangedRules) == 0 {
		t.Error("expected changed rules")
	}
}

func TestComputeHash(t *testing.T) {
	schema := createValidSchema()
	hash1 := computeHash(schema)

	// 相同 Schema 应该有相同哈希
	hash2 := computeHash(schema)
	if hash1 != hash2 {
		t.Error("same schema should have same hash")
	}

	// 修改 Schema 应该有不同哈希
	schema.Rules[0].Thresholds[0] = 20.0
	hash3 := computeHash(schema)
	if hash1 == hash3 {
		t.Error("different schema should have different hash")
	}
}

func TestVersionStore_NoDuplicateSave(t *testing.T) {
	vs := NewVersionStore(nil)
	schema := createValidSchema()

	// 保存版本
	v1, err := vs.SaveVersion(schema, "test", "user")
	if err != nil {
		t.Fatal(err)
	}

	// 修改 Schema 后保存 (应该创建新版本)
	time.Sleep(10 * time.Millisecond)
	schema.Rules[0].Thresholds[0] = 15.0
	v2, err := vs.SaveVersion(schema, "modified", "user")
	if err != nil {
		t.Fatal(err)
	}

	// 两次保存应该有不同哈希
	if v1.Hash == v2.Hash {
		t.Error("different content should have different hash")
	}

	// 应该有两个版本
	history, err := vs.GetHistory("test-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Errorf("expected 2 versions, got %d", len(history))
	}
}
