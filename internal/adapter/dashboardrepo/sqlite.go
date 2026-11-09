// Package dashboardrepo 提供 dashboard 读模型所需的 SQLite 适配器。
package dashboardrepo

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// klineTypeDaily 是通达信 A 股日线的 ktype，与 validationrepo.KlineTypeDaily 一致。
const klineTypeDaily = 9

// KlineDateReader 从真实日线表读取最新交易日，用于判定数据新鲜度。
type KlineDateReader struct{ db *storage.Storage }

// NewKlineDateReader 构造只读适配器。
func NewKlineDateReader(db *storage.Storage) (*KlineDateReader, error) {
	if db == nil {
		return nil, errors.New("dashboard kline date reader requires non-nil storage")
	}
	return &KlineDateReader{db: db}, nil
}

// LatestKlineDate 返回库内最新一根有效日线的交易日（YYYY-MM-DD）。
// 没有任何真实日线时返回空字符串，而不是伪造日期。
func (r *KlineDateReader) LatestKlineDate() (string, error) {
	// kline_latest 是 kline LEFT JOIN kline_revision 且带相关子查询的视图，
	// 基表千万行级；对它直接做 MAX(REPLACE(date,'-','')) 会全量聚合，实测单条
	// SQL 就要 16~20s，是 /api/dashboard/today 首屏 31s 的主因。
	//
	// 快路径：先在基表 kline 上走 idx_date 取最新交易日（毫秒级，库内 date
	// 已是 YYYYMMDD 纯数字），再回到视图按该日期等值 + LIMIT 1 校验那一天
	// 确实存在有效日线（走 idx_date 等值查找，同样毫秒级）。
	if latest, ok, err := r.latestFromBaseTable(); err != nil {
		return "", err
	} else if ok {
		return latest, nil
	}
	// 快路径未命中（最新交易日整批都不是有效日线，例如当日数据全被隔离或
	// 仍是历史遗留格式）时回退到原查询，返回值与改动前完全一致。
	return r.latestFromView()
}

// latestFromBaseTable 用基表索引取最新交易日，并确认该日存在有效日线。
// 第二个返回值为 false 表示快路径不适用，调用方应回退到视图全量查询。
func (r *KlineDateReader) latestFromBaseTable() (string, bool, error) {
	var newest sql.NullString
	if err := r.db.DB().QueryRow(`SELECT MAX(date) FROM kline WHERE ktype = ?`,
		klineTypeDaily).Scan(&newest); err != nil {
		return "", false, err
	}
	if !newest.Valid {
		return "", false, nil
	}
	candidate := strings.TrimSpace(newest.String)
	if len(candidate) != 8 {
		return "", false, nil
	}
	var one int
	err := r.db.DB().QueryRow(`SELECT 1 FROM kline_latest
		WHERE ktype = ? AND date = ?
		AND open > 0 AND high > 0 AND low > 0 AND close > 0
		LIMIT 1`, klineTypeDaily, candidate).Scan(&one)
	switch {
	case err == nil:
		return candidate[:4] + "-" + candidate[4:6] + "-" + candidate[6:], true, nil
	case errors.Is(err, sql.ErrNoRows):
		// 最新交易日的日线存在但全部无效，交给回退分支决定返回值。
		return "", false, nil
	default:
		return "", false, err
	}
}

// latestFromView 是改动前的原始实现：对视图做全量聚合，慢但覆盖所有历史格式。
func (r *KlineDateReader) latestFromView() (string, error) {
	var raw sql.NullString
	err := r.db.DB().QueryRow(`SELECT MAX(REPLACE(date,'-','')) FROM kline_latest
		WHERE ktype = ? AND open > 0 AND high > 0 AND low > 0 AND close > 0
		AND length(REPLACE(date,'-','')) = 8`, klineTypeDaily).Scan(&raw)
	if err != nil {
		return "", err
	}
	if !raw.Valid || raw.String == "" {
		return "", nil
	}
	d := strings.TrimSpace(raw.String)
	if len(d) != 8 {
		return "", fmt.Errorf("unexpected kline date format %q", d)
	}
	return d[:4] + "-" + d[4:6] + "-" + d[6:], nil
}
