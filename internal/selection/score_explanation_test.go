package selection

import (
	"encoding/json"
	"github.com/sjzsdu/tongstock/internal/methodregistry"
	"math"
	"testing"
)

func TestScoreExplanationPreservesActualDecisionInputs(t *testing.T) {
	health := &methodregistry.HealthState{Score: .6}
	m := &methodregistry.Method{ID: "m", Health: health, Versions: []methodregistry.MethodVersion{{Evidence: &methodregistry.EvidenceSummary{Confidence: "strong", OOSTrades: 120, OOSMaxDrawdown: -.25}}}}
	detail := methodScoreExplanation(m, map[string]float64{})
	// .72 + .04 - .08 - .08 = .6; .75*.6 + .25*.6 = .6
	if detail.Score != .6 || math.Abs(detail.BeforeHealth-.6) > 1e-10 || detail.SampleBonus != .04 || detail.DrawdownPenalty != .08 || detail.MissingAmountPenalty != .08 || detail.HealthScore == nil {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Score != methodScore(m, map[string]float64{}) {
		t.Fatal("explanation changed score")
	}
}
func TestCandidateScoreExplanationMatches06372(t *testing.T) {
	m := &methodregistry.Method{ID: "m", Versions: []methodregistry.MethodVersion{{Evidence: &methodregistry.EvidenceSummary{Confidence: "moderate", OOSTrades: 100}}}}
	triggers := []Trigger{{MethodID: "m", Score: .59}, {MethodID: "m", Score: .59}, {MethodID: "m", Score: .59}}
	detail := candidateScoreExplanation(triggers, map[string]*methodregistry.Method{"m": m}, map[string]float64{"amount": 1}, ExitPlan{Complete: true})
	if aggregateScore(triggers) != .6372 {
		t.Fatalf("score %v", aggregateScore(triggers))
	}
	if len(detail.ExtraTriggerBonuses) != 2 || math.Abs(detail.ExtraTriggerBonuses[0]-.0236) > 1e-10 || detail.BuyThreshold != .65 || !detail.ExitComplete {
		t.Fatalf("detail %+v", detail)
	}
}
func TestOldCandidateRemainsReadableWithoutScoreExplanation(t *testing.T) {
	var c Candidate
	if err := json.Unmarshal([]byte(`{"code":"600000","score":0.6372,"action":"watch","triggers":[]}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.ScoreExplanation != nil || c.Score != .6372 {
		t.Fatalf("legacy candidate %+v", c)
	}
}
