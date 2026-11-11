package statesnapshot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/signalstate"
)

// CompareItem is one aligned state/price observation for a stock.
type CompareItem struct {
	Date   string                                `json:"date"`
	Close  *float64                              `json:"close"`
	States map[signalstate.Dim]signalstate.State `json:"states"`
}

type CompareTransition struct {
	Dim  signalstate.Dim   `json:"dim"`
	Date string            `json:"date"`
	From signalstate.State `json:"from"`
	To   signalstate.State `json:"to"`
}

// CompareDuration describes one contiguous, valid state run. ReturnPct is
// computed from the first and last close in the run so it can be reproduced
// directly from kline_latest with (end_close/start_close-1)*100.
type CompareDuration struct {
	Dim         signalstate.Dim   `json:"dim"`
	State       signalstate.State `json:"state"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	TradingDays int               `json:"trading_days"`
	StartClose  *float64          `json:"start_close"`
	EndClose    *float64          `json:"end_close"`
	ReturnPct   *float64          `json:"return_pct"`
}

type CompareStock struct {
	Code        string              `json:"code"`
	Name        string              `json:"name"`
	Items       []CompareItem       `json:"items"`
	Transitions []CompareTransition `json:"transitions"`
	Durations   []CompareDuration   `json:"durations"`
}

type CompareOutput struct {
	From   string         `json:"from"`
	To     string         `json:"to"`
	Stocks []CompareStock `json:"stocks"`
}

type compareObservation struct {
	date   string
	close  *float64
	states map[signalstate.Dim]signalstate.State
}

func (s Store) Compare(ctx context.Context, codes []string, from, to string) (CompareOutput, error) {
	out := CompareOutput{From: from, To: to, Stocks: []CompareStock{}}
	if len(codes) == 0 {
		return out, fmt.Errorf("codes is required")
	}
	pool, err := json.Marshal(codes)
	if err != nil {
		return out, err
	}
	// kline_latest 是「kline LEFT JOIN kline_revision + 相关子查询」的视图。
	// 直接 JOIN 这个视图会让 SQLite 先把全表 kline 物化一遍，2 只股票 6 天也要 70s+。
	// 这里只对已命中的状态行做 kline/revision 的点查，语义与 kline_latest 完全一致。
	query := `SELECT s.code,s.trade_date,s.dim,s.state,s.insufficient,COALESCE(r.close,k.close),
COALESCE((SELECT name FROM stockinfo WHERE code=s.code),'')
FROM signal_state_daily s
LEFT JOIN kline k ON k.code=s.code AND k.ktype=s.ktype AND k.date=s.trade_date
LEFT JOIN kline_revision r ON r.code=s.code AND r.ktype=s.ktype AND r.date=s.trade_date
	AND r.version=(SELECT MAX(x.version) FROM kline_revision x WHERE x.code=s.code AND x.ktype=s.ktype AND x.date=s.trade_date)
WHERE s.ktype=? AND s.code IN (SELECT value FROM json_each(?)) AND s.trade_date>=? AND s.trade_date<=?
ORDER BY s.code,s.trade_date,s.dim`
	rows, err := s.DB.QueryContext(ctx, query, Day, string(pool), from, to)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	byCode := make(map[string]*CompareStock, len(codes))
	observations := make(map[string][]compareObservation, len(codes))
	for rows.Next() {
		var code, date, dim, state, name string
		var insufficient bool
		var close sql.NullFloat64
		if err := rows.Scan(&code, &date, &dim, &state, &insufficient, &close, &name); err != nil {
			return out, err
		}
		stock := byCode[code]
		if stock == nil {
			stock = &CompareStock{Code: code, Name: name, Items: []CompareItem{}, Transitions: []CompareTransition{}, Durations: []CompareDuration{}}
			byCode[code] = stock
		}
		items := observations[code]
		if len(items) == 0 || items[len(items)-1].date != date {
			obs := compareObservation{date: date, states: map[signalstate.Dim]signalstate.State{}}
			if close.Valid {
				v := close.Float64
				obs.close = &v
			}
			items = append(items, obs)
		}
		obs := &items[len(items)-1]
		value := signalstate.State(state)
		if insufficient {
			value = signalstate.StateInsufficient
		}
		obs.states[signalstate.Dim(dim)] = value
		observations[code] = items
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, code := range codes {
		stock := byCode[code]
		if stock == nil {
			continue
		}
		buildCompareFacts(stock, observations[code])
		out.Stocks = append(out.Stocks, *stock)
	}
	return out, nil
}

func buildCompareFacts(stock *CompareStock, observations []compareObservation) {
	for _, obs := range observations {
		states := make(map[signalstate.Dim]signalstate.State, len(signalstate.Dims()))
		for _, dim := range signalstate.Dims() {
			states[dim] = obs.states[dim]
		}
		stock.Items = append(stock.Items, CompareItem{Date: formatCompareDate(obs.date), Close: obs.close, States: states})
	}
	for _, dim := range signalstate.Dims() {
		var previous signalstate.State
		var run *CompareDuration
		for _, obs := range observations {
			state := obs.states[dim]
			if state == signalstate.StateInsufficient || !state.IsTriState() {
				if run != nil {
					stock.Durations = append(stock.Durations, *run)
					run = nil
				}
				previous = ""
				continue
			}
			if previous != "" && previous != state {
				stock.Transitions = append(stock.Transitions, CompareTransition{Dim: dim, Date: formatCompareDate(obs.date), From: previous, To: state})
			}
			if run == nil || run.State != state {
				if run != nil {
					stock.Durations = append(stock.Durations, *run)
				}
				run = &CompareDuration{Dim: dim, State: state, From: formatCompareDate(obs.date), To: formatCompareDate(obs.date), TradingDays: 1, StartClose: obs.close, EndClose: obs.close}
			} else {
				run.To = formatCompareDate(obs.date)
				run.TradingDays++
				run.EndClose = obs.close
			}
			if run.StartClose != nil && run.EndClose != nil && *run.StartClose != 0 {
				value := (*run.EndClose / *run.StartClose - 1) * 100
				run.ReturnPct = &value
			}
			previous = state
		}
		if run != nil {
			stock.Durations = append(stock.Durations, *run)
		}
	}
}

func formatCompareDate(raw string) string {
	if len(raw) == 8 {
		return raw[:4] + "-" + raw[4:6] + "-" + raw[6:]
	}
	return strings.TrimSpace(raw)
}
