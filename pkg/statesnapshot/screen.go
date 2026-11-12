package statesnapshot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

type Condition struct {
	Dim   signalstate.Dim   `json:"dim"`
	State signalstate.State `json:"state"`
}
type Transition struct {
	Dim        signalstate.Dim     `json:"dim"`
	From       []signalstate.State `json:"from"`
	To         signalstate.State   `json:"to"`
	WithinDays int                 `json:"within_days"`
	// Unit 为窗口单位：natural（默认，自然日）或 trading（交易日）。
	// 旧调用方不传时保持自然日，避免改变既有口径。
	Unit string `json:"within_unit,omitempty"`
}
type StateFilter struct {
	Conditions []Condition `json:"conditions"`
	Transition *Transition `json:"transition,omitempty"`
}

func (f StateFilter) Validate() error {
	seen := map[signalstate.Dim]bool{}
	for _, c := range f.Conditions {
		if !c.Dim.Valid() || !c.State.IsTriState() || seen[c.Dim] {
			return fmt.Errorf("conditions 必须为不重复的维度与三态条件")
		}
		seen[c.Dim] = true
	}
	if t := f.Transition; t != nil {
		unit := t.Unit
		if unit == "" {
			unit = "natural"
		}
		if unit != "natural" && unit != "trading" {
			return fmt.Errorf("transition.within_unit 只能是 natural 或 trading")
		}
		if !t.Dim.Valid() || !t.To.IsTriState() || len(t.From) == 0 || t.WithinDays < 1 || t.WithinDays > 365 {
			return fmt.Errorf("transition 需要合法 dim/from/to，within_days 为 1–365 个自然日或交易日")
		}
		for _, v := range t.From {
			if !v.IsTriState() || v == t.To {
				return fmt.Errorf("迁移起点须为与终点不同的三态")
			}
		}
	}
	return nil
}

type ScreenItem struct {
	Name      string                                `json:"name"`
	Code      string                                `json:"code"`
	Date      string                                `json:"date"`
	States    map[signalstate.Dim]signalstate.State `json:"states"`
	Close     *float64                              `json:"close"`
	PrevClose *float64                              `json:"prev_close"`
	// TransitionDate 是命中迁移条件时，窗口内最近一次迁移发生的交易日。
	TransitionDate string `json:"transition_date,omitempty"`
}

// SkippedStock 是股票池里没有参与本次筛选的股票及其原因。
type SkippedStock struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// 未参与筛选的三种原因：客观无行情、历史数据不足、有行情但当日状态未生成。
const (
	SkipNoQuote             = "no_quote"
	SkipInsufficientHistory = "insufficient_history"
	SkipNotComputed         = "not_computed"
)

// Coverage 描述本次筛选对股票池的覆盖情况，供前端区分「客观无行情」「待补齐」「没有该日状态」。
type Coverage struct {
	FirstDate       string         `json:"first_date"`
	LastDate        string         `json:"last_date"`
	Covered         int            `json:"covered"`
	Total           int            `json:"total"`
	TradeDateExists bool           `json:"trade_date_exists"`
	Skipped         []SkippedStock `json:"skipped"`
}

type ScreenOutput struct {
	Results           []ScreenItem
	InsufficientCount int
	MissingCount      int
	Date              string
	Coverage          Coverage
}

// Screen reads one shared snapshot date, never each stock's latest bar. A
// transition means adjacent observations changed inside the inclusive calendar
// window [date-within_days+1, date]. The preceding observation may predate it.
func (s Store) Screen(ctx context.Context, codes []string, date string, f StateFilter) (ScreenOutput, error) {
	codes = uniqueCodes(codes)
	out := ScreenOutput{Results: []ScreenItem{}, Date: date, Coverage: Coverage{Total: len(codes), Skipped: []SkippedStock{}}}
	if err := f.Validate(); err != nil {
		return out, err
	}
	// 可用状态日期范围与「该日是否有任何状态行」用来区分「没有这一天」和「这天还没算」。
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MIN(trade_date),''), COALESCE(MAX(trade_date),'') FROM signal_state_daily WHERE ktype=?`, Day).Scan(&out.Coverage.FirstDate, &out.Coverage.LastDate); err != nil {
		return out, err
	}
	if date == "" {
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(trade_date),'') FROM signal_state_daily WHERE ktype=?`, Day).Scan(&date); err != nil {
			return out, err
		}
		out.Date = date
	}
	if date == "" {
		out.MissingCount = len(codes)
		return out, nil
	}
	parsed, err := time.Parse("20060102", date)
	if err != nil {
		return out, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM signal_state_daily WHERE ktype=? AND trade_date=?)`, Day, date).Scan(&out.Coverage.TradeDateExists); err != nil {
		return out, err
	}
	if !out.Coverage.TradeDateExists {
		// 非交易日、未来日或该日整体未生成：不逐只归类，交给前端走「这一天没有状态」的空态。
		out.MissingCount = len(codes)
		return out, nil
	}
	// json_each keeps parameter count bounded for full-market pools.
	pool, err := json.Marshal(codes)
	if err != nil {
		return out, err
	}
	args := []any{date, Day, string(pool)}
	cols := []string{}
	for _, dim := range signalstate.Dims() {
		cols = append(cols, fmt.Sprintf("MAX(CASE WHEN dim='%s' THEN state END) AS %s", dim, dim))
	}
	query := `WITH vectors AS (SELECT code,trade_date,COUNT(*) AS dims,MAX(insufficient) AS bad,` + strings.Join(cols, ",") + ` FROM signal_state_daily WHERE trade_date=? AND ktype=? AND code IN (SELECT value FROM json_each(?)) GROUP BY code,trade_date)
 SELECT v.code,COALESCE((SELECT name FROM stockinfo WHERE code=v.code),''),v.trade_date,v.dims,v.bad,v.cross,v.osc,v.boll,v.ma,
 (SELECT close FROM kline WHERE code=v.code AND ktype=9 AND date=v.trade_date),
 (SELECT close FROM kline WHERE code=v.code AND ktype=9 AND date<v.trade_date ORDER BY date DESC LIMIT 1),`
	if t := f.Transition; t != nil {
		start := parsed.AddDate(0, 0, 1-t.WithinDays).Format("20060102")
		if t.Unit == "trading" {
			// 「近 N 个交易日」按快照库里实际存在的交易日回数，而不是自然日。
			start, err = s.tradingWindowStart(ctx, date, t.WithinDays)
			if err != nil {
				return out, err
			}
		}
		fromJSON, _ := json.Marshal(t.From)
		// 标量形式同时给出「是否命中」和「最近一次迁移日期」；无迁移时为 NULL。
		query += `(SELECT MAX(e.trade_date) FROM signal_state_daily e JOIN signal_state_daily p ON p.code=e.code AND p.ktype=e.ktype AND p.dim=e.dim AND p.trade_date=(SELECT MAX(trade_date) FROM signal_state_daily WHERE code=e.code AND ktype=e.ktype AND dim=e.dim AND trade_date<e.trade_date)
   WHERE e.code=v.code AND e.ktype=9 AND e.dim=? AND e.trade_date BETWEEN ? AND v.trade_date AND e.state=? AND e.insufficient=0 AND p.insufficient=0 AND p.state IN (SELECT value FROM json_each(?)))`
		args = append(args, t.Dim, start, t.To, string(fromJSON))
	} else {
		query += `1`
	}
	query += ` FROM vectors v ORDER BY v.code`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	found := 0
	seen := make(map[string]struct{}, len(codes))
	for rows.Next() {
		var item ScreenItem
		var dims, bad int
		var transitionDate sql.NullString
		var cross, osc, boll, ma *string
		if err = rows.Scan(&item.Code, &item.Name, &item.Date, &dims, &bad, &cross, &osc, &boll, &ma, &item.Close, &item.PrevClose, &transitionDate); err != nil {
			return out, err
		}
		found++
		seen[item.Code] = struct{}{}
		item.States = map[signalstate.Dim]signalstate.State{}
		for i, d := range signalstate.Dims() {
			p := []*string{cross, osc, boll, ma}[i]
			if p != nil {
				item.States[d] = signalstate.State(*p)
			}
			if !item.States[d].IsTriState() {
				bad = 1
			}
		}
		if bad != 0 || dims != 4 {
			out.InsufficientCount++
			out.Coverage.Skipped = append(out.Coverage.Skipped, SkippedStock{Code: item.Code, Name: item.Name, Reason: SkipInsufficientHistory})
			continue
		}
		match := transitionDate.Valid
		for _, c := range f.Conditions {
			if item.States[c.Dim] != c.State {
				match = false
			}
		}
		if match {
			item.Date = parsed.Format("2006-01-02")
			if transitionDate.Valid {
				item.TransitionDate = formatCompareDate(transitionDate.String)
			}
			out.Results = append(out.Results, item)
		}
	}
	// 单连接池：先把结果集读完并关闭，才能发下一条查询。
	iterErr := rows.Err()
	if closeErr := rows.Close(); iterErr == nil {
		iterErr = closeErr
	}
	if iterErr != nil {
		return out, iterErr
	}
	missing := make([]string, 0, len(codes)-found)
	for _, code := range codes {
		if _, ok := seen[code]; !ok {
			missing = append(missing, code)
		}
	}
	unquoted, err := s.classifyMissing(ctx, missing, date)
	if err != nil {
		return out, err
	}
	out.Coverage.Skipped = append(out.Coverage.Skipped, unquoted...)
	sort.Slice(out.Coverage.Skipped, func(i, j int) bool { return out.Coverage.Skipped[i].Code < out.Coverage.Skipped[j].Code })
	out.Coverage.Covered = len(codes) - len(out.Coverage.Skipped)
	out.MissingCount = len(codes) - found
	return out, nil
}

// classifyMissing 给「当日没有状态行」的股票归类：当日有无成交行情决定是待补齐还是客观无行情。
func (s Store) classifyMissing(ctx context.Context, codes []string, date string) ([]SkippedStock, error) {
	if len(codes) == 0 {
		return []SkippedStock{}, nil
	}
	pool, err := json.Marshal(codes)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.value, COALESCE(si.name,''), CASE WHEN k.code IS NULL THEN 0 ELSE 1 END
 FROM json_each(?) p
 LEFT JOIN stockinfo si ON si.code = p.value
 LEFT JOIN kline k ON k.code = p.value AND k.ktype = ? AND k.date = ?
 ORDER BY p.value`, string(pool), Day, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skipped := make([]SkippedStock, 0, len(codes))
	for rows.Next() {
		var item SkippedStock
		var hasBar int
		if err := rows.Scan(&item.Code, &item.Name, &hasBar); err != nil {
			return nil, err
		}
		item.Reason = SkipNoQuote
		if hasBar != 0 {
			item.Reason = SkipNotComputed
		}
		skipped = append(skipped, item)
	}
	return skipped, rows.Err()
}

func uniqueCodes(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

// tradingWindowStart 返回以 date 为终点、向前数 within 个「有快照的交易日」的起始日。
// 不足 within 天时返回现有最早的一天；完全没有快照时返回 date 本身（不会有迁移命中）。
func (s Store) tradingWindowStart(ctx context.Context, date string, within int) (string, error) {
	if within < 1 {
		within = 1
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT trade_date FROM (
SELECT DISTINCT trade_date FROM signal_state_daily WHERE ktype=? AND trade_date<=? ORDER BY trade_date DESC LIMIT ?
) ORDER BY trade_date ASC LIMIT 1`, Day, date, within)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var start string
		if err := rows.Scan(&start); err != nil {
			return "", err
		}
		return start, nil
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return date, nil
}
