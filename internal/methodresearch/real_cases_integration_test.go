package methodresearch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/sjzsdu/tongstock/internal/methods"
)

// archivedSourceProvider returns evidence captured from the real URLs named in
// each case. It contains no generated prices, returns, confidence, or fake URLs.
type archivedSourceProvider struct{ draft *ResearchDraft }

func (p archivedSourceProvider) Research(context.Context, ResearchInput) (*ResearchDraft, error) {
	return p.draft, nil
}

func TestRealSourceCasesCompleteConflictAndInsufficient(t *testing.T) {
	t.Run("complete cited turtle rules compile and queue", func(t *testing.T) {
		result := runArchived(t, turtleDraft(), ResearchInput{Kind: InputName, Value: "海龟交易法", StockCode: "600519"})
		if result.Status != StatusComplete {
			t.Fatalf("status=%s reasons=%v", result.Status, result.RejectionReasons)
		}
		if len(result.Conflicts) != 0 || len(result.Compilations) != 1 || !result.Compilations[0].Executable {
			t.Fatalf("unexpected compilation: %+v", result.Compilations)
		}
		if len(result.ValidationJobs) != 1 || result.ValidationJobs[0].Status != "queued" {
			t.Fatalf("executable variant not queued: %+v", result.ValidationJobs)
		}
		if result.ResultHash == "" || result.ResultHash != result.ComputeHash() {
			t.Fatal("research hash is not reproducible")
		}
	})

	t.Run("real Yang secondary accounts remain explicit variants", func(t *testing.T) {
		result := runArchived(t, yangConflictDraft(), ResearchInput{Kind: InputName, Value: "杨永兴隔夜套利法"})
		if result.Status != StatusConflict {
			t.Fatalf("status=%s reasons=%v", result.Status, result.RejectionReasons)
		}
		if len(result.Conflicts) != 1 || result.Conflicts[0].Key != "market.max_cap_cny" {
			t.Fatalf("conflicts=%+v", result.Conflicts)
		}
		if len(result.Compilations) != 2 {
			t.Fatalf("variants silently lost: %+v", result.Compilations)
		}
		if result.FamilyID == "" || result.Compilations[0].FamilyID != result.FamilyID || result.Compilations[1].FamilyID != result.FamilyID {
			t.Fatalf("variants do not share family: %+v", result.Compilations)
		}
		for _, compiled := range result.Compilations {
			if compiled.Executable {
				t.Fatalf("intraday rule must not masquerade as daily executable: %+v", compiled)
			}
		}
	})

	t.Run("unknown named method fails closed", func(t *testing.T) {
		result := runArchived(t, &ResearchDraft{MethodName: "不存在的精确稳赚法"}, ResearchInput{Kind: InputName, Value: "不存在的精确稳赚法"})
		if result.Status != StatusInsufficient || len(result.Compilations) != 0 {
			t.Fatalf("insufficient source generated rules: %+v", result)
		}
	})
}

func runArchived(t *testing.T, draft *ResearchDraft, input ResearchInput) *ResearchResult {
	t.Helper()
	researcher, err := New(archivedSourceProvider{draft: draft}, nil)
	if err != nil {
		t.Fatal(err)
	}
	researcher.now = func() time.Time { return time.Date(2026, 8, 2, 3, 4, 5, 0, time.UTC) }
	result, err := researcher.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func archivedSource(id, sourceURL, title, publisher, excerpt string) SourceDocument {
	return SourceDocument{ID: id, URL: sourceURL, Title: title, Publisher: publisher,
		RetrievedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Tier: TierSecondary,
		ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(excerpt)))}
}

func number(v float64) *float64 { return &v }
func indicator(name string) *methods.Expr {
	return &methods.Expr{Type: methods.NodeIndicator, Indicator: name}
}
func compareExpr(left, right *methods.Expr, op methods.CmpOp) *methods.Expr {
	return &methods.Expr{Type: methods.NodeCompare, Left: left, Right: right, Op: op}
}

func turtleDraft() *ResearchDraft {
	entryExcerpt := "System One enters on a new twenty-day high or low."
	exitExcerpt := "System 1 exits long positions on a 10-day low breakout."
	positionExcerpt := "Small initial bets of two percent of account equity limit single-trade risk."
	return &ResearchDraft{MethodName: "海龟交易法 System 1",
		Sources: []SourceDocument{
			archivedSource("turtletrader", "https://www.turtletrader.com/rules/", "The Original Turtle Trading Rules", "TurtleTrader", entryExcerpt+positionExcerpt),
			archivedSource("followingtrend", "https://www.followingthetrend.com/?mdocs-file=2551", "OriginalTurtleRules.pdf", "Following the Trend", exitExcerpt),
		},
		Citations: []Citation{{ID: "c-entry", SourceID: "turtletrader", Locator: "The Rules / System One", Excerpt: entryExcerpt}, {ID: "c-exit", SourceID: "followingtrend", Locator: "Exits", Excerpt: exitExcerpt}, {ID: "c-position", SourceID: "turtletrader", Locator: "Risk aversion", Excerpt: positionExcerpt}},
		Claims: []RuleClaim{
			{ID: "entry-20", Field: "entry", Key: "entry.long_breakout", Value: "突破此前20日最高价", Provenance: ProvenanceSecondary, CitationIDs: []string{"c-entry"}},
			{ID: "exit-10", Field: "exit", Key: "exit.long_breakout", Value: "跌破此前10日最低价", Provenance: ProvenanceSecondary, CitationIDs: []string{"c-exit"}},
			{ID: "position-2pct", Field: "position", Key: "position.initial_equity", Value: "初始投入账户权益的2%", Provenance: ProvenanceSecondary, CitationIDs: []string{"c-position"}},
		},
		Variants: []VariantDraft{{ID: "system-1-long", Label: "System 1 多头", ClaimIDs: []string{"entry-20", "exit-10", "position-2pct"}, Candidate: methods.Candidate{
			Name: "海龟交易法 System 1 多头", SourceKind: "structured",
			Entry:        compareExpr(indicator("close"), indicator("prevhigh20"), methods.CmpGT),
			Exit:         compareExpr(indicator("close"), indicator("prevlow10"), methods.CmpLT),
			PositionMode: "pct_equity", PositionPct: number(0.02),
		}}}}
}

func yangConflictDraft() *ResearchDraft {
	first := "下午两点半后筛选，市值超过300亿剔除。"
	second := "下午2:30后筛选，市值小于200亿。"
	exit := "买入后次日早盘冲高卖出。"
	ambiguous := func(source string) *methods.Expr {
		return &methods.Expr{Type: methods.NodeAmbiguous, AmbiguousSource: source, AmbiguousReasons: []string{"日内分时规则尚无可复现分钟快照执行器"}}
	}
	base := methods.Candidate{Name: "杨永兴隔夜套利法", SourceKind: "natural_lang", Entry: ambiguous("尾盘分时选股"), Exit: ambiguous("次日早盘冲高卖出")}
	return &ResearchDraft{MethodName: "杨永兴隔夜套利法",
		Sources: []SourceDocument{
			archivedSource("sina-laohe", "https://www.sina.cn/news/detail/5319853612204726.html", "杨永兴隔夜套利法", "长红持筹者老何", first),
			archivedSource("sina-caijiajia", "https://www.sina.cn/news/detail/5290792584480386.html", "隔夜持股法步骤解析", "财佳佳V", second+exit),
		},
		Citations: []Citation{{ID: "yc1", SourceID: "sina-laohe", Locator: "步骤1、步骤3", Excerpt: first}, {ID: "yc2", SourceID: "sina-caijiajia", Locator: "第1、5步", Excerpt: second}, {ID: "yc3", SourceID: "sina-caijiajia", Locator: "退出规则", Excerpt: exit}},
		Claims: []RuleClaim{
			{ID: "entry-tail", Field: "entry", Key: "entry.time", Value: "14:30后", Provenance: ProvenanceSecondary, CitationIDs: []string{"yc1", "yc2"}},
			{ID: "cap-300", Field: "market", Key: "market.max_cap_cny", Value: "300亿元", Provenance: ProvenanceSecondary, CitationIDs: []string{"yc1"}},
			{ID: "cap-200", Field: "market", Key: "market.max_cap_cny", Value: "200亿元", Provenance: ProvenanceSecondary, CitationIDs: []string{"yc2"}},
			{ID: "exit-morning", Field: "exit", Key: "exit.time", Value: "次日早盘冲高卖出（无精确时刻）", Provenance: ProvenanceSecondary, CitationIDs: []string{"yc3"}},
			{ID: "position-missing", Field: "position", Key: "position.sizing", Value: "二手来源未说明仓位大小", Provenance: ProvenanceSecondary, CitationIDs: []string{"yc3"}},
		},
		Variants: []VariantDraft{
			{ID: "secondary-cap-300", Label: "二手来源300亿上限", ClaimIDs: []string{"entry-tail", "cap-300", "exit-morning", "position-missing"}, Candidate: base},
			{ID: "secondary-cap-200", Label: "二手来源200亿上限", ClaimIDs: []string{"entry-tail", "cap-200", "exit-morning", "position-missing"}, Candidate: base},
		}}
}
