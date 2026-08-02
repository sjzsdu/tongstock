package discoveryrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/internal/discovery"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type TraceRepository struct {
	db *sql.DB
}

func NewTraceRepository(store *storage.Storage) (*TraceRepository, error) {
	if store == nil || store.DB() == nil {
		return nil, fmt.Errorf("discovery trace storage is required")
	}
	return &TraceRepository{db: store.DB()}, nil
}

func (r *TraceRepository) Save(ctx context.Context, result *discovery.Result) error {
	if result == nil || result.ResearchID == "" || result.ResultHash == "" {
		return fmt.Errorf("research result, research_id and result_hash are required")
	}
	if actual := result.ComputeHash(); actual != result.ResultHash {
		return fmt.Errorf("research result hash mismatch: declared=%s actual=%s", result.ResultHash, actual)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	created := result.GeneratedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO discovery_research_trace
		(research_id, result_hash, snapshot_id, conclusion, discovery_trials, trace_json, created_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(research_id) DO UPDATE SET
		result_hash=excluded.result_hash, snapshot_id=excluded.snapshot_id,
		conclusion=excluded.conclusion, discovery_trials=excluded.discovery_trials,
		trace_json=excluded.trace_json, created_at_ns=excluded.created_at_ns`,
		result.ResearchID, result.ResultHash, result.SnapshotID, result.Conclusion,
		result.DiscoveryTrials, string(payload), created.UnixNano())
	if err != nil {
		return fmt.Errorf("save discovery trace %s: %w", result.ResearchID, err)
	}
	return nil
}

func (r *TraceRepository) Get(ctx context.Context, researchID string) (*discovery.Result, error) {
	if researchID == "" {
		return nil, fmt.Errorf("research_id is required")
	}
	var payload string
	if err := r.db.QueryRowContext(ctx, `SELECT trace_json FROM discovery_research_trace
		WHERE research_id = ?`, researchID).Scan(&payload); err != nil {
		return nil, err
	}
	result := &discovery.Result{}
	if err := json.Unmarshal([]byte(payload), result); err != nil {
		return nil, err
	}
	if actual := result.ComputeHash(); actual != result.ResultHash {
		return nil, fmt.Errorf("persisted research trace hash mismatch")
	}
	return result, nil
}
