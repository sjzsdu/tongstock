import { describe, expect, it } from 'vitest';
import type { SelectionCandidate, SelectionRun, SelectionTrigger } from '../api/client';
import {
  FACTOR_METHOD_ID,
  attributeCandidate,
  buyCapPerMethod,
  flatViewNote,
  groupCandidatesByMethod,
  hasMethodAttribution,
  isTruncatedDowngrade,
} from './selectionGrouping';

function trigger(methodId: string, methodName = methodId.toUpperCase()): SelectionTrigger {
  return { method_id: methodId, method_name: methodName, score: 0.7, facts: [] };
}

function candidate(overrides: Partial<SelectionCandidate> & { code: string }): SelectionCandidate {
  return {
    rank: 1,
    action: 'buy',
    score: 0.8,
    data_date: '2026-10-28',
    buy_window: '次日开盘',
    position_cap_pct: 0.1,
    exit: { max_holding_days: 5, complete: true },
    explanation: '',
    triggers: [],
    ...overrides,
  };
}

function run(overrides: Partial<SelectionRun> = {}): SelectionRun {
  return {
    id: 'selection-1',
    snapshot_id: 'snap-1',
    feature_snapshot_id: 'feat-1',
    snapshot_date: '2026-10-28',
    candidate_count: 0,
    buy_count: 0,
    candidates: [],
    exclusions: [],
    ...overrides,
  };
}

/** 两个方法各自 5 只名额内买入 + 各自 1 只被截断降级。 */
function twoMethodRun(): SelectionRun {
  const candidates: SelectionCandidate[] = [];
  for (let i = 0; i < 5; i += 1) {
    candidates.push(candidate({
      code: `A${i}`,
      kept_by_method_ids: ['method-a'],
      triggers: [trigger('method-a', '放量突破')],
    }));
    candidates.push(candidate({
      code: `B${i}`,
      kept_by_method_ids: ['method-b'],
      triggers: [trigger('method-b', '回调企稳')],
    }));
  }
  candidates.push(candidate({
    code: 'A99',
    action: 'watch',
    truncated_from_buy: true,
    truncated_by_method_ids: ['method-a'],
    triggers: [trigger('method-a', '放量突破')],
  }));
  candidates.push(candidate({
    code: 'B99',
    action: 'watch',
    truncated_from_buy: true,
    truncated_by_method_ids: ['method-b'],
    triggers: [trigger('method-b', '回调企稳')],
  }));
  return run({ buy_cap_per_method: 5, buy_count_by_method: { 'method-a': 5, 'method-b': 5 }, truncated_from_buy: 2, candidates, candidate_count: candidates.length, buy_count: 10 });
}

describe('buyCapPerMethod', () => {
  it('读 run 上的字段，不在前端写死 5', () => {
    expect(buyCapPerMethod(run({ buy_cap_per_method: 3 }))).toBe(3);
    expect(buyCapPerMethod(run({ buy_cap_per_method: 5 }))).toBe(5);
  });

  it('旧 run 没有该字段时视为未知，而不是猜一个默认值', () => {
    expect(buyCapPerMethod(run())).toBeUndefined();
    expect(buyCapPerMethod(undefined)).toBeUndefined();
    expect(buyCapPerMethod(run({ buy_cap_per_method: 0 }))).toBeUndefined();
  });
});

describe('groupCandidatesByMethod', () => {
  it('按方法分组：每组只装该方法名额内的 buy 候选，并带上该方法的上限', () => {
    const groups = groupCandidatesByMethod(twoMethodRun().candidates, 5);
    expect(groups.map((g) => g.methodId).sort()).toEqual(['method-a', 'method-b']);
    for (const group of groups) {
      expect(group.kept).toHaveLength(5);
      expect(group.cap).toBe(5);
      expect(group.kept.every((c) => c.code.startsWith(group.methodId === 'method-a' ? 'A' : 'B'))).toBe(true);
    }
  });

  it('被多个方法保留的票在每个分组里都出现，且归属方法数为 2', () => {
    const shared = candidate({
      code: 'SHARED',
      kept_by_method_ids: ['method-a', 'method-b'],
      triggers: [trigger('method-a', '放量突破'), trigger('method-b', '回调企稳')],
    });
    const groups = groupCandidatesByMethod([shared], 5);
    expect(groups).toHaveLength(2);
    for (const group of groups) expect(group.kept.map((c) => c.code)).toEqual(['SHARED']);
    expect(attributeCandidate(shared).kept).toHaveLength(2);
  });

  it('被一个方法截断、被另一个方法保住的票仍然是 buy，同时出现在两处', () => {
    const mixed = candidate({
      code: 'MIXED',
      kept_by_method_ids: ['method-a'],
      truncated_by_method_ids: ['method-b'],
      triggers: [trigger('method-a', '放量突破'), trigger('method-b', '回调企稳')],
    });
    const groups = groupCandidatesByMethod([mixed], 5);
    const a = groups.find((g) => g.methodId === 'method-a')!;
    const b = groups.find((g) => g.methodId === 'method-b')!;
    expect(a.kept.map((c) => c.code)).toEqual(['MIXED']);
    expect(b.kept).toHaveLength(0);
    expect(b.truncated.map((c) => c.code)).toEqual(['MIXED']);
    expect(isTruncatedDowngrade(mixed)).toBe(false);
  });

  it('名额已满降级观察与真正的 watch 分得开', () => {
    const downgraded = candidate({
      code: 'CAPPED',
      action: 'watch',
      truncated_from_buy: true,
      truncated_by_method_ids: ['method-a'],
      triggers: [trigger('method-a', '放量突破')],
    });
    const watch = candidate({
      code: 'WATCHED',
      action: 'watch',
      kept_by_method_ids: ['method-a'],
      triggers: [trigger('method-a', '放量突破')],
    });
    const groups = groupCandidatesByMethod([downgraded, watch], 5);
    const group = groups[0];
    expect(group.truncated.map((c) => c.code)).toEqual(['CAPPED']);
    expect(group.others.map((c) => c.code)).toEqual(['WATCHED']);
    expect(group.kept).toHaveLength(0);
    expect(isTruncatedDowngrade(downgraded)).toBe(true);
    expect(isTruncatedDowngrade(watch)).toBe(false);
  });

  it('因子通道候选不占名额，单独成组且不进买入名额', () => {
    const factor = candidate({
      code: 'FACTOR1',
      action: 'watch',
      triggers: [trigger(FACTOR_METHOD_ID, '因子组合（截面排序）')],
    });
    const groups = groupCandidatesByMethod([factor], 5);
    expect(groups).toHaveLength(1);
    expect(groups[0].methodId).toBe(FACTOR_METHOD_ID);
    expect(groups[0].methodName).toBe('因子组合（截面排序）');
    expect(groups[0].kept).toHaveLength(0);
    expect(groups[0].others.map((c) => c.code)).toEqual(['FACTOR1']);
  });

  it('因子通道触发事实没带方法名时用可读的兜底名，不把 ID 直接怼给用户', () => {
    const factor = candidate({
      code: 'FACTOR1',
      action: 'watch',
      triggers: [{ method_id: FACTOR_METHOD_ID, method_name: '', score: 0.5, facts: [] }],
    });
    expect(groupCandidatesByMethod([factor], 5)[0].methodName).toBe('因子通道');
  });

  it('旧 run 没有名额归属字段时按触发方法退让分组，不把 buy 丢掉', () => {
    const legacy = candidate({ code: 'OLD1', kept_by_method_ids: undefined, triggers: [trigger('method-a', '放量突破')] });
    expect(hasMethodAttribution([legacy])).toBe(false);
    const groups = groupCandidatesByMethod([legacy]);
    expect(groups).toHaveLength(1);
    expect(groups[0].methodName).toBe('放量突破');
    expect(groups[0].kept.map((c) => c.code)).toEqual(['OLD1']);
    expect(groups[0].cap).toBeUndefined();
  });

  it('空榜单返回空分组，不伪造空壳方法组', () => {
    expect(groupCandidatesByMethod([])).toEqual([]);
  });
});

describe('flatViewNote', () => {
  it('买入总数超过单方法上限时说明来源方法数', () => {
    const note = flatViewNote(twoMethodRun());
    expect(note.buyCount).toBe(10);
    expect(note.methodCount).toBe(2);
    expect(note.cap).toBe(5);
    expect(note.exceedsCap).toBe(true);
    expect(note.truncatedCount).toBe(2);
  });

  it('买入数未超过上限时不触发 per-method 说明', () => {
    const note = flatViewNote(run({
      buy_cap_per_method: 5,
      candidates: [
        candidate({ code: 'A0', kept_by_method_ids: ['method-a'], triggers: [trigger('method-a')] }),
        candidate({ code: 'A1', kept_by_method_ids: ['method-a'], triggers: [trigger('method-a')] }),
      ],
      buy_count: 2,
    }));
    expect(note.buyCount).toBe(2);
    expect(note.methodCount).toBe(1);
    expect(note.exceedsCap).toBe(false);
  });

  it('空榜单：没有买入也没有方法来源，且不判定为超限', () => {
    const note = flatViewNote(run());
    expect(note).toEqual({ buyCount: 0, methodCount: 0, cap: undefined, truncatedCount: 0, exceedsCap: false });
  });

  it('旧 run 用触发方法兜底统计来源方法数', () => {
    const note = flatViewNote(run({
      candidates: [
        candidate({ code: 'A0', triggers: [trigger('method-a'), trigger('method-b')] }),
        candidate({ code: 'A1', triggers: [trigger('method-b')] }),
      ],
      buy_count: 2,
    }));
    expect(note.methodCount).toBe(2);
    expect(note.cap).toBeUndefined();
    expect(note.exceedsCap).toBe(false);
  });
});
