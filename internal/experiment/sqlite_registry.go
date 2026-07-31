package experiment

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteRegistry 将实验、运行和制品持久化到 TongStock 共享数据库。
type SQLiteRegistry struct {
	s *storage.Storage
}

func NewSQLiteRegistry(s *storage.Storage) (*SQLiteRegistry, error) {
	if s == nil || s.DB() == nil {
		return nil, fmt.Errorf("experiment registry storage is required")
	}
	if s.Dialect() != storage.SQLite {
		return nil, fmt.Errorf("experiment registry requires sqlite storage")
	}
	return &SQLiteRegistry{s: s}, nil
}

func (r *SQLiteRegistry) Create(exp *Experiment) error {
	if exp == nil || exp.ID == "" {
		return fmt.Errorf("experiment and ID are required")
	}
	config, err := json.Marshal(exp.Config)
	if err != nil {
		return fmt.Errorf("marshal experiment config: %w", err)
	}
	environment, err := json.Marshal(exp.Environment)
	if err != nil {
		return fmt.Errorf("marshal experiment environment: %w", err)
	}
	tags, err := json.Marshal(exp.Tags)
	if err != nil {
		return fmt.Errorf("marshal experiment tags: %w", err)
	}
	_, err = r.s.DB().Exec(`INSERT INTO experiment_registry
		(id, name, description, status, config_json, config_hash, environment_json,
		 created_at_ns, updated_at_ns, completed_at_ns, created_by, tags_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		exp.ID, exp.Name, exp.Description, exp.Status, string(config), exp.ConfigHash,
		string(environment), exp.CreatedAt.UnixNano(), exp.UpdatedAt.UnixNano(),
		nullableTime(exp.CompletedAt), exp.CreatedBy, string(tags))
	if err != nil {
		return fmt.Errorf("create experiment %s: %w", exp.ID, err)
	}
	return nil
}

func (r *SQLiteRegistry) GetByID(id string) (*Experiment, error) {
	return scanExperiment(r.s.DB().QueryRow(`SELECT id, name, description, status,
		config_json, config_hash, environment_json, created_at_ns, updated_at_ns,
		completed_at_ns, created_by, tags_json
		FROM experiment_registry WHERE id = ?`, id))
}

func (r *SQLiteRegistry) List() ([]*Experiment, error) {
	rows, err := r.s.DB().Query(`SELECT id, name, description, status,
		config_json, config_hash, environment_json, created_at_ns, updated_at_ns,
		completed_at_ns, created_by, tags_json
		FROM experiment_registry ORDER BY created_at_ns DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Experiment
	for rows.Next() {
		exp, err := scanExperiment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, exp)
	}
	return result, rows.Err()
}

func (r *SQLiteRegistry) Update(exp *Experiment) error {
	if exp == nil || exp.ID == "" {
		return fmt.Errorf("experiment and ID are required")
	}
	config, err := json.Marshal(exp.Config)
	if err != nil {
		return err
	}
	environment, err := json.Marshal(exp.Environment)
	if err != nil {
		return err
	}
	tags, err := json.Marshal(exp.Tags)
	if err != nil {
		return err
	}
	result, err := r.s.DB().Exec(`UPDATE experiment_registry SET
		name = ?, description = ?, status = ?, config_json = ?, config_hash = ?,
		environment_json = ?, created_at_ns = ?, updated_at_ns = ?, completed_at_ns = ?,
		created_by = ?, tags_json = ? WHERE id = ?`,
		exp.Name, exp.Description, exp.Status, string(config), exp.ConfigHash,
		string(environment), exp.CreatedAt.UnixNano(), exp.UpdatedAt.UnixNano(),
		nullableTime(exp.CompletedAt), exp.CreatedBy, string(tags), exp.ID)
	if err != nil {
		return fmt.Errorf("update experiment %s: %w", exp.ID, err)
	}
	return requireAffected(result, "experiment", exp.ID)
}

func (r *SQLiteRegistry) Delete(id string) error {
	tx, err := r.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM experiment_run_artifact
		WHERE run_id IN (SELECT id FROM experiment_run WHERE experiment_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM experiment_run WHERE experiment_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM experiment_snapshot_binding WHERE experiment_id = ?`, id); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM experiment_registry WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := requireAffected(result, "experiment", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRegistry) CreateRun(run *ExperimentRun) error {
	if run == nil || run.ID == "" || run.ExperimentID == "" {
		return fmt.Errorf("run ID and experiment ID are required")
	}
	metrics, err := nullableJSON(run.Metrics)
	if err != nil {
		return err
	}
	_, err = r.s.DB().Exec(`INSERT INTO experiment_run
		(id, experiment_id, status, start_time_ns, end_time_ns, duration_ns,
		 metrics_json, error_message, logs, config_hash, result_hash, reproducible,
		 reproducibility_note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ExperimentID, run.Status, run.StartTime.UnixNano(),
		nullableTime(run.EndTime), int64(run.Duration), metrics, run.ErrorMessage,
		run.Logs, run.ConfigHash, run.ResultHash, boolInt(run.Reproducible),
		run.ReproducibilityNote)
	if err != nil {
		return fmt.Errorf("create run %s: %w", run.ID, err)
	}
	return nil
}

func (r *SQLiteRegistry) GetRun(runID string) (*ExperimentRun, error) {
	run, err := scanRun(r.s.DB().QueryRow(`SELECT id, experiment_id, status,
		start_time_ns, end_time_ns, duration_ns, metrics_json, error_message, logs,
		config_hash, result_hash, reproducible, reproducibility_note
		FROM experiment_run WHERE id = ?`, runID))
	if err != nil {
		return nil, err
	}
	run.Artifacts, err = r.listArtifacts(run.ID)
	return run, err
}

func (r *SQLiteRegistry) ListRuns(experimentID string) ([]*ExperimentRun, error) {
	rows, err := r.s.DB().Query(`SELECT id, experiment_id, status,
		start_time_ns, end_time_ns, duration_ns, metrics_json, error_message, logs,
		config_hash, result_hash, reproducible, reproducibility_note
		FROM experiment_run WHERE experiment_id = ? ORDER BY start_time_ns, id`, experimentID)
	if err != nil {
		return nil, err
	}
	var runs []*ExperimentRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, run := range runs {
		run.Artifacts, err = r.listArtifacts(run.ID)
		if err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (r *SQLiteRegistry) UpdateRun(run *ExperimentRun) error {
	if run == nil || run.ID == "" {
		return fmt.Errorf("run and ID are required")
	}
	metrics, err := nullableJSON(run.Metrics)
	if err != nil {
		return err
	}
	tx, err := r.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE experiment_run SET
		experiment_id = ?, status = ?, start_time_ns = ?, end_time_ns = ?,
		duration_ns = ?, metrics_json = ?, error_message = ?, logs = ?,
		config_hash = ?, result_hash = ?, reproducible = ?, reproducibility_note = ?
		WHERE id = ?`,
		run.ExperimentID, run.Status, run.StartTime.UnixNano(), nullableTime(run.EndTime),
		int64(run.Duration), metrics, run.ErrorMessage, run.Logs, run.ConfigHash,
		run.ResultHash, boolInt(run.Reproducible), run.ReproducibilityNote, run.ID)
	if err != nil {
		return err
	}
	if err := requireAffected(result, "run", run.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM experiment_run_artifact WHERE run_id = ?`, run.ID); err != nil {
		return err
	}
	for _, artifact := range run.Artifacts {
		if _, err := tx.Exec(`INSERT INTO experiment_run_artifact
			(id, run_id, type, name, description, content, content_hash, file_path, created_at_ns)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			artifact.ID, run.ID, artifact.Type, artifact.Name, artifact.Description,
			[]byte(artifact.Content), artifact.ContentHash, artifact.FilePath,
			artifact.CreatedAt.UnixNano()); err != nil {
			return fmt.Errorf("insert artifact %s: %w", artifact.ID, err)
		}
	}
	return tx.Commit()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanExperiment(row scanner) (*Experiment, error) {
	var exp Experiment
	var config, environment, tags string
	var createdAt, updatedAt int64
	var completedAt sql.NullInt64
	if err := row.Scan(&exp.ID, &exp.Name, &exp.Description, &exp.Status,
		&config, &exp.ConfigHash, &environment, &createdAt, &updatedAt,
		&completedAt, &exp.CreatedBy, &tags); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(config), &exp.Config); err != nil {
		return nil, fmt.Errorf("decode experiment %s config: %w", exp.ID, err)
	}
	if err := json.Unmarshal([]byte(environment), &exp.Environment); err != nil {
		return nil, fmt.Errorf("decode experiment %s environment: %w", exp.ID, err)
	}
	if err := json.Unmarshal([]byte(tags), &exp.Tags); err != nil {
		return nil, fmt.Errorf("decode experiment %s tags: %w", exp.ID, err)
	}
	exp.CreatedAt = time.Unix(0, createdAt)
	exp.UpdatedAt = time.Unix(0, updatedAt)
	exp.CompletedAt = timeFromNull(completedAt)
	return &exp, nil
}

func scanRun(row scanner) (*ExperimentRun, error) {
	var run ExperimentRun
	var startTime int64
	var endTime sql.NullInt64
	var duration int64
	var metrics sql.NullString
	var reproducible int
	if err := row.Scan(&run.ID, &run.ExperimentID, &run.Status, &startTime,
		&endTime, &duration, &metrics, &run.ErrorMessage, &run.Logs,
		&run.ConfigHash, &run.ResultHash, &reproducible,
		&run.ReproducibilityNote); err != nil {
		return nil, err
	}
	run.StartTime = time.Unix(0, startTime)
	run.EndTime = timeFromNull(endTime)
	run.Duration = time.Duration(duration)
	run.Reproducible = reproducible != 0
	if metrics.Valid && metrics.String != "" {
		var value MetricSet
		if err := json.Unmarshal([]byte(metrics.String), &value); err != nil {
			return nil, fmt.Errorf("decode run %s metrics: %w", run.ID, err)
		}
		run.Metrics = &value
	}
	return &run, nil
}

func (r *SQLiteRegistry) listArtifacts(runID string) ([]Artifact, error) {
	rows, err := r.s.DB().Query(`SELECT id, type, name, description, content,
		content_hash, file_path, created_at_ns
		FROM experiment_run_artifact WHERE run_id = ? ORDER BY type, name, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []Artifact
	for rows.Next() {
		var artifact Artifact
		var content []byte
		var createdAt int64
		if err := rows.Scan(&artifact.ID, &artifact.Type, &artifact.Name,
			&artifact.Description, &content, &artifact.ContentHash,
			&artifact.FilePath, &createdAt); err != nil {
			return nil, err
		}
		artifact.Content = append(json.RawMessage(nil), content...)
		artifact.CreatedAt = time.Unix(0, createdAt)
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UnixNano()
}

func timeFromNull(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(0, value.Int64)
	return &result
}

func nullableJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func requireAffected(result sql.Result, kind, id string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%s %s not found", kind, id)
	}
	return nil
}
