package stockdata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type Service struct {
	repository Repository
	provider   Provider
	policy     FreshnessPolicy
	clock      Clock
	lifecycle  context.Context
	group      singleflight.Group

	diagnosticMu sync.RWMutex
	diagnostics  map[string]DecisionDiagnostic
}

type DecisionDiagnostic struct {
	SyncKey      string      `json:"sync_key"`
	Fresh        bool        `json:"fresh"`
	Reason       string      `json:"reason"`
	SyncStatus   string      `json:"sync_status"`
	ErrorCode    ErrorCode   `json:"error_code,omitempty"`
	SyncedRanges []TimeRange `json:"synced_ranges,omitempty"`
	ResultAsOf   time.Time   `json:"result_as_of,omitempty"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

func NewService(repository Repository, provider Provider, policy FreshnessPolicy, clock Clock) (*Service, error) {
	return NewServiceWithContext(context.Background(), repository, provider, policy, clock)
}

func NewServiceWithContext(
	lifecycle context.Context,
	repository Repository,
	provider Provider,
	policy FreshnessPolicy,
	clock Clock,
) (*Service, error) {
	if repository == nil || provider == nil || policy == nil {
		return nil, errors.New("repository, provider and policy are required")
	}
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{
		repository: repository, provider: provider, policy: policy, clock: clock,
		lifecycle: lifecycle, diagnostics: make(map[string]DecisionDiagnostic),
	}, nil
}

// InspectFreshness reports cached coverage and freshness without triggering a sync.
func (s *Service) InspectFreshness(ctx context.Context, spec DataSpec) (Coverage, FreshnessDecision, error) {
	if err := validateRequest(DataRequest{Spec: spec}); err != nil {
		return Coverage{}, FreshnessDecision{}, &Error{Code: ErrInvalidRequest, Op: "validate", Err: err}
	}
	coverage, err := s.repository.InspectCoverage(ctx, spec)
	if err != nil {
		return Coverage{}, FreshnessDecision{}, &Error{Code: ErrPersistence, Op: "inspect", Err: err}
	}
	decision, err := s.policy.Evaluate(ctx, s.clock.Now(), spec, coverage)
	if err != nil {
		return Coverage{}, FreshnessDecision{}, &Error{Code: ErrPersistence, Op: "freshness", Err: err}
	}
	return coverage, decision, nil
}

func (s *Service) Query(ctx context.Context, request DataRequest) (DataResult, error) {
	if err := validateRequest(request); err != nil {
		return DataResult{}, &Error{Code: ErrInvalidRequest, Op: "validate", Err: err}
	}
	if request.Mode == "" {
		request.Mode = RequireFresh
	}
	coverage, err := s.repository.InspectCoverage(ctx, request.Spec)
	if err != nil {
		return DataResult{}, &Error{Code: ErrPersistence, Op: "inspect", Err: err}
	}
	decision, err := s.policy.Evaluate(ctx, s.clock.Now(), request.Spec, coverage)
	if err != nil {
		return DataResult{}, &Error{Code: ErrPersistence, Op: "freshness", Err: err}
	}
	if request.ForceRefresh {
		decision.Fresh = false
		decision.Reason = "force_refresh"
		if len(decision.MissingRanges) == 0 {
			decision.MissingRanges = []TimeRange{{Start: request.Spec.Start, End: request.Spec.End}}
		}
	}

	if decision.Fresh || request.Mode == CacheOnly {
		if !coverage.Exists {
			return DataResult{}, &Error{Code: ErrCacheMiss, Op: "query_cache", Err: sql.ErrNoRows}
		}
		dataset, queryErr := s.repository.Query(ctx, request.Spec)
		if queryErr != nil {
			code := ErrPersistence
			if errors.Is(queryErr, sql.ErrNoRows) || !coverage.Exists {
				code = ErrCacheMiss
			}
			return DataResult{}, &Error{Code: code, Op: "query_cache", Err: queryErr}
		}
		result := makeResult(dataset, coverage, decision, "cache", nil)
		s.record(request.Spec, decision, "cache", "", nil, result.Metadata.AsOf)
		return result, nil
	}

	key := syncKey(request.Spec) + ":" + rangesKey(decision.MissingRanges)
	// The shared refresh must outlive any single waiter. Each caller can stop
	// waiting through its own context without canceling the refresh used by
	// other CLI/API callers for the same key.
	refresh := s.group.DoChan(key, func() (any, error) {
		return s.syncAndReload(s.lifecycle, request, decision)
	})
	var value any
	var syncErr error
	select {
	case <-ctx.Done():
		return DataResult{}, ctx.Err()
	case outcome := <-refresh:
		value, syncErr = outcome.Val, outcome.Err
	}
	if syncErr == nil {
		return value.(DataResult), nil
	}
	s.record(request.Spec, decision, "failed", CodeOf(syncErr), decision.MissingRanges, coverage.SourceUpdatedAt)
	if request.Mode == AllowStale && coverage.Exists {
		dataset, queryErr := s.repository.Query(ctx, request.Spec)
		if queryErr == nil {
			result := makeResult(dataset, coverage, decision, "stale", nil)
			result.Metadata.Freshness = "stale"
			result.Metadata.Reason = decision.Reason + ":upstream_failed"
			s.record(request.Spec, decision, "stale", CodeOf(syncErr), nil, result.Metadata.AsOf)
			return result, nil
		}
	}
	return DataResult{}, syncErr
}

func (s *Service) syncAndReload(ctx context.Context, request DataRequest, decision FreshnessDecision) (DataResult, error) {
	ranges := decision.MissingRanges
	if len(ranges) == 0 {
		ranges = []TimeRange{{Start: request.Spec.Start, End: request.Spec.End}}
	}
	var combined Dataset
	var combinedMetadata SyncMetadata
	for _, missing := range ranges {
		dataset, metadata, err := s.provider.Sync(ctx, SyncRequest{Spec: request.Spec, Range: missing})
		if err != nil {
			code := ErrUpstream
			if errors.Is(err, context.DeadlineExceeded) {
				code = ErrUpstreamTimeout
			}
			return DataResult{}, &Error{Code: code, Op: "provider_sync", Err: err}
		}
		if metadata.SourceUpdatedAt.IsZero() {
			metadata.SourceUpdatedAt = s.clock.Now()
		}
		if err := validateDataset(request.Spec, dataset, missing); err != nil {
			return DataResult{}, &Error{Code: ErrUpstream, Op: "validate_upstream", Err: err}
		}
		switch request.Spec.Type {
		case DataKline:
			combined.Klines = append(combined.Klines, dataset.Klines...)
		case DataQuote:
			combined.Quote = dataset.Quote
		case DataFinance:
			combined.Finance = dataset.Finance
		}
		if metadata.SourceUpdatedAt.After(combinedMetadata.SourceUpdatedAt) {
			combinedMetadata.SourceUpdatedAt = metadata.SourceUpdatedAt
		}
		if metadata.Quality != "" {
			combinedMetadata.Quality = metadata.Quality
		}
	}
	// All ranges are fetched and validated before any write. A later range
	// failure therefore cannot expose a partially refreshed database.
	if err := s.repository.SaveSynced(ctx, request.Spec, combined, combinedMetadata); err != nil {
		return DataResult{}, &Error{Code: ErrPersistence, Op: "save_transaction", Err: err}
	}

	// Deliberately re-read after commit. Provider memory is never returned.
	dataset, err := s.repository.Query(ctx, request.Spec)
	if err != nil {
		return DataResult{}, &Error{Code: ErrPersistence, Op: "reload", Err: err}
	}
	coverage, err := s.repository.InspectCoverage(ctx, request.Spec)
	if err != nil {
		return DataResult{}, &Error{Code: ErrPersistence, Op: "inspect_after_sync", Err: err}
	}
	result := makeResult(dataset, coverage, decision, "synced", ranges)
	s.record(request.Spec, decision, "synced", "", ranges, result.Metadata.AsOf)
	log.Printf(`{"event":"stock_data_sync","sync_key":%q,"reason":%q,"ranges":%d,"as_of":%q}`,
		syncKey(request.Spec), decision.Reason, len(ranges), result.Metadata.AsOf.Format(time.RFC3339))
	return result, nil
}

func (s *Service) Diagnostics() []DecisionDiagnostic {
	s.diagnosticMu.RLock()
	defer s.diagnosticMu.RUnlock()
	result := make([]DecisionDiagnostic, 0, len(s.diagnostics))
	for _, item := range s.diagnostics {
		result = append(result, item)
	}
	return result
}

func (s *Service) record(
	spec DataSpec,
	decision FreshnessDecision,
	status string,
	errorCode ErrorCode,
	ranges []TimeRange,
	asOf time.Time,
) {
	item := DecisionDiagnostic{
		SyncKey: syncKey(spec), Fresh: decision.Fresh, Reason: decision.Reason,
		SyncStatus: status, ErrorCode: errorCode,
		SyncedRanges: ranges, ResultAsOf: asOf, UpdatedAt: s.clock.Now(),
	}
	s.diagnosticMu.Lock()
	s.diagnostics[item.SyncKey] = item
	s.diagnosticMu.Unlock()
}

func validateRequest(request DataRequest) error {
	if request.Spec.Code == "" {
		return errors.New("code is required")
	}
	if request.ForceRefresh && request.Mode == CacheOnly {
		return errors.New("force refresh cannot be combined with cache_only")
	}
	switch request.Spec.Type {
	case DataKline:
		if request.Spec.KType == 0 {
			return errors.New("kline type is required")
		}
	case DataQuote, DataFinance:
	default:
		return fmt.Errorf("unsupported data type %q", request.Spec.Type)
	}
	return nil
}

func validateDataset(spec DataSpec, dataset Dataset, requested TimeRange) error {
	switch spec.Type {
	case DataKline:
		if len(dataset.Klines) == 0 {
			return errors.New("provider returned no klines")
		}
		seen := make(map[string]struct{}, len(dataset.Klines))
		for index, item := range dataset.Klines {
			if err := validateKlineRecord(item, time.Now()); err != nil {
				return fmt.Errorf("provider returned invalid kline at index %d: %w", index, err)
			}
			if (!requested.Start.IsZero() && item.Time.Before(requested.Start)) ||
				(!requested.End.IsZero() && item.Time.After(requested.End.Add(24*time.Hour-time.Nanosecond))) {
				return errors.New("provider returned kline outside requested range")
			}
			key := item.Time.Format("20060102")
			if _, exists := seen[key]; exists {
				return errors.New("provider returned duplicate kline")
			}
			seen[key] = struct{}{}
		}
	case DataQuote:
		if dataset.Quote == nil || dataset.Quote.Code == "" {
			return errors.New("provider returned invalid quote")
		}
		if normalizeSecurityCode(dataset.Quote.Code) != normalizeSecurityCode(spec.Code) {
			return errors.New("provider returned quote for a different security")
		}
	case DataFinance:
		if dataset.Finance == nil {
			return errors.New("provider returned invalid finance data")
		}
	}
	return nil
}

func normalizeSecurityCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 8 {
		switch value[:2] {
		case "sh", "sz", "bj":
			return value[2:]
		}
	}
	return value
}

func makeResult(dataset Dataset, coverage Coverage, decision FreshnessDecision, status string, ranges []TimeRange) DataResult {
	asOf := coverage.SourceUpdatedAt
	if asOf.IsZero() {
		asOf = coverage.End
	}
	freshness := "fresh"
	if status == "stale" {
		freshness = "stale"
	}
	return DataResult{
		Klines: dataset.Klines, Quote: dataset.Quote, Finance: dataset.Finance,
		Metadata: ResultMetadata{
			AsOf: asOf, Freshness: freshness, Reason: decision.Reason,
			SourceUpdatedAt: coverage.SourceUpdatedAt, SyncStatus: status, SyncedRanges: ranges,
		},
	}
}

func rangesKey(ranges []TimeRange) string {
	result := ""
	for _, item := range ranges {
		result += item.Start.Format("20060102") + "-" + item.End.Format("20060102") + ";"
	}
	return result
}
