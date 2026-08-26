package quality

import (
	"fmt"
	"math"
	"time"
)

// KlineRecord 代表一条 K 线数据, 用于质量检查。
type KlineRecord struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Amount float64
}

// QualityChecker 数据质量检查器。
type QualityChecker struct {
	config QualityGateConfig
}

// NewQualityChecker 创建质量检查器。
func NewQualityChecker(config QualityGateConfig) *QualityChecker {
	return &QualityChecker{config: config}
}

// CheckKline 对 K 线数据执行全面质量检查。
// stockCode: 股票代码
// records: K 线数据 (按日期升序)
// expectedTradingDays: 预期交易日列表 (用于缺失检查)
// asOf: 数据可获得时间
func (qc *QualityChecker) CheckKline(stockCode string, records []KlineRecord, expectedTradingDays []time.Time, asOf time.Time) *QualityReport {
	report := &QualityReport{
		ID:        fmt.Sprintf("qr-%s-%d", stockCode, time.Now().UnixNano()),
		StockCode: stockCode,
		Source:    "kline",
		AsOf:      asOf,
		CreatedAt: time.Now(),
	}

	var issues []QualityIssue
	totalRecords := len(records)
	passedRecords := 0
	failedRecords := 0

	// 1. 检查缺失数据
	missingIssues, _ := qc.checkMissingData(stockCode, records, expectedTradingDays)
	issues = append(issues, missingIssues...)

	// 2. 检查重复记录
	dupIssues, dupCount := qc.checkDuplicates(stockCode, records)
	issues = append(issues, dupIssues...)

	// 3. 检查时间倒序
	timeIssues := qc.checkTimeOrder(stockCode, records)
	issues = append(issues, timeIssues...)

	// 4. 检查异常价格
	priceIssues, badPriceCount := qc.checkAbnormalPrices(stockCode, records)
	issues = append(issues, priceIssues...)

	// 5. 检查异常成交量
	volIssues, badVolCount := qc.checkAbnormalVolume(stockCode, records)
	issues = append(issues, volIssues...)

	// 6. 检查证券池覆盖
	coverageIssues, coveragePct := qc.checkPoolCoverage(stockCode, records, expectedTradingDays)
	issues = append(issues, coverageIssues...)

	// 7. 检查数据源时效性
	recencyIssues := qc.checkSourceRecency(stockCode, records, asOf)
	issues = append(issues, recencyIssues...)

	// 汇总
	failedRecords = badPriceCount + badVolCount + dupCount
	passedRecords = totalRecords - failedRecords
	if passedRecords < 0 {
		passedRecords = 0
	}

	report.Issues = issues
	report.DateRange = DateRangeInfo{}
	if len(records) > 0 {
		report.DateRange.Start = records[0].Date.Format("2006-01-02")
		report.DateRange.End = records[len(records)-1].Date.Format("2006-01-02")
	}

	report.Summary = QualitySummary{
		TotalIssues:     len(issues),
		CriticalCount:   countBySeverity(issues, SeverityCritical),
		WarningCount:    countBySeverity(issues, SeverityWarning),
		InfoCount:       countBySeverity(issues, SeverityInfo),
		CheckedRecords:  totalRecords,
		PassedRecords:   passedRecords,
		FailedRecords:   failedRecords,
		CoveragePercent: coveragePct,
	}

	report.Blocked = report.ShouldBlock(qc.config)
	report.Passed = !report.Blocked

	return report
}

// checkMissingData 检查数据缺失。
func (qc *QualityChecker) checkMissingData(stockCode string, records []KlineRecord, expectedDays []time.Time) ([]QualityIssue, int) {
	var issues []QualityIssue
	if len(expectedDays) == 0 {
		return issues, 0
	}

	recordDates := make(map[string]bool, len(records))
	for _, r := range records {
		recordDates[r.Date.Format("2006-01-02")] = true
	}

	var missingDays int
	var missingDates []string
	for _, day := range expectedDays {
		key := day.Format("2006-01-02")
		if !recordDates[key] {
			missingDays++
			missingDates = append(missingDates, key)
		}
	}

	if missingDays == 0 {
		return issues, 0
	}

	severity := SeverityWarning
	if missingDays > qc.config.MaxMissingDays {
		severity = SeverityCritical
	}

	desc := fmt.Sprintf("Missing %d/%d trading days", missingDays, len(expectedDays))
	if missingDays <= 10 {
		desc += fmt.Sprintf(": %v", missingDates)
	}

	issues = append(issues, NewQualityIssue(
		CategoryMissingData, severity, stockCode, "",
		"coverage",
		fmt.Sprintf("at least %d days", len(expectedDays)-qc.config.MaxMissingDays),
		fmt.Sprintf("%d days missing", missingDays),
		desc,
	))

	return issues, missingDays
}

// checkDuplicates 检查重复记录。
func (qc *QualityChecker) checkDuplicates(stockCode string, records []KlineRecord) ([]QualityIssue, int) {
	var issues []QualityIssue
	seen := make(map[string]int)
	dupCount := 0

	for _, r := range records {
		key := r.Date.Format("2006-01-02")
		seen[key]++
	}

	var dupDates []string
	for date, count := range seen {
		if count > 1 {
			dupCount += count - 1
			dupDates = append(dupDates, date)
		}
	}

	if dupCount > 0 {
		desc := fmt.Sprintf("Found %d duplicate records", dupCount)
		if len(dupDates) <= 5 {
			desc += fmt.Sprintf(": %v", dupDates)
		}
		issues = append(issues, NewQualityIssue(
			CategoryDuplicate, SeverityCritical, stockCode, "",
			"date_uniqueness", "unique dates", fmt.Sprintf("%d duplicates", dupCount),
			desc,
		))
	}

	return issues, dupCount
}

// checkTimeOrder 检查时间倒序。
func (qc *QualityChecker) checkTimeOrder(stockCode string, records []KlineRecord) []QualityIssue {
	var issues []QualityIssue
	for i := 1; i < len(records); i++ {
		if records[i].Date.Before(records[i-1].Date) {
			issues = append(issues, NewQualityIssue(
				CategoryTimeReversal, SeverityCritical, stockCode,
				records[i].Date.Format("2006-01-02"),
				"date_order", "ascending order", "reversed",
				fmt.Sprintf("Date %s is before previous date %s",
					records[i].Date.Format("2006-01-02"),
					records[i-1].Date.Format("2006-01-02")),
			))
		}
	}
	return issues
}

// checkAbnormalPrices 检查异常价格。
func (qc *QualityChecker) checkAbnormalPrices(stockCode string, records []KlineRecord) ([]QualityIssue, int) {
	var issues []QualityIssue
	badCount := 0
	maxPct := qc.config.MaxPriceChangePct

	// 检查基本价格有效性
	for _, r := range records {
		if r.Close <= 0 || math.IsNaN(r.Close) || math.IsInf(r.Close, 0) {
			badCount++
			issues = append(issues, NewQualityIssue(
				CategoryAbnormalPrice, SeverityCritical, stockCode,
				r.Date.Format("2006-01-02"),
				"close", "> 0", fmt.Sprintf("%.2f", r.Close),
				fmt.Sprintf("Invalid close price on %s", r.Date.Format("2006-01-02")),
			))
		}
		if r.High < r.Low {
			badCount++
			issues = append(issues, NewQualityIssue(
				CategoryAbnormalPrice, SeverityCritical, stockCode,
				r.Date.Format("2006-01-02"),
				"ohlc_order", "high >= low", fmt.Sprintf("high=%.2f low=%.2f", r.High, r.Low),
				fmt.Sprintf("High < Low on %s", r.Date.Format("2006-01-02")),
			))
		}
	}

	// 检查价格跳变
	for i := 1; i < len(records); i++ {
		prev := records[i-1]
		curr := records[i]
		if prev.Close <= 0 {
			continue
		}
		pctChange := math.Abs(curr.Close-prev.Close) / prev.Close * 100
		if pctChange > maxPct*100 {
			badCount++
			issues = append(issues, NewQualityIssue(
				CategoryAbnormalPrice, SeverityWarning, stockCode,
				curr.Date.Format("2006-01-02"),
				"price_change",
				fmt.Sprintf("< %.0f%%", maxPct*100),
				fmt.Sprintf("%.1f%%", pctChange),
				fmt.Sprintf("Price jumped %.1f%% from %s (%.2f) to %s (%.2f)",
					pctChange, prev.Date.Format("2006-01-02"), prev.Close,
					curr.Date.Format("2006-01-02"), curr.Close),
			))
		}
	}

	return issues, badCount
}

// checkAbnormalVolume 检查异常成交量。
func (qc *QualityChecker) checkAbnormalVolume(stockCode string, records []KlineRecord) ([]QualityIssue, int) {
	var issues []QualityIssue
	badCount := 0
	maxRatio := qc.config.MaxVolumeRatio

	// 计算成交量均值 (排除最近 5 天)
	if len(records) < 10 {
		return issues, 0
	}

	avgVol := 0.0
	for i := 0; i < len(records)-5; i++ {
		avgVol += records[i].Volume
	}
	avgVol /= float64(len(records) - 5)

	if avgVol <= 0 {
		return issues, 0
	}

	// 检查最近 5 天的成交量异常
	for i := len(records) - 5; i < len(records); i++ {
		r := records[i]
		ratio := r.Volume / avgVol
		if ratio > maxRatio {
			badCount++
			issues = append(issues, NewQualityIssue(
				CategoryAbnormalVolume, SeverityWarning, stockCode,
				r.Date.Format("2006-01-02"),
				"volume_ratio",
				fmt.Sprintf("< %.0fx average", maxRatio),
				fmt.Sprintf("%.1fx", ratio),
				fmt.Sprintf("Volume is %.1fx the %d-day average on %s",
					ratio, len(records)-5, r.Date.Format("2006-01-02")),
			))
		}
	}

	return issues, badCount
}

// checkPoolCoverage 检查证券池覆盖。
func (qc *QualityChecker) checkPoolCoverage(stockCode string, records []KlineRecord, expectedDays []time.Time) ([]QualityIssue, float64) {
	var issues []QualityIssue
	if len(expectedDays) == 0 {
		return issues, 100.0
	}

	recordDates := make(map[string]bool, len(records))
	for _, r := range records {
		recordDates[r.Date.Format("2006-01-02")] = true
	}

	coveredDays := 0
	for _, day := range expectedDays {
		if recordDates[day.Format("2006-01-02")] {
			coveredDays++
		}
	}

	coveragePct := float64(coveredDays) / float64(len(expectedDays)) * 100

	if coveragePct < qc.config.MinCoveragePercent {
		severity := SeverityWarning
		if coveragePct < 50.0 {
			severity = SeverityCritical
		}
		issues = append(issues, NewQualityIssue(
			CategoryPoolCoverage, severity, stockCode, "",
			"coverage",
			fmt.Sprintf(">= %.0f%%", qc.config.MinCoveragePercent),
			fmt.Sprintf("%.1f%%", coveragePct),
			fmt.Sprintf("Pool coverage is %.1f%% (%d/%d days)",
				coveragePct, coveredDays, len(expectedDays)),
		))
	}

	return issues, coveragePct
}

// checkSourceRecency 检查数据源时效性。
func (qc *QualityChecker) checkSourceRecency(stockCode string, records []KlineRecord, asOf time.Time) []QualityIssue {
	var issues []QualityIssue
	if len(records) == 0 {
		return issues
	}

	lastRecord := records[len(records)-1]
	lag := asOf.Sub(lastRecord.Date)

	// 如果最后一条记录距 asOf 超过 2 个交易日 (约 14 天)
	maxLag := time.Duration(qc.config.MaxMissingDays*2) * 24 * time.Hour
	if lag > maxLag {
		severity := SeverityWarning
		if lag > maxLag*2 {
			severity = SeverityCritical
		}
		issues = append(issues, NewQualityIssue(
			CategorySourceDegradation, severity, stockCode,
			lastRecord.Date.Format("2006-01-02"),
			"data_lag",
			fmt.Sprintf("<= %d days", qc.config.MaxMissingDays*2),
			fmt.Sprintf("%.0f days", lag.Hours()/24),
			fmt.Sprintf("Data is %.0f days behind as-of time", lag.Hours()/24),
		))
	}

	return issues
}

// CheckFinancialLag 检查财务数据滞后。
// 独立方法, 因为财务数据是独立的数据源。
func (qc *QualityChecker) CheckFinancialLag(stockCode string, latestReportDate time.Time, asOf time.Time) QualityIssue {
	lag := asOf.Sub(latestReportDate)
	maxLag := time.Duration(qc.config.MaxFinancialLagDays) * 24 * time.Hour

	severity := SeverityInfo
	if lag > maxLag {
		severity = SeverityCritical
	} else if lag > maxLag/2 {
		severity = SeverityWarning
	}

	return NewQualityIssue(
		CategoryFinancialLag, severity, stockCode,
		latestReportDate.Format("2006-01-02"),
		"financial_lag",
		fmt.Sprintf("<= %d days", qc.config.MaxFinancialLagDays),
		fmt.Sprintf("%.0f days", lag.Hours()/24),
		fmt.Sprintf("Financial data is %.0f days old (max %d days)",
			lag.Hours()/24, qc.config.MaxFinancialLagDays),
	)
}

func countBySeverity(issues []QualityIssue, s Severity) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == s {
			count++
		}
	}
	return count
}
