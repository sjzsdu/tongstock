package stockdata

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/pkg/storage"
	"github.com/sjzsdu/tongstock/pkg/tdx/protocol"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type providerFunc func(context.Context, SyncRequest) (Dataset, SyncMetadata, error)

func (fn providerFunc) Sync(ctx context.Context, request SyncRequest) (Dataset, SyncMetadata, error) {
	return fn(ctx, request)
}

func testRepository(t *testing.T) (*storage.Storage, *SQLiteRepository) {
	t.Helper()
	store, err := storage.New(storage.Config{
		Driver: "sqlite3",
		DSN:    filepath.Join(t.TempDir(), "stockdata.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository, err := NewSQLiteRepository(store)
	if err != nil {
		t.Fatal(err)
	}
	return store, repository
}

func TestFreshQuoteReadsDatabaseWithoutProvider(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 10, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	if err := repository.SaveSynced(context.Background(), spec,
		Dataset{Quote: &protocol.QuoteItem{Code: spec.Code, Price: 12.3}},
		SyncMetadata{SourceUpdatedAt: now.Add(-5 * time.Second), Quality: "validated"}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service, err := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		calls.Add(1)
		return Dataset{}, SyncMetadata{}, errors.New("provider must not be called")
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || result.Quote == nil || result.Quote.Price != 12.3 {
		t.Fatalf("calls=%d result=%+v", calls.Load(), result)
	}
	if result.Metadata.SyncStatus != "cache" {
		t.Fatalf("sync status = %q", result.Metadata.SyncStatus)
	}
}

func TestConcurrentMissingQuoteSyncIsCoalesced(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return Dataset{Quote: &protocol.QuoteItem{Code: spec.Code, Price: 9.8}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	})
	service, _ := NewService(repository, provider, NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			result, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
			if err == nil && (result.Quote == nil || result.Quote.Price != 9.8) {
				err = errors.New("inconsistent result")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestCoalescedWaiterCanCancelWithoutCancelingRefresh(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return Dataset{Quote: &protocol.QuoteItem{Code: spec.Code, Price: 9.8}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	leaderDone := make(chan error, 1)
	go func() {
		_, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
		leaderDone <- err
	}()
	<-started

	waiterCtx, cancel := context.WithCancel(context.Background())
	waiterDone := make(chan error, 1)
	go func() {
		_, err := service.Query(waiterCtx, DataRequest{Spec: spec, Mode: RequireFresh})
		waiterDone <- err
	}()
	cancel()
	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter error = %v, want context canceled", err)
	}

	close(release)
	if err := <-leaderDone; err != nil {
		t.Fatalf("shared refresh failed after waiter cancellation: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", calls.Load())
	}
}

func TestLifecycleCancellationStopsSharedRefresh(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	lifecycle, cancelLifecycle := context.WithCancel(context.Background())
	started := make(chan struct{})
	service, _ := NewServiceWithContext(
		lifecycle,
		repository,
		providerFunc(func(ctx context.Context, _ SyncRequest) (Dataset, SyncMetadata, error) {
			close(started)
			<-ctx.Done()
			return Dataset{}, SyncMetadata{}, ctx.Err()
		}),
		NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local),
		fixedClock{now},
	)
	done := make(chan error, 1)
	go func() {
		_, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
		done <- err
	}()
	<-started
	cancelLifecycle()
	select {
	case err := <-done:
		if CodeOf(err) != ErrUpstream {
			t.Fatalf("Query error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shared refresh ignored lifecycle cancellation")
	}
}

func TestKlineSyncRequestsOnlyMissingRange(t *testing.T) {
	_, repository := testRepository(t)
	ctx := context.Background()
	monday := time.Date(2026, 7, 27, 0, 0, 0, 0, time.Local)
	tuesday := monday.AddDate(0, 0, 1)
	wednesday := monday.AddDate(0, 0, 2)
	now := time.Date(2026, 7, 29, 16, 0, 0, 0, time.Local)
	spec := DataSpec{
		Type: DataKline, Market: "sh", Code: "600000", Granularity: "day", KType: 9,
		Start: monday, End: wednesday,
	}
	if err := repository.SaveSynced(ctx, spec, Dataset{Klines: []*protocol.Kline{
		validKline(monday, 10), validKline(wednesday, 12),
	}}, SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}); err != nil {
		t.Fatal(err)
	}
	var gotRange TimeRange
	service, _ := NewService(repository, providerFunc(func(_ context.Context, request SyncRequest) (Dataset, SyncMetadata, error) {
		gotRange = request.Range
		return Dataset{Klines: []*protocol.Kline{validKline(tuesday, 11)}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	result, err := service.Query(ctx, DataRequest{Spec: spec, Mode: RequireFresh})
	if err != nil {
		t.Fatal(err)
	}
	if !gotRange.Start.Equal(tuesday) || !gotRange.End.Equal(tuesday) {
		t.Fatalf("sync range = %s..%s, want Tuesday only", gotRange.Start, gotRange.End)
	}
	if len(result.Klines) != 3 {
		t.Fatalf("result rows = %d, want 3", len(result.Klines))
	}
}

func TestKlineMultipleGapsAreMinimalAndCommitAtomically(t *testing.T) {
	store, repository := testRepository(t)
	ctx := context.Background()
	monday := time.Date(2026, 7, 27, 0, 0, 0, 0, time.Local)
	tuesday := monday.AddDate(0, 0, 1)
	wednesday := monday.AddDate(0, 0, 2)
	thursday := monday.AddDate(0, 0, 3)
	friday := monday.AddDate(0, 0, 4)
	now := time.Date(2026, 7, 31, 16, 0, 0, 0, time.Local)
	spec := DataSpec{
		Type: DataKline, Market: "sh", Code: "600000", Granularity: "day", KType: 9,
		Start: monday, End: friday,
	}
	if err := repository.SaveSynced(ctx, spec, Dataset{Klines: []*protocol.Kline{
		validKline(monday, 10), validKline(wednesday, 12), validKline(friday, 14),
	}}, SyncMetadata{SourceUpdatedAt: now.Add(-time.Hour), Quality: "validated"}); err != nil {
		t.Fatal(err)
	}
	before, err := repository.InspectCoverage(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	var requests []TimeRange
	service, _ := NewService(repository, providerFunc(func(_ context.Context, request SyncRequest) (Dataset, SyncMetadata, error) {
		requests = append(requests, request.Range)
		if request.Range.Start.Equal(tuesday) && request.Range.End.Equal(tuesday) {
			return Dataset{Klines: []*protocol.Kline{validKline(tuesday, 11)}},
				SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
		}
		if request.Range.Start.Equal(thursday) && request.Range.End.Equal(thursday) {
			return Dataset{}, SyncMetadata{}, errors.New("second range failed")
		}
		return Dataset{}, SyncMetadata{}, errors.New("non-minimal range requested")
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	if _, err := service.Query(ctx, DataRequest{Spec: spec, Mode: RequireFresh}); CodeOf(err) != ErrUpstream {
		t.Fatalf("Query error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %+v, want two isolated gaps", requests)
	}
	var rows int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM kline WHERE code = ? AND ktype = ?`,
		spec.Code, spec.KType).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("partial refresh leaked into database: row count = %d", rows)
	}
	after, err := repository.InspectCoverage(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !after.LastSyncAt.Equal(before.LastSyncAt) || !after.SourceUpdatedAt.Equal(before.SourceUpdatedAt) {
		t.Fatalf("watermark advanced on partial failure: before=%+v after=%+v", before, after)
	}
}

func TestUpstreamFailureModes(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	if err := repository.SaveSynced(context.Background(), spec,
		Dataset{Quote: &protocol.QuoteItem{Code: spec.Code, Price: 8.8}},
		SyncMetadata{SourceUpdatedAt: now.Add(-time.Hour), Quality: "validated"}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		calls.Add(1)
		return Dataset{}, SyncMetadata{}, errors.New("TDX unavailable")
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	if _, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh}); CodeOf(err) != ErrUpstream {
		t.Fatalf("RequireFresh error = %v", err)
	}
	stale, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: AllowStale})
	if err != nil || stale.Metadata.Freshness != "stale" || stale.Quote.Price != 8.8 {
		t.Fatalf("AllowStale result=%+v error=%v", stale, err)
	}
	diagnostics := service.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].SyncStatus != "stale" ||
		diagnostics[0].ErrorCode != ErrUpstream {
		t.Fatalf("stale diagnostics = %+v", diagnostics)
	}
	beforeCacheOnly := calls.Load()
	cached, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: CacheOnly})
	if err != nil || cached.Quote.Price != 8.8 {
		t.Fatalf("CacheOnly result=%+v error=%v", cached, err)
	}
	if calls.Load() != beforeCacheOnly {
		t.Fatal("CacheOnly called provider")
	}
	diagnostics = service.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].SyncStatus != "cache" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestCacheOnlyMissingDataReturnsStableCacheMiss(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{
		Type: DataKline, Market: "sh", Code: "600000",
		Granularity: "day", KType: 9,
	}
	var calls atomic.Int32
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		calls.Add(1)
		return Dataset{}, SyncMetadata{}, errors.New("provider must not be called")
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	_, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: CacheOnly})
	if CodeOf(err) != ErrCacheMiss {
		t.Fatalf("error = %v, want cache_miss", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("provider calls = %d", calls.Load())
	}
}

func TestTransactionFailureDoesNotAdvanceWatermark(t *testing.T) {
	store, repository := testRepository(t)
	if _, err := store.DB().Exec(`CREATE TRIGGER reject_quote BEFORE INSERT ON quote_snapshot
		BEGIN SELECT RAISE(ABORT, 'reject quote'); END`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		return Dataset{Quote: &protocol.QuoteItem{Code: spec.Code, Price: 10}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	if _, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh}); CodeOf(err) != ErrPersistence {
		t.Fatalf("Query error = %v", err)
	}
	var states int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM data_sync_state WHERE sync_key = ?`, syncKey(spec)).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if states != 0 {
		t.Fatalf("watermark rows = %d", states)
	}
}

func TestMismatchedUpstreamSecurityIsRejectedBeforeWrite(t *testing.T) {
	store, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataQuote, Market: "sh", Code: "600000"}
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		return Dataset{Quote: &protocol.QuoteItem{Code: "000001", Price: 10}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	if _, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh}); CodeOf(err) != ErrUpstream {
		t.Fatalf("Query error = %v", err)
	}
	var rows int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM quote_snapshot`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("mismatched quote was persisted: rows=%d", rows)
	}
}

func TestFinanceSyncThenCache(t *testing.T) {
	_, repository := testRepository(t)
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.Local)
	spec := DataSpec{Type: DataFinance, Market: "sh", Code: "600000"}
	var calls atomic.Int32
	service, _ := NewService(repository, providerFunc(func(context.Context, SyncRequest) (Dataset, SyncMetadata, error) {
		calls.Add(1)
		return Dataset{Finance: &protocol.FinanceInfo{ZongGuBen: 123}},
			SyncMetadata{SourceUpdatedAt: now, Quality: "validated"}, nil
	}), NewMarketFreshnessPolicy(WeekdayCalendar{}, time.Local), fixedClock{now})

	first, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Query(context.Background(), DataRequest{Spec: spec, Mode: RequireFresh})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || first.Finance.ZongGuBen != 123 || second.Metadata.SyncStatus != "cache" {
		t.Fatalf("calls=%d first=%+v second=%+v", calls.Load(), first, second)
	}
}

func TestSQLiteTradingCalendarUsesKnownHolidayAndFallsBackOutsideCoverage(t *testing.T) {
	store, _ := testRepository(t)
	friday := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	tuesday := time.Date(2026, 9, 29, 0, 0, 0, 0, time.Local)
	for _, day := range []time.Time{friday, tuesday} {
		if _, err := store.DB().Exec(`INSERT INTO workday(unix, date) VALUES (?, ?)`,
			day.Unix(), day.Format("20060102")); err != nil {
			t.Fatal(err)
		}
	}
	calendar, err := NewSQLiteTradingCalendar(store)
	if err != nil {
		t.Fatal(err)
	}
	holiday := time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)
	trading, err := calendar.IsTradingDay(context.Background(), "sh", holiday)
	if err != nil {
		t.Fatal(err)
	}
	if trading {
		t.Fatal("known weekday holiday was treated as a trading day")
	}
	outside := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local)
	trading, err = calendar.IsTradingDay(context.Background(), "sh", outside)
	if err != nil || !trading {
		t.Fatalf("outside coverage fallback = %v, err=%v", trading, err)
	}
}

func validKline(day time.Time, price float64) *protocol.Kline {
	return &protocol.Kline{
		Time: day, Open: price, High: price + 1, Low: price - 1,
		Close: price, Volume: 100, Amount: 1000,
	}
}
