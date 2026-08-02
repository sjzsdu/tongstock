package positiondecisionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"strings"
)

type SQLiteRepository struct{ db *sql.DB }

func New(s *storage.Storage) (*SQLiteRepository, error) {
	if s == nil || s.DB() == nil {
		return nil, fmt.Errorf("position decision storage required")
	}
	return &SQLiteRepository{db: s.DB()}, nil
}
func (r *SQLiteRepository) GetLink(ctx context.Context, id int64) (positiondecision.Link, error) {
	var x positiondecision.Link
	err := r.db.QueryRowContext(ctx, `SELECT trade_id,quantity,selection_run_id,method_id,method_version_id,buy_reason FROM position_method_link WHERE trade_id=?`, id).Scan(&x.TradeID, &x.Quantity, &x.SelectionRunID, &x.MethodID, &x.MethodVersionID, &x.BuyReason)
	return x, err
}
func (r *SQLiteRepository) SaveLink(ctx context.Context, x positiondecision.Link) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO position_method_link(trade_id,quantity,selection_run_id,method_id,method_version_id,buy_reason,created_at_ns) VALUES(?,?,?,?,?,?,strftime('%s','now')*1000000000) ON CONFLICT(trade_id) DO NOTHING`, x.TradeID, x.Quantity, x.SelectionRunID, x.MethodID, x.MethodVersionID, x.BuyReason)
	return err
}
func (r *SQLiteRepository) Save(ctx context.Context, x *positiondecision.Run) error {
	raw, err := json.Marshal(x)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO position_decision_run(run_id,run_hash,engine_version,snapshot_id,feature_snapshot_id,snapshot_date,decision_json,created_at_ns) VALUES(?,?,?,?,?,?,?,?)`, x.ID, x.RunHash, x.EngineVersion, x.SnapshotID, x.FeatureSnapshotID, x.SnapshotDate, string(raw), x.CreatedAt.UnixNano())
	return err
}
func (r *SQLiteRepository) Get(ctx context.Context, id string) (*positiondecision.Run, error) {
	col := "run_id"
	if strings.HasPrefix(id, "hash:") {
		col = "run_hash"
		id = strings.TrimPrefix(id, "hash:")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT decision_json FROM position_decision_run WHERE `+col+`=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, positiondecision.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var x positiondecision.Run
	err = json.Unmarshal([]byte(raw), &x)
	return &x, err
}
func (r *SQLiteRepository) List(ctx context.Context, date string, limit int) ([]*positiondecision.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := `SELECT decision_json FROM position_decision_run`
	args := []any{}
	if date != "" {
		q += ` WHERE snapshot_date=?`
		args = append(args, date)
	}
	q += ` ORDER BY snapshot_date DESC,created_at_ns DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*positiondecision.Run{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var x positiondecision.Run
		if err := json.Unmarshal([]byte(raw), &x); err != nil {
			return nil, err
		}
		out = append(out, &x)
	}
	return out, rows.Err()
}

var _ positiondecision.Repository = (*SQLiteRepository)(nil)
