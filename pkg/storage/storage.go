package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// Dialect SQL 方言类型
type Dialect string

const (
	SQLite   Dialect = "sqlite"
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
	var driverName string
	var dsn string
	var dialect Dialect

	switch cfg.Driver {
	case "postgres", "postgresql":
		driverName = "postgres"
		dsn = cfg.DSN
		dialect = Postgres
	case "mysql":
		driverName = "mysql"
		dsn = cfg.DSN
		dialect = MySQL
	default:
		driverName = "sqlite3"
		dialect = SQLite
		if cfg.DSN == "" {
			home, _ := os.UserHomeDir()
			dsn = filepath.Join(home, ".tongstock", "data.db")
		} else {
			dsn = cfg.DSN
		}
		// 创建目录
		if dir := filepath.Dir(dsn); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("创建数据库目录失败: %w", err)
			}
		}
		dsn += "?cache=shared&_busy_timeout=5000"
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	if dialect != SQLite {
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, fmt.Errorf("连接数据库失败: %w", err)
		}
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return &Storage{db: db, dialect: dialect}, nil
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

