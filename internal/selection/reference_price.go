package selection

import (
	"math"

	"github.com/sjzsdu/tongstock/internal/marketsnapshot"
)

// ReferencePriceFromFeature reads only the explicitly named close feature.
// Callers must verify the snapshot identity and content before using it.
func ReferencePriceFromFeature(feature *marketsnapshot.FeatureSnapshot, code string) *ReferencePrice {
	if feature == nil {
		return nil
	}
	close, ok := feature.Values[code]["close"]
	if !ok || close <= 0 || math.IsNaN(close) || math.IsInf(close, 0) {
		return nil
	}
	return &ReferencePrice{Close: close, Date: feature.SnapshotDate, PriceAdjustment: feature.PriceAdjustment, FeatureSnapshotID: feature.ID}
}
