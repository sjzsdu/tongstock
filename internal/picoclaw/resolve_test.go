package picoclaw

import "testing"

func TestResolveEmbeddedAgentAcceptsAliasAndCaseInsensitiveID(t *testing.T) {
	agents := []EmbeddedAgent{{ID: "Risk-Reviewer", Aliases: []string{"risk", "风险复核"}}}

	for _, input := range []string{"risk-reviewer", " RISK-REVIEWER ", "RISK", " 风险复核 "} {
		t.Run(input, func(t *testing.T) {
			got, ok := ResolveEmbeddedAgent(agents, input)
			if !ok {
				t.Fatalf("ResolveEmbeddedAgent(%q) did not match", input)
			}
			if got.ID != "Risk-Reviewer" {
				t.Fatalf("canonical ID = %q, want %q", got.ID, "Risk-Reviewer")
			}
		})
	}

	if _, ok := ResolveEmbeddedAgent(agents, "missing"); ok {
		t.Fatal("unexpected match for unknown agent")
	}
}
