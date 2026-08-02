package validationrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/internal/validation"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteEvidenceRepository 将 EvidenceBundle 作为可审计制品持久化。
type SQLiteEvidenceRepository struct {
	db *sql.DB
}

func NewEvidenceRepository(store *storage.Storage) (*SQLiteEvidenceRepository, error) {
	if store == nil || store.DB() == nil {
		return nil, fmt.Errorf("validation evidence storage is required")
	}
	return &SQLiteEvidenceRepository{db: store.DB()}, nil
}

func (r *SQLiteEvidenceRepository) Save(ctx context.Context, bundle *validation.EvidenceBundle) error {
	if bundle == nil || bundle.ResultHash == "" {
		return fmt.Errorf("evidence bundle and result_hash are required")
	}
	if actual := bundle.ComputeResultHash(); actual != bundle.ResultHash {
		return fmt.Errorf("evidence result hash mismatch: declared=%s actual=%s", bundle.ResultHash, actual)
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("marshal evidence bundle: %w", err)
	}
	createdAt := bundle.GeneratedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO validation_evidence_artifact
		(result_hash, job_hash, method_hash, snapshot_id, confidence, passable, evidence_json, created_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(result_hash) DO NOTHING`, bundle.ResultHash, bundle.JobHash,
		bundle.MethodHash, bundle.SnapshotID, bundle.Confidence, boolToInt(bundle.Passable),
		string(payload), createdAt.UnixNano())
	if err != nil {
		return fmt.Errorf("save validation evidence %s: %w", bundle.ResultHash, err)
	}
	return nil
}

func (r *SQLiteEvidenceRepository) Get(ctx context.Context, resultHash string) (*validation.EvidenceBundle, error) {
	if resultHash == "" {
		return nil, fmt.Errorf("result_hash is required")
	}
	return scanEvidence(r.db.QueryRowContext(ctx, `SELECT evidence_json
		FROM validation_evidence_artifact WHERE result_hash = ?`, resultHash))
}

func (r *SQLiteEvidenceRepository) ListByMethod(ctx context.Context, methodHash string, limit int) ([]*validation.EvidenceBundle, error) {
	if methodHash == "" {
		return nil, fmt.Errorf("method_hash is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT evidence_json
		FROM validation_evidence_artifact WHERE method_hash = ?
		ORDER BY created_at_ns DESC, result_hash LIMIT ?`, methodHash, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bundles []*validation.EvidenceBundle
	for rows.Next() {
		bundle, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}
	return bundles, rows.Err()
}

type evidenceScanner interface {
	Scan(dest ...any) error
}

func scanEvidence(row evidenceScanner) (*validation.EvidenceBundle, error) {
	var payload string
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	bundle := &validation.EvidenceBundle{}
	if err := json.Unmarshal([]byte(payload), bundle); err != nil {
		return nil, fmt.Errorf("decode validation evidence: %w", err)
	}
	if actual := bundle.ComputeResultHash(); actual != bundle.ResultHash {
		return nil, fmt.Errorf("persisted evidence hash mismatch: declared=%s actual=%s", bundle.ResultHash, actual)
	}
	return bundle, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ validation.EvidenceRepository = (*SQLiteEvidenceRepository)(nil)
