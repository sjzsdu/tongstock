package statesnapshot

import (
	"context"
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
		if !t.Dim.Valid() || !t.To.IsTriState() || len(t.From) == 0 || t.WithinDays < 1 || t.WithinDays > 365 {
			return fmt.Errorf("transition 需要合法 dim/from/to，within_days 为 1–365 个自然日")
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
		fromJSON, _ := json.Marshal(t.From)
		query += `EXISTS(SELECT 1 FROM signal_state_daily e JOIN signal_state_daily p ON p.code=e.code AND p.ktype=e.ktype AND p.dim=e.dim AND p.trade_date=(SELECT MAX(trade_date) FROM signal_state_daily WHERE code=e.code AND ktype=e.ktype AND dim=e.dim AND trade_date<e.trade_date)
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
		var transition bool
		var cross, osc, boll, ma *string
		if err = rows.Scan(&item.Code, &item.Name, &item.Date, &dims, &bad, &cross, &osc, &boll, &ma, &item.Close, &item.PrevClose, &transition); err != nil {
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
		match := transition
		for _, c := range f.Conditions {
			if item.States[c.Dim] != c.State {
				match = false
			}
		}
		if match {
			item.Date = parsed.Format("2006-01-02")
			out.Results = append(out.Results, item)
		}
	}
	out.MissingCount = len(codes) - found
	return out, rows.Err()
}
