package stockdata

import (
	"context"
	"errors"
)

// OfflineProvider 是 cache_only 组合使用的上游边界。
// 它不持有任何网络 client；若代码错误地尝试刷新，会显式失败。
type OfflineProvider struct{}

func NewOfflineProvider() *OfflineProvider {
	return &OfflineProvider{}
}

func (p *OfflineProvider) Sync(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
	return Dataset{}, SyncMetadata{}, errors.New("upstream disabled by cache_only")
}
