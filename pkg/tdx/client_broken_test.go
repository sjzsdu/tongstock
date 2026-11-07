package tdx

import (
	"strings"
	"testing"

	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

func TestInvalidateLockedClosesAndMarksBroken(t *testing.T) {
	client, conn := newTrackedClient()

	client.mu.Lock()
	client.invalidateLocked()
	client.mu.Unlock()

	if !client.broken {
		t.Fatal("broken = false, want true after invalidation")
	}
	if !conn.closed.Load() {
		t.Fatal("connection was not closed on invalidation")
	}
}

func TestSendOnBrokenClientWithoutRedialReportsError(t *testing.T) {
	client, _ := newTrackedClient()

	client.mu.Lock()
	client.invalidateLocked()
	client.mu.Unlock()

	if _, err := client.send(protocol.MHeart.Frame()); err == nil {
		t.Fatal("send() on broken client error = nil, want connection invalid error")
	} else if !strings.Contains(err.Error(), "失效") {
		t.Fatalf("send() error = %v, want explicit invalid-connection error", err)
	}
}
