package automation

import (
	"context"
	"errors"
	"time"
)

const Version = "daily-automation-v1"

// StaleJobTimeout 是自动化任务锁的过期阈值：一个 running 任务超过该时长
// 未完成（进程崩溃/panic/卡死），Claim 将自动接管并重试，避免永久占锁。
const StaleJobTimeout = 30 * time.Minute

var ErrBusy = errors.New("automation job is already running")

type Job struct {
	ID             string    `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	SnapshotID     string    `json:"snapshot_id"`
	Status         string    `json:"status"`
	Attempt        int       `json:"attempt"`
	SelectionRunID string    `json:"selection_run_id,omitempty"`
	PositionRunID  string    `json:"position_run_id,omitempty"`
	Error          string    `json:"error,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
}
type Event struct {
	Key       string    `json:"key"`
	JobID     string    `json:"job_id"`
	Type      string    `json:"type"`
	Priority  string    `json:"priority"`
	Payload   any       `json:"payload"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type Repository interface {
	Claim(context.Context, string, string) (*Job, bool, error)
	Complete(context.Context, *Job, []Event) error
	Fail(context.Context, *Job, error) error
	ListJobs(context.Context, int) ([]Job, error)
	ListEvents(context.Context, string, int) ([]Event, error)
}
