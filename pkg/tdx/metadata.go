package tdx

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/cache"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// TTL constants for cache stores
const (
	xdxrTTL    = 7 * 24 * time.Hour
	financeTTL = 7 * 24 * time.Hour
	// companyTTL caps both the F10 catalogue and the cached F10 document.
	// The document is regenerated daily, so a month-long catalogue only
	// guarantees stale byte offsets. Correctness does not rest on this value
	// (block reads verify their boundaries against the document), but keeping
	// it inside the document's update period avoids paying for a catalogue
	// refresh on every read.
	companyTTL = 24 * time.Hour
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

// companyCacheKey builds the cache key of F10 data. Stock codes reach us as
// "000001", "sz000001" or "SZ000001" depending on the caller and file names
// have been seen in more than one spelling, so without normalization a single
// stock can accumulate several entries that never overwrite each other (and
// that go stale independently).
func companyCacheKey(code, filename string) string {
	return normalizeCompanyCode(code) + strings.ToLower(strings.TrimSpace(filename))
}

func normalizeCompanyCode(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if len(c) == 8 {
		switch c[:2] {
		case "sh", "sz", "bj":
			return c[2:]
		}
	}
	return c
}

// GetCategory reads cached company categories for a code.
func (s *CompanyStore) GetCategory(code string) ([]*protocol.CompanyCategoryItem, error) {
	data, err := s.cache.Get("company_cat", companyCacheKey(code, ""))
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
	if err := s.cache.Set("company_cat", companyCacheKey(code, ""), data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	return nil
}

// GetContent reads the cached F10 document of a code and filename. The bytes
// are returned exactly as they came off the wire (GBK, which is what Start and
// Length are counted in), so callers can slice them with catalogue offsets.
func (s *CompanyStore) GetContent(code, filename string) ([]byte, error) {
	data, err := s.cache.Get("company_content", companyCacheKey(code, filename))
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}
	var doc []byte
	if err := json.Unmarshal(data, &doc); err != nil {
		// Entries written before documents were stored as raw bytes hold a
		// UTF-8 string, which cannot be sliced by GBK offsets. Treat them as
		// a miss and let the caller refetch instead of serving misaligned
		// text.
		return nil, nil
	}
	return doc, nil
}

// SaveContent caches the raw (GBK) F10 document of a code and filename.
func (s *CompanyStore) SaveContent(code, filename string, doc []byte) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := s.cache.Set("company_content", companyCacheKey(code, filename), data, cache.WithTTL(s.ttl)); err != nil {
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
