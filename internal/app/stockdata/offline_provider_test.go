package stockdata

import (
	"context"
	"strings"
	"testing"
)

func TestOfflineProviderFailsClosed(t *testing.T) {
	_, _, err := NewOfflineProvider().Sync(context.Background(), SyncRequest{})
	if err == nil || !strings.Contains(err.Error(), "cache_only") {
		t.Fatalf("offline provider error = %v", err)
	}
}
