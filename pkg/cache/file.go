package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type fileCache struct {
	baseDir string
	mu      sync.RWMutex
}

type fileMeta struct {
	Key       string `json:"key"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

// NewFileCache 创建基于文件系统的缓存实例
func NewFileCache(baseDir string) (Cache, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}
	return &fileCache{baseDir: baseDir}, nil
}

func (c *fileCache) hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)[:32]
}

func (c *fileCache) datPath(bucket, key string) string {
	return filepath.Join(c.baseDir, bucket, c.hashKey(key)+".dat")
}

func (c *fileCache) metaPath(bucket, key string) string {
	return filepath.Join(c.baseDir, bucket, c.hashKey(key)+".meta")
}

func (c *fileCache) readMeta(path string) (*fileMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var m fileMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *fileCache) deleteFiles(bucket, key string) {
	os.Remove(c.datPath(bucket, key))
	os.Remove(c.metaPath(bucket, key))
}

func (c *fileCache) Get(bucket, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	meta, err := c.readMeta(c.metaPath(bucket, key))
	if err != nil {
		return nil, err
	}

	if meta.ExpiresAt > 0 && time.Now().Unix() > meta.ExpiresAt {
		return nil, ErrExpired
	}

	data, err := os.ReadFile(c.datPath(bucket, key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

func (c *fileCache) Set(bucket, key string, value []byte, opts ...Option) error {
	o := applyOptions(opts)

	c.mu.Lock()
	defer c.mu.Unlock()

	bucketDir := filepath.Join(c.baseDir, bucket)
	if err := os.MkdirAll(bucketDir, 0755); err != nil {
		return err
	}

	now := time.Now().Unix()
	var expiresAt int64
	if o.TTL > 0 {
		expiresAt = now + int64(o.TTL.Seconds())
	}

	datPath := c.datPath(bucket, key)
	if err := os.WriteFile(datPath, value, 0644); err != nil {
		return err
	}

	meta := fileMeta{
		Key:       key,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		os.Remove(datPath)
		return err
	}
	metaPath := c.metaPath(bucket, key)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		os.Remove(datPath)
		return err
	}

	return nil
}

func (c *fileCache) Delete(bucket, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteFiles(bucket, key)
	return nil
}

func (c *fileCache) Has(bucket, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	meta, err := c.readMeta(c.metaPath(bucket, key))
	if err != nil {
		return false
	}
	if meta.ExpiresAt > 0 && time.Now().Unix() > meta.ExpiresAt {
		return false
	}
	return true
}

func (c *fileCache) List(bucket string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bucketDir := filepath.Join(c.baseDir, bucket)
	entries, err := os.ReadDir(bucketDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	now := time.Now().Unix()
	var keys []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".meta") {
			continue
		}
		metaPath := filepath.Join(bucketDir, entry.Name())
		meta, err := c.readMeta(metaPath)
		if err != nil {
			continue
		}
		if meta.ExpiresAt > 0 && now > meta.ExpiresAt {
			continue
		}
		keys = append(keys, meta.Key)
	}
	return keys, nil
}

func (c *fileCache) Clear(bucket string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return os.RemoveAll(filepath.Join(c.baseDir, bucket))
}

func (c *fileCache) Close() error {
	return nil
}
