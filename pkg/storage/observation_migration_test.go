package storage

import (
	"path/filepath"
	"testing"
)

// 验证：Migrate 不要求版本连续，也不会改写已有 schema_migrations 行。
// 现在 26 已被 signal_state_daily 占用，所以用 28（在 27 之后留空）来验证。
func TestObservationMigrationLeavesUnknownEarlierVersionUntouched(t *testing.T) {
	store, err := New(Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "migration.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// 先插入一个"未知"的版本号，模拟并行分支留下的空号
	if _, err = store.DB().Exec(`INSERT INTO schema_migrations(version,name,applied_at)VALUES(28,'unknown_earlier_migration',1)`); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(); err != nil {
		t.Fatal(err)
	}
	var name string
	if err = store.DB().QueryRow(`SELECT name FROM schema_migrations WHERE version=28`).Scan(&name); err != nil || name != "unknown_earlier_migration" {
		t.Fatalf("existing migration altered %q %v", name, err)
	}
	var count int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM forward_observation`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(); err != nil {
		t.Fatal(err)
	}
}
