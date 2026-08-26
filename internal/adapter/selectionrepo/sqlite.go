package selectionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type SQLiteRepository struct{ db *sql.DB }

func New(store *storage.Storage) (*SQLiteRepository, error) {
	if store == nil || store.DB() == nil {
		return nil, fmt.Errorf("selection storage is required")
	}
	return &SQLiteRepository{db: store.DB()}, nil
}

func (r *SQLiteRepository) Save(ctx context.Context, run *selection.Run) error {
	if run == nil || run.ID == "" || run.RunHash == "" {
		return fmt.Errorf("complete selection run is required")
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO daily_selection_run(run_id,run_hash,snapshot_id,feature_snapshot_id,snapshot_date,status,eligible_methods,scanned_stocks,candidate_count,buy_count,run_json,created_at_ns) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, run.ID, run.RunHash, run.SnapshotID, run.FeatureSnapshotID, run.SnapshotDate, run.Status, run.EligibleMethods, run.ScannedStocks, run.CandidateCount, run.BuyCount, string(raw), run.CreatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("save selection run: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return tx.Commit()
	}
	for _, c := range run.Candidates {
		ids := make([]string, 0, len(c.Triggers))
		for _, t := range c.Triggers {
			ids = append(ids, t.MethodID)
		}
		craw, _ := json.Marshal(c)
		mids, _ := json.Marshal(ids)
		if _, err = tx.ExecContext(ctx, `INSERT INTO daily_selection_candidate(run_id,code,rank,action,score,method_ids_json,candidate_json) VALUES(?,?,?,?,?,?,?)`, run.ID, c.Code, c.Rank, c.Action, c.Score, string(mids), string(craw)); err != nil {
			return err
		}
	}
	for i, x := range run.Exclusions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO daily_selection_exclusion(run_id,ordinal,method_id,code,reason_code,detail) VALUES(?,?,?,?,?,?)`, run.ID, i, x.MethodID, x.Code, x.ReasonCode, x.Detail); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) Get(ctx context.Context, id, methodID string) (*selection.Run, error) {
	column := "run_id"
	value := id
	if strings.HasPrefix(id, "hash:") {
		column = "run_hash"
		value = strings.TrimPrefix(id, "hash:")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT run_json FROM daily_selection_run WHERE `+column+`=?`, value).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, selection.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var run selection.Run
	if err = json.Unmarshal([]byte(raw), &run); err != nil {
		return nil, err
	}
	filter(&run, methodID)
	return &run, nil
}

func (r *SQLiteRepository) List(ctx context.Context, date, methodID string, limit int) ([]*selection.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	query := `SELECT run_json FROM daily_selection_run`
	args := []any{}
	if date != "" {
		query += ` WHERE snapshot_date=?`
		args = append(args, date)
	}
	query += ` ORDER BY snapshot_date DESC,created_at_ns DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*selection.Run{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var run selection.Run
		if err := json.Unmarshal([]byte(raw), &run); err != nil {
			return nil, err
		}
		filter(&run, methodID)
		if methodID == "" || len(run.Candidates) > 0 {
			out = append(out, &run)
		}
	}
	return out, rows.Err()
}

func filter(run *selection.Run, methodID string) {
	if methodID == "" {
		return
	}
	out := run.Candidates[:0]
	for _, c := range run.Candidates {
		found := false
		for _, t := range c.Triggers {
			if t.MethodID == methodID {
				found = true
				break
			}
		}
		if found {
			out = append(out, c)
		}
	}
	run.Candidates = out
	run.CandidateCount = len(out)
	run.BuyCount = 0
	for _, c := range out {
		if c.Action == selection.ActionBuy {
			run.BuyCount++
		}
	}
}

var _ selection.Repository = (*SQLiteRepository)(nil)
