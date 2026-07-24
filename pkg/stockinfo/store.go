package stockinfo

import (
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// StockInfo 股票基础信息
type StockInfo struct {
	Code            string  `json:"code"`              // 股票代码
	Name            string  `json:"name"`              // 股票名称
	Exchange        string  `json:"exchange"`          // 交易所(sh/sz/bj)
	Price           float64 `json:"price"`             // 最新价格
	Open            float64 `json:"open"`              // 开盘价
	High            float64 `json:"high"`              // 最高价
	Low             float64 `json:"low"`               // 最低价
	LastClose       float64 `json:"lastClose"`         // 昨收价
	ChangePct       float64 `json:"changePct"`         // 涨跌幅(%)
	Volume          float64 `json:"volume"`            // 成交量(手)
	Amount          float64 `json:"amount"`            // 成交额(万元)
	TurnoverRate    float64 `json:"turnoverRate"`      // 换手率(%)
	LiuTongGuBen    float64 `json:"liuTongGuBen"`      // 流通股本(万股)
	ZongGuBen       float64 `json:"zongGuBen"`         // 总股本(万股)
	MarketCap       float64 `json:"marketCap"`         // 流通市值(亿元)
	TotalMarketCap  float64 `json:"totalMarketCap"`    // 总市值(亿元)
	JingZiChan      float64 `json:"jingZiChan"`        // 净资产(万元)
	JingLiRun       float64 `json:"jingLiRun"`         // 净利润(万元)
	MeiGuJingZiChan float64 `json:"meiGuJingZiChan"`   // 每股净资产(元)
	Province        uint16  `json:"province"`          // 省份代码
	Industry        uint16  `json:"industry"`          // 行业代码
	IPODate         uint32  `json:"ipoDate"`           // 上市日期
	UpdatedAt       int64   `json:"updatedAt"`         // 更新时间戳
}

// Store 股票信息存储
type Store struct {
	s *storage.Storage
}

// New 创建存储实例
func New(s *storage.Storage) (*Store, error) {
	store := &Store{s: s}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) init() error {
	_, err := s.s.DB().Exec(s.createTableSQL())
	return err
}

func (s *Store) createTableSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `CREATE TABLE IF NOT EXISTS stockinfo (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			exchange TEXT NOT NULL DEFAULT '',
			price DOUBLE PRECISION NOT NULL DEFAULT 0,
			open DOUBLE PRECISION NOT NULL DEFAULT 0,
			high DOUBLE PRECISION NOT NULL DEFAULT 0,
			low DOUBLE PRECISION NOT NULL DEFAULT 0,
			last_close DOUBLE PRECISION NOT NULL DEFAULT 0,
			change_pct DOUBLE PRECISION NOT NULL DEFAULT 0,
			volume DOUBLE PRECISION NOT NULL DEFAULT 0,
			amount DOUBLE PRECISION NOT NULL DEFAULT 0,
			turnover_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
			liu_tong_gu_ben DOUBLE PRECISION NOT NULL DEFAULT 0,
			zong_gu_ben DOUBLE PRECISION NOT NULL DEFAULT 0,
			market_cap DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_market_cap DOUBLE PRECISION NOT NULL DEFAULT 0,
			jing_zi_chan DOUBLE PRECISION NOT NULL DEFAULT 0,
			jing_li_run DOUBLE PRECISION NOT NULL DEFAULT 0,
			mei_gu_jing_zi_chan DOUBLE PRECISION NOT NULL DEFAULT 0,
			province INTEGER NOT NULL DEFAULT 0,
			industry INTEGER NOT NULL DEFAULT 0,
			ipo_date INTEGER NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0
		)`
	case storage.MySQL:
		return `CREATE TABLE IF NOT EXISTS stockinfo (
			code VARCHAR(20) PRIMARY KEY,
			name VARCHAR(100) NOT NULL DEFAULT '',
			exchange VARCHAR(10) NOT NULL DEFAULT '',
			price DOUBLE NOT NULL DEFAULT 0,
			open DOUBLE NOT NULL DEFAULT 0,
			high DOUBLE NOT NULL DEFAULT 0,
			low DOUBLE NOT NULL DEFAULT 0,
			last_close DOUBLE NOT NULL DEFAULT 0,
			change_pct DOUBLE NOT NULL DEFAULT 0,
			volume DOUBLE NOT NULL DEFAULT 0,
			amount DOUBLE NOT NULL DEFAULT 0,
			turnover_rate DOUBLE NOT NULL DEFAULT 0,
			liu_tong_gu_ben DOUBLE NOT NULL DEFAULT 0,
			zong_gu_ben DOUBLE NOT NULL DEFAULT 0,
			market_cap DOUBLE NOT NULL DEFAULT 0,
			total_market_cap DOUBLE NOT NULL DEFAULT 0,
			jing_zi_chan DOUBLE NOT NULL DEFAULT 0,
			jing_li_run DOUBLE NOT NULL DEFAULT 0,
			mei_gu_jing_zi_chan DOUBLE NOT NULL DEFAULT 0,
			province INT NOT NULL DEFAULT 0,
			industry INT NOT NULL DEFAULT 0,
			ipo_date INT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0
		)`
	default: // SQLite
		return `CREATE TABLE IF NOT EXISTS stockinfo (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			exchange TEXT NOT NULL DEFAULT '',
			price REAL NOT NULL DEFAULT 0,
			open REAL NOT NULL DEFAULT 0,
			high REAL NOT NULL DEFAULT 0,
			low REAL NOT NULL DEFAULT 0,
			last_close REAL NOT NULL DEFAULT 0,
			change_pct REAL NOT NULL DEFAULT 0,
			volume REAL NOT NULL DEFAULT 0,
			amount REAL NOT NULL DEFAULT 0,
			turnover_rate REAL NOT NULL DEFAULT 0,
			liu_tong_gu_ben REAL NOT NULL DEFAULT 0,
			zong_gu_ben REAL NOT NULL DEFAULT 0,
			market_cap REAL NOT NULL DEFAULT 0,
			total_market_cap REAL NOT NULL DEFAULT 0,
			jing_zi_chan REAL NOT NULL DEFAULT 0,
			jing_li_run REAL NOT NULL DEFAULT 0,
			mei_gu_jing_zi_chan REAL NOT NULL DEFAULT 0,
			province INTEGER NOT NULL DEFAULT 0,
			industry INTEGER NOT NULL DEFAULT 0,
			ipo_date INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`
	}
}

// ph 返回占位符 ? 或 $N
func (s *Store) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// GetAll 获取所有股票信息
func (s *Store) GetAll() ([]StockInfo, error) {
	rows, err := s.s.DB().Query(`SELECT code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at FROM stockinfo ORDER BY exchange, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var infos []StockInfo
	for rows.Next() {
		var info StockInfo
		if err := rows.Scan(
			&info.Code, &info.Name, &info.Exchange,
			&info.Price, &info.Open, &info.High, &info.Low, &info.LastClose, &info.ChangePct,
			&info.Volume, &info.Amount, &info.TurnoverRate,
			&info.LiuTongGuBen, &info.ZongGuBen, &info.MarketCap, &info.TotalMarketCap,
			&info.JingZiChan, &info.JingLiRun, &info.MeiGuJingZiChan,
			&info.Province, &info.Industry, &info.IPODate, &info.UpdatedAt,
		); err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

// GetByCode 根据代码获取股票信息
func (s *Store) GetByCode(code string) (*StockInfo, error) {
	query := fmt.Sprintf(`SELECT code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at FROM stockinfo WHERE code = %s`, s.ph(1))
	row := s.s.DB().QueryRow(query, code)

	var info StockInfo
	err := row.Scan(
		&info.Code, &info.Name, &info.Exchange,
		&info.Price, &info.Open, &info.High, &info.Low, &info.LastClose, &info.ChangePct,
		&info.Volume, &info.Amount, &info.TurnoverRate,
		&info.LiuTongGuBen, &info.ZongGuBen, &info.MarketCap, &info.TotalMarketCap,
		&info.JingZiChan, &info.JingLiRun, &info.MeiGuJingZiChan,
		&info.Province, &info.Industry, &info.IPODate, &info.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetByMarketCap 根据市值范围获取股票
func (s *Store) GetByMarketCap(minCap, maxCap float64) ([]StockInfo, error) {
	var query string
	if minCap > 0 && maxCap > 0 {
		query = fmt.Sprintf(`SELECT code, name, exchange, price, market_cap FROM stockinfo WHERE market_cap >= %s AND market_cap <= %s ORDER BY market_cap ASC`, s.ph(1), s.ph(2))
		return s.queryStockInfo(query, minCap, maxCap)
	} else if minCap > 0 {
		query = fmt.Sprintf(`SELECT code, name, exchange, price, market_cap FROM stockinfo WHERE market_cap >= %s ORDER BY market_cap ASC`, s.ph(1))
		return s.queryStockInfo(query, minCap)
	} else if maxCap > 0 {
		query = fmt.Sprintf(`SELECT code, name, exchange, price, market_cap FROM stockinfo WHERE market_cap <= %s ORDER BY market_cap ASC`, s.ph(1))
		return s.queryStockInfo(query, maxCap)
	}
	return s.GetAll()
}

func (s *Store) queryStockInfo(query string, args ...interface{}) ([]StockInfo, error) {
	rows, err := s.s.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var infos []StockInfo
	for rows.Next() {
		var info StockInfo
		if err := rows.Scan(
			&info.Code, &info.Name, &info.Exchange,
			&info.Price, &info.MarketCap,
		); err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

// GetByExchange 根据交易所获取股票
func (s *Store) GetByExchange(exchange string) ([]StockInfo, error) {
	query := fmt.Sprintf(`SELECT code, name, exchange, price, market_cap FROM stockinfo WHERE exchange = %s ORDER BY code`, s.ph(1))
	return s.queryStockInfo(query, exchange)
}

// Count 获取股票数量
func (s *Store) Count() (int, error) {
	var count int
	err := s.s.DB().QueryRow(`SELECT COUNT(*) FROM stockinfo`).Scan(&count)
	return count, err
}

// Upsert 插入或更新股票信息
func (s *Store) Upsert(info StockInfo) error {
	if info.Code == "" {
		return fmt.Errorf("code is required")
	}
	now := time.Now().Unix()
	info.UpdatedAt = now

	switch s.s.Dialect() {
	case storage.Postgres:
		_, err := s.s.DB().Exec(`
			INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
			ON CONFLICT(code) DO UPDATE SET
				name = $2, exchange = $3, price = $4, open = $5, high = $6, low = $7, last_close = $8, change_pct = $9,
				volume = $10, amount = $11, turnover_rate = $12, liu_tong_gu_ben = $13, zong_gu_ben = $14,
				market_cap = $15, total_market_cap = $16, jing_zi_chan = $17, jing_li_run = $18, mei_gu_jing_zi_chan = $19,
				province = $20, industry = $21, ipo_date = $22, updated_at = $23
		`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
			info.Volume, info.Amount, info.TurnoverRate,
			info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
			info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
			info.Province, info.Industry, info.IPODate, info.UpdatedAt)
		return err
	case storage.MySQL:
		_, err := s.s.DB().Exec(`
			INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				name = VALUES(name), exchange = VALUES(exchange), price = VALUES(price), open = VALUES(open), high = VALUES(high), low = VALUES(low), last_close = VALUES(last_close), change_pct = VALUES(change_pct),
				volume = VALUES(volume), amount = VALUES(amount), turnover_rate = VALUES(turnover_rate), liu_tong_gu_ben = VALUES(liu_tong_gu_ben), zong_gu_ben = VALUES(zong_gu_ben),
				market_cap = VALUES(market_cap), total_market_cap = VALUES(total_market_cap), jing_zi_chan = VALUES(jing_zi_chan), jing_li_run = VALUES(jing_li_run), mei_gu_jing_zi_chan = VALUES(mei_gu_jing_zi_chan),
				province = VALUES(province), industry = VALUES(industry), ipo_date = VALUES(ipo_date), updated_at = VALUES(updated_at)
		`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
			info.Volume, info.Amount, info.TurnoverRate,
			info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
			info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
			info.Province, info.Industry, info.IPODate, info.UpdatedAt)
		return err
	default: // SQLite
		_, err := s.s.DB().Exec(`
			INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = excluded.name, exchange = excluded.exchange, price = excluded.price, open = excluded.open, high = excluded.high, low = excluded.low, last_close = excluded.last_close, change_pct = excluded.change_pct,
				volume = excluded.volume, amount = excluded.amount, turnover_rate = excluded.turnover_rate, liu_tong_gu_ben = excluded.liu_tong_gu_ben, zong_gu_ben = excluded.zong_gu_ben,
				market_cap = excluded.market_cap, total_market_cap = excluded.total_market_cap, jing_zi_chan = excluded.jing_zi_chan, jing_li_run = excluded.jing_li_run, mei_gu_jing_zi_chan = excluded.mei_gu_jing_zi_chan,
				province = excluded.province, industry = excluded.industry, ipo_date = excluded.ipo_date, updated_at = excluded.updated_at
		`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
			info.Volume, info.Amount, info.TurnoverRate,
			info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
			info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
			info.Province, info.Industry, info.IPODate, info.UpdatedAt)
		return err
	}
}

// BatchUpsert 批量插入或更新
func (s *Store) BatchUpsert(infos []StockInfo) error {
	tx, err := s.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, info := range infos {
		if info.Code == "" {
			continue
		}
		now := time.Now().Unix()
		info.UpdatedAt = now

		switch s.s.Dialect() {
		case storage.Postgres:
			_, err := tx.Exec(`
				INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
				ON CONFLICT(code) DO UPDATE SET
					name = $2, exchange = $3, price = $4, open = $5, high = $6, low = $7, last_close = $8, change_pct = $9,
					volume = $10, amount = $11, turnover_rate = $12, liu_tong_gu_ben = $13, zong_gu_ben = $14,
					market_cap = $15, total_market_cap = $16, jing_zi_chan = $17, jing_li_run = $18, mei_gu_jing_zi_chan = $19,
					province = $20, industry = $21, ipo_date = $22, updated_at = $23
			`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
				info.Volume, info.Amount, info.TurnoverRate,
				info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
				info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
				info.Province, info.Industry, info.IPODate, info.UpdatedAt)
			if err != nil {
				return err
			}
		case storage.MySQL:
			_, err := tx.Exec(`
				INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE
					name = VALUES(name), exchange = VALUES(exchange), price = VALUES(price), open = VALUES(open), high = VALUES(high), low = VALUES(low), last_close = VALUES(last_close), change_pct = VALUES(change_pct),
					volume = VALUES(volume), amount = VALUES(amount), turnover_rate = VALUES(turnover_rate), liu_tong_gu_ben = VALUES(liu_tong_gu_ben), zong_gu_ben = VALUES(zong_gu_ben),
					market_cap = VALUES(market_cap), total_market_cap = VALUES(total_market_cap), jing_zi_chan = VALUES(jing_zi_chan), jing_li_run = VALUES(jing_li_run), mei_gu_jing_zi_chan = VALUES(mei_gu_jing_zi_chan),
					province = VALUES(province), industry = VALUES(industry), ipo_date = VALUES(ipo_date), updated_at = VALUES(updated_at)
			`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
				info.Volume, info.Amount, info.TurnoverRate,
				info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
				info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
				info.Province, info.Industry, info.IPODate, info.UpdatedAt)
			if err != nil {
				return err
			}
		default: // SQLite
			_, err := tx.Exec(`
				INSERT INTO stockinfo (code, name, exchange, price, open, high, low, last_close, change_pct, volume, amount, turnover_rate, liu_tong_gu_ben, zong_gu_ben, market_cap, total_market_cap, jing_zi_chan, jing_li_run, mei_gu_jing_zi_chan, province, industry, ipo_date, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(code) DO UPDATE SET
					name = excluded.name, exchange = excluded.exchange, price = excluded.price, open = excluded.open, high = excluded.high, low = excluded.low, last_close = excluded.last_close, change_pct = excluded.change_pct,
					volume = excluded.volume, amount = excluded.amount, turnover_rate = excluded.turnover_rate, liu_tong_gu_ben = excluded.liu_tong_gu_ben, zong_gu_ben = excluded.zong_gu_ben,
					market_cap = excluded.market_cap, total_market_cap = excluded.total_market_cap, jing_zi_chan = excluded.jing_zi_chan, jing_li_run = excluded.jing_li_run, mei_gu_jing_zi_chan = excluded.mei_gu_jing_zi_chan,
					province = excluded.province, industry = excluded.industry, ipo_date = excluded.ipo_date, updated_at = excluded.updated_at
			`, info.Code, info.Name, info.Exchange, info.Price, info.Open, info.High, info.Low, info.LastClose, info.ChangePct,
				info.Volume, info.Amount, info.TurnoverRate,
				info.LiuTongGuBen, info.ZongGuBen, info.MarketCap, info.TotalMarketCap,
				info.JingZiChan, info.JingLiRun, info.MeiGuJingZiChan,
				info.Province, info.Industry, info.IPODate, info.UpdatedAt)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// Delete 删除股票信息
func (s *Store) Delete(code string) error {
	query := fmt.Sprintf(`DELETE FROM stockinfo WHERE code = %s`, s.ph(1))
	_, err := s.s.DB().Exec(query, code)
	return err
}

// Exists 检查是否存在
func (s *Store) Exists(code string) (bool, error) {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM stockinfo WHERE code = %s`, s.ph(1))
	err := s.s.DB().QueryRow(query, code).Scan(&count)
	return count > 0, err
}

// GetStale 获取需要更新的股票（超过指定时间未更新）
func (s *Store) GetStale(maxAgeMinutes int64) ([]string, error) {
	cutoff := time.Now().Unix() - maxAgeMinutes*60
	query := fmt.Sprintf(`SELECT code FROM stockinfo WHERE updated_at < %s ORDER BY updated_at ASC`, s.ph(1))
	rows, err := s.s.DB().Query(query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

// BuildFromQuoteAndFinance 从行情和财务数据构建 StockInfo
func BuildFromQuoteAndFinance(exchange string, code string, name string, quote *protocol.QuoteItem, finance *protocol.FinanceInfo) *StockInfo {
	if code == "" {
		return nil
	}

	info := &StockInfo{
		Code:     code,
		Name:     name,
		Exchange: exchange,
	}

	if quote != nil {
		info.Price = quote.Price
		info.Open = quote.Open
		info.High = quote.High
		info.Low = quote.Low
		info.LastClose = quote.LastClose
		info.Volume = quote.Volume
		info.Amount = quote.Amount
		if quote.LastClose > 0 {
			info.ChangePct = (quote.Price - quote.LastClose) / quote.LastClose * 100
		}
	}

	if finance != nil {
		// LiuTongGuBen 和 ZongGuBen 在 FinanceInfo 中的单位是万股
		info.LiuTongGuBen = finance.LiuTongGuBen
		info.ZongGuBen = finance.ZongGuBen
		info.JingZiChan = finance.JingZiChan / 10000 // 转换为万元
		info.JingLiRun = finance.JingLiRun / 10000   // 转换为万元
		info.MeiGuJingZiChan = finance.MeiGuJingZiChan
		info.Province = finance.Province
		info.Industry = finance.Industry
		info.IPODate = finance.IPODate

		// 计算市值（亿元）
		if info.LiuTongGuBen > 0 && info.Price > 0 {
			info.MarketCap = info.LiuTongGuBen * info.Price / 10000
		}
		if info.ZongGuBen > 0 && info.Price > 0 {
			info.TotalMarketCap = info.ZongGuBen * info.Price / 10000
		}

		// 计算换手率（%）
		if info.LiuTongGuBen > 0 && info.Volume > 0 {
			info.TurnoverRate = (info.Volume * 100) / info.LiuTongGuBen / 10000 * 100
		}
	}

	return info
}
