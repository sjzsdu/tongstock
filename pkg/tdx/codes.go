package tdx

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/cache"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type CodeStore struct {
	cache  cache.Cache
	dateMu sync.RWMutex
	date   time.Time
	ttl    time.Duration
}

var (
	store     *CodeStore
	storeOnce sync.Once
)

func GetCodeStore(cachePath string) (*CodeStore, error) {
	var err error
	storeOnce.Do(func() {
		var c cache.Cache
		cfg := config.Get()
		if cfg.Cache.Backend == "file" {
			c, err = cache.NewFileCache(cfg.Cache.Dir)
		} else {
			if cachePath == "" {
				cachePath = config.DBPath()
			}
			c, err = cache.NewSQLiteCache(cachePath)
		}
		if err != nil {
			return
		}
		store = &CodeStore{cache: c, ttl: 24 * time.Hour}
	})
	return store, err
}

func (s *CodeStore) SaveCodes(codes []*protocol.CodeItem, exchange protocol.Exchange) error {
	data, err := json.Marshal(codes)
	if err != nil {
		return err
	}
	key := exchange.String()
	if err := s.cache.Set("codes", key, data, cache.WithTTL(s.ttl)); err != nil {
		return err
	}
	s.dateMu.Lock()
	s.date = time.Now()
	s.dateMu.Unlock()
	return nil
}

func (s *CodeStore) GetCodes(exchange protocol.Exchange) ([]*protocol.CodeItem, error) {
	key := exchange.String()
	data, err := s.cache.Get("codes", key)
	if err != nil {
		if err == cache.ErrNotFound || err == cache.ErrExpired {
			return nil, nil
		}
		return nil, err
	}

	var items []*protocol.CodeItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *CodeStore) GetCode(code string) (*protocol.CodeItem, error) {
	exchanges := []protocol.Exchange{protocol.ExchangeSZ, protocol.ExchangeSH, protocol.ExchangeBJ}
	for _, ex := range exchanges {
		items, err := s.GetCodes(ex)
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			if c.Code == code {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("code not found")
}

func (s *CodeStore) NeedUpdate(exchange protocol.Exchange, maxAge time.Duration) bool {
	s.dateMu.RLock()
	defer s.dateMu.RUnlock()

	if s.date.IsZero() {
		return true
	}
	return time.Since(s.date) > maxAge
}

func (s *CodeStore) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

func (s *CodeStore) String() string {
	s.dateMu.RLock()
	defer s.dateMu.RUnlock()

	return fmt.Sprintf("CodeStore{date: %s}", s.date.Format("2006-01-02"))
}
