package ledger

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"github.com/sjzsdu/tongstock/internal/methods"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

func observationFixture() ObservationContract {
	target := .2
	return ObservationContract{ID: "o", ContractHash: "c", SignalDate: "2026-11-02", Baseline: 10, Outcome: methods.OutcomeRule{HorizonDays: 2, TargetReturnPct: &target, PriceBasis: "close", Success: "close_gte_target"}}
}
func observationTime(date string) time.Time {
	location, _ := time.LoadLocation("Asia/Shanghai")
	value, _ := time.ParseInLocation("2006-01-02 15:04", date, location)
	return value
}
func observationBars() []ObservationBar {
	return []ObservationBar{{Date: "2026-11-02", Open: 10, High: 20, Low: 9, Close: 10, Volume: 1}, {Date: "2026-11-03", Open: 10, High: 13, Low: 9, Close: 11, Volume: 1}, {Date: "2026-11-04", Open: 11, High: 12, Low: 10, Close: 12, Volume: 1}}
}
func TestObservationHorizonAndPriceBasis(t *testing.T) {
	c := observationFixture()
	dates := []string{"2026-11-03", "2026-11-04", "2026-11-05"}
	bars := observationBars()
	before := EvaluateObservation(c, dates, bars, true, nil, observationTime("2026-11-04 14:55"))
	if before.Status != ObservationOngoing || before.Progress != 1 {
		t.Fatalf("unfinished close counted %+v", before)
	}
	after := EvaluateObservation(c, dates, bars, true, nil, observationTime("2026-11-04 15:30"))
	if after.Status != ObservationHit || after.HitDate != "2026-11-04" || !after.Complete {
		t.Fatalf("H-th day close must count %+v", after)
	}
	c.Outcome.PriceBasis = "high"
	c.Outcome.Success = "price_gte_target"
	early := EvaluateObservation(c, dates, bars, true, nil, observationTime("2026-11-03 16:00"))
	if early.Status != ObservationHit || early.HitDate != "2026-11-03" || early.Complete {
		t.Fatalf("early high hit should not be mature sample %+v", early)
	}
	c.Outcome.PriceBasis = "close"
	bars[2].Close = 11
	miss := EvaluateObservation(c, dates, bars, true, nil, observationTime("2026-11-05 16:00"))
	if miss.Status != ObservationMiss || !miss.Complete {
		t.Fatalf("complete window must miss %+v", miss)
	}
}
func TestObservationGapsAndAdjustmentAreNotFailures(t *testing.T) {
	c := observationFixture()
	dates := []string{"2026-11-03", "2026-11-04"}
	bars := observationBars()
	cases := []struct {
		name   string
		bars   []ObservationBar
		known  bool
		events []string
		status string
	}{{"missing", bars[:2], true, nil, ObservationInsufficient}, {"unknown-corporate", bars, false, nil, ObservationInsufficient}, {"corporate-action", bars, true, []string{"2026-11-03"}, ObservationInsufficient}, {"later-hit-gap", []ObservationBar{bars[0], bars[2]}, true, nil, ObservationHit}}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			r := EvaluateObservation(c, dates, item.bars, item.known, item.events, observationTime("2026-11-04 16:00"))
			if r.Status != item.status || r.Complete {
				t.Fatalf("result %+v", r)
			}
		})
	}
	bars[0].Close = 9
	mismatch := EvaluateObservation(c, dates, bars, true, nil, observationTime("2026-11-04 16:00"))
	if mismatch.Status != ObservationInsufficient {
		t.Fatal("mixed price bases accepted")
	}
}
func TestObservationUndefinedAndSameDayAndIdempotentData(t *testing.T) {
	c := observationFixture()
	bars := observationBars()
	same := EvaluateObservation(c, []string{"2026-11-02"}, bars, false, nil, observationTime("2026-11-02 16:00"))
	if same.Status != ObservationOngoing || same.HitDate != "" {
		t.Fatalf("signal day must not count %+v", same)
	}
	a := EvaluateObservation(c, []string{"2026-11-03"}, bars, true, nil, observationTime("2026-11-03 16:00"))
	b := EvaluateObservation(c, []string{"2026-11-03"}, bars, true, nil, observationTime("2026-11-03 17:00"))
	if a.DataHash != b.DataHash {
		t.Fatal("clock made duplicate evaluations")
	}
	c.Outcome.TargetReturnPct = nil
	if r := EvaluateObservation(c, nil, bars, true, nil, observationTime("2026-11-04 16:00")); r.Status != ObservationUndefined {
		t.Fatal("undefined target inferred from other fields")
	}
}

type observationRunFixture struct{ run *selection.Run }

func (f observationRunFixture) Get(context.Context, string, string) (*selection.Run, error) {
	return f.run, nil
}

type observationMethodFixture struct{ method *methodregistry.Method }

func (f observationMethodFixture) Get(context.Context, string) (*methodregistry.Method, error) {
	return f.method, nil
}

type observationFeatureFixture struct {
	feature *marketsnapshot.FeatureSnapshot
}

func (f observationFeatureFixture) LoadFeatureSnapshot(string, bool) (*marketsnapshot.FeatureSnapshot, error) {
	return f.feature, nil
}
func TestObservationPersistsExactVersionDeduplicatesAndSurvivesRestart(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "observations.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(); err != nil {
		t.Fatal(err)
	}
	f := &marketsnapshot.FeatureSnapshot{ID: "f", MarketSnapshotID: "m", SnapshotDate: "2026-11-02", Status: marketsnapshot.StatusReady, LeakChecked: true, FeatureIDs: []string{"close"}, PriceAdjustment: "forward", Values: map[string]map[string]float64{"600000": {"close": 10}}}
	f.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(f)
	c := observationFixture()
	newTarget := .9
	method := &methodregistry.Method{ID: "method", Versions: []methodregistry.MethodVersion{{ID: "v-old", Method: &methods.CompiledMethod{Outcome: c.Outcome}}, {ID: "v-new", Method: &methods.CompiledMethod{Outcome: methods.OutcomeRule{HorizonDays: 9, TargetReturnPct: &newTarget}}}}}
	run := &selection.Run{ID: "run", SnapshotID: "m", FeatureSnapshotID: "f", SnapshotDate: f.SnapshotDate, Candidates: []selection.Candidate{{Code: "600000", Triggers: []selection.Trigger{{MethodID: "method", MethodVersionID: "v-old", MethodName: "原方法"}}}}}
	service, _ := NewObservationService(store, observationRunFixture{run}, observationMethodFixture{method}, observationFeatureFixture{f})
	first, err := service.Create(context.Background(), "run", "600000", "method")
	if err != nil {
		t.Fatal(err)
	}
	summary, summaryErr := service.Summary(context.Background())
	if summaryErr != nil || summary[ObservationOngoing] != 1 {
		t.Fatalf("new unassessed observation must count ongoing %+v %v", summary, summaryErr)
	}
	second, err := service.Create(context.Background(), "run", "600000", "method")
	if err != nil || first.Contract.ID != second.Contract.ID {
		t.Fatalf("create not idempotent %v", err)
	}
	restored, _ := NewObservationService(store, observationRunFixture{run}, observationMethodFixture{method}, observationFeatureFixture{f})
	loaded, err := restored.Get(context.Background(), first.Contract.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Contract.MethodVersionID != "v-old" || loaded.Contract.Outcome.HorizonDays != 2 || *loaded.Contract.Outcome.TargetReturnPct != .2 || loaded.Contract.Baseline != 10 {
		t.Fatalf("contract changed %+v", loaded.Contract)
	}
	var count int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM forward_observation`).Scan(&count)
	if count != 1 {
		t.Fatalf("count %d", count)
	}
	if _, err = store.DB().Exec(`UPDATE forward_observation SET contract_json='{}'`); err == nil {
		t.Fatal("contract overwritable")
	}
	raw, _ := json.Marshal(first.Contract)
	if len(raw) == 0 {
		t.Fatal("empty contract")
	}
}

func TestObservationRefreshPersistsIdempotentEvaluationsAndNoGetWrites(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "refresh.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(); err != nil {
		t.Fatal(err)
	}
	f := &marketsnapshot.FeatureSnapshot{ID: "f", MarketSnapshotID: "m", SnapshotDate: "2026-11-02", Status: marketsnapshot.StatusReady, LeakChecked: true, FeatureIDs: []string{"close"}, Values: map[string]map[string]float64{"600000": {"close": 10}}}
	f.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(f)
	c := observationFixture()
	method := &methodregistry.Method{Versions: []methodregistry.MethodVersion{{ID: "v", Method: &methods.CompiledMethod{Outcome: c.Outcome}}}}
	run := &selection.Run{ID: "r", SnapshotID: "m", FeatureSnapshotID: "f", SnapshotDate: f.SnapshotDate, Candidates: []selection.Candidate{{Code: "600000", Triggers: []selection.Trigger{{MethodID: "method", MethodVersionID: "v"}}}}}
	service, _ := NewObservationService(store, observationRunFixture{run}, observationMethodFixture{method}, observationFeatureFixture{f})
	service.now = func() time.Time { return observationTime("2026-11-05 16:00") }
	for _, bar := range observationBars() {
		_, err = store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)VALUES('600000',9,?,?,?,?,?,?,1)`, compactObservationDate(bar.Date), bar.Open, bar.High, bar.Low, bar.Close, bar.Volume)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.DB().Exec(`INSERT INTO cache(bucket,key,value,created_at,expires_at)VALUES('xdxr','600000','[]',?,0)`, service.now().Unix()); err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), "r", "600000", "method")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err = service.Refresh(context.Background(), created.Contract.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM forward_observation_evaluation`).Scan(&count)
	if count != 1 {
		t.Fatalf("same data produced %d rows", count)
	}
	list, _, err := service.List(context.Background(), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Evaluation == nil || len(list[0].Evaluation.Bars) != 0 {
		t.Fatalf("list must omit bars %+v", list)
	}
	loaded, err := service.Get(context.Background(), created.Contract.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Evaluation.Status != ObservationHit || len(loaded.Evaluation.Bars) != 3 || !loaded.Evaluation.Complete {
		t.Fatalf("stored evaluation %+v", loaded.Evaluation)
	}
	if loaded.Evaluation.AsOf != "2026-11-04" || loaded.Evaluation.DataAsOf != "2026-11-04" {
		t.Fatalf("as-of dates %+v", loaded.Evaluation)
	}
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM forward_observation_evaluation`).Scan(&count)
	if count != 1 {
		t.Fatal("GET wrote evaluation")
	}
	summary, err := service.Summary(context.Background())
	if err != nil || summary["concluded_complete"] != 1 || summary["hit_complete"] != 1 {
		t.Fatalf("summary %+v %v", summary, err)
	}
}

func TestObservationRefreshIsolatesUnreadableContractFromTheRest(t *testing.T) {
	store, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "isolate.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(); err != nil {
		t.Fatal(err)
	}
	f := &marketsnapshot.FeatureSnapshot{ID: "f", MarketSnapshotID: "m", SnapshotDate: "2026-11-02", Status: marketsnapshot.StatusReady, LeakChecked: true, FeatureIDs: []string{"close"}, Values: map[string]map[string]float64{"600000": {"close": 10}}}
	f.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(f)
	c := observationFixture()
	method := &methodregistry.Method{Versions: []methodregistry.MethodVersion{{ID: "v", Method: &methods.CompiledMethod{Outcome: c.Outcome}}}}
	run := &selection.Run{ID: "r", SnapshotID: "m", FeatureSnapshotID: "f", SnapshotDate: f.SnapshotDate, Candidates: []selection.Candidate{{Code: "600000", Triggers: []selection.Trigger{{MethodID: "method", MethodVersionID: "v"}}}}}
	service, _ := NewObservationService(store, observationRunFixture{run}, observationMethodFixture{method}, observationFeatureFixture{f})
	service.now = func() time.Time { return observationTime("2026-11-05 16:00") }
	for _, bar := range observationBars() {
		if _, err = store.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount)VALUES('600000',9,?,?,?,?,?,?,1)`, compactObservationDate(bar.Date), bar.Open, bar.High, bar.Low, bar.Close, bar.Volume); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.DB().Exec(`INSERT INTO cache(bucket,key,value,created_at,expires_at)VALUES('xdxr','600000','[]',?,0)`, service.now().Unix()); err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), "r", "600000", "method")
	if err != nil {
		t.Fatal(err)
	}
	// A second, newer record whose contract JSON is unreadable. It is refreshed first
	// (newest first), so a fail-fast batch would abandon the healthy record behind it.
	if _, err = store.DB().Exec(`INSERT INTO forward_observation(id,selection_run_id,stock_code,method_version_id,created_at_ns,contract_hash,contract_json)VALUES('ob-clone','r','600000','v2',?,?,?)`, created.Contract.CreatedAt.UnixNano()+1, created.Contract.ContractHash, `{"broken":`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB().Exec(`INSERT INTO forward_observation_evaluation(observation_id,data_hash,evaluated_at_ns,status,result_json)VALUES('ob-clone','x',1,'ongoing','{broken')`); err != nil {
		t.Fatal(err)
	}
	refreshed, failed, err := service.Refresh(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("batch refresh aborted: %v", err)
	}
	if refreshed != 1 || failed != 1 {
		t.Fatalf("refreshed=%d failed=%d", refreshed, failed)
	}
	loaded, err := service.Get(context.Background(), created.Contract.ID, true)
	if err != nil || loaded.Evaluation == nil || loaded.Evaluation.Status != ObservationHit {
		t.Fatalf("healthy record not evaluated: %+v %v", loaded, err)
	}
	list, total, err := service.List(context.Background(), 30, 0)
	if err != nil || len(list) != 1 || total != 2 {
		t.Fatalf("list %d total %d err %v", len(list), total, err)
	}
	summary, err := service.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary["total"] != 2 || summary["corrupt"] != 1 || summary[ObservationHit] != 1 {
		t.Fatalf("summary %+v", summary)
	}
}

func TestObservationCorporateCutoffIsOneRefreshPerCompletedSession(t *testing.T) {
	for _, test := range []struct{ now, cutoff string }{
		{"2026-11-06 15:30", "2026-11-06 15:00"}, // 周五收盘后
		{"2026-11-06 14:00", "2026-11-05 15:00"}, // 收盘前仍看上一个交易日
		{"2026-11-07 10:00", "2026-11-06 15:00"}, // 周六回看周五
		{"2026-11-09 09:00", "2026-11-06 15:00"}, // 周一开盘前回看周五
		{"2026-11-09 16:00", "2026-11-09 15:00"},
	} {
		got := observationCorporateCutoff(observationTime(test.now)).Format("2006-01-02 15:04")
		if got != test.cutoff {
			t.Fatalf("%s cutoff %s want %s", test.now, got, test.cutoff)
		}
	}
}

func TestObservationDuplicateDatesAndInvalidPriceDoNotShortenWindowOrHit(t *testing.T) {
	c := observationFixture()
	bars := observationBars()
	bars[2].Open = 0
	result := EvaluateObservation(c, []string{"2026-11-04", "2026-11-03", "2026-11-03"}, bars, true, nil, observationTime("2026-11-04 16:00"))
	if result.Status != ObservationInsufficient || result.Progress != 1 || result.HitDate != "" {
		t.Fatalf("invalid OHLC or duplicate session counted %+v", result)
	}
}

func TestObservationOutcomeBasisConsistency(t *testing.T) {
	base := observationFixture()
	target := 0.2
	base.Outcome.TargetReturnPct = &target
	base.Outcome.PriceBasis = "high"
	base.Outcome.Success = "close_gte_target"
	result := EvaluateObservation(base, []string{"2026-11-03"}, []ObservationBar{{Date: "2026-11-02", Open: 10, High: 10.2, Low: 9.9, Close: 10, Volume: 1}, {Date: "2026-11-03", Open: 10, High: 12.5, Low: 9.9, Close: 10.1, Volume: 1}}, true, nil, time.Date(2026, 11, 11, 16, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)))
	if result.Status != ObservationUndefined {
		t.Fatalf("close comparator with high basis should be undefined: %+v", result)
	}
	base.Outcome.PriceBasis = "close"
	base.Outcome.Success = "close_lte_target"
	result = EvaluateObservation(base, []string{"2026-11-03"}, []ObservationBar{{Date: "2026-11-02", Open: 10, High: 10.2, Low: 9.9, Close: 10, Volume: 1}, {Date: "2026-11-03", Open: 10, High: 10.1, Low: 8.8, Close: 9, Volume: 1}}, true, nil, time.Date(2026, 11, 11, 16, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)))
	if result.Status != ObservationHit {
		t.Fatalf("close_lte comparator should evaluate: %+v", result)
	}
}
