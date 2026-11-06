package marketsnapshotrepo

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	required := make(map[string]bool, len(def.RequiredCodes))
	for _, code := range def.RequiredCodes {
		required[strings.TrimSpace(code)] = true
	}
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
		if len(required) > 0 && !required[r.code] {
			mem.Selected = false
			mem.ExcludeReasons = append(mem.ExcludeReasons, "not_required")
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
	found := map[string]bool{}
	for rows.Next() {
		var (
			c, last, status, lerr string
			rc                    int
		)
		if err := rows.Scan(&c, &last, &rc, &status, &lerr); err != nil {
			return nil, err
		}
		cs := marketsnapshot.CodeStatus{Code: c, KlineLastDate: last, KlineRowCount: rc, LastError: lerr}
		found[c] = true
		if last != "" && last < date {
			// 粗略 gap：计算 last_date 到 date 的日差（工作日近似，只用于排序）
			cs.GapDays = dayDiff(last, date)
		}
		result[c] = cs
	}
	// Older real databases can contain complete kline history without the newer
	// sync-state table being backfilled. Derive the watermark from the source
	// rows themselves; never invent readiness from a configured date.
	compactDate := strings.ReplaceAll(date, "-", "")
	for _, code := range codes {
		var last string
		var count int
		err := p.db.DB().QueryRow(`SELECT COALESCE(MAX(date),''), COUNT(*) FROM kline_latest WHERE code=? AND ktype=? AND date<=?`, code, ktype, compactDate).Scan(&last, &count)
		if err != nil {
			return nil, err
		}
		if last != "" {
			formatted := last
			if len(last) == 8 {
				formatted = last[:4] + "-" + last[4:6] + "-" + last[6:]
			}
			cs := result[code]
			cs.Code, cs.KlineLastDate, cs.KlineRowCount = code, formatted, count
			if formatted < date {
				cs.GapDays = dayDiff(formatted, date)
			}
			result[code] = cs
		} else if !found[code] {
			delete(result, code)
		}
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
	if err != nil || cnt > 0 {
		return cnt > 0, err
	}
	// Fail on absence of actual market rows, but allow legacy databases whose
	// workday table has not yet been materialized.
	err = c.db.DB().QueryRow(`SELECT COUNT(*) FROM kline_latest WHERE ktype=9 AND date=?`, strings.ReplaceAll(date, "-", "")).Scan(&cnt)
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
// scope 类特征（Category=="scope"）没有 point-in-time 序列，直接从 stockinfo 当前值读取。
func (e *SQLiteFeatureEngine) Compute(date string, codes []string, features []marketsnapshot.FeatureSpec) (map[string]map[string]float64, error) {
	out := map[string]map[string]float64{}
	queryDate := strings.ReplaceAll(date, "-", "")

	// 分离 scope 特征（从 stockinfo 取）与 K 线派生特征（计算）。
	scopeCols := map[string]string{
		"market_cap":       "market_cap",
		"total_market_cap": "total_market_cap",
		"turnover_rate":    "turnover_rate",
		"float_shares":     "liu_tong_gu_ben",
	}
	var scopeFeatures []marketsnapshot.FeatureSpec
	var klineFeatures []marketsnapshot.FeatureSpec
	for _, f := range features {
		if f.Category == "scope" {
			scopeFeatures = append(scopeFeatures, f)
		} else {
			klineFeatures = append(klineFeatures, f)
		}
	}

	// 批量加载 scope 特征值（一次查询，避免逐 code）。
	scopeValues := map[string]map[string]float64{}
	if len(scopeFeatures) > 0 {
		var cols []string
		var names []string
		for _, f := range scopeFeatures {
			if col, ok := scopeCols[f.Name]; ok {
				cols = append(cols, col)
				names = append(names, f.Name)
			}
		}
		if len(cols) > 0 {
			rows, err := e.db.DB().Query(`SELECT code, ` + strings.Join(cols, ", ") + ` FROM stockinfo`)
			if err != nil {
				return nil, fmt.Errorf("load scope features: %w", err)
			}
			for rows.Next() {
				vals := make([]float64, len(cols))
				args := make([]any, len(cols))
				for i := range vals {
					args[i] = &vals[i]
				}
				var code string
				scanArgs := append([]any{&code}, args...)
				if err := rows.Scan(scanArgs...); err != nil {
					rows.Close()
					return nil, err
				}
				perCode := map[string]float64{}
				for i, name := range names {
					if vals[i] != 0 {
						perCode[name] = vals[i]
					}
				}
				scopeValues[code] = perCode
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			rows.Close()
		}
	}

	// 户均持股 = 流通股本(万股)×10000 / 股东户数（finance_snapshot.GuDongRenShu），
	// 单位 股/户。股东户数来自 TDX 财务协议快照，无 point-in-time 序列，
	// 与其他 scope 特征一样只用于最新快照筛选。
	needHolders := false
	for _, f := range scopeFeatures {
		if f.Name == "avg_shares_per_holder" {
			needHolders = true
			break
		}
	}
	if needHolders {
		holders := map[string]float64{}
		rows, err := e.db.DB().Query(`SELECT code, payload FROM finance_snapshot`)
		if err == nil {
			for rows.Next() {
				var c, raw string
				if rows.Scan(&c, &raw) != nil {
					continue
				}
				var payload struct {
					GuDongRenShu float64 `json:"GuDongRenShu"`
				}
				if json.Unmarshal([]byte(raw), &payload) == nil && payload.GuDongRenShu > 0 {
					holders[c] = payload.GuDongRenShu
				}
			}
			rows.Close()
		} else {
			return nil, fmt.Errorf("load holders for avg_shares_per_holder: %w", err)
		}
		shares := map[string]float64{}
		rows2, err := e.db.DB().Query(`SELECT code, liu_tong_gu_ben FROM stockinfo WHERE liu_tong_gu_ben > 0`)
		if err != nil {
			return nil, fmt.Errorf("load float shares for avg_shares_per_holder: %w", err)
		}
		for rows2.Next() {
			var c string
			var v float64
			if rows2.Scan(&c, &v) == nil {
				shares[c] = v
			}
		}
		rows2.Close()
		if err := rows2.Err(); err != nil {
			return nil, err
		}
		for c, sh := range shares {
			if h := holders[c]; h > 0 {
				if scopeValues[c] == nil {
					scopeValues[c] = map[string]float64{}
				}
				scopeValues[c]["avg_shares_per_holder"] = sh * 10000 / h
			}
		}
	}

	// avg_turnover：近 120 根日线的平均换手率（%，流通市值口径）。
	// 口径与验证侧 LiveSource 逐 bar 换手率一致（amount 百元 → 成交额元），
	// 但窗口为近 120 日，近似验证侧的约 4 年窗口平均（已诚实标注在 FeatureSpec）。
	// 公式：逐日 turnover% = (amount/close)/liu_tong_gu_ben(万股)。
	needAvgTurnover := false
	for _, f := range scopeFeatures {
		if f.Name == "avg_turnover" {
			needAvgTurnover = true
			break
		}
	}
	if needAvgTurnover {
		// 单条 SQL 用窗口函数取每只股票最近 120 根日线并聚合 AVG(amount/close)，
		// 避免 5000+ 次逐 code 查询。amount 为百元、close 为元。
		rows, err := e.db.DB().Query(`SELECT code, AVG(amount * 1.0 / close) FROM (
				SELECT code, amount, close,
					ROW_NUMBER() OVER (PARTITION BY code ORDER BY date DESC) AS rn
				FROM kline_latest WHERE ktype = 9 AND date <= ? AND close > 0 AND amount > 0
			) WHERE rn <= 120 GROUP BY code`, queryDate)
		if err != nil {
			return nil, fmt.Errorf("load avg turnover: %w", err)
		}
		acRatio := map[string]float64{} // code → AVG(amount/close)（百元/元）
		for rows.Next() {
			var c string
			var v float64
			if rows.Scan(&c, &v) == nil && v > 0 {
				acRatio[c] = v
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		// 流通股本（万股）批量加载。
		sharesWan := map[string]float64{}
		rows3, err := e.db.DB().Query(`SELECT code, liu_tong_gu_ben FROM stockinfo WHERE liu_tong_gu_ben > 0`)
		if err != nil {
			return nil, fmt.Errorf("load float shares for avg_turnover: %w", err)
		}
		for rows3.Next() {
			var c string
			var v float64
			if rows3.Scan(&c, &v) == nil {
				sharesWan[c] = v
			}
		}
		rows3.Close()
		if err := rows3.Err(); err != nil {
			return nil, err
		}
		for c, ratio := range acRatio {
			if wan := sharesWan[c]; wan > 0 {
				if scopeValues[c] == nil {
					scopeValues[c] = map[string]float64{}
				}
				scopeValues[c]["avg_turnover"] = ratio / wan
			}
		}
	}

	for _, code := range codes {
		perCode := map[string]float64{}
		for name, v := range scopeValues[code] {
			perCode[name] = v
		}
		// K 线派生特征需要真实日线。
		if len(klineFeatures) > 0 {
			rows, err := e.db.DB().Query(`SELECT date, open, high, low, close, volume, amount
				FROM kline_latest WHERE code = ? AND ktype = 9 AND date <= ? ORDER BY date DESC LIMIT 260`, code, queryDate)
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
			if len(bars) > 0 {
				for _, f := range klineFeatures {
					v, ok := computeBuiltinIndicator(f.Name, bars)
					if ok {
						perCode[f.Name] = v
					}
				}
			}
		}
		out[code] = perCode
	}
	return out, nil
}

// computeBuiltinIndicator 委托给 methods 包的唯一实现，保证离线物化与在线执行
// 使用完全相同的算法（研究-交易一致性）。任何新指标只需在 methods 里加一次。
func computeBuiltinIndicator(name string, bars []methods.Bar) (float64, bool) {
	return methods.ComputeBuiltinIndicator(name, bars)
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

// NewStockNameReader 返回 code→name 批量补全函数（读 stockinfo 当前值）。
// 用于方法股票池等展示场景：市场快照持久化不携带 name，重载后需要补全。
// 未命中的 code 不出现在返回 map 中。
func NewStockNameReader(db *storage.Storage) func(codes []string) map[string]string {
	return func(codes []string) map[string]string {
		if len(codes) == 0 {
			return nil
		}
		marks := make([]string, len(codes))
		args := make([]any, len(codes))
		for i, c := range codes {
			marks[i] = "?"
			args[i] = c
		}
		rows, err := db.DB().Query(
			`SELECT code, name FROM stockinfo WHERE code IN (`+strings.Join(marks, ",")+`)`, args...)
		if err != nil {
			return nil
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var code, name string
			if rows.Scan(&code, &name) == nil {
				out[code] = name
			}
		}
		return out
	}
}
