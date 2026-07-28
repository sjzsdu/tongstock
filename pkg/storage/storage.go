package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Dialect SQL 方言类型
type Dialect string

const (
	SQLite Dialect = "sqlite"
	// Postgres and MySQL remain as compile-time compatibility constants for
	// legacy Store code. New rejects both drivers: TongStock officially
	// supports SQLite only (see docs/adr/0001-sqlite-only.md).
	Postgres Dialect = "postgres"
	MySQL    Dialect = "mysql"
)

// Storage 统一存储能力
type Storage struct {
	db      *sql.DB
	dialect Dialect
}

// Config 存储配置
type Config struct {
	Driver string // sqlite3, postgres, mysql
	DSN    string
}

// New 创建存储实例
func New(cfg Config) (*Storage, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver != "" && driver != "sqlite" && driver != "sqlite3" {
		return nil, fmt.Errorf("不支持数据库驱动 %q：TongStock 仅支持 sqlite3", cfg.Driver)
	}

	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		home, _ := os.UserHomeDir()
		dsn = filepath.Join(home, ".tongstock", "data.db")
	}
	if strings.HasPrefix(dsn, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dsn = filepath.Join(home, strings.TrimPrefix(dsn, "~/"))
		}
	}
	if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
		if dir := filepath.Dir(dsn); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("创建数据库目录失败: %w", err)
			}
		}
	}
	if !strings.Contains(dsn, "?") {
		dsn += "?cache=shared&_busy_timeout=5000&_foreign_keys=on"
	} else {
		dsn += "&_busy_timeout=5000&_foreign_keys=on"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	// A single writer connection gives local SQLite deterministic transaction
	// semantics and avoids per-connection in-memory databases in tests.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Storage{db: db, dialect: SQLite}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("迁移数据库失败: %w", err)
	}
	return s, nil
}

// DB 返回数据库连接
func (s *Storage) DB() *sql.DB {
	return s.db
}

// Dialect 返回 SQL 方言
func (s *Storage) Dialect() Dialect {
	return s.dialect
}

// Close 关闭连接
func (s *Storage) Close() error {
	return s.db.Close()
}
