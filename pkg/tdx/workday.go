package tdx

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

type Workday struct {
	db  *sql.DB
	mu  sync.RWMutex
	loc *time.Location
}

var (
	workday     *Workday
	workdayOnce sync.Once
)

func GetWorkday(dbPath string) (*Workday, error) {
	var err error
	workdayOnce.Do(func() {
		database, e := openDatabase(dbPath)
		if e != nil {
			err = e
			return
		}
		workday = &Workday{db: database, loc: time.Local}
		err = workday.init()
	})
	return workday, err
}

func (w *Workday) init() error {
	_, err := w.db.Exec(`
		CREATE TABLE IF NOT EXISTS workday (
			unix INTEGER PRIMARY KEY,
			date TEXT
		);
	`)
	return err
}

func (w *Workday) UpdateFromKline(client *Client, indexCode string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	resp, err := client.GetKlineDayAll(indexCode)
	if err != nil {
		return err
	}

	tx, err := w.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO workday (unix, date) VALUES (?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range resp {
		day := time.Date(k.Time.Year(), k.Time.Month(), k.Time.Day(), 0, 0, 0, 0, w.loc)
		_, err := stmt.Exec(day.Unix(), day.Format("20060102"))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (w *Workday) Is(t time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	day := time.Date(t.Year(), t.Month(), t.Day(), 15, 0, 0, 0, w.loc)
	var exists int
	err := w.db.QueryRow(`SELECT 1 FROM workday WHERE unix = ?`, day.Unix()).Scan(&exists)
	return err == nil
}

func (w *Workday) TodayIs() bool {
	return w.Is(time.Now())
}

func (w *Workday) Range(start, end time.Time, fn func(time.Time) bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	loc := w.loc
	for d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc); d.Before(end); d = d.AddDate(0, 0, 1) {
		day := time.Date(d.Year(), d.Month(), d.Day(), 15, 0, 0, 0, loc)
		var exists int
		if err := w.db.QueryRow(`SELECT 1 FROM workday WHERE unix = ?`, day.Unix()).Scan(&exists); err == nil {
			if !fn(day) {
				return
			}
		}
	}
}

func (w *Workday) RangeDesc(fn func(time.Time) bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	rows, err := w.db.Query(`SELECT unix FROM workday ORDER BY unix DESC`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var unix int64
		if err := rows.Scan(&unix); err != nil {
			return
		}
		t := time.Unix(unix, 0).In(w.loc)
		if !fn(t) {
			return
		}
	}
}

func (w *Workday) RangeYear(year int, fn func(time.Time) bool) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.Local)
	w.Range(start, end, fn)
}

func (w *Workday) GetLastWorkday() (time.Time, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var unix int64
	err := w.db.QueryRow(`SELECT unix FROM workday ORDER BY unix DESC LIMIT 1`).Scan(&unix)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(unix, 0).In(w.loc), nil
}

func (w *Workday) GetNextWorkday(t time.Time) (time.Time, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	day := time.Date(t.Year(), t.Month(), t.Day(), 15, 0, 0, 0, w.loc).AddDate(0, 0, 1)
	for {
		var exists int
		err := w.db.QueryRow(`SELECT 1 FROM workday WHERE unix = ?`, day.Unix()).Scan(&exists)
		if err == nil {
			return day, nil
		}
		day = day.AddDate(0, 0, 1)
		if day.After(time.Now()) {
			return time.Time{}, fmt.Errorf("no more workdays")
		}
	}
}

func (w *Workday) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.db != nil {
		return w.db.Close()
	}
	return nil
}

func (w *Workday) String() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	count := 0
	_ = w.db.QueryRow(`SELECT COUNT(*) FROM workday`).Scan(&count)
	return fmt.Sprintf("Workday{count: %d}", count)
}
