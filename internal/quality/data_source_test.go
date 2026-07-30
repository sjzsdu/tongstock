package quality

import (
	"testing"
	"time"
)

func TestQualityDataSource_FetchKlineData_NilFetcher(t *testing.T) {
	ds := &QualityDataSource{}
	opts := &EvaluateOptions{}
	err := ds.FetchKlineData([]string{"sh000001"}, 4, "", "", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(opts.KlineData) != 0 {
		t.Fatalf("expected empty KlineData, got %d entries", len(opts.KlineData))
	}
}

func TestQualityDataSource_FetchKlineData_WithFetcher(t *testing.T) {
	fetcher := KlineDataFetcherFunc(func(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error) {
		return []KlineRecord{
			{Date: time.Date(2025, 1, 2, 0, 0, 0, 0, time.Local), Open: 10.0, High: 11.0, Low: 9.5, Close: 10.5, Volume: 1000, Amount: 10000},
			{Date: time.Date(2025, 1, 3, 0, 0, 0, 0, time.Local), Open: 10.5, High: 12.0, Low: 10.0, Close: 11.5, Volume: 2000, Amount: 23000},
		}, nil
	})

	ds := &QualityDataSource{Fetcher: fetcher}
	opts := &EvaluateOptions{}
	err := ds.FetchKlineData([]string{"sh000001", "sz399001"}, 4, "", "", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(opts.KlineData) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(opts.KlineData))
	}
	if len(opts.KlineData["sh000001"]) != 2 {
		t.Fatalf("expected 2 records for sh000001, got %d", len(opts.KlineData["sh000001"]))
	}
}

func TestKlineDataFetcherFunc_GetKline(t *testing.T) {
	called := false
	fn := KlineDataFetcherFunc(func(code string, ktype uint8, startDate, endDate string) ([]KlineRecord, error) {
		called = true
		return []KlineRecord{}, nil
	})

	_, err := fn.GetKline("sh000001", 4, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected fetcher to be called")
	}
}

func TestQualityDataSource_NilReceiver(t *testing.T) {
	var ds *QualityDataSource
	opts := &EvaluateOptions{}
	err := ds.FetchKlineData([]string{"sh000001"}, 4, "", "", opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	records, err := ds.FetchKlineDataForCode("sh000001", 4, "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if records != nil {
		t.Fatal("expected nil records")
	}
}
