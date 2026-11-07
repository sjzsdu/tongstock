package validationrepo

// LiveSource 是项目红线的数据源实现：研究/验证永远直接读活表 kline 的
// 真实全量数据，不冻结快照、不使用副本。它同时实现三个端口：
//   - SnapshotStore 形状（List/GetByID/VerifyContent/CreateKlineSnapshot）——
//     供 methodseed/methodautomation/factorlab/paradigmspromote 现有装配直接替换；
//   - UniverseResolver（ResolveUniverse）；
//   - validation.BarProvider（LoadBars）。
//
// 所有以 snapshotID 命名的方法都会忽略该参数：任何 ID 返回的都是活表数据源
// （身份标识固定为 LiveSnapshotID），保证证据链里记录的数据源诚实可见。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/internal/backtest"
	"github.com/sjzsdu/tongstock/internal/paradigm"
	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// LiveSnapshotID 是活表数据源在证据链中的固定身份标识。
const LiveSnapshotID = "live-kline"

// liveWindowYears 是研究/验证的默认数据窗口（年）。全历史回测（1990 起）
// 成本过高且远古市场结构与当前不可比，默认取最新 4 年。
const liveWindowYears = 4

// LiveSource 直读活表 kline 的全量数据源。
type LiveSource struct {
	store *storage.Storage

	mu         sync.Mutex
	cached     *paradigm.DatasetSnapshot
	floatMu    sync.Mutex
	floatMap   map[string]float64 // code → 流通股本（股）
	floatOnce  bool
	holderMu   sync.Mutex
	holderMap  map[string]float64 // code → 股东户数（户）
	holderOnce bool
	uniMu      sync.Mutex
	uniCounts  map[string]int // code → 窗口内日线数（进程内缓存，避免重复全表扫描）
}

// universeCounts 返回窗口起始日以来每只股票的日线数（进程内缓存）。
// 全表 GROUP BY 在 3GB 库上约 10 秒，必须只做一次并被
// liveSnapshot / ResolveUniverse 共用。
func (s *LiveSource) universeCounts(windowStart string) (map[string]int, error) {
	s.uniMu.Lock()
	defer s.uniMu.Unlock()
	if s.uniCounts != nil {
		return s.uniCounts, nil
	}
	rows, err := s.store.DB().Query(
		`SELECT code, COUNT(*) FROM kline_latest WHERE ktype=? AND date >= ? GROUP BY code`,
		KlineTypeDaily, windowStart,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve live universe: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		counts[code] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.uniCounts = counts
	return counts, nil
}

// floatShares 返回一只股票的流通股本（股）；缺失返回 0。结果进程内缓存。
func (s *LiveSource) floatShares(code string) float64 {
	s.floatMu.Lock()
	defer s.floatMu.Unlock()
	if !s.floatOnce {
		s.floatOnce = true
		s.floatMap = map[string]float64{}
		if rows, err := s.store.DB().Query(
			`SELECT code, liu_tong_gu_ben FROM stockinfo WHERE liu_tong_gu_ben > 0`); err == nil {
			for rows.Next() {
				var c string
				var v float64
				if rows.Scan(&c, &v) == nil {
					s.floatMap[c] = v * 1e4 // 万股 → 股
				}
			}
			rows.Close()
		}
	}
	return s.floatMap[code]
}

// NewLiveSource 创建活表数据源。
func NewLiveSource(store *storage.Storage) *LiveSource {
	return &LiveSource{store: store}
}

// holderCount 返回一只股票的股东户数（户）；缺失返回 0。结果进程内缓存。
// 数据来自 finance_snapshot（TDX 财务协议快照的 GuDongRenShu 字段），
// 无 point-in-time 序列，仅最新值（与 selection scope 口径一致）。
func (s *LiveSource) holderCount(code string) float64 {
	s.holderMu.Lock()
	defer s.holderMu.Unlock()
	if !s.holderOnce {
		s.holderOnce = true
		s.holderMap = map[string]float64{}
		if rows, err := s.store.DB().Query(`SELECT code, payload FROM finance_snapshot`); err == nil {
			for rows.Next() {
				var c, raw string
				if rows.Scan(&c, &raw) != nil {
					continue
				}
				var payload struct {
					GuDongRenShu float64 `json:"GuDongRenShu"`
				}
				if json.Unmarshal([]byte(raw), &payload) == nil && payload.GuDongRenShu > 0 {
					s.holderMap[c] = payload.GuDongRenShu
				}
			}
			rows.Close()
		}
	}
	return s.holderMap[code]
}

// StockFacts 实现 validation.StockFactsProvider：提供 scope 判定所需股票级事实。
// 注意：调用方必须在没有打开的 kline rows 时调用（单连接池）。
func (s *LiveSource) StockFacts(ctx context.Context, code string) (*validation.StockFacts, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &validation.StockFacts{
		FloatShares: s.floatShares(code),
		HolderCount: s.holderCount(code),
	}, nil
}

// liveSnapshot 构造（并缓存）活表数据源的描述对象：窗口取最新 4 年，
// universe 为窗口内拥有足够日线的全部股票。
func (s *LiveSource) liveSnapshot() (*paradigm.DatasetSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil {
		return s.cached, nil
	}
	var minDate, maxDate string
	if err := s.store.DB().QueryRow(
		`SELECT MIN(date), MAX(date) FROM kline_latest WHERE ktype=?`, KlineTypeDaily,
	).Scan(&minDate, &maxDate); err != nil || maxDate == "" {
		if err != nil {
			return nil, fmt.Errorf("probe live kline range: %w", err)
		}
		return nil, fmt.Errorf("live kline table is empty: run a market-wide daily sync first")
	}
	end, err := time.Parse("20060102", maxDate)
	if err != nil {
		return nil, fmt.Errorf("parse live kline max date %s: %w", maxDate, err)
	}
	start := end.AddDate(-liveWindowYears, 0, 0)
	codes, _, err := s.resolveLiveUniverse(start.Format("20060102"), 30, 0)
	if err != nil {
		return nil, err
	}
	snap := &paradigm.DatasetSnapshot{
		ID:              LiveSnapshotID,
		Version:         "live-kline-v1",
		DateRange:       paradigm.DateRange{Start: start.Format("2006-01-02"), End: end.Format("2006-01-02")},
		Universe:        codes,
		Market:          "A",
		PriceAdjustment: paradigm.PriceRaw,
		Description:     "活表全量数据源（项目红线：直接读 kline 真实全量，不冻结快照）",
		CreatedAt:       time.Now().UTC(),
	}
	s.cached = snap
	return snap, nil
}

// GetByID 返回活表数据源描述（忽略传入 ID）。
func (s *LiveSource) GetByID(id string) (*paradigm.DatasetSnapshot, error) {
	return s.liveSnapshot()
}

// VerifyContent 活表即数据源本体，无需校验（忽略传入 ID，恒成功）。
func (s *LiveSource) VerifyContent(snapshotID string) error { return nil }

// List 供 resolveSnapshot 的扫描路径使用：永远只返回活表数据源一项。
func (s *LiveSource) List(limit, offset int) ([]*paradigm.DatasetSnapshot, error) {
	if offset > 0 {
		return nil, nil
	}
	snap, err := s.liveSnapshot()
	if err != nil {
		return nil, err
	}
	return []*paradigm.DatasetSnapshot{snap}, nil
}

// CreateKlineSnapshot 按红线禁用：不再冻结任何快照。
func (s *LiveSource) CreateKlineSnapshot(snapshot *paradigm.DatasetSnapshot, ktype uint8) error {
	return fmt.Errorf("freezing snapshots is disabled: live kline is the single source of truth")
}

// ResolveUniverse 实现 UniverseResolver：活表内窗口期拥有 >=minBars 根日线的
// 全部股票；maxCodes>0 时按代码升序等步长抽样（与原快照语义一致）。
func (s *LiveSource) ResolveUniverse(ctx context.Context, snapshotID string, minBars, maxCodes int) ([]string, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	snap, err := s.liveSnapshot()
	if err != nil {
		return nil, 0, err
	}
	windowStart := normalizeDate(snap.DateRange.Start)
	codes, skipped, err := s.resolveLiveUniverse(windowStart, minBars, maxCodes)
	if err != nil {
		return nil, 0, err
	}
	if len(codes) == 0 {
		return nil, 0, fmt.Errorf("live kline has no code with >=%d bars since %s: run a market-wide daily sync first", minBars, windowStart)
	}
	return codes, skipped, nil
}

func (s *LiveSource) resolveLiveUniverse(windowStart string, minBars, maxCodes int) ([]string, int, error) {
	counts, err := s.universeCounts(windowStart)
	if err != nil {
		return nil, 0, err
	}
	codes := make([]string, 0, len(counts))
	skipped := 0
	for code, n := range counts {
		if n < minBars {
			skipped++
			continue
		}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	if maxCodes > 0 && len(codes) > maxCodes {
		// 均匀抽样，避免只取代码序靠前的交易所偏置。
		stride := float64(len(codes)) / float64(maxCodes)
		sampled := make([]string, 0, maxCodes)
		for i := 0; i < maxCodes; i++ {
			sampled = append(sampled, codes[int(float64(i)*stride)])
		}
		codes = sampled
	}
	return codes, skipped, nil
}

// LoadBars 实现 validation.BarProvider：直读活表 kline（区间过滤在 SQL 完成，
// 区间前取一根真实前收盘，与冻结实现的 PreClose 语义一致）。
// 同时按流通股本（stockinfo.liu_tong_gu_ben 当前值，窗口近似）计算逐 bar 换手率
// （%，stockinfo 口径），供 entry 规则引用 turnover_rate；缺股本的股票该值为 0，
// 相关条件在执行器中按 fail-closed 处理。
func (s *LiveSource) LoadBars(ctx context.Context, snapshotID, code, dateStart, dateEnd string) ([]validation.BacktestBar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	start := normalizeDate(dateStart)
	end := normalizeDate(dateEnd)

	var prevClose float64
	if start != "" {
		if err := s.store.DB().QueryRow(
			`SELECT close FROM kline_latest WHERE code=? AND ktype=? AND date < ? ORDER BY date DESC LIMIT 1`,
			code, KlineTypeDaily, start,
		).Scan(&prevClose); err != nil && err.Error() != "sql: no rows in result set" {
			return nil, fmt.Errorf("probe prev close %s: %w", code, err)
		}
	}

	query := `SELECT date, open, high, low, close, volume, amount FROM kline_latest WHERE code=? AND ktype=?`
	args := []any{code, KlineTypeDaily}
	if start != "" {
		query += ` AND date >= ?`
		args = append(args, start)
	}
	if end != "" {
		query += ` AND date <= ?`
		args = append(args, end)
	}
	query += ` ORDER BY date`
	// floatShares 首次调用会发全表查询；必须在 kline rows 打开之前完成，
	// 否则单连接池（SetMaxOpenConns(1)）下 rows 占住唯一连接、自己等自己死锁。
	board := backtest.BoardForCode(code)
	floatShares := s.floatShares(code) // 股；0 = 未知
	rows, err := s.store.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load live kline %s: %w", code, err)
	}
	defer rows.Close()
	bars := make([]validation.BacktestBar, 0, 1024)
	for rows.Next() {
		var date string
		var open, high, low, close, volume, amount float64
		if err := rows.Scan(&date, &open, &high, &low, &close, &volume, &amount); err != nil {
			return nil, err
		}
		if len(date) == 8 {
			date = date[:4] + "-" + date[4:6] + "-" + date[6:]
		}
		bar := validation.BacktestBar{
			Code: code, Date: date,
			Open: open, High: high, Low: low, Close: close,
			Volume: volume, Amount: amount, Board: board,
			PreClose: prevClose, Suspended: volume <= 0,
		}
		// kline.amount 存储单位为百元（真实成交额 = 列值 × 100）。
		if floatShares > 0 && close > 0 && amount > 0 {
			bar.TurnoverRate = amount * 100 / (floatShares * close) * 100
		}
		if !bar.Suspended && (bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0) {
			return nil, fmt.Errorf("invalid live bar %s %s: non-positive price", code, bar.Date)
		}
		bars = append(bars, bar)
		prevClose = close
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("live kline %s has no bars in [%s,%s]: fail closed", code, dateStart, dateEnd)
	}
	return bars, nil
}

var (
	_ paradigmSnapshotStore         = (*LiveSource)(nil)
	_ validation.BarProvider        = (*LiveSource)(nil)
	_ validation.StockFactsProvider = (*LiveSource)(nil)
)

// paradigmSnapshotStore 汇总各研究服务对快照存储的方法集要求（超集）。
type paradigmSnapshotStore interface {
	List(limit, offset int) ([]*paradigm.DatasetSnapshot, error)
	GetByID(id string) (*paradigm.DatasetSnapshot, error)
	VerifyContent(snapshotID string) error
	CreateKlineSnapshot(snapshot *paradigm.DatasetSnapshot, ktype uint8) error
}
