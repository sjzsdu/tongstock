package paradigms

import (
	"testing"
)

func TestIsValidTransition(t *testing.T) {
	cases := []struct {
		from, to, action string
		want             bool
	}{
		{StatePending, StateReviewed, "review", true},
		{StatePending, StateRejected, "reject", true},
		{StateReviewed, StateVerified, "verify", true},
		{StateReviewed, StateRejected, "reject", true},
		{StateVerified, StatePromoted, "promote", true},
		{StateVerified, StateSuspended, "suspend", true},
		{StateVerified, StateDegraded, "downgrade", true},
		{StateVerified, StateRejected, "reject", true},
		{StatePromoted, StateDegraded, "downgrade", true},
		{StatePromoted, StateSuspended, "suspend", true},
		{StatePromoted, StateRejected, "reject", true},
		{StateDegraded, StatePromoted, "promote", true},
		{StateDegraded, StateSuspended, "suspend", true},
		{StateDegraded, StateRejected, "reject", true},
		{StateSuspended, StateVerified, "resume", true},
		{StateSuspended, StatePromoted, "promote", true},
		{StateSuspended, StateRejected, "reject", true},

		// 非法转换
		{StateVerified, StateReviewed, "", false},
		{StateRejected, StateVerified, "", false}, // 淘汰不可恢复
		{StatePending, StatePromoted, "", false},
		{StatePromoted, StateVerified, "", false},
		{StatePromoted, StatePromoted, "", false},
	}
	for _, c := range cases {
		got := IsValidTransition(c.from, c.to, c.action)
		if got != c.want {
			t.Errorf("IsValidTransition(%q, %q, %q) = %v; want %v",
				c.from, c.to, c.action, got, c.want)
		}
	}
}

func TestValidateTransitionError(t *testing.T) {
	if err := ValidateTransition(StateVerified, StateVerified, ""); err == nil {
		t.Error("expected error for same-state transition")
	}
	if err := ValidateTransition(StatePending, StatePromoted, ""); err == nil {
		t.Error("expected error for illegal transition")
	}
	if err := ValidateTransition(StateVerified, StatePromoted, "promote"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildTransitionRecord(t *testing.T) {
	p := &Paradigm{ID: "p1", ReviewStatus: StateVerified}
	rec := BuildTransitionRecord(p, StatePromoted, "promote", "样本外稳定超过阈值", "admin", "hash-1", false)
	if rec.ParadigmID != "p1" {
		t.Errorf("wrong paradigm id: %s", rec.ParadigmID)
	}
	if rec.From != StateVerified || rec.To != StatePromoted {
		t.Errorf("wrong transition: %s -> %s", rec.From, rec.To)
	}
	if rec.Action != "promote" {
		t.Errorf("wrong action: %s", rec.Action)
	}
	if rec.Auto {
		t.Error("expected auto=false")
	}
}

func TestStoreTransition(t *testing.T) {
	store, _ := NewStore("")
	p := &Paradigm{ID: "p1", Name: "test", ReviewStatus: StatePending}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}

	// pending -> reviewed
	got, rec, ver, err := store.Transition("p1", StateReviewed, "人工评审通过", "admin", "", false)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	if got.ReviewStatus != StateReviewed {
		t.Errorf("expected reviewed, got %s", got.ReviewStatus)
	}
	if rec.Action != "review" {
		t.Errorf("expected review action, got %s", rec.Action)
	}
	if ver == nil || ver.Version < 1 {
		t.Errorf("expected version record, got %+v", ver)
	}

	// reviewed -> verified
	_, _, _, err = store.Transition("p1", StateVerified, "样本外验证", "admin", "", false)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	// verified -> promoted
	_, _, _, err = store.Transition("p1", StatePromoted, "达到晋级标准", "admin", "hash-1", false)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	// promoted -> rejected 应该失败
	if _, _, _, err = store.Transition("p1", StateVerified, "无法恢复", "admin", "", false); err == nil {
		t.Error("expected error for promoted->verified (illegal)")
	}

	// check versions count
	versions := store.GetVersions("p1")
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}

	// check transitions count
	loaded, _ := store.Get("p1")
	if len(loaded.Transitions) != 3 {
		t.Errorf("expected 3 transitions, got %d", len(loaded.Transitions))
	}
}

func TestStoreTransitionInvalid(t *testing.T) {
	store, _ := NewStore("")
	p := &Paradigm{ID: "p2", Name: "x", ReviewStatus: "unknown"}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.Transition("p2", StatePromoted, "bad", "admin", "", false); err == nil {
		t.Error("expected error for unknown from state")
	}
}

func TestHelpers(t *testing.T) {
	if !CanShowOnDiscover(StateVerified) {
		t.Error("verified should be shown on discover")
	}
	if !CanShowOnDiscover(StatePromoted) {
		t.Error("promoted should be shown on discover")
	}
	if CanShowOnDiscover(StateRejected) {
		t.Error("rejected should NOT be shown on discover")
	}
	if !IsDecisionActive(StatePromoted) {
		t.Error("promoted should be active decision")
	}
	if !IsDecisionActive(StateVerified) {
		t.Error("verified should be active decision")
	}
	statuses := ListDecisionStatuses()
	if len(statuses) != 2 {
		t.Errorf("expected 2 decision statuses, got %d", len(statuses))
	}
}
