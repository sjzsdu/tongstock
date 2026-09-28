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
	var raw sql.NullString
	err := r.db.DB().QueryRow(`SELECT MAX(REPLACE(date,'-','')) FROM kline
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
