package config

import (
	"os"
	"path/filepath"
)

// HomeDir 返回用户主目录下的 ~/.tongstock/ 路径
// 如果无法获取用户主目录，则回退到当前工作目录下的 .tongstock/
func HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".tongstock")
	}
	return filepath.Join(".", ".tongstock")
}

// CacheDir 返回 ~/.tongstock/cache/ 路径
func CacheDir() string {
	return filepath.Join(HomeDir(), "cache")
}

// ConfigPath 返回 ~/.tongstock/config.yaml 路径
func ConfigPath() string {
	return filepath.Join(HomeDir(), "config.yaml")
}

// IndicatorConfigPath 返回 ~/.tongstock/indicator.yaml 路径
func IndicatorConfigPath() string {
	return filepath.Join(HomeDir(), "indicator.yaml")
}

// DBPath 返回默认的 SQLite 数据库路径: ~/.tongstock/cache/tongstock.db
func DBPath() string {
	return filepath.Join(CacheDir(), "tongstock.db")
}

// EnsureHomeDir 创建 ~/.tongstock/ 与 ~/.tongstock/cache/ 目录（如果不存在）
func EnsureHomeDir() error {
	if err := os.MkdirAll(HomeDir(), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(CacheDir(), 0755); err != nil {
		return err
	}
	return nil
}
