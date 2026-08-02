// Package validationrepo 提供 validation 域的 SQLite 适配器。
// 验证只读 snapshot_kline_bar 中的不可变真实 K 线，禁止回退到可变 kline 表。
package validationrepo

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// KlineTypeDaily 是项目中通达信 A 股日线的真实 ktype。
const KlineTypeDaily uint8 = 9

// SQLiteBarProvider 从已验证内容哈希的数据快照加载日线。
type SQLiteBarProvider struct {
	snapshots *paradigm.DatasetSnapshotStore
}

// New 创建只读冻结快照的 BarProvider。
func New(store *storage.Storage) *SQLiteBarProvider {
	return &SQLiteBarProvider{snapshots: paradigm.NewDatasetSnapshotStore(store)}
}

// LoadBars 实现 validation.BarProvider。
func (p *SQLiteBarProvider) LoadBars(ctx context.Context, snapshotID, code, dateStart, dateEnd string) ([]validation.BacktestBar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshot_id is required: live kline fallback is forbidden")
	}
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	if p == nil || p.snapshots == nil {
		return nil, fmt.Errorf("snapshot provider is not initialized")
	}
	if err := p.snapshots.VerifyContent(snapshotID); err != nil {
		return nil, fmt.Errorf("verify frozen snapshot %s: %w", snapshotID, err)
	}
	frozen, err := p.snapshots.GetFrozenKlines(snapshotID, code, KlineTypeDaily)
	if err != nil {
		return nil, fmt.Errorf("load frozen K line %s: %w", code, err)
	}

	start := normalizeDate(dateStart)
	end := normalizeDate(dateEnd)
	board := backtest.BoardForCode(code)
	bars := make([]validation.BacktestBar, 0, len(frozen))
	var prevClose float64
	for _, row := range frozen {
		dateKey := row.Date.Format("20060102")
		inRange := (start == "" || dateKey >= start) && (end == "" || dateKey <= end)
		if inRange {
			bar := validation.BacktestBar{
				Code: code, Date: row.Date.Format("2006-01-02"),
				Open: row.Open, High: row.High, Low: row.Low, Close: row.Close,
				Volume: row.Volume, Amount: row.Amount, Board: board,
				PreClose: prevClose, Suspended: row.Volume <= 0,
			}
			if !bar.Suspended && (bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0) {
				return nil, fmt.Errorf("invalid frozen bar %s %s: non-positive price", code, bar.Date)
			}
			bars = append(bars, bar)
		}
		prevClose = row.Close
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("snapshot %s has no %s bars in [%s,%s]: fail closed", snapshotID, code, dateStart, dateEnd)
	}
	return bars, nil
}

// SQLiteBenchmarkProvider 从同一冻结快照计算基准日收益。
type SQLiteBenchmarkProvider struct {
	bars validation.BarProvider
}

// NewBenchmark 创建基准适配器。
func NewBenchmark(store *storage.Storage) *SQLiteBenchmarkProvider {
	return &SQLiteBenchmarkProvider{bars: New(store)}
}

// LoadDailyReturns 实现 validation.BenchmarkProvider。
func (p *SQLiteBenchmarkProvider) LoadDailyReturns(ctx context.Context, snapshotID, code, dateStart, dateEnd string) (map[string]float64, error) {
	bars, err := p.bars.LoadBars(ctx, snapshotID, code, dateStart, dateEnd)
	if err != nil {
		return nil, err
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date < bars[j].Date })
	returns := make(map[string]float64, len(bars)-1)
	var previous *validation.BacktestBar
	for i := range bars {
		bar := &bars[i]
		if bar.Suspended || bar.Close <= 0 {
			continue
		}
		if previous != nil && previous.Close > 0 {
			returns[bar.Date] = (bar.Close - previous.Close) / previous.Close
		}
		previous = bar
	}
	if len(returns) == 0 {
		return nil, fmt.Errorf("snapshot %s has insufficient benchmark bars for %s", snapshotID, code)
	}
	return returns, nil
}

func normalizeDate(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "")
}

var (
	_ validation.BarProvider       = (*SQLiteBarProvider)(nil)
	_ validation.BenchmarkProvider = (*SQLiteBenchmarkProvider)(nil)
)
