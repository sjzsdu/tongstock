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

func TestClaimTakesOverStaleRunningJob(t *testing.T) {
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
	key := automation.Version + ":snapshot-stale"

	j, owner, err := r.Claim(ctx, key, "snapshot-stale")
	if err != nil || !owner {
		t.Fatalf("first claim: %v %v", owner, err)
	}
	_ = j
	// 把 started_at 拨到超时阈值之前，模拟进程崩溃后遗留的 running 任务。
	stale := time.Now().UTC().Add(-automation.StaleJobTimeout - time.Minute).UnixNano()
	if _, err = s.DB().Exec(`UPDATE automation_job_run SET started_at_ns=? WHERE idempotency_key=?`, stale, key); err != nil {
		t.Fatal(err)
	}

	// 超时任务应被接管（owner=true），而不是 ErrBusy。
	_, owner2, err := r.Claim(ctx, key, "snapshot-stale")
	if err != nil {
		t.Fatalf("claim after stale: %v", err)
	}
	if !owner2 {
		t.Fatal("expected takeover of stale job")
	}

	// 接管后新任务未超时，再次 claim 应 ErrBusy（并发保护）。
	if _, _, err = r.Claim(ctx, key, "snapshot-stale"); !errors.Is(err, automation.ErrBusy) {
		t.Fatalf("concurrent claim after takeover: %v", err)
	}

	var attempt int
	if err = s.DB().QueryRow(`SELECT attempt FROM automation_job_run WHERE idempotency_key=?`, key).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if attempt < 2 {
		t.Fatalf("attempt=%d, want >=2 after takeover retry", attempt)
	}
}

func TestUnlockReleasesRunningJob(t *testing.T) {
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
	key := automation.Version + ":snapshot-locked"

	if _, _, err = r.Claim(ctx, key, "snapshot-locked"); err != nil {
		t.Fatal(err)
	}
	unlocked, err := r.Unlock(ctx, "snapshot-locked")
	if err != nil || !unlocked {
		t.Fatalf("unlock: %v %v", unlocked, err)
	}
	// 解锁后再 claim 应成功（owner=true 重试），而非 ErrBusy。
	_, owner, err := r.Claim(ctx, key, "snapshot-locked")
	if err != nil {
		t.Fatalf("claim after unlock: %v", err)
	}
	if !owner {
		t.Fatal("expected owner after unlock")
	}
	// 此时又有 running 任务，再次 unlock 应返回 true（释放新任务）。
	if again, err := r.Unlock(ctx, "snapshot-locked"); err != nil || !again {
		t.Fatalf("second unlock on fresh running job: %v %v", again, err)
	}
	// 释放后无 running 任务，unlock 应返回 false。
	if again, err := r.Unlock(ctx, "snapshot-locked"); err != nil || again {
		t.Fatalf("third unlock with no running job: %v %v", again, err)
	}
}
