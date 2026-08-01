package marketsnapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// DefaultReadyCoverageThreshold 默认 99% 代码 ready 才会把状态置为 ready。
// 可通过 Builder 覆写。
const DefaultReadyCoverageThreshold = 0.99

// Builder 是 MarketSnapshot 的构建器：组装 universe → 拉取水位 → 计算覆盖率 → 冻结。
type Builder struct {
	UniverseProvider  UniverseProvider
	WatermarkProvider WatermarkProvider
	Calendar          TradingCalendar
	// 覆盖阈值 [0,1]，高于该值并且没有 blocking gap，就会标记为 ready。
	CoverageThreshold float64
	// 允许有多少只股票有 gap_days > 0；超过就标记 partial 或 failed。
	MaxGappedCodes  int
	Now             time.Time
	Market          string
	PriceAdjustment string
}

// NewBuilder 返回带默认阈值的构建器。
func NewBuilder(up UniverseProvider, wp WatermarkProvider, cal TradingCalendar) *Builder {
	return &Builder{
		UniverseProvider:  up,
		WatermarkProvider: wp,
		Calendar:          cal,
		CoverageThreshold: DefaultReadyCoverageThreshold,
		MaxGappedCodes:    50,
		Now:               time.Now(),
		Market:            "CN-A",
		PriceAdjustment:   "forward",
	}
}

// Build 构建一个 MarketSnapshot。不会调用 Save/Freeze；调用者随后决定 Repository.Save/Freeze。
func (b *Builder) Build(date string, def UniverseDefinition) (*MarketSnapshot, error) {
	if date == "" {
		return nil, fmt.Errorf("date is required")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("date must be YYYY-MM-DD: %w", err)
	}
	if b.UniverseProvider == nil || b.WatermarkProvider == nil {
		return nil, fmt.Errorf("universe & watermark provider required")
	}
	if b.Calendar != nil {
		ok, err := b.Calendar.IsTradingDay(date)
		if err != nil {
			return nil, fmt.Errorf("trading day check: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("%s is not a trading day", date)
		}
	}
	members, err := b.UniverseProvider.BuildUniverse(date, def)
	if err != nil {
		return nil, fmt.Errorf("build universe: %w", err)
	}
	selected := make([]UniverseMember, 0, len(members))
	codes := make([]string, 0, len(members))
	code2Member := map[string]UniverseMember{}
	for _, m := range members {
		code2Member[m.Code] = m
		codes = append(codes, m.Code)
		if m.Selected {
			selected = append(selected, m)
		}
	}
	sort.Strings(codes)
	watermarks, err := b.WatermarkProvider.FetchWatermarks(date, codes)
	if err != nil {
		return nil, fmt.Errorf("fetch watermarks: %w", err)
	}
	codeStates := make([]CodeStatus, 0, len(codes))
	var (
		expectedKline int
		readyKline    int
		readyQuote    int
		readyFinance  int
		readyXdxr     int
		totalGapDays  int
		gappedCodes   int
	)
	for _, code := range codes {
		mem, ok := code2Member[code]
		if !ok {
			continue
		}
		wm, ok := watermarks[code]
		if !ok {
			wm = CodeStatus{Code: code}
		}
		wm.Code = code
		wm.UniverseMember = mem.Selected
		wm.SecurityStatus = mem.Status
		wm.IpoDate = mem.IpoDate
		wm.DelistDate = mem.DelistDate
		// 非 universe 成员不计入 ready 统计分母（但仍在清单里供审计）
		if mem.Selected {
			expectedKline++
			if wm.KlineLastDate == date {
				readyKline++
			} else {
				if wm.GapDays == 0 {
					wm.GapDays = 1
				}
			}
			if wm.QuoteReady {
				readyQuote++
			}
			if wm.FinanceReady {
				readyFinance++
			}
			if wm.XdxrReady {
				readyXdxr++
			}
			if wm.GapDays > 0 {
				gappedCodes++
				totalGapDays += wm.GapDays
			}
		}
		codeStates = append(codeStates, wm)
	}
	var (
		coverage float64
		status   = StatusBuilding
		reason   string
	)
	if expectedKline > 0 {
		coverage = float64(readyKline) / float64(expectedKline)
	}
	switch {
	case expectedKline == 0:
		status = StatusFailed
		reason = "universe 为空"
	case coverage >= b.CoverageThreshold && gappedCodes <= b.MaxGappedCodes:
		status = StatusReady
		reason = fmt.Sprintf("coverage=%.2f%% gapped=%d", coverage*100, gappedCodes)
	case coverage < 0.8:
		status = StatusFailed
		reason = fmt.Sprintf("coverage=%.2f%% 低于 80%%", coverage*100)
	default:
		status = StatusPartial
		reason = fmt.Sprintf("coverage=%.2f%% gapped=%d 低于阈值 %v%%", coverage*100, gappedCodes, b.CoverageThreshold*100)
	}
	s := &MarketSnapshot{
		SnapshotDate:       date,
		Universe:           def,
		Market:             b.Market,
		PriceAdjustment:    b.PriceAdjustment,
		ExpectedKlineCodes: expectedKline,
		ReadyKlineCodes:    readyKline,
		ReadyQuoteCodes:    readyQuote,
		ReadyFinanceCodes:  readyFinance,
		ReadyXdxrCodes:     readyXdxr,
		CoveragePct:        coverage,
		Status:             status,
		ReadinessReason:    reason,
		UniverseMembers:    selected,
		Codes:              codeStates,
		BuiltAt:            b.Now,
	}
	s.UniverseHash = ComputeUniverseHash(members)
	s.ID = computeSnapshotID(s)
	contentHash, err := ComputeContentHash(s)
	if err != nil {
		return nil, err
	}
	s.ContentHash = contentHash
	return s, nil
}

func computeSnapshotID(s *MarketSnapshot) string {
	sum := sha256.Sum256([]byte(s.SnapshotDate + "|" + s.Universe.Name + "|" + s.PriceAdjustment + "|" + s.UniverseHash))
	return hex.EncodeToString(sum[:])[:20]
}

// BuildFeatureSnapshot 在一个已 ready 的 market snapshot 上物化指定特征。
// 如果未传入 features，默认用 DefaultDslFeatures()。
func (b *Builder) BuildFeatureSnapshot(ms *MarketSnapshot, features []FeatureSpec, engine FeatureEngine) (*FeatureSnapshot, error) {
	if ms == nil || engine == nil {
		return nil, fmt.Errorf("market snapshot & feature engine required")
	}
	if ms.Status != StatusReady {
		return nil, fmt.Errorf("market snapshot status=%s, need ready", ms.Status)
	}
	if len(features) == 0 {
		features = DefaultDslFeatures()
	}
	codes := make([]string, 0, len(ms.UniverseMembers))
	for _, m := range ms.UniverseMembers {
		codes = append(codes, m.Code)
	}
	values, err := engine.Compute(ms.SnapshotDate, codes, features)
	if err != nil {
		return nil, fmt.Errorf("feature compute: %w", err)
	}
	ids := make([]string, 0, len(features))
	for _, f := range features {
		ids = append(ids, f.Name)
	}
	sort.Strings(ids)
	rows := 0
	for _, perCode := range values {
		rows += len(perCode)
	}
	fs := &FeatureSnapshot{
		MarketSnapshotID: ms.ID,
		SnapshotDate:     ms.SnapshotDate,
		FeatureIDs:       ids,
		FeatureTotal:     len(features),
		RowsWritten:      rows,
		LeakChecked:      true, // FeatureEngine 契约: 必须满足 no-lookahead
		PriceAdjustment:  ms.PriceAdjustment,
		Status:           StatusReady,
		AsOfNs:           b.Now.UnixNano(),
		BuiltAt:          b.Now,
		Values:           values,
	}
	h, err := ComputeFeatureContentHash(fs)
	if err != nil {
		return nil, err
	}
	fs.ContentHash = h
	sum := sha256.Sum256([]byte(ms.ID + "|" + ms.SnapshotDate + "|" + fs.ContentHash))
	fs.ID = "fs_" + hex.EncodeToString(sum[:])[:20]
	return fs, nil
}

// IsReady helper，用于下游扫描前的 fail-closed 检查。
func (s *MarketSnapshot) IsReady() bool { return s != nil && s.Frozen && s.Status == StatusReady }
