package methodresearchai

import (
	"context"
	"net/url"
	"testing"
)

func TestSourceURLGuardRejectsLocalNetworks(t *testing.T) {
	for _, raw := range []string{"http://localhost/private", "http://127.0.0.1/private", "http://169.254.169.254/latest/meta-data"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := ensurePublicURL(context.Background(), u); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestExtractJSONObjectIgnoresProseAndBracesInStrings(t *testing.T) {
	got := extractJSONObject("result follows\n```json\n{\"summary\":\"a {quoted} value\",\"sources\":[]}\n```")
	if got != `{"summary":"a {quoted} value","sources":[]}` {
		t.Fatalf("got %q", got)
	}
}
