package discoveryrepo

import (
	"context"
	"fmt"

	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

const dailyKType uint8 = 9

// SnapshotBarProvider 仅从经过内容哈希校验的冻结快照中读取发现数据。
type SnapshotBarProvider struct {
	snapshots *paradigm.DatasetSnapshotStore
}

func New(store *storage.Storage) *SnapshotBarProvider {
	return &SnapshotBarProvider{snapshots: paradigm.NewDatasetSnapshotStore(store)}
}

func (p *SnapshotBarProvider) Load(ctx context.Context, snapshotID, code string) ([]methods.Bar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshotID == "" || code == "" {
		return nil, fmt.Errorf("snapshot_id and code are required")
	}
	if err := p.snapshots.VerifyContent(snapshotID); err != nil {
		return nil, fmt.Errorf("verify frozen snapshot: %w", err)
	}
	frozen, err := p.snapshots.GetFrozenKlines(snapshotID, code, dailyKType)
	if err != nil {
		return nil, err
	}
	bars := make([]methods.Bar, 0, len(frozen))
	for _, bar := range frozen {
		if bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 || bar.Volume <= 0 {
			continue // 停牌或坏行不作为可观测交易日，绝不补值。
		}
		bars = append(bars, methods.Bar{
			Date: bar.Date.Format("2006-01-02"), Open: bar.Open, High: bar.High,
			Low: bar.Low, Close: bar.Close, Volume: bar.Volume, Amount: bar.Amount,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("snapshot %s has no usable real bars for %s", snapshotID, code)
	}
	return bars, nil
}

var _ discovery.BarProvider = (*SnapshotBarProvider)(nil)
