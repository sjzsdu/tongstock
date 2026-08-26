package ledger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// NewSQLiteSignalLedger 恢复共享数据库中的前向运行、信号、持仓和权益曲线。
func NewSQLiteSignalLedger(s *storage.Storage) (*SignalLedger, error) {
	if s == nil || s.DB() == nil {
		return nil, fmt.Errorf("forward ledger storage is required")
	}
	result := NewSignalLedger()
	result.storage = s

	rows, err := s.DB().Query(`SELECT data_json FROM forward_run ORDER BY start_date_ns, id`)
	if err != nil {
		return nil, fmt.Errorf("load forward runs: %w", err)
	}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var run ForwardRun
		if err := json.Unmarshal([]byte(raw), &run); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode forward run: %w", err)
		}
		if run.Positions == nil {
			run.Positions = map[string]PositionState{}
		}
		result.runs[run.ID] = &run
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.DB().Query(`SELECT data_json FROM forward_signal ORDER BY signal_date_ns, id`)
	if err != nil {
		return nil, fmt.Errorf("load forward signals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entry SignalEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, fmt.Errorf("decode forward signal: %w", err)
		}
		if entry.ContentHash != computeSignalHash(entry) {
			return nil, fmt.Errorf("signal %s hash mismatch while loading", entry.ID)
		}
		result.indexSignal(entry)
	}
	return result, rows.Err()
}

func (l *SignalLedger) indexSignal(entry SignalEntry) {
	l.entries[entry.ID] = entry
	l.byRun[entry.RunID] = append(l.byRun[entry.RunID], entry.ID)
	l.byParadigm[entry.ParadigmVersionID] = append(l.byParadigm[entry.ParadigmVersionID], entry.ID)
	l.byStock[entry.StockCode] = append(l.byStock[entry.StockCode], entry.ID)
	dateKey := entry.SignalDate.Format("2006-01-02")
	l.byDate[dateKey] = append(l.byDate[dateKey], entry.ID)
}

func (l *SignalLedger) saveRunLocked(run *ForwardRun) error {
	if l.storage == nil {
		return nil
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = l.storage.DB().Exec(`INSERT INTO forward_run
		(id, paradigm_version_id, start_date_ns, status, updated_at_ns, data_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status,
			updated_at_ns=excluded.updated_at_ns, data_json=excluded.data_json`,
		run.ID, run.ParadigmVersionID, run.StartDate.UnixNano(), run.Status,
		run.UpdatedAt.UnixNano(), string(raw))
	if err != nil {
		return fmt.Errorf("persist forward run %s: %w", run.ID, err)
	}
	return nil
}

func (l *SignalLedger) saveSignalLocked(entry SignalEntry) error {
	if l.storage == nil {
		return nil
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = l.storage.DB().Exec(`INSERT INTO forward_signal
		(id, run_id, paradigm_version_id, stock_code, signal_date_ns, content_hash, data_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET content_hash=excluded.content_hash,
			data_json=excluded.data_json`,
		entry.ID, entry.RunID, entry.ParadigmVersionID, entry.StockCode,
		entry.SignalDate.UnixNano(), entry.ContentHash, string(raw))
	if err != nil {
		return fmt.Errorf("persist forward signal %s: %w", entry.ID, err)
	}
	return nil
}

func (l *SignalLedger) saveAppendedSignalLocked(
	entry SignalEntry,
	run *ForwardRun,
	expectedRunUpdatedAt time.Time,
) error {
	if l.storage == nil {
		return nil
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	runJSON, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tx, err := l.storage.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO forward_signal
		(id, run_id, paradigm_version_id, stock_code, signal_date_ns, content_hash, data_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.RunID, entry.ParadigmVersionID, entry.StockCode,
		entry.SignalDate.UnixNano(), entry.ContentHash, string(entryJSON)); err != nil {
		return fmt.Errorf("persist forward signal %s: %w", entry.ID, err)
	}
	result, err := tx.Exec(`UPDATE forward_run SET status=?, updated_at_ns=?, data_json=?
		WHERE id=? AND updated_at_ns=?`,
		run.Status, run.UpdatedAt.UnixNano(), string(runJSON), run.ID,
		expectedRunUpdatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("persist forward run %s: %w", run.ID, err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("forward run %s changed concurrently; reload and retry", run.ID)
	}
	return tx.Commit()
}

func (l *SignalLedger) saveExecutionStateLocked(
	entry SignalEntry,
	run *ForwardRun,
	expectedSignalHash string,
	expectedRunUpdatedAt time.Time,
) error {
	if l.storage == nil {
		return nil
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	runJSON, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tx, err := l.storage.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE forward_signal SET content_hash=?, data_json=?
		WHERE id=? AND content_hash=?`,
		entry.ContentHash, string(entryJSON), entry.ID, expectedSignalHash)
	if err != nil {
		return fmt.Errorf("persist signal execution %s: %w", entry.ID, err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("signal %s changed concurrently; reload before executing", entry.ID)
	}
	result, err = tx.Exec(`UPDATE forward_run SET status=?, updated_at_ns=?, data_json=?
		WHERE id=? AND updated_at_ns=?`,
		run.Status, run.UpdatedAt.UnixNano(), string(runJSON), run.ID,
		expectedRunUpdatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("persist forward account %s: %w", run.ID, err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("forward account %s changed concurrently; reload before executing", run.ID)
	}
	return tx.Commit()
}
