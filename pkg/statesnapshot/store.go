// Package statesnapshot persists the causal signalstate kernel output. It never
// reads the legacy event stream and never fetches data from the network.
package statesnapshot

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mattn/go-sqlite3"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/pkg/param"
	"github.com/sjzsdu/tongstock/pkg/signalstate"
	"github.com/sjzsdu/tongstock/pkg/ta"
)

const Day uint8 = 9

type Store struct{ DB *sql.DB }
type Options struct {
	Codes    []string
	From     string // inclusive YYYYMMDD; full history is always read for indicator seeds
	Mode     string // full or incremental
	Verify   bool   // compare every persisted dimension/hash with the reference kernel
	Progress func(code string, changed int64)
}
type Result struct {
	Codes    int   `json:"codes"`
	Bars     int   `json:"bars"`
	Changed  int64 `json:"changed"`
	Verified int   `json:"verified"`
}
type Row struct {
	Code          string            `json:"code"`
	Date          string            `json:"trade_date"`
	KType         uint8             `json:"ktype"`
	Dim           signalstate.Dim   `json:"dim"`
	State         signalstate.State `json:"state"`
	Insufficient  bool              `json:"insufficient"`
	AdjustVersion string            `json:"adjust_version"`
	CalcVersion   string            `json:"calc_version"`
	CalcHash      string            `json:"calc_hash"`
}

func (s Store) Codes(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT code FROM (SELECT DISTINCT code FROM kline WHERE ktype=? UNION SELECT code FROM signal_state_seed WHERE ktype=?) ORDER BY code`, Day, Day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

func (s Store) Run(ctx context.Context, opt Options) (Result, error) {
	var out Result
	if opt.Mode != "full" && opt.Mode != "incremental" {
		return out, fmt.Errorf("mode must be full or incremental")
	}
	if opt.From != "" {
		if _, err := time.Parse("20060102", opt.From); err != nil {
			return out, fmt.Errorf("from: %w", err)
		}
	}
	codes := opt.Codes
	if len(codes) == 0 {
		var err error
		codes, err = s.Codes(ctx)
		if err != nil {
			return out, err
		}
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			continue
		}
		seen[code] = true
		var r Result
		var err error
		for attempt := 0; attempt < 8; attempt++ {
			r, err = s.runCode(ctx, code, opt)
			var locked sqlite3.Error
			if !errors.As(err, &locked) || (locked.Code != sqlite3.ErrBusy && locked.Code != sqlite3.ErrLocked) {
				break
			}
			if attempt == 7 {
				break
			}
			timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return out, ctx.Err()
			case <-timer.C:
			}
		}
		if err != nil {
			return out, fmt.Errorf("states %s: %w", code, err)
		}
		out.Codes++
		out.Bars += r.Bars
		out.Changed += r.Changed
		out.Verified += r.Verified
		if opt.Progress != nil {
			opt.Progress(code, r.Changed)
		}
	}
	return out, nil
}

func (s Store) runCode(ctx context.Context, code string, opt Options) (Result, error) {
	var out Result
	// Reading bars, checking the seed, and writing all four dimensions in the same
	// transaction prevents a concurrent sync from changing the input halfway through.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT date,open,high,low,close,volume,amount FROM kline_latest WHERE code=? AND ktype=? ORDER BY date`, code, Day)
	if err != nil {
		return out, err
	}
	var bars []ta.KlineInput
	for rows.Next() {
		var b ta.KlineInput
		var date string
		if err = rows.Scan(&date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.Amount); err != nil {
			rows.Close()
			return out, err
		}
		b.Time, err = time.Parse("20060102", date)
		if err != nil {
			b.Time, err = time.Parse("2006-01-02", date)
		}
		if err != nil {
			rows.Close()
			return out, fmt.Errorf("invalid bar date %q", date)
		}
		bars = append(bars, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	def := param.ResolveStateDefinition(code)
	definitionJSON, _ := json.Marshal(def.Normalized())
	definition := signalstate.DefinitionHash(def)
	var oldDefinition, oldHash, lastDate string
	var oldCount int
	err = tx.QueryRowContext(ctx, `SELECT definition,input_hash,bar_count,last_date FROM signal_state_seed WHERE code=? AND ktype=?`, code, Day).Scan(&oldDefinition, &oldHash, &oldCount, &lastDate)
	if err != nil && err != sql.ErrNoRows {
		return out, err
	}
	h := signalstate.NewInputHasher()
	prefix := ""
	for i, b := range bars {
		signalstate.HashInput(h, b)
		if i+1 == oldCount {
			prefix = fmt.Sprintf("%x", h.Sum(nil))
		}
	}
	inputHash := fmt.Sprintf("%x", h.Sum(nil))
	var recordedDefinition struct {
		Hash       string
		Parameters json.RawMessage
	}
	_ = json.Unmarshal([]byte(oldDefinition), &recordedDefinition)
	unchangedPrefix := recordedDefinition.Hash == definition && oldCount <= len(bars) && (oldCount == 0 || prefix == oldHash)
	rebuild := !unchangedPrefix
	// A full run explicitly recomputes the requested range. Incremental runs skip
	// unchanged complete days, including on repeated syncs or after a suspended day.
	existing := map[string]int{}
	rows, err = tx.QueryContext(ctx, `SELECT trade_date,COUNT(*) FROM signal_state_daily WHERE code=? AND ktype=? GROUP BY trade_date`, code, Day)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var date string
		var n int
		if err = rows.Scan(&date, &n); err != nil {
			rows.Close()
			return out, err
		}
		existing[date] = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	from := opt.From
	if rebuild && (opt.Mode == "incremental" || oldCount > 0) {
		from = ""
	}
	need := opt.Mode == "full" || rebuild || opt.Verify
	for _, b := range bars {
		date := b.Time.Format("20060102")
		if date >= from && existing[date] != 4 {
			need = true
		}
	}
	var vectors []signalstate.StateVector
	if need {
		start := 0
		if opt.Mode == "incremental" && !rebuild && !opt.Verify {
			start = len(bars)
			for i, b := range bars {
				if b.Time.Format("20060102") >= from && existing[b.Time.Format("20060102")] != 4 {
					start = i
					break
				}
			}
		}
		vectors = signalstate.ComputeSeriesFrom(code, Day, bars, def, start)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO signal_state_daily(code,trade_date,ktype,dim,state,insufficient,adjust_version,calc_version,calc_hash) VALUES(?,?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?,?),(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(code,trade_date,ktype,dim) DO UPDATE SET state=excluded.state,insufficient=excluded.insufficient,adjust_version=excluded.adjust_version,calc_version=excluded.calc_version,calc_hash=excluded.calc_hash
 WHERE signal_state_daily.calc_hash<>excluded.calc_hash OR signal_state_daily.state<>excluded.state OR signal_state_daily.insufficient<>excluded.insufficient OR signal_state_daily.adjust_version<>excluded.adjust_version OR signal_state_daily.calc_version<>excluded.calc_version`)
	if err != nil {
		return out, err
	}
	defer stmt.Close()
	for _, v := range vectors {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		date := v.Date.Format("20060102")
		if date < from {
			continue
		}
		out.Bars++
		if opt.Mode == "incremental" && !rebuild && existing[date] == 4 {
			continue
		}
		args := make([]any, 0, 36)
		for _, d := range signalstate.Dims() {
			args = append(args, code, date, Day, d, v.State(d), v.IsInsufficient(d), v.AdjustVersion, v.CalcVersion, v.CalcHash)
		}
		res, e := stmt.ExecContext(ctx, args...)
		if e != nil {
			return out, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return out, e
		}
		out.Changed += n
	}
	// A withdrawn bar must not leave a phantom suspended-day state behind.
	active := make(map[string]bool, len(bars))
	for _, b := range bars {
		active[b.Time.Format("20060102")] = true
	}
	for date := range existing {
		if date < from || active[date] {
			continue
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM signal_state_daily WHERE code=? AND ktype=? AND trade_date=?`, code, Day, date)
		if err != nil {
			return out, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return out, err
		}
		out.Changed += n
	}
	if opt.Verify {
		expected := map[string]signalstate.StateVector{}
		for _, v := range vectors {
			date := v.Date.Format("20060102")
			if date >= from {
				expected[date] = v
			}
		}
		check, err := readRows(ctx, tx, ` WHERE code=? AND ktype=? AND trade_date>=?`, []any{code, Day, from})
		if err != nil {
			return out, err
		}
		for _, r := range check {
			v, ok := expected[r.Date]
			if !ok || r.CalcHash != v.CalcHash || r.State != v.State(r.Dim) || r.Insufficient != v.IsInsufficient(r.Dim) || r.AdjustVersion != v.AdjustVersion || r.CalcVersion != v.CalcVersion {
				return out, fmt.Errorf("reference mismatch: %s/%s", r.Date, r.Dim)
			}
		}
		if len(check) != len(expected)*4 {
			return out, fmt.Errorf("coverage mismatch: bars=%d dimensions=%d", len(expected), len(check))
		}
		out.Verified = len(check)
	}
	last := ""
	if len(bars) > 0 {
		last = bars[len(bars)-1].Time.Format("20060102")
	}
	seed, _ := json.Marshal(struct {
		Hash       string
		Parameters json.RawMessage
	}{definition, definitionJSON})
	_, err = tx.ExecContext(ctx, `INSERT INTO signal_state_seed(code,ktype,definition,input_hash,bar_count,last_date) VALUES(?,?,?,?,?,?) ON CONFLICT(code,ktype) DO UPDATE SET definition=excluded.definition,input_hash=excluded.input_hash,bar_count=excluded.bar_count,last_date=excluded.last_date WHERE definition<>excluded.definition OR input_hash<>excluded.input_hash OR bar_count<>excluded.bar_count OR last_date<>excluded.last_date`, code, Day, string(seed), inputHash, len(bars), last)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readRows(ctx context.Context, db querier, where string, args []any) ([]Row, error) {
	rows, err := db.QueryContext(ctx, `SELECT code,trade_date,ktype,dim,state,insufficient,adjust_version,calc_version,calc_hash FROM signal_state_daily`+where+` ORDER BY code,trade_date,dim`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err = rows.Scan(&r.Code, &r.Date, &r.KType, &r.Dim, &r.State, &r.Insufficient, &r.AdjustVersion, &r.CalcVersion, &r.CalcHash); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s Store) At(ctx context.Context, date string, codes []string) ([]Row, error) {
	where := ` WHERE trade_date=? AND ktype=?`
	args := []any{date, Day}
	if len(codes) > 0 {
		where += ` AND code IN (` + strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",") + `)`
		for _, c := range codes {
			args = append(args, c)
		}
	}
	return readRows(ctx, s.DB, where, args)
}
func (s Store) Series(ctx context.Context, code, from, to string) ([]Row, error) {
	where := ` WHERE code=? AND ktype=?`
	args := []any{code, Day}
	if from != "" {
		where += ` AND trade_date>=?`
		args = append(args, from)
	}
	if to != "" {
		where += ` AND trade_date<=?`
		args = append(args, to)
	}
	return readRows(ctx, s.DB, where, args)
}
