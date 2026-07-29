package stockpool

import (
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// Status 证券状态
type Status string

const (
	StatusNormal    Status = "normal"    // 正常交易
	StatusST        Status = "st"        // ST 股 (特别处理)
	StatusStarST    Status = "*st"       // *ST 股 (退市预警)
	StatusSuspended Status = "suspended" // 停牌
	StatusDelisted  Status = "delisted"  // 已退市
	StatusHalted    Status = "halted"    // 临时停牌
)

// AsOfSecurity 在某一日期的证券快照
type AsOfSecurity struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	Status   Status `json:"status"`
	IPODate  string `json:"ipo_date"`
}

// SecuritiesMasterStore point-in-time 证券主数据存储
type SecuritiesMasterStore struct {
	s *storage.Storage
}

// NewSecuritiesMasterStore 创建存储
func NewSecuritiesMasterStore(s *storage.Storage) *SecuritiesMasterStore {
	return &SecuritiesMasterStore{s: s}
}

// ReconstructPool 按日期重建证券池
// 规则 (date 为查询日期, YYYY-MM-DD):
//  1. 已上市: ipo_date <= date (含上市当日)
//  2. 未退市: delist_date 为空, 或 delist_date >= date (delist_date 为最后交易日, 含当日)
//  3. 当日未停牌 (suspended/halted):
//     security_status_history 中 effective_from <= date <= effective_to 且 status in (suspended, halted) 的证券排除
//
// includeSuspended=true 用于分析场景 (研究通常要排除停牌股)
// includeST=true 用于保留 ST 股 (研究时通常需要标记但不排除)
func (m *SecuritiesMasterStore) ReconstructPool(date time.Time, includeSuspended, includeST bool) ([]AsOfSecurity, error) {
	dateStr := date.Format("2006-01-02")
	query := `
SELECT si.code, si.name, si.exchange,
       si.ipo_date_txt, si.st_flag,
       (SELECT ssh.status FROM security_status_history ssh
          WHERE ssh.code = si.code
            AND ssh.effective_from <= ?
            AND (ssh.effective_to >= ? OR ssh.effective_to = '')
          ORDER BY ssh.effective_from DESC LIMIT 1) AS active_status
FROM stockinfo si
WHERE si.ipo_date_txt <= ?
  AND (si.delist_date = '' OR si.delist_date >= ?)
`
	args := []interface{}{dateStr, dateStr, dateStr, dateStr}

	if !includeSuspended {
		// 排除当日停牌 (suspended/halted) 证券
		query += `
  AND NOT EXISTS (
    SELECT 1 FROM security_status_history s
     WHERE s.code = si.code
       AND s.status IN ('suspended', 'halted')
       AND s.effective_from <= ?
       AND (s.effective_to >= ? OR s.effective_to = '')
  )`
		args = append(args, dateStr, dateStr)
	}

	if !includeST {
		query += `
  AND NOT EXISTS (
    SELECT 1 FROM security_status_history s
     WHERE s.code = si.code
       AND s.status IN ('st', '*st')
       AND s.effective_from <= ?
       AND (s.effective_to >= ? OR s.effective_to = '')
  )`
		args = append(args, dateStr, dateStr)
	}

	rows, err := m.s.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AsOfSecurity
	for rows.Next() {
		var code, name, exchange, ipoDate string
		var stFlag int
		var activeStatus *string
		if err := rows.Scan(&code, &name, &exchange, &ipoDate, &stFlag, &activeStatus); err != nil {
			return nil, err
		}
		status := StatusNormal
		if stFlag > 0 {
			status = StatusST
		}
		if activeStatus != nil {
			switch *activeStatus {
			case "st", "*st":
				status = Status(*activeStatus)
			case "suspended", "halted", "delisted":
				status = Status(*activeStatus)
			}
		}
		result = append(result, AsOfSecurity{
			Code:     code,
			Name:     name,
			Exchange: exchange,
			Status:   status,
			IPODate:  ipoDate,
		})
	}
	return result, rows.Err()
}

// GetSecurityAtDate 查询单只证券在某日期的状态
func (m *SecuritiesMasterStore) GetSecurityAtDate(code string, date time.Time) (*AsOfSecurity, error) {
	pool, err := m.ReconstructPool(date, false, true)
	if err != nil {
		return nil, err
	}
	for _, s := range pool {
		if s.Code == code {
			return &s, nil
		}
	}
	return nil, nil
}

// InsertStatus 写入一段证券状态变更历史
func (m *SecuritiesMasterStore) InsertStatus(code string, effectiveFrom, effectiveTo string, status Status, reason, source string) error {
	if code == "" || effectiveFrom == "" || status == "" {
		return fmt.Errorf("code, effective_from and status are required")
	}
	_, err := m.s.DB().Exec(`INSERT OR REPLACE INTO security_status_history(code, effective_from, effective_to, status, reason, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		code, effectiveFrom, effectiveTo, string(status), reason, source, time.Now().Unix())
	return err
}

// UpdateIpoDelist 同步 stockinfo 的上市/退市日期 (upsert)
func (m *SecuritiesMasterStore) UpdateIpoDelist(code, ipoDate, delistDate string, stFlag int) error {
	_, err := m.s.DB().Exec(`INSERT OR REPLACE INTO stockinfo (code, name, exchange, ipo_date_txt, delist_date, st_flag) VALUES (?, '', '', ?, ?, ?)`,
		code, ipoDate, delistDate, stFlag)
	return err
}

// EnsureIpoDefault 为已有 stockinfo 记录回填 ipo_date_txt (使用 ipo_date 字段)
func (m *SecuritiesMasterStore) EnsureIpoDefault() (int, error) {
	res, err := m.s.DB().Exec(`UPDATE stockinfo SET ipo_date_txt = CAST(ipo_date AS TEXT) WHERE (ipo_date_txt = '' OR ipo_date_txt IS NULL) AND ipo_date > 0`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
