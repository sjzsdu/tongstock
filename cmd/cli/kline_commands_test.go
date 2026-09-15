package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/app/stockdata"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

func TestValidateKlineOptions(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		kind      string
		count     int
		all       bool
		wantType  string
		wantKType uint8
		wantErr   string
	}{
		{name: "day", code: "600000", kind: "day", count: 500, wantType: "day", wantKType: 9},
		{name: "normalizes minute", code: "600000", kind: " MINUTE ", count: 1, wantType: "minute", wantKType: 7},
		{name: "all permits zero count", code: "600000", kind: "week", count: 0, all: true, wantType: "week", wantKType: 5},
		{name: "missing code", kind: "day", count: 1, wantErr: "--code"},
		{name: "invalid type", code: "600000", kind: "daily", count: 1, wantErr: "无效的 --type"},
		{name: "non-positive count", code: "600000", kind: "day", count: 0, wantErr: "--count 必须大于 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ktype, err := validateKlineOptions(tt.code, tt.kind, tt.count, tt.all)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateKlineOptions() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateKlineOptions() unexpected error: %v", err)
			}
			if kind != tt.wantType || ktype != tt.wantKType {
				t.Fatalf("validateKlineOptions() = (%q, %d), want (%q, %d)", kind, ktype, tt.wantType, tt.wantKType)
			}
		})
	}
}

func TestNewestKlinesLimitsNewestInChronologicalOrder(t *testing.T) {
	items := []*protocol.Kline{
		{Time: mustKlineTime(t, "2026-09-03")},
		nil,
		{Time: mustKlineTime(t, "2026-09-01")},
		{Time: mustKlineTime(t, "2026-09-04")},
		{Time: mustKlineTime(t, "2026-09-02")},
	}

	got := newestKlines(items, 2, false)
	if len(got) != 2 {
		t.Fatalf("newestKlines() length = %d, want 2", len(got))
	}
	if got[0].Time.Day() != 3 || got[1].Time.Day() != 4 {
		t.Fatalf("newestKlines() days = [%d, %d], want [3, 4]", got[0].Time.Day(), got[1].Time.Day())
	}

	all := newestKlines(items, 1, true)
	if len(all) != 4 || all[0].Time.Day() != 1 || all[3].Time.Day() != 4 {
		t.Fatalf("newestKlines(all) did not preserve full chronological set: %#v", all)
	}
}

func TestOutputKlineJSON(t *testing.T) {
	item := sampleKline(t)
	metadata := stockdata.ResultMetadata{Freshness: "fresh", Reason: "cache_current", SyncStatus: "cached"}
	var output bytes.Buffer
	if err := outputKlineJSON(&output, "600000", "1m", 7, []*protocol.Kline{item}, metadata); err != nil {
		t.Fatalf("outputKlineJSON() error: %v", err)
	}

	var decoded struct {
		Code     string                   `json:"code"`
		Type     string                   `json:"type"`
		Items    []klineJSONItem          `json:"items"`
		Metadata stockdata.ResultMetadata `json:"metadata"`
	}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output is not machine-readable: %v\n%s", err, output.String())
	}
	if decoded.Code != "600000" || decoded.Type != "1m" || len(decoded.Items) != 1 {
		t.Fatalf("unexpected JSON envelope: %#v", decoded)
	}
	if decoded.Items[0].Time != "2026-09-15 09:31:00" || decoded.Items[0].Amount != item.Amount {
		t.Fatalf("unexpected JSON item: %#v", decoded.Items[0])
	}
	if decoded.Metadata.Freshness != "fresh" || decoded.Metadata.SyncStatus != "cached" {
		t.Fatalf("metadata missing from JSON: %#v", decoded.Metadata)
	}
}

func TestOutputKlineTable(t *testing.T) {
	var output bytes.Buffer
	if err := outputKlineTable(&output, 9, []*protocol.Kline{sampleKline(t)}); err != nil {
		t.Fatalf("outputKlineTable() error: %v", err)
	}
	text := output.String()
	for _, want := range []string{"TIME", "OPEN", "HIGH", "LOW", "CLOSE", "VOLUME", "AMOUNT", "2026-09-15", "10.100", "12345.00", "67890.50"} {
		if !strings.Contains(text, want) {
			t.Errorf("table output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "09:31:00") {
		t.Errorf("day table should format timestamps as dates:\n%s", text)
	}

	output.Reset()
	if err := outputKlineTable(&output, 7, []*protocol.Kline{sampleKline(t)}); err != nil {
		t.Fatalf("outputKlineTable(minute) error: %v", err)
	}
	if !strings.Contains(output.String(), "2026-09-15 09:31:00") {
		t.Errorf("minute table should include time:\n%s", output.String())
	}
}

func sampleKline(t *testing.T) *protocol.Kline {
	t.Helper()
	return &protocol.Kline{
		Time: mustKlineTimestamp(t, "2026-09-15 09:31:00"),
		Open: 10.1, High: 10.8, Low: 9.9, Close: 10.5, Volume: 12345, Amount: 67890.5,
	}
}

func mustKlineTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func mustKlineTimestamp(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
