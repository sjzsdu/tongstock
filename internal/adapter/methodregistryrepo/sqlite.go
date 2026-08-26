package methodregistryrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type SQLiteRepository struct{ db *sql.DB }

func New(store *storage.Storage) (*SQLiteRepository, error) {
	if store == nil || store.DB() == nil {
		return nil, fmt.Errorf("method registry storage is required")
	}
	return &SQLiteRepository{db: store.DB()}, nil
}

func (r *SQLiteRepository) Save(ctx context.Context, m *methodregistry.Method, event methodregistry.AuditEvent) error {
	if m == nil || m.ID == "" || m.FamilyID == "" {
		return fmt.Errorf("complete method aggregate is required")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO investment_method_registry(method_id,family_id,variant_id,status,market,universe,holding_min_days,holding_max_days,current_version,method_json,created_at_ns,updated_at_ns) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(method_id) DO UPDATE SET family_id=excluded.family_id,variant_id=excluded.variant_id,status=excluded.status,market=excluded.market,universe=excluded.universe,holding_min_days=excluded.holding_min_days,holding_max_days=excluded.holding_max_days,current_version=excluded.current_version,method_json=excluded.method_json,updated_at_ns=excluded.updated_at_ns`, m.ID, m.FamilyID, m.VariantID, m.Status, m.Market, m.Universe, m.HoldingMinDays, m.HoldingMaxDays, m.CurrentVersion, string(raw), m.CreatedAt.UnixNano(), m.UpdatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("save investment method: %w", err)
	}
	if event.ID != "" {
		eventRaw, _ := json.Marshal(event)
		if _, err = tx.ExecContext(ctx, `INSERT INTO investment_method_audit(event_id,method_id,from_status,to_status,action,evidence_hash,event_json,created_at_ns) VALUES(?,?,?,?,?,?,?,?)`, event.ID, event.MethodID, event.From, event.To, event.Action, event.EvidenceHash, string(eventRaw), event.CreatedAt.UnixNano()); err != nil {
			return fmt.Errorf("append method audit: %w", err)
		}
	}
	return tx.Commit()
}
func (r *SQLiteRepository) Get(ctx context.Context, id string) (*methodregistry.Method, error) {
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT method_json FROM investment_method_registry WHERE method_id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, methodregistry.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decode(raw)
}
func (r *SQLiteRepository) Query(ctx context.Context, q methodregistry.Query) ([]*methodregistry.Method, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT method_json FROM investment_method_registry ORDER BY updated_at_ns DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowed := map[methodregistry.Status]bool{}
	for _, s := range q.Status {
		allowed[s] = true
	}
	var out []*methodregistry.Method
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		m, err := decode(raw)
		if err != nil {
			return nil, err
		}
		if len(allowed) > 0 && !allowed[m.Status] {
			continue
		}
		if q.Market != "" && !strings.EqualFold(m.Market, q.Market) {
			continue
		}
		if q.Universe != "" && m.Universe != q.Universe {
			continue
		}
		if q.FamilyID != "" && m.FamilyID != q.FamilyID {
			continue
		}
		if q.HoldingMinDays != nil && m.HoldingMaxDays > 0 && m.HoldingMaxDays < *q.HoldingMinDays {
			continue
		}
		if q.HoldingMaxDays != nil && m.HoldingMinDays > *q.HoldingMaxDays {
			continue
		}
		out = append(out, m)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, rows.Err()
}
func (r *SQLiteRepository) ListAudit(ctx context.Context, id string) ([]methodregistry.AuditEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT event_json FROM investment_method_audit WHERE method_id=? ORDER BY created_at_ns,event_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []methodregistry.AuditEvent
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var event methodregistry.AuditEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, rows.Err()
}
func decode(raw string) (*methodregistry.Method, error) {
	var m methodregistry.Method
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

var _ methodregistry.Repository = (*SQLiteRepository)(nil)
