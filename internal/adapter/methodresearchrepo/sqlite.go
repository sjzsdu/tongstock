package methodresearchrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/sjzsdu/tongstock/internal/methodresearch"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type SQLiteRepository struct{ db *sql.DB }

func New(store *storage.Storage) (*SQLiteRepository, error) {
	if store == nil || store.DB() == nil {
		return nil, fmt.Errorf("method research storage is required")
	}
	return &SQLiteRepository{db: store.DB()}, nil
}

func (r *SQLiteRepository) Save(ctx context.Context, result *methodresearch.ResearchResult) error {
	if result == nil || result.ResearchID == "" || result.FamilyID == "" || result.ResultHash == "" {
		return fmt.Errorf("complete method research result is required")
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO method_research_artifact(research_id,family_id,result_hash,status,method_name,result_json,created_at_ns) VALUES(?,?,?,?,?,?,?)`, result.ResearchID, result.FamilyID, result.ResultHash, result.Status, result.MethodName, string(b), result.CreatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("save method research artifact: %w", err)
	}
	for _, s := range result.Sources {
		sb, _ := json.Marshal(s)
		if _, err = tx.ExecContext(ctx, `INSERT INTO method_source_evidence(research_id,source_id,source_url,content_hash,tier,source_json,retrieved_at_ns) VALUES(?,?,?,?,?,?,?)`, result.ResearchID, s.ID, s.URL, s.ContentHash, s.Tier, string(sb), s.RetrievedAt.UnixNano()); err != nil {
			return fmt.Errorf("save source evidence %s: %w", s.ID, err)
		}
	}
	for _, job := range result.ValidationJobs {
		if job.JobID == "" || job.FamilyID != result.FamilyID || job.MethodHash == "" || job.Status != "queued" {
			return fmt.Errorf("invalid validation handoff for variant %s", job.VariantID)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO method_validation_queue(job_id,research_id,family_id,variant_id,method_hash,scope,stock_code,status,created_at_ns) VALUES(?,?,?,?,?,?,?,?,?)`, job.JobID, result.ResearchID, job.FamilyID, job.VariantID, job.MethodHash, job.Scope, job.StockCode, job.Status, result.CreatedAt.UnixNano()); err != nil {
			return fmt.Errorf("enqueue method validation %s: %w", job.JobID, err)
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (*methodresearch.ResearchResult, error) {
	var raw string
	if err := r.db.QueryRowContext(ctx, `SELECT result_json FROM method_research_artifact WHERE research_id=?`, id).Scan(&raw); err != nil {
		return nil, err
	}
	var result methodresearch.ResearchResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	if got := result.ComputeHash(); got != result.ResultHash {
		return nil, fmt.Errorf("method research integrity mismatch: stored=%s computed=%s", result.ResultHash, got)
	}
	return &result, nil
}

var _ methodresearch.Repository = (*SQLiteRepository)(nil)
