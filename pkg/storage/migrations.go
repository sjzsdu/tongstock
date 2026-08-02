package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type migration struct {
	version int
	name    string
	sql     string
	after   func(*sql.Tx) error
}

var sqliteMigrations = []migration{
	{
		version: 1,
		name:    "unified_base_schema",
		sql: `
CREATE TABLE IF NOT EXISTS history_stocks (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', analyzed_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS watchlist (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', "group" TEXT NOT NULL DEFAULT 'default',
	note TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS trades (
	id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL, price REAL NOT NULL, signal TEXT NOT NULL DEFAULT '',
	ktype TEXT NOT NULL DEFAULT 'day', reason TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS stockpool (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	filters TEXT NOT NULL DEFAULT '[]', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS stockinfo (
	code TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', exchange TEXT NOT NULL DEFAULT '',
	price REAL NOT NULL DEFAULT 0, open REAL NOT NULL DEFAULT 0, high REAL NOT NULL DEFAULT 0,
	low REAL NOT NULL DEFAULT 0, last_close REAL NOT NULL DEFAULT 0, change_pct REAL NOT NULL DEFAULT 0,
	volume REAL NOT NULL DEFAULT 0, amount REAL NOT NULL DEFAULT 0, turnover_rate REAL NOT NULL DEFAULT 0,
	liu_tong_gu_ben REAL NOT NULL DEFAULT 0, zong_gu_ben REAL NOT NULL DEFAULT 0,
	market_cap REAL NOT NULL DEFAULT 0, total_market_cap REAL NOT NULL DEFAULT 0,
	jing_zi_chan REAL NOT NULL DEFAULT 0, jing_li_run REAL NOT NULL DEFAULT 0,
	mei_gu_jing_zi_chan REAL NOT NULL DEFAULT 0, province INTEGER NOT NULL DEFAULT 0,
	industry INTEGER NOT NULL DEFAULT 0, ipo_date INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS kline (
	code TEXT, ktype INTEGER, date TEXT, open REAL, high REAL, low REAL, close REAL,
	volume REAL, amount REAL, PRIMARY KEY (code, ktype, date)
);
CREATE INDEX IF NOT EXISTS idx_code_ktype ON kline(code, ktype);
CREATE INDEX IF NOT EXISTS idx_date ON kline(date);
CREATE TABLE IF NOT EXISTS kline_sync_state (
	code TEXT, ktype INTEGER, first_date TEXT NOT NULL DEFAULT '', last_date TEXT NOT NULL DEFAULT '',
	row_count INTEGER NOT NULL DEFAULT 0, last_sync_at INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', PRIMARY KEY (code, ktype)
);
CREATE TABLE IF NOT EXISTS workday (unix INTEGER PRIMARY KEY, date TEXT);
CREATE TABLE IF NOT EXISTS cache (
	bucket TEXT NOT NULL, key TEXT NOT NULL, value BLOB, created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (bucket, key)
);
CREATE INDEX IF NOT EXISTS idx_cache_bucket ON cache(bucket);
CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY, stock_code TEXT NOT NULL DEFAULT '', agent TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_stock_code ON chat_sessions(stock_code);
CREATE TABLE IF NOT EXISTS paradigms (
	id TEXT PRIMARY KEY, stock_code TEXT NOT NULL DEFAULT '', side TEXT NOT NULL DEFAULT '',
	cache_key TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL, data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_paradigms_stock_code ON paradigms(stock_code);
CREATE INDEX IF NOT EXISTS idx_paradigms_cache_key ON paradigms(cache_key);
CREATE TABLE IF NOT EXISTS news_items (
	id TEXT PRIMARY KEY, source TEXT NOT NULL, news_type TEXT NOT NULL, title TEXT NOT NULL,
	summary TEXT, content TEXT, publish_time DATETIME NOT NULL, hot_score INTEGER DEFAULT 0,
	tags TEXT, related_stocks TEXT, url TEXT, original_id TEXT UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_news_source ON news_items(source);
CREATE INDEX IF NOT EXISTS idx_news_publish_time ON news_items(publish_time);
CREATE INDEX IF NOT EXISTS idx_news_hot_score ON news_items(hot_score);
CREATE INDEX IF NOT EXISTS idx_news_related_stocks ON news_items(related_stocks);
CREATE TABLE IF NOT EXISTS hot_events (
	id TEXT PRIMARY KEY, title TEXT NOT NULL, keywords TEXT, related_stocks TEXT,
	hot_index INTEGER DEFAULT 0, source_counts TEXT, news_item_ids TEXT,
	status TEXT DEFAULT 'active', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_event_hot_index ON hot_events(hot_index);
CREATE INDEX IF NOT EXISTS idx_event_status ON hot_events(status);
CREATE INDEX IF NOT EXISTS idx_event_updated_at ON hot_events(updated_at);
CREATE TABLE IF NOT EXISTS alert_records (
	id TEXT PRIMARY KEY, rule_id TEXT, news_id TEXT, stock_code TEXT, level TEXT, type TEXT,
	title TEXT, summary TEXT, source TEXT, read BOOLEAN DEFAULT FALSE, trigger_time TEXT,
	created_at TEXT, FOREIGN KEY (news_id) REFERENCES news_items(id)
);
CREATE INDEX IF NOT EXISTS idx_alert_records_read ON alert_records(read);
CREATE INDEX IF NOT EXISTS idx_alert_records_trigger_time ON alert_records(trigger_time);
CREATE INDEX IF NOT EXISTS idx_alert_records_stock_code ON alert_records(stock_code);
`,
		after: func(tx *sql.Tx) error {
			if err := ensureSQLiteColumn(tx, "history_stocks", "name", `TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
			for _, column := range []struct {
				name string
				ddl  string
			}{
				{"group", `TEXT NOT NULL DEFAULT 'default'`},
				{"note", `TEXT NOT NULL DEFAULT ''`},
				{"updated_at", `INTEGER NOT NULL DEFAULT 0`},
			} {
				if err := ensureSQLiteColumn(tx, "watchlist", column.name, column.ddl); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 2,
		name:    "stock_data_read_model",
		sql: `
CREATE TABLE IF NOT EXISTS quote_snapshot (
	code TEXT PRIMARY KEY, payload TEXT NOT NULL, source_updated_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS finance_snapshot (
	code TEXT PRIMARY KEY, payload TEXT NOT NULL, source_updated_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS data_sync_state (
	sync_key TEXT PRIMARY KEY, data_type TEXT NOT NULL, market TEXT NOT NULL DEFAULT '',
	code TEXT NOT NULL DEFAULT '', granularity TEXT NOT NULL DEFAULT '',
	range_start TEXT NOT NULL DEFAULT '', range_end TEXT NOT NULL DEFAULT '',
	coverage_start TEXT NOT NULL DEFAULT '', coverage_end TEXT NOT NULL DEFAULT '',
	source_updated_at INTEGER NOT NULL DEFAULT 0, last_sync_at INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT '', quality TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '', rows_written INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_data_sync_lookup
	ON data_sync_state(data_type, market, code, granularity);
`,
	},
	{
		version: 3,
		name:    "xdxr_and_adjustment_factor",
		sql: `
CREATE TABLE IF NOT EXISTS xdxr_event (
	code TEXT NOT NULL,
	date TEXT NOT NULL,   -- 除权除息/公司行为日期 (YYYY-MM-DD)
	category INTEGER NOT NULL,
	fen_hong REAL NOT NULL DEFAULT 0,       -- 每股分红 (元)
	pei_gu_jia REAL NOT NULL DEFAULT 0,      -- 配股价
	song_zhuan_gu REAL NOT NULL DEFAULT 0,   -- 每10股送转
	pei_gu REAL NOT NULL DEFAULT 0,          -- 每10股配股
	suo_gu REAL NOT NULL DEFAULT 0,          -- 缩股比例
	qian_liu_tong REAL NOT NULL DEFAULT 0,   -- 前流通股本(万股)
	hou_liu_tong REAL NOT NULL DEFAULT 0,    -- 后流通股本(万股)
	qian_zong REAL NOT NULL DEFAULT 0,       -- 前总股本(万股)
	hou_zong REAL NOT NULL DEFAULT 0,        -- 后总股本(万股)
	source_updated_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (code, date, category)
);
CREATE INDEX IF NOT EXISTS idx_xdxr_event_code_date
	ON xdxr_event(code, date);

CREATE TABLE IF NOT EXISTS adjustment_factor (
	code TEXT NOT NULL,
	date TEXT NOT NULL,  -- 除权除息日
	prev_close REAL NOT NULL DEFAULT 0,     -- 前收盘价 (不复权)
	forward_factor REAL NOT NULL DEFAULT 1, -- 前复权因子: 历史价格 * forward_factor
	backward_factor REAL NOT NULL DEFAULT 1,-- 后复权因子: 历史价格 / backward_factor
	cum_forward REAL NOT NULL DEFAULT 1,    -- 累计前复权因子 (截至该日)
	cum_backward REAL NOT NULL DEFAULT 1,   -- 累计后复权因子 (截至该日)
	reason TEXT NOT NULL DEFAULT '',       -- 触发原因 (ex_dividend / split / ...)
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (code, date)
);
CREATE INDEX IF NOT EXISTS idx_adj_factor_code
	ON adjustment_factor(code);
`,
	},
	{
		version: 4,
		name:    "point_in_time_securities_master",
		sql: `
-- 证券状态历史: 记录每只证券的可交易/ST/停牌等状态变更区间
CREATE TABLE IF NOT EXISTS security_status_history (
	code TEXT NOT NULL,
	effective_from TEXT NOT NULL,       -- 状态生效起始日 (YYYY-MM-DD), 包含
	effective_to TEXT NOT NULL,         -- 状态失效日 (YYYY-MM-DD), 包含; '' 表示仍在生效
	status TEXT NOT NULL,               -- normal / st / *st / suspended / delisted / halted
	reason TEXT NOT NULL DEFAULT '',    -- 触发原因 (停牌/摘牌/ST 原因等)
	source TEXT NOT NULL DEFAULT '',    -- 数据来源 (tdx/f10/manual)
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (code, effective_from, status)
);
CREATE INDEX IF NOT EXISTS idx_status_hist_code_date
	ON security_status_history(code, effective_from, effective_to);
CREATE INDEX IF NOT EXISTS idx_status_hist_status
	ON security_status_history(status);
`,
		after: func(tx *sql.Tx) error {
			// 扩展 stockinfo 增加上市/退市/ST 标记字段
			additional := []struct {
				name string
				ddl  string
			}{
				{"ipo_date_txt", `TEXT NOT NULL DEFAULT ''`},
				{"delist_date", `TEXT NOT NULL DEFAULT ''`},
				{"st_flag", `INTEGER NOT NULL DEFAULT 0`},
			}
			for _, c := range additional {
				if err := ensureSQLiteColumn(tx, "stockinfo", c.name, c.ddl); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 5,
		name:    "dataset_snapshot_and_lineage",
		sql: `
-- 数据快照: 用于绑定实验, 确保不可变和可追溯
CREATE TABLE IF NOT EXISTS dataset_snapshot (
	id TEXT PRIMARY KEY,
	version TEXT NOT NULL,
	date_range_start TEXT NOT NULL,
	date_range_end TEXT NOT NULL,
	universe TEXT NOT NULL DEFAULT '[]',  -- JSON array of codes
	market TEXT NOT NULL DEFAULT 'ALL',
	price_adjustment TEXT NOT NULL DEFAULT 'raw',
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	frozen INTEGER NOT NULL DEFAULT 0   -- 1 = immutable
);
CREATE INDEX IF NOT EXISTS idx_ds_created_at
	ON dataset_snapshot(created_at);

-- 快照中的数据源明细 (血缘追踪)
CREATE TABLE IF NOT EXISTS snapshot_data_source (
	snapshot_id TEXT NOT NULL,
	source_type TEXT NOT NULL,         -- kline / quote / finance / news / factor
	source_version TEXT NOT NULL,
	as_of INTEGER NOT NULL DEFAULT 0,
	source_updated_at INTEGER NOT NULL DEFAULT 0,
	hash TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (snapshot_id, source_type),
	FOREIGN KEY (snapshot_id) REFERENCES dataset_snapshot(id)
);
CREATE INDEX IF NOT EXISTS idx_snapshot_source_type
	ON snapshot_data_source(source_type);

-- 实验-快照绑定: 记录每个实验使用的不可变快照 ID 列表
CREATE TABLE IF NOT EXISTS experiment_snapshot_binding (
	experiment_id TEXT NOT NULL,
	snapshot_id TEXT NOT NULL,
	bound_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (experiment_id, snapshot_id)
);
CREATE INDEX IF NOT EXISTS idx_exp_binding_exp
	ON experiment_snapshot_binding(experiment_id);
`,
	},
	{
		version: 6,
		name:    "feature_registry",
		sql: `
-- 特征规格表: 统一特征定义 (TA/量价/相对强弱/市场状态/财务/事件)
CREATE TABLE IF NOT EXISTS feature_spec (
	id TEXT NOT NULL,
	version INTEGER NOT NULL DEFAULT 1,
	name TEXT NOT NULL,
	category TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	default_params TEXT NOT NULL DEFAULT '{}',   -- JSON
	window INTEGER NOT NULL DEFAULT 0,
	min_samples INTEGER NOT NULL DEFAULT 1,
	dependencies TEXT NOT NULL DEFAULT '[]',     -- JSON array
	timing TEXT NOT NULL DEFAULT 'end_of_day',
	data_required TEXT NOT NULL DEFAULT '[]',   -- JSON array
	formula_hash TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (id, version)
);
CREATE INDEX IF NOT EXISTS idx_feature_spec_category
	ON feature_spec(category);
CREATE INDEX IF NOT EXISTS idx_feature_spec_status
	ON feature_spec(status);

-- 特征集合表: 一组特征的打包定义
CREATE TABLE IF NOT EXISTS feature_set_spec (
	id TEXT NOT NULL,
	version INTEGER NOT NULL DEFAULT 1,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	features TEXT NOT NULL DEFAULT '[]',         -- JSON array of feature IDs
	category TEXT NOT NULL DEFAULT 'technical',
	price_req TEXT NOT NULL DEFAULT 'raw',
	created_at INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (id, version)
);
CREATE INDEX IF NOT EXISTS idx_feature_set_category
	ON feature_set_spec(category);

-- 特征计算结果表: 记录每次特征计算的结果
CREATE TABLE IF NOT EXISTS feature_value (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feature_id TEXT NOT NULL,
	feature_version INTEGER NOT NULL DEFAULT 1,
	stock_code TEXT NOT NULL,
	date TEXT NOT NULL,
	value REAL NOT NULL DEFAULT 0,
	source_data TEXT NOT NULL DEFAULT '',
	computed_at INTEGER NOT NULL DEFAULT 0,
	as_of INTEGER NOT NULL DEFAULT 0,
	leak_checked INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_feature_value_lookup
	ON feature_value(stock_code, date, feature_id);
CREATE INDEX IF NOT EXISTS idx_feature_value_feature
	ON feature_value(feature_id, feature_version);
`,
	},
	{
		version: 7,
		name:    "quality_gate",
		sql: `
-- 数据质量报告表: 每个数据快照/数据集对应一个质量报告
CREATE TABLE IF NOT EXISTS quality_report (
	id TEXT PRIMARY KEY,
	snapshot_id TEXT NOT NULL DEFAULT '',
	stock_code TEXT NOT NULL DEFAULT '',
	date_range_start TEXT NOT NULL DEFAULT '',
	date_range_end TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'kline',
	as_of INTEGER NOT NULL DEFAULT 0,
	passed INTEGER NOT NULL DEFAULT 1,
	blocked INTEGER NOT NULL DEFAULT 0,
	total_issues INTEGER NOT NULL DEFAULT 0,
	critical_count INTEGER NOT NULL DEFAULT 0,
	warning_count INTEGER NOT NULL DEFAULT 0,
	info_count INTEGER NOT NULL DEFAULT 0,
	checked_records INTEGER NOT NULL DEFAULT 0,
	passed_records INTEGER NOT NULL DEFAULT 0,
	failed_records INTEGER NOT NULL DEFAULT 0,
	coverage_percent REAL NOT NULL DEFAULT 0,
	report_hash TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_quality_report_snapshot
	ON quality_report(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_quality_report_stock
	ON quality_report(stock_code);
CREATE INDEX IF NOT EXISTS idx_quality_report_created
	ON quality_report(created_at);

-- 质量问题明细表
CREATE TABLE IF NOT EXISTS quality_issue (
	id TEXT PRIMARY KEY,
	report_id TEXT NOT NULL,
	category TEXT NOT NULL,
	severity TEXT NOT NULL,
	stock_code TEXT NOT NULL DEFAULT '',
	date TEXT NOT NULL DEFAULT '',
	metric TEXT NOT NULL DEFAULT '',
	expected TEXT NOT NULL DEFAULT '',
	actual TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (report_id) REFERENCES quality_report(id)
);
CREATE INDEX IF NOT EXISTS idx_quality_issue_report
	ON quality_issue(report_id);
CREATE INDEX IF NOT EXISTS idx_quality_issue_severity
	ON quality_issue(severity);

-- 质量门配置表: 可持久化质量门参数
CREATE TABLE IF NOT EXISTS quality_gate_config (
	id TEXT PRIMARY KEY DEFAULT 'default',
	max_price_change_pct REAL NOT NULL DEFAULT 5.0,
	max_volume_ratio REAL NOT NULL DEFAULT 10.0,
	min_coverage_percent REAL NOT NULL DEFAULT 95.0,
	max_missing_days INTEGER NOT NULL DEFAULT 5,
	max_financial_lag_days INTEGER NOT NULL DEFAULT 60,
	block_on_critical INTEGER NOT NULL DEFAULT 1,
	updated_at INTEGER NOT NULL DEFAULT 0
);
`,
	},
	{
		version: 8,
		name:    "immutable_kline_snapshot_content",
		sql: `
CREATE TABLE IF NOT EXISTS snapshot_kline_manifest (
	snapshot_id TEXT NOT NULL,
	code TEXT NOT NULL,
	ktype INTEGER NOT NULL,
	start_date TEXT NOT NULL,
	end_date TEXT NOT NULL,
	row_count INTEGER NOT NULL,
	content_hash TEXT NOT NULL,
	PRIMARY KEY (snapshot_id, code, ktype),
	FOREIGN KEY (snapshot_id) REFERENCES dataset_snapshot(id)
);
CREATE INDEX IF NOT EXISTS idx_snapshot_kline_manifest_snapshot
	ON snapshot_kline_manifest(snapshot_id);

CREATE TABLE IF NOT EXISTS snapshot_kline_bar (
	snapshot_id TEXT NOT NULL,
	code TEXT NOT NULL,
	ktype INTEGER NOT NULL,
	date TEXT NOT NULL,
	open REAL NOT NULL,
	high REAL NOT NULL,
	low REAL NOT NULL,
	close REAL NOT NULL,
	volume REAL NOT NULL,
	amount REAL NOT NULL,
	PRIMARY KEY (snapshot_id, code, ktype, date),
	FOREIGN KEY (snapshot_id) REFERENCES dataset_snapshot(id)
);
CREATE INDEX IF NOT EXISTS idx_snapshot_kline_bar_lookup
	ON snapshot_kline_bar(snapshot_id, code, ktype, date);
`,
		after: func(tx *sql.Tx) error {
			return ensureSQLiteColumn(tx, "dataset_snapshot", "content_hash", `TEXT NOT NULL DEFAULT ''`)
		},
	},
	{
		version: 9,
		name:    "persistent_experiment_runs",
		sql: `
CREATE TABLE IF NOT EXISTS experiment_registry (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	config_json TEXT NOT NULL,
	config_hash TEXT NOT NULL,
	environment_json TEXT NOT NULL,
	created_at_ns INTEGER NOT NULL,
	updated_at_ns INTEGER NOT NULL,
	completed_at_ns INTEGER,
	created_by TEXT NOT NULL DEFAULT '',
	tags_json TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_experiment_registry_created
	ON experiment_registry(created_at_ns);
CREATE INDEX IF NOT EXISTS idx_experiment_registry_config_hash
	ON experiment_registry(config_hash);

CREATE TABLE IF NOT EXISTS experiment_run (
	id TEXT PRIMARY KEY,
	experiment_id TEXT NOT NULL,
	status TEXT NOT NULL,
	start_time_ns INTEGER NOT NULL,
	end_time_ns INTEGER,
	duration_ns INTEGER NOT NULL DEFAULT 0,
	metrics_json TEXT,
	error_message TEXT NOT NULL DEFAULT '',
	logs TEXT NOT NULL DEFAULT '',
	config_hash TEXT NOT NULL,
	result_hash TEXT NOT NULL DEFAULT '',
	reproducible INTEGER NOT NULL DEFAULT 0,
	reproducibility_note TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (experiment_id) REFERENCES experiment_registry(id)
);
CREATE INDEX IF NOT EXISTS idx_experiment_run_experiment
	ON experiment_run(experiment_id, start_time_ns);
CREATE INDEX IF NOT EXISTS idx_experiment_run_result_hash
	ON experiment_run(result_hash);

CREATE TABLE IF NOT EXISTS experiment_run_artifact (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	content BLOB,
	content_hash TEXT NOT NULL DEFAULT '',
	file_path TEXT NOT NULL DEFAULT '',
	created_at_ns INTEGER NOT NULL,
	FOREIGN KEY (run_id) REFERENCES experiment_run(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_experiment_artifact_run
	ON experiment_run_artifact(run_id, type, name);
`,
	},
	{
		version: 10,
		name:    "persistent_forward_ledger",
		sql: `
CREATE TABLE IF NOT EXISTS forward_run (
	id TEXT PRIMARY KEY,
	paradigm_version_id TEXT NOT NULL,
	start_date_ns INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_at_ns INTEGER NOT NULL,
	data_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_forward_run_created
	ON forward_run(start_date_ns DESC);
CREATE INDEX IF NOT EXISTS idx_forward_run_paradigm
	ON forward_run(paradigm_version_id, start_date_ns DESC);

CREATE TABLE IF NOT EXISTS forward_signal (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	paradigm_version_id TEXT NOT NULL,
	stock_code TEXT NOT NULL,
	signal_date_ns INTEGER NOT NULL,
	content_hash TEXT NOT NULL,
	data_json TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES forward_run(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_forward_signal_run
	ON forward_signal(run_id, signal_date_ns, id);
CREATE INDEX IF NOT EXISTS idx_forward_signal_paradigm
	ON forward_signal(paradigm_version_id, signal_date_ns, id);
CREATE INDEX IF NOT EXISTS idx_forward_signal_stock
	ON forward_signal(stock_code, signal_date_ns, id);
`,
	},
	{
		version: 11,
		name:    "daily_market_and_feature_snapshot_with_readiness",
		sql: `
-- 每日市场快照: 绑定特定交易日的 universe + 数据水位 + 完整率 + 内容哈希。
-- 下游（选股、回测、AI 方法挖掘）只能引用 status = 'ready' 且 frozen = 1 的 snapshot_id。
CREATE TABLE IF NOT EXISTS market_snapshot (
	id TEXT PRIMARY KEY,
	snapshot_date TEXT NOT NULL,                -- YYYY-MM-DD，必须是交易日
	universe_definition TEXT NOT NULL DEFAULT '',  -- universe_xx 的名字或 SQL 表达式
	market TEXT NOT NULL DEFAULT 'CN-A',
	price_adjustment TEXT NOT NULL DEFAULT 'forward',
	kline_expected_codes INTEGER NOT NULL DEFAULT 0,
	kline_ready_codes INTEGER NOT NULL DEFAULT 0,
	quote_ready_codes INTEGER NOT NULL DEFAULT 0,
	finance_ready_codes INTEGER NOT NULL DEFAULT 0,
	xdxr_ready_codes INTEGER NOT NULL DEFAULT 0,
	coverage_pct REAL NOT NULL DEFAULT 0.0,
	status TEXT NOT NULL DEFAULT 'building',   -- building / ready / failed / partial
	readiness_reason TEXT NOT NULL DEFAULT '',
	universe_hash TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	frozen INTEGER NOT NULL DEFAULT 0,
	built_at INTEGER NOT NULL DEFAULT 0,
	frozen_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_market_snapshot_date_univ ON market_snapshot(snapshot_date, universe_definition, price_adjustment);
CREATE INDEX IF NOT EXISTS idx_market_snapshot_status ON market_snapshot(status);

-- 快照内的单股水位 (用于 tracing 缺口)
CREATE TABLE IF NOT EXISTS market_snapshot_code_state (
	snapshot_id TEXT NOT NULL,
	code TEXT NOT NULL,
	universe_member INTEGER NOT NULL DEFAULT 1,
	ipo_date TEXT NOT NULL DEFAULT '',
	delist_date TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'normal',       -- normal / st / suspended / delisted / halted
	kline_last_date TEXT NOT NULL DEFAULT '',
	kline_row_count INTEGER NOT NULL DEFAULT 0,
	quote_ok INTEGER NOT NULL DEFAULT 0,
	finance_ok INTEGER NOT NULL DEFAULT 0,
	xdxr_ok INTEGER NOT NULL DEFAULT 0,
	gap_days INTEGER NOT NULL DEFAULT 0,          -- 到 snapshot_date 连续缺失的天数
	last_error TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (snapshot_id, code),
	FOREIGN KEY (snapshot_id) REFERENCES market_snapshot(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_mss_code ON market_snapshot_code_state(code);

-- 每日特征快照: 对固定 {universe, date, feature_list} 的全量物化，
-- 作为选股/AI 训练的确定性输入。同一 (snapshot_id, feature_version_list) 哈希不变。
CREATE TABLE IF NOT EXISTS feature_snapshot (
	id TEXT PRIMARY KEY,
	market_snapshot_id TEXT NOT NULL,
	snapshot_date TEXT NOT NULL,
	feature_ids_json TEXT NOT NULL,           -- sorted feature IDs JSON
	feature_total INTEGER NOT NULL DEFAULT 0,
	rows_written INTEGER NOT NULL DEFAULT 0,
	leak_checked INTEGER NOT NULL DEFAULT 0,
	price_adjustment TEXT NOT NULL DEFAULT 'forward',
	status TEXT NOT NULL DEFAULT 'building',
	as_of_ns INTEGER NOT NULL DEFAULT 0,
	content_hash TEXT NOT NULL DEFAULT '',
	built_at INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (market_snapshot_id) REFERENCES market_snapshot(id)
);
CREATE INDEX IF NOT EXISTS idx_feature_snapshot_market ON feature_snapshot(market_snapshot_id);
CREATE INDEX IF NOT EXISTS idx_feature_snapshot_date ON feature_snapshot(snapshot_date);

CREATE TABLE IF NOT EXISTS feature_snapshot_value (
	snapshot_id TEXT NOT NULL,
	code TEXT NOT NULL,
	feature_id TEXT NOT NULL,
	feature_version INTEGER NOT NULL DEFAULT 1,
	value REAL NOT NULL DEFAULT 0,
	as_of INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (snapshot_id, code, feature_id, feature_version),
	FOREIGN KEY (snapshot_id) REFERENCES feature_snapshot(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fsv_lookup ON feature_snapshot_value(snapshot_id, code, feature_id);
`,
	},
	{
		version: 12,
		name:    "validation_evidence_artifact",
		sql: `
CREATE TABLE IF NOT EXISTS validation_evidence_artifact (
	result_hash TEXT PRIMARY KEY,
	job_hash TEXT NOT NULL,
	method_hash TEXT NOT NULL,
	snapshot_id TEXT NOT NULL,
	confidence TEXT NOT NULL,
	passable INTEGER NOT NULL DEFAULT 0,
	evidence_json TEXT NOT NULL,
	created_at_ns INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_validation_evidence_job
	ON validation_evidence_artifact(job_hash, created_at_ns DESC);
CREATE INDEX IF NOT EXISTS idx_validation_evidence_method
	ON validation_evidence_artifact(method_hash, created_at_ns DESC);
CREATE INDEX IF NOT EXISTS idx_validation_evidence_snapshot
	ON validation_evidence_artifact(snapshot_id, created_at_ns DESC);
`,
	},
}

// Migrate upgrades the SQLite database transactionally. Store constructors
// may assume the current schema and must not own or close the connection.
func (s *Storage) Migrate() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("storage is not initialized")
	}
	if s.dialect != SQLite {
		return fmt.Errorf("migrations only support sqlite")
	}
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at INTEGER NOT NULL
)`); err != nil {
		return err
	}

	for _, m := range sqliteMigrations {
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(m.sql); err == nil && m.after != nil {
			err = m.after(tx)
		}
		if err == nil {
			_, err = tx.Exec(
				`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
				m.version, m.name, time.Now().Unix(),
			)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func ensureSQLiteColumn(tx *sql.Tx, table, column, ddl string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + quoteIdentifier(table) + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE ` + quoteIdentifier(table) + ` ADD COLUMN ` + quoteIdentifier(column) + ` ` + ddl)
	return err
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// SchemaVersion returns the latest applied migration version.
func (s *Storage) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	return version, err
}

// Ping verifies the shared database connection.
func (s *Storage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
