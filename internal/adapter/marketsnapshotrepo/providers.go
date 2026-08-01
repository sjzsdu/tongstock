package marketsnapshotrepo

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteUniverseProvider 从 stockinfo + security_status_history 构建 PIT 股票宇宙。
type SQLiteUniverseProvider struct{ db *storage.Storage }

func NewSQLiteUniverseProvider(db *storage.Storage) *SQLiteUniverseProvider {
	return &SQLiteUniverseProvider{db: db}
}

func (p *SQLiteUniverseProvider) BuildUniverse(date string, def marketsnapshot.UniverseDefinition) ([]marketsnapshot.UniverseMember, error) {
	// Step 1: 取所有在 stockinfo 中的股票，叠加 ipo/delist/ST 信息
	rows, err := p.db.DB().Query(`SELECT code, name, COALESCE(exchange,''), COALESCE(ipo_date_txt,''), COALESCE(delist_date,''), COALESCE(st_flag,0)
		FROM stockinfo ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type rawRow struct {
		code, name, market, ipo, delist string
		stFlag                          int
	}
	var raw []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.code, &r.name, &r.market, &r.ipo, &r.delist, &r.stFlag); err != nil {
			return nil, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Step 2: 拉 security_status_history 里当天有效的状态覆盖
	statusMap := map[string]string{}
	srows, err := p.db.DB().Query(`SELECT code, status FROM security_status_history
		WHERE effective_from <= ? AND (effective_to = '' OR effective_to >= ?)`, date, date)
	if err != nil {
		// 老库可能没有 status 历史，忽略该表
	} else {
		defer srows.Close()
		for srows.Next() {
			var c, st string
			if err := srows.Scan(&c, &st); err != nil {
				return nil, err
			}
			statusMap[c] = st
		}
	}
	out := make([]marketsnapshot.UniverseMember, 0, len(raw))
	dDate, _ := time.Parse("2006-01-02", date)
	for _, r := range raw {
		mem := marketsnapshot.UniverseMember{
			Code:       r.code,
			Name:       r.name,
			Market:     r.market,
			IpoDate:    r.ipo,
			DelistDate: r.delist,
			Status:     "normal",
			Selected:   true,
		}
		if st, ok := statusMap[r.code]; ok && st != "" {
			mem.Status = st
		} else if r.stFlag > 0 {
			mem.Status = "st"
		}
		// board 简单推导（非精确，够用）
		switch {
		case strings.HasPrefix(r.code, "300") || strings.HasPrefix(r.code, "301"):
			mem.Board = "gem"
		case strings.HasPrefix(r.code, "688"):
			mem.Board = "star"
		case strings.HasPrefix(r.code, "8") || strings.HasPrefix(r.code, "4"):
			mem.Board = "beijing"
		default:
			mem.Board = "main"
		}
		// 过滤规则
		if def.ExcludeDelisted && mem.Status == "delisted" {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "delisted")
		}
		if mem.DelistDate != "" && mem.DelistDate <= date {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "delisted<=date")
		}
		if def.ExcludeST && (mem.Status == "st" || mem.Status == "*st") {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "st")
		}
		if def.ExcludeSuspended && mem.Status == "suspended" {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "suspended")
		}
		if def.MinIpoDays > 0 && mem.IpoDate != "" && !dDate.IsZero() {
			ipoT, err := time.Parse("2006-01-02", mem.IpoDate)
			if err == nil {
				days := int(dDate.Sub(ipoT).Hours() / 24)
				if days < def.MinIpoDays {
					mem.Selected = false
					mem.ExcludeReasons = append(mem.ExcludeReasons, fmt.Sprintf("ipo_days=%d<%d", days, def.MinIpoDays))
				}
			}
		}
		if def.Board != "" && mem.Board != def.Board {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "board="+mem.Board)
		}
		out = append(out, mem)
	}
	return out, nil
}

// SQLiteWatermarkProvider 从 kline_sync_state + quote/finance_snapshot + xdxr_event 查表聚合。
type SQLiteWatermarkProvider struct{ db *storage.Storage }

func NewSQLiteWatermarkProvider(db *storage.Storage) *SQLiteWatermarkProvider {
	return &SQLiteWatermarkProvider{db: db}
}

func (p *SQLiteWatermarkProvider) FetchWatermarks(date string, codes []string) (map[string]marketsnapshot.CodeStatus, error) {
	result := map[string]marketsnapshot.CodeStatus{}
	if len(codes) == 0 {
		return result, nil
	}
	ktype := 9 // 通达信日线=9，对齐现有 SyncDailyKlines 默认
	qMarks := strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",")
	args := make([]any, 0, len(codes))
	for _, c := range codes {
		args = append(args, c)
	}
	// kline_sync_state
	q := fmt.Sprintf(`SELECT code, COALESCE(last_date,''), COALESCE(row_count,0), COALESCE(status,''), COALESCE(error,'')
		FROM kline_sync_state WHERE ktype = ? AND code IN (%s)`, qMarks)
	kArgs := append([]any{ktype}, args...)
	rows, err := p.db.DB().Query(q, kArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			c, last, status, lerr string
			rc                    int
		)
		if err := rows.Scan(&c, &last, &rc, &status, &lerr); err != nil {
			return nil, err
		}
		cs := marketsnapshot.CodeStatus{Code: c, KlineLastDate: last, KlineRowCount: rc, LastError: lerr}
		if last != "" && last < date {
			// 粗略 gap：计算 last_date 到 date 的日差（工作日近似，只用于排序）
			cs.GapDays = dayDiff(last, date)
		}
		result[c] = cs
	}
	// quote_snapshot: 只要有行就算 ready（用于今日行情覆盖）
	q2 := fmt.Sprintf(`SELECT code FROM quote_snapshot WHERE code IN (%s)`, qMarks)
	r2, err := p.db.DB().Query(q2, args...)
	if err == nil {
		defer r2.Close()
		for r2.Next() {
			var c string
			if err := r2.Scan(&c); err != nil {
				return nil, err
			}
			cs := result[c]
			cs.Code = c
			cs.QuoteReady = true
			result[c] = cs
		}
	}
	// finance_snapshot
	q3 := fmt.Sprintf(`SELECT code FROM finance_snapshot WHERE code IN (%s)`, qMarks)
	r3, err := p.db.DB().Query(q3, args...)
	if err == nil {
		defer r3.Close()
		for r3.Next() {
			var c string
			if err := r3.Scan(&c); err != nil {
				return nil, err
			}
			cs := result[c]
			cs.Code = c
			cs.FinanceReady = true
			result[c] = cs
		}
	}
	// xdxr_event: 当天或之前至少有过一次，即 xdxr_ok
	q4 := fmt.Sprintf(`SELECT DISTINCT code FROM xdxr_event WHERE date <= ? AND code IN (%s)`, qMarks)
	xArgs := append([]any{date}, args...)
	r4, err := p.db.DB().Query(q4, xArgs...)
	if err == nil {
		defer r4.Close()
		for r4.Next() {
			var c string
			if err := r4.Scan(&c); err != nil {
				return nil, err
			}
			cs := result[c]
			cs.Code = c
			cs.XdxrReady = true
			result[c] = cs
		}
	}
	return result, nil
}

// SQLiteTradingCalendar 基于 workday 表实现 TradingCalendar。
type SQLiteTradingCalendar struct{ db *storage.Storage }

func NewSQLiteTradingCalendar(db *storage.Storage) *SQLiteTradingCalendar {
	return &SQLiteTradingCalendar{db: db}
}

func (c *SQLiteTradingCalendar) unixOf(date string) (int64, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local).Unix(), nil
}

func (c *SQLiteTradingCalendar) IsTradingDay(date string) (bool, error) {
	u, err := c.unixOf(date)
	if err != nil {
		return false, err
	}
	var cnt int
	err = c.db.DB().QueryRow(`SELECT COUNT(*) FROM workday WHERE unix = ?`, u).Scan(&cnt)
	return cnt > 0, err
}

func (c *SQLiteTradingCalendar) PrevTradingDay(date string) (string, error) {
	u, err := c.unixOf(date)
	if err != nil {
		return "", err
	}
	var unix int64
	err = c.db.DB().QueryRow(`SELECT unix FROM workday WHERE unix < ? ORDER BY unix DESC LIMIT 1`, u).Scan(&unix)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no prev trading day for %s", date)
	}
	if err != nil {
		return "", err
	}
	return time.Unix(unix, 0).Format("2006-01-02"), nil
}

// SQLiteFeatureEngine 使用 methods 包的指标计算逻辑，直接在 kline 表上计算 DSL 需要的指标。
type SQLiteFeatureEngine struct{ db *storage.Storage }

func NewSQLiteFeatureEngine(db *storage.Storage) *SQLiteFeatureEngine {
	return &SQLiteFeatureEngine{db: db}
}

// Compute 对 {date, codes, features} 批量计算值。
// 实现策略：对每只 code，读取截至 date 的 250 条日线，用 methods.EvalValue 计算每支指标。
// 这是 deterministic 的（只要 kline 不变结果不变），满足特征快照哈希。
func (e *SQLiteFeatureEngine) Compute(date string, codes []string, features []marketsnapshot.FeatureSpec) (map[string]map[string]float64, error) {
	out := map[string]map[string]float64{}
	for _, code := range codes {
		// 读取 250 行前复权日线；若不足就取所有
		rows, err := e.db.DB().Query(`SELECT date, open, high, low, close, volume, amount
			FROM kline WHERE code = ? AND ktype = 9 AND date <= ? ORDER BY date DESC LIMIT 260`, code, date)
		if err != nil {
			return nil, err
		}
		var bars []methods.Bar
		for rows.Next() {
			var b methods.Bar
			if err := rows.Scan(&b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.Amount); err != nil {
				rows.Close()
				return nil, err
			}
			bars = append(bars, b)
		}
		rows.Close()
		// reverse: oldest first
		for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
			bars[i], bars[j] = bars[j], bars[i]
		}
		if len(bars) == 0 {
			continue
		}
		perCode := map[string]float64{}
		// 构造 env，用 methods 内部的 evalValue 函数计算
		// 由于 evalValue 是 unexported，用显式 switch 覆盖内建指标：
		for _, f := range features {
			v, ok := computeBuiltinIndicator(f.Name, bars)
			if ok {
				perCode[f.Name] = v
			}
		}
		out[code] = perCode
	}
	return out, nil
}

// computeBuiltinIndicator 封装 methods.Indicator 的内建实现（与 executor.go 保持一致）。
// 这样保持方法 DSL 与离线特征物化使用同一套算法，保证研究-交易一致性。
func computeBuiltinIndicator(name string, bars []methods.Bar) (float64, bool) {
	if len(bars) == 0 {
		return 0, false
	}
	last := bars[len(bars)-1]
	switch strings.ToLower(name) {
	case "close":
		return last.Close, true
	case "open":
		return last.Open, true
	case "high":
		return last.High, true
	case "low":
		return last.Low, true
	case "volume":
		return last.Volume, true
	case "amount":
		return last.Amount, true
	}
	// maN / rsiN
	switch {
	case strings.HasPrefix(name, "ma"):
		n, err := strconv.Atoi(strings.TrimPrefix(name, "ma"))
		if err != nil || n <= 0 {
			return 0, false
		}
		if len(bars) < n {
			return 0, false
		}
		sum := 0.0
		for i := len(bars) - n; i < len(bars); i++ {
			sum += bars[i].Close
		}
		return sum / float64(n), true
	case strings.HasPrefix(name, "rsi"):
		n, err := strconv.Atoi(strings.TrimPrefix(name, "rsi"))
		if err != nil || n <= 0 {
			return 0, false
		}
		if len(bars) < n+1 {
			return 0, false
		}
		gains, losses, count := 0.0, 0.0, 0
		for i := len(bars) - n; i < len(bars); i++ {
			diff := bars[i].Close - bars[i-1].Close
			if diff > 0 {
				gains += diff
			} else {
				losses += -diff
			}
			count++
		}
		if count == 0 || (gains == 0 && losses == 0) {
			return 50, true
		}
		avgG := gains / float64(count)
		avgL := losses / float64(count)
		if avgL == 0 {
			return 100, true
		}
		rs := avgG / avgL
		return 100 - 100/(1+rs), true
	}
	return 0, false
}

func dayDiff(a, b string) int {
	ta, err := time.Parse("2006-01-02", a)
	if err != nil {
		return 0
	}
	tb, err := time.Parse("2006-01-02", b)
	if err != nil {
		return 0
	}
	d := int(tb.Sub(ta).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}
