package automationrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"time"
)

type SQLiteRepository struct{ db *sql.DB }

func New(s *storage.Storage) (*SQLiteRepository, error) {
	if s == nil || s.DB() == nil {
		return nil, fmt.Errorf("automation storage required")
	}
	return &SQLiteRepository{db: s.DB()}, nil
}
func (r *SQLiteRepository) Claim(ctx context.Context, key, snapshot string) (*automation.Job, bool, error) {
	var j automation.Job
	var start, finish int64
	err := r.db.QueryRowContext(ctx, `SELECT job_id,idempotency_key,snapshot_id,status,attempt,selection_run_id,position_run_id,error,started_at_ns,finished_at_ns FROM automation_job_run WHERE idempotency_key=?`, key).Scan(&j.ID, &j.IdempotencyKey, &j.SnapshotID, &j.Status, &j.Attempt, &j.SelectionRunID, &j.PositionRunID, &j.Error, &start, &finish)
	if err == nil {
		j.StartedAt = time.Unix(0, start)
		if finish > 0 {
			j.FinishedAt = time.Unix(0, finish)
		}
		if j.Status == "completed" {
			return &j, false, nil
		}
		if j.Status == "running" {
			return &j, false, automation.ErrBusy
		}
	} else if err != sql.ErrNoRows {
		return nil, false, err
	}
	now := time.Now().UTC()
	j = automation.Job{ID: fmt.Sprintf("job-%x", now.UnixNano()), IdempotencyKey: key, SnapshotID: snapshot, Status: "running", Attempt: 1, StartedAt: now}
	_, err = r.db.ExecContext(ctx, `INSERT INTO automation_job_run(job_id,idempotency_key,snapshot_id,status,attempt,started_at_ns) VALUES(?,?,?,?,?,?) ON CONFLICT(idempotency_key) DO UPDATE SET job_id=excluded.job_id,status='running',attempt=automation_job_run.attempt+1,error='',started_at_ns=excluded.started_at_ns,finished_at_ns=0`, j.ID, key, snapshot, j.Status, j.Attempt, now.UnixNano())
	return &j, true, err
}
func (r *SQLiteRepository) Complete(ctx context.Context, j *automation.Job, events []automation.Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE automation_job_run SET status='completed',selection_run_id=?,position_run_id=?,finished_at_ns=? WHERE idempotency_key=?`, j.SelectionRunID, j.PositionRunID, j.FinishedAt.UnixNano(), j.IdempotencyKey); err != nil {
		return err
	}
	for _, e := range events {
		raw, _ := json.Marshal(e.Payload)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO automation_outbox(event_key,job_id,event_type,priority,payload_json,status,created_at_ns) VALUES(?,?,?,?,?,'pending',?)`, e.Key, j.ID, e.Type, e.Priority, string(raw), e.CreatedAt.UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *SQLiteRepository) Fail(ctx context.Context, j *automation.Job, cause error) error {
	_, err := r.db.ExecContext(ctx, `UPDATE automation_job_run SET status='failed',error=?,finished_at_ns=? WHERE idempotency_key=?`, cause.Error(), time.Now().UnixNano(), j.IdempotencyKey)
	return err
}
func (r *SQLiteRepository) ListJobs(ctx context.Context, limit int) ([]automation.Job, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.QueryContext(ctx, `SELECT job_id,idempotency_key,snapshot_id,status,attempt,selection_run_id,position_run_id,error,started_at_ns,finished_at_ns FROM automation_job_run ORDER BY started_at_ns DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []automation.Job{}
	for rows.Next() {
		var j automation.Job
		var a, b int64
		if err := rows.Scan(&j.ID, &j.IdempotencyKey, &j.SnapshotID, &j.Status, &j.Attempt, &j.SelectionRunID, &j.PositionRunID, &j.Error, &a, &b); err != nil {
			return nil, err
		}
		j.StartedAt = time.Unix(0, a)
		if b > 0 {
			j.FinishedAt = time.Unix(0, b)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) ListEvents(ctx context.Context, status string, limit int) ([]automation.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT event_key,job_id,event_type,priority,payload_json,status,created_at_ns FROM automation_outbox`
	args := []any{}
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 ELSE 2 END,created_at_ns LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []automation.Event{}
	for rows.Next() {
		var e automation.Event
		var raw string
		var ns int64
		if err := rows.Scan(&e.Key, &e.JobID, &e.Type, &e.Priority, &raw, &e.Status, &ns); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(0, ns)
		_ = json.Unmarshal([]byte(raw), &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

var _ automation.Repository = (*SQLiteRepository)(nil)
