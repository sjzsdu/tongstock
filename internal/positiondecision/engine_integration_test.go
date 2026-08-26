package positiondecision_test

import (
	"context"
	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/trading"
	"path/filepath"
	"testing"
	"time"
)

func TestRealAdaptersPersistRiskActionsAndConstraints(t *testing.T) {
	ctx := context.Background()
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "positions.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	tr, _ := trading.New(s)
	id1, _ := tr.Create(trading.Trade{Code: "000001", Name: "平安银行", Action: trading.TradeBuy, Price: 10})
	id2, _ := tr.Create(trading.Trade{Code: "000002", Name: "万科A", Action: trading.TradeBuy, Price: 5})
	_, _ = s.DB().Exec(`UPDATE trades SET created_at=? WHERE id IN (?,?)`, time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local).Unix(), id1, id2)
	sn, _ := marketsnapshotrepo.New(s)
	m := &marketsnapshot.MarketSnapshot{ID: "market-real", SnapshotDate: "2024-01-02", Universe: marketsnapshot.UniverseDefinition{Name: "universe_usable"}, Market: "CN-A", PriceAdjustment: "forward", ExpectedKlineCodes: 2, ReadyKlineCodes: 2, CoveragePct: 1, Status: marketsnapshot.StatusReady, UniverseMembers: []marketsnapshot.UniverseMember{{Code: "000001", Selected: true, Status: "suspended"}, {Code: "000002", Selected: true, Status: "normal"}}, Codes: []marketsnapshot.CodeStatus{{Code: "000001", UniverseMember: true, SecurityStatus: "suspended", KlineLastDate: "2024-01-02"}, {Code: "000002", UniverseMember: true, SecurityStatus: "normal", KlineLastDate: "2024-01-02"}}}
	m.UniverseHash = marketsnapshot.ComputeUniverseHash(m.UniverseMembers)
	m.ContentHash, _ = marketsnapshot.ComputeContentHash(m)
	if err = sn.SaveMarketSnapshot(m); err != nil {
		t.Fatal(err)
	}
	_ = sn.FreezeMarketSnapshot(m.ID)
	f := &marketsnapshot.FeatureSnapshot{ID: "feature-real", MarketSnapshotID: m.ID, SnapshotDate: m.SnapshotDate, FeatureIDs: []string{"close"}, FeatureTotal: 1, RowsWritten: 2, LeakChecked: true, PriceAdjustment: "forward", Status: marketsnapshot.StatusReady, Values: map[string]map[string]float64{"000001": {"close": 9}, "000002": {"close": 6.5}}}
	f.ContentHash, _ = marketsnapshot.ComputeFeatureContentHash(f)
	if err = sn.SaveFeatureSnapshot(f); err != nil {
		t.Fatal(err)
	}
	mr, _ := methodregistryrepo.New(s)
	rr, _ := positiondecisionrepo.New(s)
	e, _ := positiondecision.NewEngine(sn, tr, mr, rr)
	run, err := e.Run(ctx, positiondecision.Request{MarketSnapshotID: m.ID, FeatureSnapshotID: f.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Decisions) != 2 {
		t.Fatalf("decisions=%d", len(run.Decisions))
	}
	if run.Decisions[0].Action != "exit" || run.Decisions[0].Executable {
		t.Fatalf("suspended stop decision=%+v", run.Decisions[0])
	}
	if run.Decisions[1].Action != "reduce" || !run.Decisions[1].Inferred {
		t.Fatalf("profit decision=%+v", run.Decisions[1])
	}
	again, _ := e.Run(ctx, positiondecision.Request{MarketSnapshotID: m.ID, FeatureSnapshotID: f.ID})
	if again.ID != run.ID || !again.CreatedAt.Equal(run.CreatedAt) {
		t.Fatal("decision replay is not immutable")
	}
}
