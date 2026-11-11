package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

const (
	ObservationEvaluationVersion = "selection-observation-v1"
	ObservationOngoing           = "ongoing"
	ObservationHit               = "hit"
	ObservationMiss              = "expired"
	ObservationInsufficient      = "insufficient_data"
	ObservationUndefined         = "target_undefined"
)

func observationLocation() *time.Location {
	if location, err := time.LoadLocation("Asia/Shanghai"); err == nil && location != nil {
		return location
	}
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

type ObservationContract struct {
	ID                 string                          `json:"id"`
	SelectionRunID     string                          `json:"selection_run_id"`
	Code               string                          `json:"code"`
	Name               string                          `json:"name"`
	MethodID           string                          `json:"method_id"`
	MethodVersionID    string                          `json:"method_version_id"`
	MethodName         string                          `json:"method_name"`
	SignalDate         string                          `json:"signal_date"`
	Baseline           float64                         `json:"baseline"`
	BaselineKind       string                          `json:"baseline_kind"`
	SnapshotID         string                          `json:"snapshot_id"`
	FeatureSnapshotID  string                          `json:"feature_snapshot_id"`
	FeatureHash        string                          `json:"feature_hash"`
	SnapshotAdjustment string                          `json:"snapshot_adjustment"`
	Outcome            methods.OutcomeRule             `json:"outcome"`
	Rules              *methods.CompiledMethod         `json:"rules"`
	Evidence           *methodregistry.EvidenceSummary `json:"evidence,omitempty"`
	CreatedAt          time.Time                       `json:"created_at"`
	ContractHash       string                          `json:"contract_hash"`
}
type ObservationBar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}
type ObservationEvaluation struct {
	EvaluationVersion string           `json:"evaluation_version"`
	Status            string           `json:"status"`
	Reason            string           `json:"reason"`
	AsOf              string           `json:"as_of"`
	DataAsOf          string           `json:"data_as_of,omitempty"`
	Progress          int              `json:"progress"`
	Horizon           int              `json:"horizon"`
	Target            *float64         `json:"target,omitempty"`
	HitDate           string           `json:"hit_date,omitempty"`
	Deadline          string           `json:"deadline,omitempty"`
	LatestClose       *float64         `json:"latest_close,omitempty"`
	ReturnPct         *float64         `json:"return_pct,omitempty"`
	Complete          bool             `json:"complete"`
	MissingDates      []string         `json:"missing_dates,omitempty"`
	DataHash          string           `json:"data_hash"`
	EvaluatedAt       time.Time        `json:"evaluated_at"`
	Bars              []ObservationBar `json:"bars,omitempty"`
}
type Observation struct {
	Contract   ObservationContract    `json:"contract"`
	Evaluation *ObservationEvaluation `json:"evaluation,omitempty"`
}
type observationSelections interface {
	Get(context.Context, string, string) (*selection.Run, error)
}
type observationMethods interface {
	Get(context.Context, string) (*methodregistry.Method, error)
}
type observationSnapshots interface {
	LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error)
}
type ObservationService struct {
	db         *sql.DB
	selections observationSelections
	methods    observationMethods
	snapshots  observationSnapshots
	now        func() time.Time
}

func NewObservationService(store *storage.Storage, selections observationSelections, registry observationMethods, snapshots observationSnapshots) (*ObservationService, error) {
	if store == nil || selections == nil || registry == nil || snapshots == nil {
		return nil, fmt.Errorf("observation dependencies required")
	}
	return &ObservationService{db: store.DB(), selections: selections, methods: registry, snapshots: snapshots, now: time.Now}, nil
}
func observationHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *ObservationService) Create(ctx context.Context, runID, code, methodID string) (*Observation, error) {
	run, err := s.selections.Get(ctx, runID, "")
	if err != nil {
		return nil, err
	}
	var candidate *selection.Candidate
	for i := range run.Candidates {
		if run.Candidates[i].Code == code {
			candidate = &run.Candidates[i]
			break
		}
	}
	if candidate == nil {
		return nil, fmt.Errorf("股票不属于该选股运行")
	}
	var trigger *selection.Trigger
	for i := range candidate.Triggers {
		if candidate.Triggers[i].MethodID == methodID || methodID == "" && i == 0 {
			trigger = &candidate.Triggers[i]
			break
		}
	}
	if trigger == nil || trigger.MethodVersionID == "" {
		return nil, fmt.Errorf("该候选未保留指定方法的精确版本")
	}
	id := "ob-" + observationHash([]string{run.ID, code, trigger.MethodVersionID})[:24]
	if found, getErr := s.Get(ctx, id, false); getErr == nil {
		return found, nil
	}
	method, err := s.methods.Get(ctx, trigger.MethodID)
	if err != nil {
		return nil, err
	}
	var version *methodregistry.MethodVersion
	for i := range method.Versions {
		if method.Versions[i].ID == trigger.MethodVersionID {
			version = &method.Versions[i]
			break
		}
	}
	if version == nil || version.Method == nil {
		return nil, fmt.Errorf("触发时的方法版本已不可用，不能使用当前规则替代")
	}
	f, err := s.snapshots.LoadFeatureSnapshot(run.FeatureSnapshotID, true)
	if err != nil {
		return nil, err
	}
	if f == nil || f.ID != run.FeatureSnapshotID || f.MarketSnapshotID != run.SnapshotID || f.SnapshotDate != run.SnapshotDate || f.Status != marketsnapshot.StatusReady || !f.LeakChecked {
		return nil, fmt.Errorf("冻结特征快照与选股运行不一致")
	}
	hash, err := marketsnapshot.ComputeFeatureContentHash(f)
	if err != nil || hash != f.ContentHash {
		return nil, fmt.Errorf("特征快照哈希校验失败")
	}
	reference := selection.ReferencePriceFromFeature(f, code)
	if reference == nil {
		return nil, fmt.Errorf("没有可信的信号日收盘基准")
	}
	name := code
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM stockinfo WHERE code=?`, code).Scan(&name)
	if strings.TrimSpace(name) == "" {
		name = code
	}
	contract := ObservationContract{ID: id, SelectionRunID: run.ID, Code: code, Name: name, MethodID: trigger.MethodID, MethodVersionID: trigger.MethodVersionID, MethodName: trigger.MethodName, SignalDate: run.SnapshotDate, Baseline: reference.Close, BaselineKind: "signal_close", SnapshotID: run.SnapshotID, FeatureSnapshotID: run.FeatureSnapshotID, FeatureHash: f.ContentHash, SnapshotAdjustment: f.PriceAdjustment, Outcome: version.Method.Outcome, Rules: version.Method, Evidence: trigger.Evidence, CreatedAt: s.now().UTC()}
	contract.ContractHash = observationHash(contract)
	raw, err := json.Marshal(contract)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO forward_observation(id,selection_run_id,stock_code,method_version_id,created_at_ns,contract_hash,contract_json) VALUES(?,?,?,?,?,?,?)`, id, run.ID, code, trigger.MethodVersionID, contract.CreatedAt.UnixNano(), contract.ContractHash, string(raw))
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id, false)
}

// latestEvaluationJoin picks only the newest evaluation row per observation; all
// list/summary/refresh reads share it so a record is never read twice.
const latestEvaluationJoin = ` LEFT JOIN forward_observation_evaluation e ON e.id=(SELECT MAX(x.id) FROM forward_observation_evaluation x WHERE x.observation_id=o.id)`

// decodeObservation verifies the frozen contract before handing it out. Bars are
// stripped for list reads; rules AST is only exposed by the detail endpoint.
func decodeObservation(contractJSON, contractHash string, evaluationJSON sql.NullString, withBars bool) (*Observation, error) {
	var contract ObservationContract
	if err := json.Unmarshal([]byte(contractJSON), &contract); err != nil {
		return nil, err
	}
	saved := contract.ContractHash
	contract.ContractHash = ""
	if observationHash(contract) != contractHash || saved != contractHash {
		return nil, fmt.Errorf("observation contract hash mismatch")
	}
	contract.ContractHash = saved
	result := &Observation{Contract: contract}
	if !evaluationJSON.Valid {
		return result, nil
	}
	var evaluation ObservationEvaluation
	if err := json.Unmarshal([]byte(evaluationJSON.String), &evaluation); err != nil {
		return nil, err
	}
	if !withBars {
		evaluation.Bars = nil
	}
	result.Evaluation = &evaluation
	return result, nil
}

func (s *ObservationService) Get(ctx context.Context, id string, withBars bool) (*Observation, error) {
	var contractJSON, contractHash string
	var evaluationJSON sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT o.contract_json,o.contract_hash,e.result_json FROM forward_observation o`+latestEvaluationJoin+` WHERE o.id=?`, id).Scan(&contractJSON, &contractHash, &evaluationJSON); err != nil {
		return nil, err
	}
	return decodeObservation(contractJSON, contractHash, evaluationJSON, withBars)
}
func (s *ObservationService) List(ctx context.Context, limit, offset int, statuses ...string) ([]Observation, int, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	where := ""
	args := []any{}
	if len(statuses) > 0 && statuses[0] != "" {
		where = ` WHERE COALESCE(e.status,'ongoing')=?`
		args = append(args, statuses[0])
	}
	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT o.contract_json,o.contract_hash,e.result_json FROM forward_observation o`+latestEvaluationJoin+where+` ORDER BY o.created_at_ns DESC,o.id LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	results := []Observation{}
	for rows.Next() {
		var contractJSON, contractHash string
		var evaluationJSON sql.NullString
		if err = rows.Scan(&contractJSON, &contractHash, &evaluationJSON); err != nil {
			rows.Close()
			return nil, 0, err
		}
		item, decodeErr := decodeObservation(contractJSON, contractHash, evaluationJSON, false)
		if decodeErr != nil {
			// A corrupt row must not make an otherwise readable page fail.
			continue
		}
		item.Contract.Rules = nil
		results = append(results, *item)
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return nil, 0, scanErr
	}
	var total int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM forward_observation o`+where, args...).Scan(&total)
	return results, total, err
}

// EvaluateObservation evaluates an immutable target against complete market sessions.
// Missing stock bars never shorten the market horizon or become automatic failures.
func EvaluateObservation(c ObservationContract, dates []string, bars []ObservationBar, corporateKnown bool, eventDates []string, now time.Time) ObservationEvaluation {
	r := ObservationEvaluation{EvaluationVersion: ObservationEvaluationVersion, Status: ObservationOngoing, Reason: "等待后续完整交易日", Horizon: c.Outcome.HorizonDays, EvaluatedAt: now.UTC(), MissingDates: []string{}}
	location := observationLocation()
	local := now.In(location)
	end := local.Format("2006-01-02")
	if local.Hour() < 15 {
		end = local.AddDate(0, 0, -1).Format("2006-01-02")
	}
	byDate := map[string]ObservationBar{}
	for _, b := range bars {
		if b.Date >= c.SignalDate && b.Date <= end {
			byDate[b.Date] = b
		}
	}
	anchor, hasAnchor := byDate[c.SignalDate]
	if hasAnchor {
		r.Bars = append(r.Bars, anchor)
	}
	sortedDates := append([]string{}, dates...)
	sort.Strings(sortedDates)
	seenDates := map[string]bool{}
	windowLimit := r.Horizon
	if windowLimit < 1 || windowLimit > 252 {
		windowLimit = 252
	}
	validDates := []string{}
	for _, date := range sortedDates {
		if seenDates[date] {
			continue
		}
		seenDates[date] = true
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		if date > c.SignalDate && date <= end {
			validDates = append(validDates, date)
			if len(validDates) >= windowLimit {
				break
			}
		}
	}
	for _, date := range validDates {
		r.AsOf = date
		if b, ok := byDate[date]; ok && validObservationBar(b) {
			r.Progress++
			r.Bars = append(r.Bars, b)
			r.DataAsOf = date
			close := b.Close
			ret := close/c.Baseline - 1
			r.LatestClose = &close
			r.ReturnPct = &ret
		} else {
			r.MissingDates = append(r.MissingDates, date)
		}
	}
	if len(validDates) >= r.Horizon && r.Horizon > 0 {
		r.Deadline = validDates[r.Horizon-1]
	}
	finish := func() ObservationEvaluation {
		r.DataHash = observationHash(struct {
			Contract string
			Version  string
			Dates    []string
			Bars     []ObservationBar
			Known    bool
			Events   []string
			Status   string
			AsOf     string
		}{c.ContractHash, ObservationEvaluationVersion, validDates, r.Bars, corporateKnown, eventDates, r.Status, r.AsOf})
		return r
	}
	if c.Outcome.TargetReturnPct == nil || r.Horizon <= 0 || r.Horizon > 252 {
		r.Status = ObservationUndefined
		r.Reason = "该方法版本编译出的 outcome 没有冻结目标（未定义止盈阈值或持有期限），不从历史胜率推断"
		return finish()
	}
	basis := c.Outcome.PriceBasis
	if basis != "close" && basis != "open" && basis != "high" && basis != "low" {
		r.Status = ObservationUndefined
		r.Reason = "目标价格类型未定义"
		return finish()
	}
	if c.Outcome.Success != "close_gte_target" && c.Outcome.Success != "close_lte_target" && c.Outcome.Success != "price_gte_target" && c.Outcome.Success != "price_lte_target" {
		r.Status = ObservationUndefined
		r.Reason = "目标比较方式未定义"
		return finish()
	}
	if strings.HasPrefix(c.Outcome.Success, "close_") && basis != "close" {
		r.Status = ObservationUndefined
		r.Reason = "close 比较方式必须使用 close 价格类型"
		return finish()
	}
	if !hasAnchor || anchor.Close <= 0 || c.Baseline <= 0 || math.Abs(anchor.Close-c.Baseline) > math.Max(1e-6, c.Baseline*1e-6) {
		r.Status = ObservationInsufficient
		r.Reason = "锚日原始收盘价缺失或与冻结基准不一致，不能混用复权价格"
		return finish()
	}
	if len(validDates) > 0 && !corporateKnown {
		r.Status = ObservationInsufficient
		r.Reason = "本地公司行为资料未覆盖观察区间，无法确认价格口径；请同步除权资料后刷新"
		return finish()
	}
	for _, date := range eventDates {
		if date > c.SignalDate && date <= r.AsOf {
			r.Status = ObservationInsufficient
			r.Reason = "区间存在公司行为，尚未对齐到冻结锚价的复权口径"
			return finish()
		}
	}
	target := c.Baseline * (1 + *c.Outcome.TargetReturnPct)
	r.Target = &target
	if target <= 0 || math.IsNaN(target) || math.IsInf(target, 0) {
		r.Status = ObservationUndefined
		r.Reason = "目标阈值无效"
		return finish()
	}
	for _, date := range validDates {
		b, ok := byDate[date]
		if !ok || !validObservationBar(b) {
			continue
		}
		value := b.Close
		switch basis {
		case "open":
			value = b.Open
		case "high":
			value = b.High
		case "low":
			value = b.Low
		}
		hit := value >= target
		if c.Outcome.Success == "price_lte_target" || c.Outcome.Success == "close_lte_target" {
			hit = value > 0 && value <= target
		}
		if hit && value > 0 {
			r.Status = ObservationHit
			r.HitDate = date
			r.Complete = len(r.MissingDates) == 0 && r.Deadline != ""
			r.Reason = "后续完整交易日的指定价格已达到冻结目标"
			if len(r.MissingDates) > 0 {
				r.Reason += "；区间仍有缺口，不能证明这是首次命中，不计入完整数据验证率"
			}
			return finish()
		}
	}
	if len(r.MissingDates) > 0 {
		r.Status = ObservationInsufficient
		r.Reason = "观察窗口存在缺失或停牌日线，不能判为未达标"
		return finish()
	}
	if r.Deadline != "" {
		r.Status = ObservationMiss
		r.Complete = true
		r.Reason = "未来H个完整市场交易日数据齐全，指定价格未达到目标"
	} else if len(validDates) > 0 {
		r.Reason = "本地尚未获得H个后续完整市场交易日，继续观察"
	}
	return finish()
}

func compactObservationDate(date string) string { return strings.ReplaceAll(date, "-", "") }

func (s *ObservationService) refreshOne(ctx context.Context, id string) error {
	observation, err := s.Get(ctx, id, false)
	if err != nil {
		return err
	}
	c := observation.Contract
	now := s.now()
	location := observationLocation()
	local := now.In(location)
	// A post-15:00 clock alone does not prove today's cached bar was refreshed after close.
	closeAt := time.Date(local.Year(), local.Month(), local.Day(), 15, 0, 0, 0, location)
	var synced int64
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(ts) FROM (SELECT last_sync_at ts FROM kline_sync_state WHERE code=? AND ktype=9 UNION ALL SELECT last_sync_at ts FROM data_sync_state WHERE code=? AND data_type='kline')`, c.Code, c.Code).Scan(&synced)
	evaluationTime := now
	if local.Hour() >= 15 && synced < closeAt.Unix() {
		evaluationTime = closeAt.Add(-time.Minute)
	}
	last := evaluationTime.In(location)
	end := last.Format("2006-01-02")
	if last.Hour() < 15 {
		end = last.AddDate(0, 0, -1).Format("2006-01-02")
	}
	// Real market dates, never a weekday approximation. kline is immutable and indexed by date.
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT date FROM kline WHERE ktype=9 AND date>? AND date<=? ORDER BY date LIMIT 252`, compactObservationDate(c.SignalDate), compactObservationDate(end))
	if err != nil {
		return err
	}
	dates := []string{}
	for rows.Next() {
		var date string
		if err = rows.Scan(&date); err != nil {
			rows.Close()
			return err
		}
		if len(date) == 8 {
			dates = append(dates, date[:4]+"-"+date[4:6]+"-"+date[6:])
		}
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	windowEnd := end
	if c.Outcome.HorizonDays > 0 && len(dates) >= c.Outcome.HorizonDays {
		windowEnd = dates[c.Outcome.HorizonDays-1]
	}
	rows, err = s.db.QueryContext(ctx, `SELECT date,open,high,low,close,volume FROM kline_latest WHERE code=? AND ktype=9 AND date>=? AND date<=? ORDER BY date LIMIT 253`, c.Code, compactObservationDate(c.SignalDate), compactObservationDate(windowEnd))
	if err != nil {
		return err
	}
	bars := []ObservationBar{}
	for rows.Next() {
		var b ObservationBar
		var date string
		if err = rows.Scan(&date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume); err != nil {
			rows.Close()
			return err
		}
		if len(date) == 8 {
			b.Date = date[:4] + "-" + date[4:6] + "-" + date[6:]
			bars = append(bars, b)
		}
	}
	scanErr = rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	var corporateRaw []byte
	var corporateAt, corporateExpires int64
	coverageErr := s.db.QueryRowContext(ctx, `SELECT value,created_at,expires_at FROM cache WHERE bucket='xdxr' AND key=?`, c.Code).Scan(&corporateRaw, &corporateAt, &corporateExpires)
	var corporateItems []struct {
		Date time.Time `json:"Date"`
	}
	// An expired cache row is stale data, not evidence that the window is covered.
	corporateKnown := coverageErr == nil && (corporateExpires <= 0 || corporateExpires >= s.now().Unix()) && json.Unmarshal(corporateRaw, &corporateItems) == nil
	coveredDate, parseErr := time.ParseInLocation("2006-01-02", windowEnd, location)
	if parseErr != nil || corporateAt < coveredDate.Add(15*time.Hour).Unix() {
		corporateKnown = false
	}
	eventDates := []string{}
	for _, item := range corporateItems {
		date := item.Date.In(location).Format("2006-01-02")
		if date > c.SignalDate && date <= windowEnd {
			eventDates = append(eventDates, date)
		}
	}
	rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT date FROM xdxr_event WHERE code=? AND date>? AND date<=? ORDER BY date`, c.Code, c.SignalDate, windowEnd)
	if err != nil {
		return err
	}
	for rows.Next() {
		var date string
		if err = rows.Scan(&date); err != nil {
			rows.Close()
			return err
		}
		eventDates = append(eventDates, date)
	}
	scanErr = rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	result := EvaluateObservation(c, dates, bars, corporateKnown, eventDates, evaluationTime)
	result.EvaluatedAt = now.UTC()
	if evaluationTime != now {
		result.Reason += "；今日K线未确认收盘后同步，不参与本次验证"
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO forward_observation_evaluation(observation_id,data_hash,evaluated_at_ns,status,result_json) VALUES(?,?,?,?,?)`, id, result.DataHash, now.UnixNano(), result.Status, string(raw))
	return err
}

// refreshableObservationFilter skips records that can no longer change: a frozen
// target with no outcome rule never gains one, and a finished window that already
// reached a conclusion keeps it. Everything else is re-evaluated from local bars.
const refreshableObservationFilter = `
 WHERE COALESCE(e.status,'ongoing') <> 'target_undefined'
   AND NOT (COALESCE(e.status,'ongoing') IN ('hit','expired')
            AND e.result_json IS NOT NULL
            AND json_valid(e.result_json)
            AND json_extract(e.result_json,'$.complete') = 1)`

// Refresh only reads already synchronized local prices and corporate-action cache.
// One unreadable contract must not stop the rest of the batch: failures are counted
// and skipped, and the whole pass is bounded by the caller's context deadline.
func (s *ObservationService) Refresh(ctx context.Context, id string, codes []string) (int, int, error) {
	if id != "" {
		if err := s.refreshOne(ctx, id); err != nil {
			return 0, 1, err
		}
		return 1, 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.id,o.stock_code FROM forward_observation o`+latestEvaluationJoin+refreshableObservationFilter+` ORDER BY o.created_at_ns DESC,o.id`)
	if err != nil {
		return 0, 0, err
	}
	selected := map[string]bool{}
	for _, code := range codes {
		selected[code] = true
	}
	type pendingObservation struct{ id, code string }
	pending := []pendingObservation{}
	for rows.Next() {
		var current pendingObservation
		if err = rows.Scan(&current.id, &current.code); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if len(selected) == 0 || selected[current.code] {
			pending = append(pending, current)
		}
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return 0, 0, scanErr
	}
	refreshed, failed := 0, 0
	for _, current := range pending {
		if err := ctx.Err(); err != nil {
			return refreshed, failed, err
		}
		if err := s.refreshOne(ctx, current.id); err != nil {
			failed++
			continue
		}
		refreshed++
	}
	return refreshed, failed, nil
}
func (s *ObservationService) Summary(ctx context.Context) (map[string]int, error) {
	result := map[string]int{"total": 0, "concluded_complete": 0, "hit_complete": 0}
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(e.status,'ongoing') AS status, COUNT(*), SUM(CASE WHEN e.result_json IS NOT NULL AND json_valid(e.result_json)=0 THEN 1 ELSE 0 END) FROM forward_observation o`+latestEvaluationJoin+` GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count, corrupt int
		if err = rows.Scan(&status, &count, &corrupt); err != nil {
			rows.Close()
			return nil, err
		}
		result["total"] += count
		result[status] += count
		result["corrupt"] += corrupt
	}
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	// Only concluded records carry the "complete window" flag, so decode just those
	// instead of every evaluation row on each page poll.
	rows, err = s.db.QueryContext(ctx, `SELECT e.status,e.result_json FROM forward_observation o`+latestEvaluationJoin+` WHERE e.status IN ('hit','expired') AND e.result_json IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status, raw string
		if err = rows.Scan(&status, &raw); err != nil {
			return nil, err
		}
		var value ObservationEvaluation
		if json.Unmarshal([]byte(raw), &value) != nil {
			continue
		}
		if value.Complete && value.Deadline != "" {
			result["concluded_complete"]++
			if status == ObservationHit {
				result["hit_complete"]++
			}
		}
	}
	return result, rows.Err()
}

func (s *ObservationService) Codes(ctx context.Context, id string, filter []string) ([]string, error) {
	query := `SELECT DISTINCT stock_code FROM forward_observation`
	args := []any{}
	if id != "" {
		query += ` WHERE id=?`
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowed := map[string]bool{}
	for _, code := range filter {
		allowed[code] = true
	}
	result := []string{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			return nil, err
		}
		if len(allowed) == 0 || allowed[code] {
			result = append(result, code)
		}
	}
	return result, rows.Err()
}
func (s *ObservationService) CorporateRefreshNeeded(ctx context.Context, code string) bool {
	var created, expires int64
	if s.db.QueryRowContext(ctx, `SELECT created_at,expires_at FROM cache WHERE bucket='xdxr' AND key=?`, code).Scan(&created, &expires) != nil {
		return true
	}
	now := s.now()
	if expires > 0 && expires < now.Unix() {
		return true
	}
	// Corporate actions rarely change intraday; refresh once per completed session
	// instead of every scheduler tick. Holidays only cost one redundant fetch.
	return created < observationCorporateCutoff(now).Unix()
}

// observationCorporateCutoff is the most recent week-day 15:00 close before now.
func observationCorporateCutoff(now time.Time) time.Time {
	local := now.In(observationLocation())
	cutoff := time.Date(local.Year(), local.Month(), local.Day(), 15, 0, 0, 0, observationLocation())
	if !local.After(cutoff) {
		cutoff = cutoff.AddDate(0, 0, -1)
	}
	for cutoff.Weekday() == time.Saturday || cutoff.Weekday() == time.Sunday {
		cutoff = cutoff.AddDate(0, 0, -1)
	}
	return cutoff
}

func validObservationBar(b ObservationBar) bool {
	for _, v := range []float64{b.Open, b.High, b.Low, b.Close, b.Volume} {
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return b.High >= math.Max(b.Open, b.Close) && b.Low <= math.Min(b.Open, b.Close) && b.High >= b.Low
}
