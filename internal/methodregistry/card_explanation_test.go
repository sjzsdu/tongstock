package methodregistry

import (
	"github.com/sjzsdu/tongstock/internal/methods"
	"testing"
)

func TestCardExplanationIdentifiesExactRulesVersion(t *testing.T) {
	m := &Method{ID: "method", Name: "名称", Versions: []MethodVersion{{ID: "version-1", Method: &methods.CompiledMethod{Description: "真实策略用途", EntryRule: &methods.Expr{Type: methods.NodeAnd}}}}}
	card := toCard(m)
	if card.MethodVersionID != "version-1" || card.Description != "真实策略用途" || card.Rules == nil || card.Rules.EntryRule == nil {
		t.Fatalf("card missing explanation/version: %+v", card)
	}
}
