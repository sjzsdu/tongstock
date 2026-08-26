package quality

import (
	"fmt"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
)

// QualityStore 质量报告存储。
type QualityStore struct {
	s *storage.Storage
}

// NewQualityStore 创建质量存储实例。
func NewQualityStore(s *storage.Storage) *QualityStore {
	return &QualityStore{s: s}
}

// SaveReport 保存质量报告及其问题明细。
func (store *QualityStore) SaveReport(report *QualityReport) error {
	if report.ID == "" {
		return fmt.Errorf("report ID is required")
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now()
	}

	tx, err := store.s.DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT OR REPLACE INTO quality_report
		(id, snapshot_id, stock_code, date_range_start, date_range_end, source,
		 as_of, passed, blocked, total_issues, critical_count, warning_count, info_count,
		 checked_records, passed_records, failed_records, coverage_percent, report_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.SnapshotID, report.StockCode,
		report.DateRange.Start, report.DateRange.End,
		report.Source, report.AsOf.Unix(),
		boolToInt(report.Passed), boolToInt(report.Blocked),
		report.Summary.TotalIssues, report.Summary.CriticalCount,
		report.Summary.WarningCount, report.Summary.InfoCount,
		report.Summary.CheckedRecords, report.Summary.PassedRecords,
		report.Summary.FailedRecords, report.Summary.CoveragePercent,
		report.ComputeHash(), report.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert report: %w", err)
	}

	for i, issue := range report.Issues {
		if issue.ID == "" {
			issue.ID = fmt.Sprintf("%s-%d", report.ID, i)
		}
		_, err = tx.Exec(`INSERT OR REPLACE INTO quality_issue
			(id, report_id, category, severity, stock_code, date,
			 metric, expected, actual, description, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			issue.ID, report.ID, string(issue.Category), string(issue.Severity),
			issue.StockCode, issue.Date, issue.Metric,
			issue.Expected, issue.Actual, issue.Description,
			time.Now().Unix())
		if err != nil {
			return fmt.Errorf("insert issue %s: %w", issue.ID, err)
		}
	}

	return tx.Commit()
}

// GetReport 按 ID 获取质量报告。
func (store *QualityStore) GetReport(id string) (*QualityReport, error) {
	row := store.s.DB().QueryRow(`SELECT id, snapshot_id, stock_code, date_range_start, date_range_end,
		source, as_of, passed, blocked, total_issues, critical_count, warning_count, info_count,
		checked_records, passed_records, failed_records, coverage_percent, report_hash, created_at
		FROM quality_report WHERE id = ?`, id)

	report, err := scanReport(row)
	if err != nil {
		return nil, err
	}

	issues, err := store.getIssues(report.ID)
	if err != nil {
		return nil, err
	}
	report.Issues = issues

	return report, nil
}

// GetReportsBySnapshot 获取快照关联的所有质量报告。
func (store *QualityStore) GetReportsBySnapshot(snapshotID string) ([]*QualityReport, error) {
	rows, err := store.s.DB().Query(`SELECT id FROM quality_report WHERE snapshot_id = ? ORDER BY created_at DESC`, snapshotID)
	if err != nil {
		return nil, err
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var reports []*QualityReport
	for _, id := range ids {
		report, err := store.GetReport(id)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// GetReportsByStock 获取股票关联的所有质量报告。
func (store *QualityStore) GetReportsByStock(stockCode string, limit, offset int) ([]*QualityReport, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := store.s.DB().Query(`SELECT id FROM quality_report WHERE stock_code = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		stockCode, limit, offset)
	if err != nil {
		return nil, err
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var reports []*QualityReport
	for _, id := range ids {
		report, err := store.GetReport(id)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// ListIssues 列出指定报告的所有问题。
func (store *QualityStore) ListIssues(reportID string, severity Severity) ([]QualityIssue, error) {
	query := `SELECT id, category, severity, stock_code, date, metric, expected, actual, description
		FROM quality_issue WHERE report_id = ?`
	args := []interface{}{reportID}

	if severity != "" {
		query += " AND severity = ?"
		args = append(args, string(severity))
	}

	rows, err := store.s.DB().Query(query+" ORDER BY severity, category", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []QualityIssue
	for rows.Next() {
		var issue QualityIssue
		var category, severityStr string
		if err := rows.Scan(&issue.ID, &category, &severityStr, &issue.StockCode,
			&issue.Date, &issue.Metric, &issue.Expected, &issue.Actual, &issue.Description); err != nil {
			return nil, err
		}
		issue.Category = Category(category)
		issue.Severity = Severity(severityStr)
		issues = append(issues, issue)
	}
	return issues, rows.Err()
}

// SaveConfig 保存质量门配置。
func (store *QualityStore) SaveConfig(config QualityGateConfig) error {
	_, err := store.s.DB().Exec(`INSERT OR REPLACE INTO quality_gate_config
		(id, max_price_change_pct, max_volume_ratio, min_coverage_percent,
		 max_missing_days, max_financial_lag_days, block_on_critical, updated_at)
		VALUES ('default', ?, ?, ?, ?, ?, ?, ?)`,
		config.MaxPriceChangePct, config.MaxVolumeRatio, config.MinCoveragePercent,
		config.MaxMissingDays, config.MaxFinancialLagDays,
		boolToInt(config.BlockOnCritical), time.Now().Unix())
	return err
}

// LoadConfig 加载质量门配置。
func (store *QualityStore) LoadConfig() (QualityGateConfig, error) {
	row := store.s.DB().QueryRow(`SELECT max_price_change_pct, max_volume_ratio, min_coverage_percent,
		max_missing_days, max_financial_lag_days, block_on_critical
		FROM quality_gate_config WHERE id = 'default'`)

	var cfg QualityGateConfig
	var blockInt int
	err := row.Scan(&cfg.MaxPriceChangePct, &cfg.MaxVolumeRatio, &cfg.MinCoveragePercent,
		&cfg.MaxMissingDays, &cfg.MaxFinancialLagDays, &blockInt)
	if err != nil {
		return DefaultQualityGateConfig(), nil // 返回默认配置
	}
	cfg.BlockOnCritical = blockInt == 1
	return cfg, nil
}

// CountReports 返回报告总数。
func (store *QualityStore) CountReports() (int, error) {
	var count int
	err := store.s.DB().QueryRow(`SELECT COUNT(*) FROM quality_report`).Scan(&count)
	return count, err
}

// CountCriticalIssues 返回 critical 级别问题总数。
func (store *QualityStore) CountCriticalIssues() (int, error) {
	var count int
	err := store.s.DB().QueryRow(`SELECT COUNT(*) FROM quality_issue WHERE severity = 'critical'`).Scan(&count)
	return count, err
}

// GateDecision 执行质量门决策。
func (store *QualityStore) GateDecision(reportID string) (*QualityGateResult, error) {
	report, err := store.GetReport(reportID)
	if err != nil {
		return nil, err
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		return nil, err
	}

	result := &QualityGateResult{
		ReportID:  reportID,
		CheckedAt: time.Now(),
	}

	if report.ShouldBlock(cfg) {
		result.Decision = "block"
		result.Blocked = true
		result.Passed = false
		result.Reason = fmt.Sprintf("Report %s has %d critical issues", reportID, report.Summary.CriticalCount)
	} else if report.HasCriticalIssues() {
		// 严重问题但未配置阻止 (warn-only 模式)
		result.Decision = "warn"
		result.Passed = true
		result.Reason = "Report has critical issues but block_on_critical is disabled"
	} else if report.Summary.WarningCount > 0 {
		result.Decision = "warn"
		result.Passed = true
		result.Reason = fmt.Sprintf("Report has %d warnings", report.Summary.WarningCount)
	} else {
		result.Decision = "pass"
		result.Passed = true
		result.Reason = "All checks passed"
	}

	for _, issue := range report.Issues {
		if issue.Severity == SeverityCritical {
			result.Issues = append(result.Issues, issue.Description)
		}
	}

	return result, nil
}

func scanReport(row interface{ Scan(dest ...any) error }) (*QualityReport, error) {
	var report QualityReport
	var snapshotID, stockCode, dateStart, dateEnd, source string
	var asOf, createdAt int64
	var passed, blocked int
	var total, critical, warning, info, checked, passedRec, failed int
	var coverage float64
	var reportHash string

	err := row.Scan(&report.ID, &snapshotID, &stockCode, &dateStart, &dateEnd,
		&source, &asOf, &passed, &blocked, &total, &critical, &warning, &info,
		&checked, &passedRec, &failed, &coverage, &reportHash, &createdAt)
	if err != nil {
		return nil, err
	}

	report.SnapshotID = snapshotID
	report.StockCode = stockCode
	report.DateRange = DateRangeInfo{Start: dateStart, End: dateEnd}
	report.Source = source
	report.AsOf = time.Unix(asOf, 0)
	report.Passed = passed == 1
	report.Blocked = blocked == 1
	report.CreatedAt = time.Unix(createdAt, 0)
	report.Summary = QualitySummary{
		TotalIssues:     total,
		CriticalCount:   critical,
		WarningCount:    warning,
		InfoCount:       info,
		CheckedRecords:  checked,
		PassedRecords:   passedRec,
		FailedRecords:   failed,
		CoveragePercent: coverage,
	}

	return &report, nil
}

func (store *QualityStore) getIssues(reportID string) ([]QualityIssue, error) {
	rows, err := store.s.DB().Query(`SELECT id, category, severity, stock_code, date, metric, expected, actual, description
		FROM quality_issue WHERE report_id = ?`, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []QualityIssue
	for rows.Next() {
		var issue QualityIssue
		var category, severityStr string
		if err := rows.Scan(&issue.ID, &category, &severityStr, &issue.StockCode,
			&issue.Date, &issue.Metric, &issue.Expected, &issue.Actual, &issue.Description); err != nil {
			return nil, err
		}
		issue.Category = Category(category)
		issue.Severity = Severity(severityStr)
		issues = append(issues, issue)
	}
	return issues, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
