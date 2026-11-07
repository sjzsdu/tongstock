// Package runinput fingerprints the inputs of one deterministic computation.
//
// A snapshot id names a stored artifact; it says nothing about the facts a run
// consumed, so two runs over the same snapshot id can disagree after a K-line
// correction. The digest computed here is the reproducible identity of the
// inputs instead: trade date, universe, per-code latest bar date, K-line data
// version, the feature set, and the engine version.
//
// This package is deliberately not imported by internal/selection or
// internal/positiondecision: recompute paths keep their own identity until the
// as-of read path replaces them. Only `tongstock reconcile asof` and tests use
// it today.
package runinput

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
	"github.com/sjzsdu/tongstock/pkg/storage"
)

// FeatureEngineVersion names the implementation that materializes features.
// Bump it whenever an indicator definition changes, otherwise a digest would
// claim two different feature streams are the same input.
const FeatureEngineVersion = "sqlite-feature-engine-v1"

// dayKType mirrors the daily bar type used by the K-line stream.
const dayKType = 9

// FeatureInput is one entry of the feature manifest: which feature, and which
// implementation produced it. Sorted by name before hashing.
type FeatureInput struct {
	Name          string `json:"name"`
	EngineVersion string `json:"engine_version"`
}

// Inputs is the complete, serializable set of facts one computation consumed.
type Inputs struct {
	TradeDate              string            `json:"trade_date"`
	Universe               []string          `json:"universe"`
	UniverseHash           string            `json:"universe_hash"`
	LatestBarDate          map[string]string `json:"latest_bar_date"`
	DataVersion            int64             `json:"data_version"`
	Features               []FeatureInput    `json:"features"`
	SelectionEngineVersion string            `json:"selection_engine_version"`
}

// Digest hashes the inputs deterministically. Map keys are sorted by
// encoding/json, the universe and feature lists are sorted here, so the same
// facts always produce the same hex string.
func Digest(in Inputs) (string, error) {
	payload := in
	payload.TradeDate = strings.TrimSpace(payload.TradeDate)
	payload.Universe = append([]string(nil), payload.Universe...)
	sort.Strings(payload.Universe)
	if payload.Universe == nil {
		payload.Universe = []string{}
	}
	payload.UniverseHash = universeHash(payload.Universe)
	payload.Features = append([]FeatureInput(nil), payload.Features...)
	sort.Slice(payload.Features, func(i, j int) bool {
		if payload.Features[i].Name == payload.Features[j].Name {
			return payload.Features[i].EngineVersion < payload.Features[j].EngineVersion
		}
		return payload.Features[i].Name < payload.Features[j].Name
	})
	if payload.Features == nil {
		payload.Features = []FeatureInput{}
	}
	if payload.LatestBarDate == nil {
		payload.LatestBarDate = map[string]string{}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal run inputs: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Build collects the inputs of a computation for tradeDate (YYYY-MM-DD).
// Every K-line query is bounded by `date <= tradeDate`, so the digest can never
// be computed from facts the trade date could not have seen.
func Build(ctx context.Context, db *sql.DB, tradeDate string, features []marketsnapshot.FeatureSpec, selectionEngineVersion string) (Inputs, error) {
	if db == nil {
		return Inputs{}, fmt.Errorf("build run inputs requires a database handle")
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(tradeDate))
	if err != nil {
		return Inputs{}, fmt.Errorf("trade date must be YYYY-MM-DD: %w", err)
	}
	date := parsed.Format("2006-01-02")
	compact := strings.ReplaceAll(date, "-", "")

	version, err := storage.DataVersion(ctx, db, storage.KlineDataVersionKey)
	if err != nil {
		return Inputs{}, fmt.Errorf("read kline data_version: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT code, MAX(date) AS last_date
		FROM kline_latest
		WHERE ktype = ? AND date <= ?
		GROUP BY code
		ORDER BY code`, dayKType, compact)
	if err != nil {
		return Inputs{}, fmt.Errorf("read latest bar dates up to %s: %w", date, err)
	}
	defer rows.Close()

	latest := map[string]string{}
	universe := []string{}
	for rows.Next() {
		var code, last string
		if err := rows.Scan(&code, &last); err != nil {
			return Inputs{}, err
		}
		latest[code] = last
		if last == compact {
			universe = append(universe, code)
		}
	}
	if err := rows.Err(); err != nil {
		return Inputs{}, err
	}
	sort.Strings(universe)

	return Inputs{
		TradeDate:              date,
		Universe:               universe,
		LatestBarDate:          latest,
		DataVersion:            version,
		Features:               featureInputs(features),
		SelectionEngineVersion: selectionEngineVersion,
	}, nil
}

// featureInputs normalizes a feature spec list into the manifest form
// (unique names, engine version attached).
func featureInputs(features []marketsnapshot.FeatureSpec) []FeatureInput {
	seen := map[string]bool{}
	out := make([]FeatureInput, 0, len(features))
	for _, f := range features {
		name := strings.TrimSpace(f.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, FeatureInput{Name: name, EngineVersion: FeatureEngineVersion})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func universeHash(codes []string) string {
	h := sha256.New()
	for _, c := range codes {
		h.Write([]byte(c))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
