package automation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	tradeengine "github.com/sjzsdu/tongstock/internal/trading"
	"time"
)

type Orchestrator struct {
	selection *selection.Engine
	positions *positiondecision.Engine
	snapshots selection.SnapshotRepository
	ledger    *ledger.SignalLedger
	repo      Repository
	now       func() time.Time
}

func New(s *selection.Engine, p *positiondecision.Engine, sn selection.SnapshotRepository, l *ledger.SignalLedger, r Repository) (*Orchestrator, error) {
	if s == nil || p == nil || sn == nil || l == nil || r == nil {
		return nil, fmt.Errorf("automation dependencies required")
	}
	return &Orchestrator{selection: s, positions: p, snapshots: sn, ledger: l, repo: r, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (o *Orchestrator) Run(ctx context.Context, snapshotID string) (job *Job, err error) {
	key := Version + ":" + snapshotID
	job, owner, err := o.repo.Claim(ctx, key, snapshotID)
	if err != nil || !owner {
		return job, err
	}
	defer func() {
		if err != nil {
			_ = o.repo.Fail(ctx, job, err)
		}
	}()
	sel, err := o.selection.Run(ctx, selection.Request{MarketSnapshotID: snapshotID})
	if err != nil {
		return job, err
	}
	pos, err := o.positions.Run(ctx, positiondecision.Request{MarketSnapshotID: snapshotID, FeatureSnapshotID: sel.FeatureSnapshotID})
	if err != nil {
		return job, err
	}
	job.SelectionRunID, job.PositionRunID = sel.ID, pos.ID
	feature, err := o.snapshots.LoadFeatureSnapshot(sel.FeatureSnapshotID, true)
	if err != nil {
		return job, err
	}
	events := []Event{}
	for _, c := range sel.Candidates {
		if c.Action != selection.ActionBuy {
			continue
		}
		for _, t := range c.Triggers {
			eventKey := stable("buy", sel.ID, c.Code, t.MethodVersionID)
			events = append(events, Event{Key: eventKey, JobID: job.ID, Type: "buy_candidate", Priority: "normal", Payload: c, Status: "pending", CreatedAt: o.now()})
			if err = o.appendSignal(sel, feature, c, t); err != nil {
				return job, err
			}
			break
		}
	}
	for _, d := range pos.Decisions {
		if d.Action != "exit" && d.Action != "reduce" {
			continue
		}
		priority := "high"
		if d.Action == "exit" {
			priority = "critical"
		}
		events = append(events, Event{Key: stable("position", pos.ID, d.Code, d.Action), JobID: job.ID, Type: "position_risk", Priority: priority, Payload: d, Status: "pending", CreatedAt: o.now()})
	}
	job.Status = "completed"
	job.FinishedAt = o.now()
	err = o.repo.Complete(ctx, job, events)
	return job, err
}
func (o *Orchestrator) appendSignal(run *selection.Run, f *marketsnapshot.FeatureSnapshot, c selection.Candidate, t selection.Trigger) error {
	date, err := time.Parse("2006-01-02", run.SnapshotDate)
	if err != nil {
		return err
	}
	runID := "fr-" + t.MethodVersionID + "-" + date.Format("20060102")
	if _, err = o.ledger.GetRun(runID); err != nil {
		if _, err = o.ledger.NewForwardRun(t.MethodVersionID, date, 100000, tradeengine.DefaultTradingConstraints(), tradeengine.DefaultCostModel()); err != nil {
			return err
		}
	}
	v := f.Values[c.Code]
	captured := f.BuiltAt
	if captured.IsZero() {
		captured = time.Unix(0, f.AsOfNs)
	}
	return o.ledger.AppendSignal(ledger.SignalEntry{ID: stable("signal", run.ID, c.Code, t.MethodVersionID), RunID: runID, ParadigmVersionID: t.MethodVersionID, StockCode: c.Code, Direction: "buy", SignalDate: date, Price: v["close"], Confidence: c.Score, Market: ledger.ExecutionMarket{Date: date, Open: v["open"], High: v["high"], Low: v["low"], Close: v["close"], Volume: v["volume"], Amount: v["amount"]}, DataSnapshot: ledger.DataSnapshot{DatasetID: run.SnapshotID, FeatureSetID: run.FeatureSnapshotID, RuleSetID: t.MethodVersionID, DataHash: f.ContentHash, CapturedAt: captured}, Source: ledger.SignalSource{RuleID: t.MethodID, RuleDesc: t.MethodName, TriggeredBy: run.ID, ContextTags: map[string]string{"selection_run_id": run.ID}}, CreatedAt: o.now()})
}
func stable(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("evt-%x", h.Sum(nil)[:10])
}
