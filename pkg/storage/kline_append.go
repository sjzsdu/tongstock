package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// KlineDataVersionKey is the data_version row bumped by every K-line revision.
// Later stages fold this value into a run's inputs_digest so a frozen result can
// name the exact K-line state it was computed from.
const KlineDataVersionKey = "kline"

// KlineBar is the value part of one K-line row. It carries no version: the
// storage layer owns versioning.
type KlineBar struct {
	Code   string
	KType  uint8
	Date   string // 20060102
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

// KlineAppendResult reports what an append-only write did.
type KlineAppendResult struct {
	Inserted bool  // the main table gained a row (version 1)
	Revised  bool  // values differed from the stored ones; a revision was appended
	Version  int64 // version now visible through kline_latest (1 = main row)
}

// AppendKline writes one bar without ever mutating the main kline table.
//
// The first write for a (code,ktype,date) key lands in kline. Any later write
// with different values is appended to kline_revision as the next version and
// bumps data_version. Re-sending identical values is a no-op, so repeated syncs
// do not inflate the revision log. The caller owns the transaction.
func AppendKline(ctx context.Context, tx *sql.Tx, bar KlineBar, reason string, now time.Time) (KlineAppendResult, error) {
	if tx == nil {
		return KlineAppendResult{}, errors.New("kline append requires a transaction")
	}
	if bar.Date == "" {
		return KlineAppendResult{}, errors.New("kline append requires a date")
	}

	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO kline
		(code, ktype, date, open, high, low, close, volume, amount)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bar.Code, bar.KType, bar.Date, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount)
	if err != nil {
		return KlineAppendResult{}, err
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return KlineAppendResult{}, err
	}
	if inserted > 0 {
		return KlineAppendResult{Inserted: true, Version: 1}, nil
	}

	stored := KlineBar{Code: bar.Code, KType: bar.KType, Date: bar.Date}
	err = tx.QueryRowContext(ctx, `SELECT open, high, low, close, volume, amount FROM kline
		WHERE code = ? AND ktype = ? AND date = ?`, bar.Code, bar.KType, bar.Date).
		Scan(&stored.Open, &stored.High, &stored.Low, &stored.Close, &stored.Volume, &stored.Amount)
	if errors.Is(err, sql.ErrNoRows) {
		// The row vanished between insert and read (concurrent repair). Retry
		// once so the caller does not see a phantom conflict.
		return AppendKline(ctx, tx, bar, reason, now)
	}
	if err != nil {
		return KlineAppendResult{}, err
	}

	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 1) FROM kline_revision
		WHERE code = ? AND ktype = ? AND date = ?`, bar.Code, bar.KType, bar.Date).Scan(&current); err != nil {
		return KlineAppendResult{}, err
	}
	if sameKlineValues(stored, bar) {
		return KlineAppendResult{Version: current}, nil
	}

	next := current + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO kline_revision
		(code, ktype, date, version, open, high, low, close, volume, amount, reason, created_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bar.Code, bar.KType, bar.Date, next, bar.Open, bar.High, bar.Low, bar.Close,
		bar.Volume, bar.Amount, reason, now.UnixNano()); err != nil {
		return KlineAppendResult{}, fmt.Errorf("append kline revision: %w", err)
	}
	if _, err := BumpDataVersion(ctx, tx, KlineDataVersionKey, now); err != nil {
		return KlineAppendResult{}, err
	}
	return KlineAppendResult{Revised: true, Version: next}, nil
}

func sameKlineValues(a, b KlineBar) bool {
	return a.Open == b.Open && a.High == b.High && a.Low == b.Low &&
		a.Close == b.Close && a.Volume == b.Volume && a.Amount == b.Amount
}

// BumpDataVersion increments a table-level version counter inside tx. Version 1
// is the baseline recorded by the schema migration, so the first correction
// makes it 2.
func BumpDataVersion(ctx context.Context, tx *sql.Tx, key string, now time.Time) (int64, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO data_version(name, version, updated_at_ns)
		VALUES (?, 1, ?)
		ON CONFLICT(name) DO UPDATE SET
			version = data_version.version + 1,
			updated_at_ns = excluded.updated_at_ns`, key, now.UnixNano()); err != nil {
		return 0, fmt.Errorf("bump data_version %q: %w", key, err)
	}
	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM data_version WHERE name = ?`, key).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// BumpDataVersionDB is BumpDataVersion outside an existing transaction.
func BumpDataVersionDB(ctx context.Context, db *sql.DB, key string, now time.Time) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	version, err := BumpDataVersion(ctx, tx, key, now)
	if err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

// DataVersion reads a table-level version counter. Missing keys read as 1, the
// baseline version of data written before versioning existed.
func DataVersion(ctx context.Context, db *sql.DB, key string) (int64, error) {
	var version int64
	err := db.QueryRowContext(ctx, `SELECT version FROM data_version WHERE name = ?`, key).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	return version, err
}

// KlineDataVersion is DataVersion for the K-line stream.
func (s *Storage) KlineDataVersion(ctx context.Context) (int64, error) {
	return DataVersion(ctx, s.DB(), KlineDataVersionKey)
}

// KlineRevision is one appended correction for a (code,ktype,date) key.
type KlineRevision struct {
	Version     int64
	Bar         KlineBar
	Reason      string
	CreatedAtNS int64
}

// KlineRevisions returns the correction history of one bar, oldest first. The
// implicit version 1 (the main-table row) is not included.
func KlineRevisions(ctx context.Context, db *sql.DB, code string, ktype uint8, date string) ([]KlineRevision, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, open, high, low, close, volume, amount, reason, created_at_ns
		FROM kline_revision WHERE code = ? AND ktype = ? AND date = ? ORDER BY version`, code, ktype, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KlineRevision
	for rows.Next() {
		var r KlineRevision
		r.Bar.Code, r.Bar.KType, r.Bar.Date = code, ktype, date
		if err := rows.Scan(&r.Version, &r.Bar.Open, &r.Bar.High, &r.Bar.Low, &r.Bar.Close,
			&r.Bar.Volume, &r.Bar.Amount, &r.Reason, &r.CreatedAtNS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
