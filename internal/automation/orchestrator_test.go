package automation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/adapter/marketsnapshotrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/methodregistryrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/positiondecisionrepo"
	"github.com/sjzsdu/tongstock/internal/adapter/selectionrepo"
	"github.com/sjzsdu/tongstock/internal/automation"
	"github.com/sjzsdu/tongstock/internal/ledger"
	"github.com/sjzsdu/tongstock/internal/positiondecision"
	"github.com/sjzsdu/tongstock/internal/selection"
	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/trading"
)

// fakeRepo 记录 Orchestrator 与仓库的交互，用于断言幂等与失败语义。
type fakeRepo struct {
	claimJob    automation.Job
	claimOwner  bool
	claimErr    error
	completed   bool
	failed      bool
	completeErr error
	failErr     error
}

func (f *fakeRepo) Claim(_ context.Context, _, _ string) (*automation.Job, bool, error) {
	return &f.claimJob, f.claimOwner, f.claimErr
}
func (f *fakeRepo) Complete(_ context.Context, _ *automation.Job, _ []automation.Event) error {
	f.completed = true
	return f.completeErr
}
func (f *fakeRepo) Fail(_ context.Context, _ *automation.Job, _ error) error {
	f.failed = true
	return f.failErr
}
func (f *fakeRepo) ListJobs(_ context.Context, _ int) ([]automation.Job, error) { return nil, nil }
func (f *fakeRepo) ListEvents(_ context.Context, _ string, _ int) ([]automation.Event, error) {
	return nil, nil
}

func newTestOrchestrator(t *testing.T, repo automation.Repository) *automation.Orchestrator {
	t.Helper()
	s, err := storage.New(storage.Config{Driver: "sqlite3", DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	sn, _ := marketsnapshotrepo.New(s)
	mr, _ := methodregistryrepo.New(s)
	sr, _ := selectionrepo.New(s)
	pr, _ := positiondecisionrepo.New(s)
	tr, _ := trading.New(s)
	se, _ := selection.NewEngine(sn, mr, sr)
	pe, _ := positiondecision.NewEngine(sn, tr, mr, pr)
	l, err := ledger.NewSQLiteSignalLedger(s)
	if err != nil {
		t.Fatal(err)
	}
	o, err := automation.New(se, pe, sn, l, repo)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestRunSkipsCompletedJob(t *testing.T) {
	repo := &fakeRepo{
		claimJob:   automation.Job{ID: "job-1", IdempotencyKey: "daily-automation-v1:s1", Status: "completed", FinishedAt: time.Now().UTC()},
		claimOwner: false,
	}
	o := newTestOrchestrator(t, repo)

	job, err := o.Run(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != "completed" {
		t.Errorf("job status = %s, want completed replay", job.Status)
	}
	if repo.completed || repo.failed {
		t.Fatal("completed job must not touch Complete/Fail")
	}
}

func TestRunPropagatesBusy(t *testing.T) {
	repo := &fakeRepo{claimErr: automation.ErrBusy}
	o := newTestOrchestrator(t, repo)

	if _, err := o.Run(context.Background(), "s1"); !errors.Is(err, automation.ErrBusy) {
		t.Fatalf("Run err = %v, want ErrBusy", err)
	}
	if repo.failed {
		t.Fatal("busy claim must not Fail the job")
	}
}

func TestRunFailsJobOnSelectionError(t *testing.T) {
	repo := &fakeRepo{
		claimJob:   automation.Job{ID: "job-2", IdempotencyKey: "daily-automation-v1:s-missing"},
		claimOwner: true,
	}
	o := newTestOrchestrator(t, repo)

	if _, err := o.Run(context.Background(), "s-missing"); err == nil {
		t.Fatal("expected selection error for missing snapshot")
	}
	if !repo.failed {
		t.Fatal("owner job with selection error must be marked failed")
	}
	if repo.completed {
		t.Fatal("failed run must not Complete")
	}
}
