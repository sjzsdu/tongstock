package server

import "testing"

func TestParseMainFinanceMetricTables(t *testing.T) {
	content := `财务分析
【1.主要财务指标】
┌────┬──────┬──────┐
│财务指标            │  2026-03-31│  2025-12-31│
├────┼──────┼──────┤
│归母净利(未调整:万) │  1452300.00│  4263300.00│
│营业总收(未调整:万) │  3527700.00│ 13144200.00│
│基本每股收益(元)    │      0.6700│      2.0700│
└────┴──────┴──────┘

┌────┬──────┬──────┐
│财务指标            │  2026-03-31│  2025-12-31│
├────┼──────┼──────┤
│净利润增长率(%)     │        3.03│       -4.21│
│每股经营现金流量(元)│      1.9500│     16.2800│
└────┴──────┴──────┘
【2.偿债能力指标】`

	tables := parseMainFinanceMetricTables(content)
	if len(tables) != 2 {
		t.Fatalf("tables len = %d, want 2", len(tables))
	}
	if got := tables[0].Periods[0]; got != "2026-03-31" {
		t.Fatalf("first period = %q", got)
	}
	if got := tables[0].Rows[0].Name; got != "归母净利(未调整:万)" {
		t.Fatalf("first row name = %q", got)
	}

	records, metrics := financeTrendRecordsFromMetricTable(tables[0])
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	if records[0].Period != "2025-12-31" || records[1].Period != "2026-03-31" {
		t.Fatalf("records not sorted ascending: %#v", records)
	}
	if records[1].Revenue == nil || *records[1].Revenue != 3527700.00 {
		t.Fatalf("revenue not parsed: %#v", records[1].Revenue)
	}
	if len(metrics) == 0 {
		t.Fatalf("metrics empty")
	}
}
