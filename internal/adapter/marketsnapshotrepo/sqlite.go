package marketsnapshotrepo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// SQLiteRepository 是 marketsnapshot.Repository 的 SQLite adapter。
// 非 owning：调用方负责 *storage.Storage 的生命周期。
type SQLiteRepository struct {
	db *storage.Storage
}

func New(db *storage.Storage) (*SQLiteRepository, error) {
	if db == nil {
		return nil, errors.New("marketsnapshot repository requires non-nil storage")
	}
	if db.Dialect() != storage.SQLite {
		return nil, fmt.Errorf("marketsnapshot repository requires sqlite, got %q", db.Dialect())
	}
	return &SQLiteRepository{db: db}, nil
}

// SaveMarketSnapshot 幂等写入：同 id 未冻结则覆盖，冻结则拒绝。
func (r *SQLiteRepository) SaveMarketSnapshot(s *marketsnapshot.MarketSnapshot) error {
	if err := s.Validate(); err != nil {
		return err
	}
	tx, err := r.db.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 检查 frozen
	var frozen int
	err = tx.QueryRow(`SELECT frozen FROM market_snapshot WHERE id = ?`, s.ID).Scan(&frozen)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if frozen == 1 {
		return fmt.Errorf("market snapshot %s is frozen and cannot be overwritten", s.ID)
	}

	univDef, err := json.Marshal(s.Universe)
	if err != nil {
		return err
	}
	builtAt := s.BuiltAt.Unix()
	frozenAt := s.FrozenAt.Unix()
	frozenI := 0
	if s.Frozen {
		frozenI = 1
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO market_snapshot
		(id, snapshot_date, universe_definition, market, price_adjustment,
		 kline_expected_codes, kline_ready_codes, quote_ready_codes, finance_ready_codes, xdxr_ready_codes,
		 coverage_pct, status, readiness_reason, universe_hash, content_hash, frozen, built_at, frozen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.SnapshotDate, string(univDef), s.Market, s.PriceAdjustment,
		s.ExpectedKlineCodes, s.ReadyKlineCodes, s.ReadyQuoteCodes, s.ReadyFinanceCodes, s.ReadyXdxrCodes,
		s.CoveragePct, s.Status, s.ReadinessReason, s.UniverseHash, s.ContentHash, frozenI, builtAt, frozenAt,
	)
	if err != nil {
		return err
	}
	// 清理旧 code_state 再重写，以保持幂等。
	if _, err := tx.Exec(`DELETE FROM market_snapshot_code_state WHERE snapshot_id = ?`, s.ID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO market_snapshot_code_state
		(snapshot_id, code, universe_member, ipo_date, delist_date, status,
		 kline_last_date, kline_row_count, quote_ok, finance_ok, xdxr_ok, gap_days, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range s.Codes {
		univMember := 0
		if c.UniverseMember {
			univMember = 1
		}
		q := 0
		if c.QuoteReady {
			q = 1
		}
		f := 0
		if c.FinanceReady {
			f = 1
		}
		x := 0
		if c.XdxrReady {
			x = 1
		}
		if _, err := stmt.Exec(
			s.ID, c.Code, univMember, c.IpoDate, c.DelistDate, c.SecurityStatus,
			c.KlineLastDate, c.KlineRowCount, q, f, x, c.GapDays, c.LastError,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) LoadMarketSnapshot(id string, includeCodes bool) (*marketsnapshot.MarketSnapshot, error) {
	row := r.db.DB().QueryRow(`SELECT
		id, snapshot_date, universe_definition, market, price_adjustment,
		kline_expected_codes, kline_ready_codes, quote_ready_codes, finance_ready_codes, xdxr_ready_codes,
		coverage_pct, status, readiness_reason, universe_hash, content_hash, frozen, built_at, frozen_at
		FROM market_snapshot WHERE id = ?`, id)
	return scanMarketSnapshot(row, includeCodes, id, r.db)
}

func (r *SQLiteRepository) FindMarketSnapshot(date, universeName, adj string) (*marketsnapshot.MarketSnapshot, error) {
	// 注意: universe_definition 字段是整个 JSON，这里只按 JSON 里的 "name" 键查找
	rows, err := r.db.DB().Query(`SELECT id FROM market_snapshot
		WHERE snapshot_date = ? AND price_adjustment = ? AND json_extract(universe_definition, '$.name') = ?
		ORDER BY built_at DESC LIMIT 1`, date, adj, universeName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var id string
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	if err := rows.Scan(&id); err != nil {
		return nil, err
	}
	return r.LoadMarketSnapshot(id, true)
}

func (r *SQLiteRepository) ListMarketSnapshots(dateStart, dateEnd, status string) ([]*marketsnapshot.MarketSnapshot, error) {
	where := []string{}
	args := []any{}
	if dateStart != "" {
		where = append(where, "snapshot_date >= ?")
		args = append(args, dateStart)
	}
	if dateEnd != "" {
		where = append(where, "snapshot_date <= ?")
		args = append(args, dateEnd)
	}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	q := `SELECT id FROM market_snapshot`
	if len(where) > 0 {
		q += " WHERE " + joinAnd(where)
	}
	q += " ORDER BY snapshot_date DESC"
	rows, err := r.db.DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]*marketsnapshot.MarketSnapshot, 0, len(ids))
	for _, id := range ids {
		s, err := r.LoadMarketSnapshot(id, false)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *SQLiteRepository) FreezeMarketSnapshot(id string) error {
	_, err := r.db.DB().Exec(`UPDATE market_snapshot SET frozen = 1, frozen_at = ? WHERE id = ? AND frozen = 0`,
		time.Now().Unix(), id)
	return err
}

// ===== Feature =====

func (r *SQLiteRepository) SaveFeatureSnapshot(s *marketsnapshot.FeatureSnapshot) error {
	if s == nil || s.ID == "" {
		return fmt.Errorf("nil/empty feature snapshot")
	}
	tx, err := r.db.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	idsJSON, err := json.Marshal(s.FeatureIDs)
	if err != nil {
		return err
	}
	builtAt := s.BuiltAt.Unix()
	leakChecked := 0
	if s.LeakChecked {
		leakChecked = 1
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO feature_snapshot
		(id, market_snapshot_id, snapshot_date, feature_ids_json, feature_total,
		 rows_written, leak_checked, price_adjustment, status, as_of_ns, content_hash, built_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.MarketSnapshotID, s.SnapshotDate, string(idsJSON), s.FeatureTotal,
		s.RowsWritten, leakChecked, s.PriceAdjustment, s.Status, s.AsOfNs, s.ContentHash, builtAt,
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM feature_snapshot_value WHERE snapshot_id = ?`, s.ID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO feature_snapshot_value
		(snapshot_id, code, feature_id, feature_version, value, as_of)
		VALUES (?, ?, ?, 1, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for code, perCode := range s.Values {
		for fid, v := range perCode {
			if _, err := stmt.Exec(s.ID, code, fid, v, s.AsOfNs); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) LoadFeatureSnapshot(id string, includeValues bool) (*marketsnapshot.FeatureSnapshot, error) {
	row := r.db.DB().QueryRow(`SELECT
		id, market_snapshot_id, snapshot_date, feature_ids_json, feature_total,
		rows_written, leak_checked, price_adjustment, status, as_of_ns, content_hash, built_at
		FROM feature_snapshot WHERE id = ?`, id)
	s := &marketsnapshot.FeatureSnapshot{}
	var idsJSON string
	var leakC, builtAt int64
	var asOfNs sql.NullInt64
	err := row.Scan(&s.ID, &s.MarketSnapshotID, &s.SnapshotDate, &idsJSON, &s.FeatureTotal,
		&s.RowsWritten, &leakC, &s.PriceAdjustment, &s.Status, &asOfNs, &s.ContentHash, &builtAt)
	if err != nil {
		return nil, err
	}
	if asOfNs.Valid {
		s.AsOfNs = asOfNs.Int64
	}
	s.LeakChecked = leakC == 1
	s.BuiltAt = time.Unix(builtAt, 0)
	if err := json.Unmarshal([]byte(idsJSON), &s.FeatureIDs); err != nil {
		return nil, err
	}
	if includeValues {
		rows, err := r.db.DB().Query(`SELECT code, feature_id, value FROM feature_snapshot_value WHERE snapshot_id = ?`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		s.Values = map[string]map[string]float64{}
		for rows.Next() {
			var code, fid string
			var v float64
			if err := rows.Scan(&code, &fid, &v); err != nil {
				return nil, err
			}
			if s.Values[code] == nil {
				s.Values[code] = map[string]float64{}
			}
			s.Values[code][fid] = v
		}
	}
	return s, nil
}

func (r *SQLiteRepository) ListFeatureSnapshots(marketSnapshotID string) ([]*marketsnapshot.FeatureSnapshot, error) {
	rows, err := r.db.DB().Query(`SELECT id FROM feature_snapshot WHERE market_snapshot_id = ? ORDER BY built_at DESC`, marketSnapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]*marketsnapshot.FeatureSnapshot, 0, len(ids))
	for _, id := range ids {
		s, err := r.LoadFeatureSnapshot(id, false)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ===== helpers =====

func scanMarketSnapshot(row *sql.Row, includeCodes bool, id string, db *storage.Storage) (*marketsnapshot.MarketSnapshot, error) {
	s := &marketsnapshot.MarketSnapshot{}
	var (
		univDef  string
		frozen   int
		builtAt  int64
		frozenAt int64
	)
	err := row.Scan(
		&s.ID, &s.SnapshotDate, &univDef, &s.Market, &s.PriceAdjustment,
		&s.ExpectedKlineCodes, &s.ReadyKlineCodes, &s.ReadyQuoteCodes, &s.ReadyFinanceCodes, &s.ReadyXdxrCodes,
		&s.CoveragePct, &s.Status, &s.ReadinessReason, &s.UniverseHash, &s.ContentHash, &frozen, &builtAt, &frozenAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(univDef), &s.Universe); err != nil {
		return nil, err
	}
	s.Frozen = frozen == 1
	s.BuiltAt = time.Unix(builtAt, 0)
	s.FrozenAt = time.Unix(frozenAt, 0)
	if includeCodes {
		crows, err := db.DB().Query(`SELECT
			code, universe_member, ipo_date, delist_date, status,
			kline_last_date, kline_row_count, quote_ok, finance_ok, xdxr_ok, gap_days, last_error
			FROM market_snapshot_code_state WHERE snapshot_id = ? ORDER BY code`, id)
		if err != nil {
			return nil, err
		}
		defer crows.Close()
		for crows.Next() {
			var c marketsnapshot.CodeStatus
			var um, q, f, x int
			if err := crows.Scan(&c.Code, &um, &c.IpoDate, &c.DelistDate, &c.SecurityStatus,
				&c.KlineLastDate, &c.KlineRowCount, &q, &f, &x, &c.GapDays, &c.LastError); err != nil {
				return nil, err
			}
			c.UniverseMember = um == 1
			c.QuoteReady = q == 1
			c.FinanceReady = f == 1
			c.XdxrReady = x == 1
			s.Codes = append(s.Codes, c)
			if c.UniverseMember {
				s.UniverseMembers = append(s.UniverseMembers, marketsnapshot.UniverseMember{
					Code: c.Code, Status: c.SecurityStatus, IpoDate: c.IpoDate, DelistDate: c.DelistDate, Selected: true,
				})
			}
		}
	}
	return s, nil
}

func joinAnd(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
