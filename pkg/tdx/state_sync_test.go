package tdx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/statesnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

type offlineStateExecutor struct{}

func (offlineStateExecutor) Do(func(*Client) error) error { return errors.New("offline") }
func (offlineStateExecutor) DoContext(context.Context, func(*Client) error) error {
	return errors.New("offline")
}
func (offlineStateExecutor) Close() error           { return nil }
func (offlineStateExecutor) Len() int               { return 0 }
func (offlineStateExecutor) Status() ExecutorStatus { return ExecutorStatus{} }

func TestDailyBatchPublishesStatesAndSurfacesSnapshotFailure(t *testing.T) {
	db, err := storage.New(storage.Config{DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	date := time.Now().Format("20060102")
	if _, err = db.DB().Exec(`INSERT INTO kline(code,ktype,date,open,high,low,close,volume,amount) VALUES('000001',9,?,10,11,9,10,100,1000)`, date); err != nil {
		t.Fatal(err)
	}
	ks, err := NewKlineStore(db)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{storage: db, klines: ks, executor: offlineStateExecutor{}}
	// The absent symbol fails upstream. Successful cached daily symbols still
	// publish states after the batch, and the failed symbol has no phantom rows.
	result := svc.SyncDailyKlines([]string{"000001", "600000"}, SyncModeAuto, 2)
	if result.Success != 1 || result.Failed != 1 {
		t.Fatalf("%+v", result)
	}
	rows, err := (statesnapshot.Store{DB: db.DB()}).At(context.Background(), date, nil)
	if err != nil || len(rows) != 4 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	result = svc.SyncDailyKlines([]string{"000001"}, SyncModeAuto, 1)
	if result.Failed != 0 {
		t.Fatalf("repeat %+v", result)
	}
	if _, err = db.DB().Exec(`DROP TABLE signal_state_daily`); err != nil {
		t.Fatal(err)
	}
	result = svc.SyncDailyKlines([]string{"000001"}, SyncModeAuto, 1)
	if result.Failed != 1 || !strings.Contains(result.Results[0].Error, "daily states:") {
		t.Fatalf("lost snapshot error: %+v", result)
	}
}
