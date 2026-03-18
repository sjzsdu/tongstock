package tdx

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/cache"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// TTL constants for cache stores
const (
	xdxrTTL    = 7 * 24 * time.Hour
	financeTTL = 7 * 24 * time.Hour
	companyTTL = 30 * 24 * time.Hour
	blockTTL   = 24 * time.Hour
)

// XdXrStore caches除权除息信息
type XdXrStore struct {
	cache cache.Cache
	ttl   time.Duration
}

// FinanceStore caches finance information
type FinanceStore struct {
	cache cache.Cache
	ttl   time.Duration
}

// CompanyStore caches company information (category and content)
type CompanyStore struct {
	cache cache.Cache
	ttl   time.Duration
}

// BlockStore caches block information
type BlockStore struct {
	cache cache.Cache
	ttl   time.Duration
}

var (
	// Optional: a simple singleton for XdXrStore cached via GetCodeStore backend
	xdxrStore     *XdXrStore
	xdxrStoreOnce sync.Once
)

// GetXdXrStore returns a store backed by the same cache backend used by CodeStore.
// It follows the GetCodeStore pattern but reuses the underlying cache instance.
func GetXdXrStore(cachePath string) (*XdXrStore, error) {
	var err error
	xdxrStoreOnce.Do(func() {
		// Reuse the same cache backend as codes store
		cs, e := GetCodeStore(cachePath)
		if e != nil {
			err = e
			return
		}
		xdxrStore = &XdXrStore{cache: cs.cache, ttl: xdxrTTL}
	})
	if err != nil {
		return nil, err
	}
	// xdxrStore may still be nil if the first call failed, guard anyway
	return xdxrStore, nil
}

// Get reads cached XdXr items by code.
func (s *XdXrStore) Get(code string) ([]*protocol.XdXrItem, error) {
	data, err := s.cache.Get("xdxr", code)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}
	var items []*protocol.XdXrItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// Save caches XdXr items for a code.
func (s *XdXrStore) Save(code string, items []*protocol.XdXrItem) error {
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := s.cache.Set("xdxr", code, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

func (s *XdXrStore) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

// Get reads cached FinanceInfo by code.
func (s *FinanceStore) Get(code string) (*protocol.FinanceInfo, error) {
	data, err := s.cache.Get("finance", code)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}
	var info *protocol.FinanceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return info, nil
}

// Save caches FinanceInfo for a code.
func (s *FinanceStore) Save(code string, info *protocol.FinanceInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := s.cache.Set("finance", code, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

func (s *FinanceStore) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

// GetCategory reads cached company categories for a code.
func (s *CompanyStore) GetCategory(code string) ([]*protocol.CompanyCategoryItem, error) {
	data, err := s.cache.Get("company_cat", code)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}
	var items []*protocol.CompanyCategoryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// SaveCategory caches company categories for a code.
func (s *CompanyStore) SaveCategory(code string, items []*protocol.CompanyCategoryItem) error {
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := s.cache.Set("company_cat", code, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

// GetContent reads cached company content for a code and filename.
func (s *CompanyStore) GetContent(code, filename string) (string, error) {
	data, err := s.cache.Get("company_content", code+filename)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return "", nil
		}
		return "", err
	}
	var content string
	if err := json.Unmarshal(data, &content); err != nil {
		// fallback: if stored as plain bytes, try to cast
		return string(data), nil
	}
	return content, nil
}

// SaveContent caches company content for a code and filename.
func (s *CompanyStore) SaveContent(code, filename, content string) error {
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	if err := s.cache.Set("company_content", code+filename, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

func (s *CompanyStore) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

// Get reads cached block data for a given block file.
func (s *BlockStore) Get(blockFile string) ([]*protocol.BlockItem, error) {
	data, err := s.cache.Get("block", blockFile)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}
	var items []*protocol.BlockItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// Save caches block data for a block file.
func (s *BlockStore) Save(blockFile string, items []*protocol.BlockItem) error {
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := s.cache.Set("block", blockFile, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

func (s *BlockStore) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}
