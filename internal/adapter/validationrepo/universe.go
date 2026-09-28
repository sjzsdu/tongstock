package validationrepo

import (
	"context"
	"fmt"
	"sort"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteSnapshotUniverse 从冻结快照内真实 K 线的代码集合解析验证股票池。
// 它只读 snapshot_kline_bar，不读可变 kline 表，保证验证输入的可复现性。
type SQLiteSnapshotUniverse struct{ db *storage.Storage }

// NewSnapshotUniverse 创建快照股票池解析器。
func NewSnapshotUniverse(store *storage.Storage) *SQLiteSnapshotUniverse {
	return &SQLiteSnapshotUniverse{db: store}
}

// ResolveUniverse 返回快照内拥有足够真实日线的代码列表（升序）。
// minBars 以下的代码被排除并计数；maxCodes<=0 表示不设上限。
func (u *SQLiteSnapshotUniverse) ResolveUniverse(ctx context.Context, snapshotID string, minBars, maxCodes int) ([]string, int, error) {
	if snapshotID == "" {
		return nil, 0, fmt.Errorf("snapshot_id is required")
	}
	if minBars <= 0 {
		minBars = 30
	}
	rows, err := u.db.DB().QueryContext(ctx, `SELECT code, COUNT(*) AS n
		FROM snapshot_kline_bar WHERE snapshot_id = ? AND ktype = ?
		GROUP BY code HAVING n >= ? ORDER BY code`, snapshotID, KlineTypeDaily, minBars)
	if err != nil {
		return nil, 0, fmt.Errorf("query snapshot universe: %w", err)
	}
	defer rows.Close()
	codes := make([]string, 0, 1024)
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, 0, err
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	sort.Strings(codes)
	skipped := 0
	if maxCodes > 0 && len(codes) > maxCodes {
		// 均匀抽样，避免只取代码序靠前的交易所偏置。
		stride := float64(len(codes)) / float64(maxCodes)
		sampled := make([]string, 0, maxCodes)
		for i := 0; i < maxCodes; i++ {
			idx := int(float64(i) * stride)
			if idx >= len(codes) {
				idx = len(codes) - 1
			}
			sampled = append(sampled, codes[idx])
		}
		skipped = len(codes) - len(sampled)
		codes = sampled
	}
	return codes, skipped, nil
}

var _ = (*SQLiteSnapshotUniverse)(nil)
