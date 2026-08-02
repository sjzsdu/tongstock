package automationrepo_test

import (
	"context"
	"errors"
	"github.com/sjzsdu/tongstock/internal/adapter/automationrepo"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"path/filepath"
	"testing"
	"time"
)

func TestJobRetryRestartAndOutboxDeduplication(t *testing.T) {
	ctx := context.Background()
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: filepath.Join(t.TempDir(), "automation.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	r, _ := automationrepo.New(s)
	j, owner, err := r.Claim(ctx, "daily-v1:snapshot-real", "snapshot-real")
	if err != nil || !owner {
		t.Fatalf("claim: %v %v", owner, err)
	}
	if _, _, err = r.Claim(ctx, "daily-v1:snapshot-real", "snapshot-real"); !errors.Is(err, automation.ErrBusy) {
		t.Fatalf("concurrent claim=%v", err)
	}
	_ = r.Fail(ctx, j, errors.New("feature snapshot incomplete"))
	retry, owner, err := r.Claim(ctx, "daily-v1:snapshot-real", "snapshot-real")
	if err != nil || !owner {
		t.Fatalf("retry: %v %v", owner, err)
	}
	retry.Status = "completed"
	retry.SelectionRunID = "selection-real"
	retry.PositionRunID = "position-real"
	retry.FinishedAt = time.Now().UTC()
	event := automation.Event{Key: "event-real", JobID: retry.ID, Type: "position_risk", Priority: "critical", Payload: map[string]any{"code": "000001", "action": "exit"}, CreatedAt: time.Now().UTC()}
	if err = r.Complete(ctx, retry, []automation.Event{event, event}); err != nil {
		t.Fatal(err)
	}
	restarted, _ := automationrepo.New(s)
	same, owner, err := restarted.Claim(ctx, "daily-v1:snapshot-real", "snapshot-real")
	if err != nil || owner || same.SelectionRunID != "selection-real" {
		t.Fatalf("restart replay: %+v owner=%v err=%v", same, owner, err)
	}
	events, err := restarted.ListEvents(ctx, "pending", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("outbox events=%d err=%v", len(events), err)
	}
}
