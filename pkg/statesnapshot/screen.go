package statesnapshot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
type ScreenOutput struct {
	Results           []ScreenItem
	InsufficientCount int
	MissingCount      int
	Date              string
}

// Screen reads one shared snapshot date, never each stock's latest bar. A
// transition means adjacent observations changed inside the inclusive calendar
// window [date-within_days+1, date]. The preceding observation may predate it.
func (s Store) Screen(ctx context.Context, codes []string, date string, f StateFilter) (ScreenOutput, error) {
	out := ScreenOutput{Results: []ScreenItem{}, Date: date}
	if err := f.Validate(); err != nil {
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
	for rows.Next() {
		var item ScreenItem
		var dims, bad int
		var transitionDate sql.NullString
		var cross, osc, boll, ma *string
		if err = rows.Scan(&item.Code, &item.Name, &item.Date, &dims, &bad, &cross, &osc, &boll, &ma, &item.Close, &item.PrevClose, &transitionDate); err != nil {
			return out, err
		}
		found++
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
	out.MissingCount = len(codes) - found
	return out, rows.Err()
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
