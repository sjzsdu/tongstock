package tdx

import (
	"fmt"
	"sync"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

// XdXrEventStore 持久化的除权除息/公司行为事件存储。
//
// 与 metadata.go 中的 XdXrStore (短期缓存, 7 天 TTL) 不同，这里的
// 事件会长期保存在数据库里，用于计算复权因子与保持历史价格/收益口径一致。
// 数据来源: TDX TypeXdXr 协议帧 (protocol.XdXrItem)。
type XdXrEventStore struct {
	s   *storage.Storage
	mu  sync.RWMutex
	loc *time.Location
}

// XdXrEvent 持久化的除权除息事件记录。
type XdXrEvent struct {
	Code        string    `json:"code"`
	Date        time.Time `json:"date"`
	Category    protocol.XdXrCategory
	FenHong     float32 // 分红(每股,元)
	PeiGuJia    float32 // 配股价
	SongZhuanGu float32 // 送转股(每10股)
	PeiGu       float32 // 配股(每10股)
	SuoGu       float32 // 缩股比例

	PanQianLiuTong float64 // 前流通股本(万股)
	PanHouLiuTong  float64 // 后流通股本(万股)
	QianZongGuBen  float64 // 前总股本(万股)
	HouZongGuBen   float64 // 后总股本(万股)

	SourceUpdatedAt time.Time `json:"source_updated_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// NewXdXrEventStore 创建持久化事件存储。
func NewXdXrEventStore(s *storage.Storage) (*XdXrEventStore, error) {
	return &XdXrEventStore{s: s, loc: time.Local}, nil
}

// Save 保存一批事件 (按 code + date + category upsert)。
func (s *XdXrEventStore) Save(code string, items []*protocol.XdXrItem, sourceUpdatedAt time.Time) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(s.saveSQL())
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, it := range items {
		dateStr := it.Date.In(s.loc).Format("2006-01-02")
		if _, err := stmt.Exec(
			code,
			dateStr,
			int(it.Category),
			float64(it.FenHong),
			float64(it.PeiGuJia),
			float64(it.SongZhuanGu),
			float64(it.PeiGu),
			float64(it.SuoGu),
			it.PanQianLiuTong,
			it.PanHouLiuTong,
			it.QianZongGuBen,
			it.HouZongGuBen,
			sourceUpdatedAt.Unix(),
			now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *XdXrEventStore) saveSQL() string {
	switch s.s.Dialect() {
	case storage.Postgres:
		return `INSERT INTO xdxr_event
			(code, date, category, fen_hong, pei_gu_jia, song_zhuan_gu, pei_gu, suo_gu,
			 qian_liu_tong, hou_liu_tong, qian_zong, hou_zong,
			 source_updated_at, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (code, date, category) DO UPDATE SET
				fen_hong=EXCLUDED.fen_hong, pei_gu_jia=EXCLUDED.pei_gu_jia,
				song_zhuan_gu=EXCLUDED.song_zhuan_gu, pei_gu=EXCLUDED.pei_gu,
				suo_gu=EXCLUDED.suo_gu,
				qian_liu_tong=EXCLUDED.qian_liu_tong, hou_liu_tong=EXCLUDED.hou_liu_tong,
				qian_zong=EXCLUDED.qian_zong, hou_zong=EXCLUDED.hou_zong,
				source_updated_at=EXCLUDED.source_updated_at`
	case storage.MySQL:
		return `INSERT INTO xdxr_event
			(code, date, category, fen_hong, pei_gu_jia, song_zhuan_gu, pei_gu, suo_gu,
			 qian_liu_tong, hou_liu_tong, qian_zong, hou_zong,
			 source_updated_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
				fen_hong=VALUES(fen_hong), pei_gu_jia=VALUES(pei_gu_jia),
				song_zhuan_gu=VALUES(song_zhuan_gu), pei_gu=VALUES(pei_gu),
				suo_gu=VALUES(suo_gu),
				qian_liu_tong=VALUES(qian_liu_tong), hou_liu_tong=VALUES(hou_liu_tong),
				qian_zong=VALUES(qian_zong), hou_zong=VALUES(hou_zong),
				source_updated_at=VALUES(source_updated_at)`
	default: // SQLite
		return `INSERT INTO xdxr_event
			(code, date, category, fen_hong, pei_gu_jia, song_zhuan_gu, pei_gu, suo_gu,
			 qian_liu_tong, hou_liu_tong, qian_zong, hou_zong,
			 source_updated_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(code, date, category) DO UPDATE SET
				fen_hong=excluded.fen_hong, pei_gu_jia=excluded.pei_gu_jia,
				song_zhuan_gu=excluded.song_zhuan_gu, pei_gu=excluded.pei_gu,
				suo_gu=excluded.suo_gu,
				qian_liu_tong=excluded.qian_liu_tong, hou_liu_tong=excluded.hou_liu_tong,
				qian_zong=excluded.qian_zong, hou_zong=excluded.hou_zong,
				source_updated_at=excluded.source_updated_at`
	}
}

// ListByCode 按日期升序列出全部事件。
func (s *XdXrEventStore) ListByCode(code string) ([]*XdXrEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.s.DB().Query(fmt.Sprintf(`
		SELECT code, date, category, fen_hong, pei_gu_jia, song_zhuan_gu, pei_gu, suo_gu,
		       qian_liu_tong, hou_liu_tong, qian_zong, hou_zong,
		       source_updated_at, created_at
		FROM xdxr_event WHERE code = %s ORDER BY date ASC
	`, s.ph(1)), code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*XdXrEvent
	for rows.Next() {
		var ev XdXrEvent
		var dateStr string
		var sourceUpdatedAt, createdAt int64
		var cat int
		if err := rows.Scan(
			&ev.Code, &dateStr, &cat,
			&ev.FenHong, &ev.PeiGuJia, &ev.SongZhuanGu, &ev.PeiGu, &ev.SuoGu,
			&ev.PanQianLiuTong, &ev.PanHouLiuTong, &ev.QianZongGuBen, &ev.HouZongGuBen,
			&sourceUpdatedAt, &createdAt,
		); err != nil {
			return nil, err
		}
		if t, perr := time.ParseInLocation("2006-01-02", dateStr, s.loc); perr == nil {
			ev.Date = t
		}
		ev.Category = protocol.XdXrCategory(cat)
		if sourceUpdatedAt > 0 {
			ev.SourceUpdatedAt = time.Unix(sourceUpdatedAt, 0)
		}
		if createdAt > 0 {
			ev.CreatedAt = time.Unix(createdAt, 0)
		}
		events = append(events, &ev)
	}
	return events, rows.Err()
}

// ph 返回占位符。
func (s *XdXrEventStore) ph(n int) string {
	if s.s.Dialect() == storage.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// Close 关闭 (由 storage 统一管理)。
func (s *XdXrEventStore) Close() error { return nil }
