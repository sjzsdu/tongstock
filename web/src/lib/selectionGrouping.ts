import type { SelectionCandidate, SelectionRun } from '../api/client';

/**
 * 选股结果页的展示口径：per-method 买入上限落地后，一次 run 的 buy 总数可以
 * 达到 `BuyCapPerMethod × 方法数`。默认的分组视图把这层结构还给用户，平铺
 * 视图保留原有列表但必须标明来源方法数，避免用户误以为单个方法吐了这么多。
 */

/** 因子通道候选在 selection 产物里的方法 ID（与后端 selection.FactorMethodID 一致）。 */
export const FACTOR_METHOD_ID = 'factor-combo';
export const FACTOR_METHOD_NAME = '因子通道';

/** 「同时被 N 个方法选中」的判定口径：把该票计入买入名额的方法数。 */
export interface CandidateMethodAttribution {
  /** 把该票计入各自买入名额的方法 ID（决定它为什么是 buy）。 */
  kept: string[];
  /** 触发了它、但名额已满把它排在外面的方法 ID。 */
  truncated: string[];
  /** 触发它的全部方法 ID（含因子通道）。 */
  triggered: string[];
  /** kept / truncated 之外、触发它的方法（真 watch / avoid / 数据不足）。 */
  others: string[];
}

/** 该方法分组里的一条候选：kept = 名额内买入，truncated = 被该方法截断。 */
export interface MethodGroup {
  methodId: string;
  methodName: string;
  /** 该方法本次的买入名额上限；旧 run 上没有该字段时为 undefined。 */
  cap?: number;
  /** 名额内买入的候选（≤ cap）。 */
  kept: SelectionCandidate[];
  /** 因该方法名额已满被截断的候选（可能仍被别的方法保住而仍是 buy）。 */
  truncated: SelectionCandidate[];
  /** 触发该方法但没占名额的候选：真 watch / avoid / 数据不足 / 旧 run 无归属字段。 */
  others: SelectionCandidate[];
}

/** 平铺视图顶部的一句话说明：为什么买入选股总数会超过单个方法的上限。 */
export interface FlatViewNote {
  buyCount: number;
  methodCount: number;
  cap?: number;
  truncatedCount: number;
  /** 总数超过单方法上限（此时必须解释 per-method 口径）。 */
  exceedsCap: boolean;
}

/** 后端 per-method 上限来自 run 字段；读不到（旧 run）时就当作「未知」，不猜 5。 */
export function buyCapPerMethod(run?: Pick<SelectionRun, 'buy_cap_per_method'> | null): number | undefined {
  const cap = run?.buy_cap_per_method;
  return typeof cap === 'number' && cap > 0 ? cap : undefined;
}

/** 旧 run（per-method 口径之前）没有 kept/truncated 归属，分组只能按触发方法退让。 */
export function hasMethodAttribution(candidates: SelectionCandidate[]): boolean {
  return candidates.some(
    (c) => (c.kept_by_method_ids?.length ?? 0) > 0 || (c.truncated_by_method_ids?.length ?? 0) > 0,
  );
}

function unique(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean)));
}

export function triggerMethodIds(candidate: SelectionCandidate): string[] {
  return unique((candidate.triggers ?? []).map((t) => t.method_id));
}

export function keptMethodIds(candidate: SelectionCandidate, legacy = false): string[] {
  if (candidate.kept_by_method_ids?.length) return unique(candidate.kept_by_method_ids);
  // 旧 run 没有名额归属字段：所有触发方法（因子通道除外）都算「选中」。
  if (legacy && candidate.action === 'buy') {
    return triggerMethodIds(candidate).filter((id) => id !== FACTOR_METHOD_ID);
  }
  return [];
}

export function truncatedMethodIds(candidate: SelectionCandidate): string[] {
  const kept = new Set(keptMethodIds(candidate));
  return unique(candidate.truncated_by_method_ids ?? []).filter((id) => !kept.has(id));
}

/** 名额已满导致的降级观察：不是「股票不达标」，界面必须和真 watch 区分。 */
export function isTruncatedDowngrade(candidate: SelectionCandidate): boolean {
  return candidate.truncated_from_buy === true && candidate.action !== 'buy';
}

export function attributeCandidate(candidate: SelectionCandidate, legacy = false): CandidateMethodAttribution {
  const kept = keptMethodIds(candidate, legacy);
  const truncated = truncatedMethodIds(candidate);
  const triggered = triggerMethodIds(candidate);
  const placed = new Set([...kept, ...truncated]);
  // 分组视图里「其它」只装既非名额内、也非被截断的触发方：
  // 真 watch / avoid / insufficient_data（含因子通道）。
  const others = triggered.filter((id) => !placed.has(id));
  return { kept, truncated, triggered, others };
}

/** 方法 ID → 展示名；run 只带 ID 的事实明细里优先用方法名。 */
export function methodNameMap(candidates: SelectionCandidate[]): Map<string, string> {
  const names = new Map<string, string>();
  for (const c of candidates) {
    for (const t of c.triggers ?? []) {
      if (t.method_id && t.method_name && !names.has(t.method_id)) names.set(t.method_id, t.method_name);
    }
  }
  if (!names.has(FACTOR_METHOD_ID)) names.set(FACTOR_METHOD_ID, FACTOR_METHOD_NAME);
  return names;
}

export function methodDisplayName(methodId: string, names: Map<string, string>): string {
  return names.get(methodId) ?? methodId;
}

function byScoreThenCode(a: SelectionCandidate, b: SelectionCandidate): number {
  if (a.score === b.score) return a.code < b.code ? -1 : a.code > b.code ? 1 : 0;
  return b.score - a.score;
}

/**
 * 按方法分组。每组包含：该方法名额内的 buy 候选、被该方法截断的候选、以及
 * 仅被该方法触发却没占名额的候选（真 watch 等）。
 * 同一只票被多个方法保留时会在多个分组里各出现一次 —— 这是分组视图的要点，
 * 调用方用 `attribution.kept.length` 标注「同时被 N 个方法选中」。
 */
export function groupCandidatesByMethod(
  candidates: SelectionCandidate[],
  cap?: number,
): MethodGroup[] {
  const legacy = !hasMethodAttribution(candidates);
  const names = methodNameMap(candidates);
  const groups = new Map<string, MethodGroup>();
  const ensure = (methodId: string): MethodGroup => {
    const existing = groups.get(methodId);
    if (existing) return existing;
    const created: MethodGroup = {
      methodId,
      methodName: methodDisplayName(methodId, names),
      cap,
      kept: [],
      truncated: [],
      others: [],
    };
    groups.set(methodId, created);
    return created;
  };

  for (const candidate of candidates) {
    const attribution = attributeCandidate(candidate, legacy);
    for (const methodId of attribution.kept) {
      if (candidate.action === 'buy') ensure(methodId).kept.push(candidate);
      else ensure(methodId).others.push(candidate);
    }
    for (const methodId of attribution.truncated) ensure(methodId).truncated.push(candidate);
    for (const methodId of attribution.others) ensure(methodId).others.push(candidate);
  }

  const list = Array.from(groups.values());
  for (const group of list) {
    group.kept.sort(byScoreThenCode);
    group.truncated.sort(byScoreThenCode);
    group.others.sort(byScoreThenCode);
    group.cap = cap;
  }
  // 有买入名额的方法排前面；同等名额按被截断数、组内最高分、方法名定序（可复现）。
  list.sort((a, b) => {
    if (a.kept.length !== b.kept.length) return b.kept.length - a.kept.length;
    if (a.truncated.length !== b.truncated.length) return b.truncated.length - a.truncated.length;
    const bestA = Math.max(0, ...a.kept.concat(a.truncated, a.others).map((c) => c.score));
    const bestB = Math.max(0, ...b.kept.concat(b.truncated, b.others).map((c) => c.score));
    if (bestA !== bestB) return bestB - bestA;
    return a.methodName < b.methodName ? -1 : a.methodName > b.methodName ? 1 : 0;
  });
  return list;
}

/** 平铺视图的说明数据：总买入数、来源方法数、单方法上限、降级数量。 */
export function flatViewNote(run: SelectionRun): FlatViewNote {
  const candidates = run.candidates ?? [];
  const legacy = !hasMethodAttribution(candidates);
  const cap = buyCapPerMethod(run);
  const buys = candidates.filter((c) => c.action === 'buy');
  const methods = new Set<string>();
  for (const candidate of buys) {
    for (const id of attributeCandidate(candidate, legacy).kept) methods.add(id);
  }
  // 旧 run 没有归属字段时退到「触发过买入候选的方法」，至少不让方法数显示为 0。
  if (methods.size === 0) {
    for (const candidate of buys) {
      for (const id of triggerMethodIds(candidate)) if (id !== FACTOR_METHOD_ID) methods.add(id);
    }
  }
  const buyCount = buys.length;
  const truncatedCount = candidates.filter(isTruncatedDowngrade).length;
  return {
    buyCount,
    methodCount: methods.size,
    cap,
    truncatedCount,
    exceedsCap: cap !== undefined && buyCount > cap,
  };
}
