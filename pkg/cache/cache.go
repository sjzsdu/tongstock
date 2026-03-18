package cache

import (
	"errors"
	"time"
)

// Cache 通用缓存接口，支持按 bucket/key 分组存储
type Cache interface {
	// Get 获取缓存值。未找到返回 ErrNotFound，已过期返回 ErrExpired
	Get(bucket, key string) ([]byte, error)
	// Set 存储缓存值，可通过 WithTTL 设置过期时间
	Set(bucket, key string, value []byte, opts ...Option) error
	// Delete 删除指定缓存项
	Delete(bucket, key string) error
	// Has 检查缓存项是否存在且未过期
	Has(bucket, key string) bool
	// List 列出 bucket 中所有未过期的 key
	List(bucket string) ([]string, error)
	// Clear 清空指定 bucket 的所有缓存
	Clear(bucket string) error
	// Close 释放资源
	Close() error
}

var (
	ErrNotFound = errors.New("cache: key not found")
	ErrExpired  = errors.New("cache: key expired")
)

type Option func(*options)

type options struct {
	TTL time.Duration
}

// WithTTL 设置缓存过期时间，0 表示永不过期
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		o.TTL = d
	}
}

func applyOptions(opts []Option) *options {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}
	return o
}
