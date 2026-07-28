package server

import (
	"context"
	"testing"
	"time"
)

func TestParadigmAlertScannerStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{}
	s.StartParadigmAlertScanner(ctx, time.Hour)
	cancel()

	done := make(chan struct{})
	go func() {
		s.WaitForBackgroundTasks()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("paradigm alert scanner did not stop after context cancellation")
	}
}
