import type { ForwardObservation } from '../api/client';

/** Targets are frozen outcome thresholds, never a win-rate or take-profit guess. */
export function observationTarget(item: ForwardObservation) {
 const target=item.contract.outcome.target_return_pct;
 return target==null?undefined:item.contract.baseline*(1+target);
}
export function distanceToObservationTarget(item: ForwardObservation) {
 if(item.evaluation?.status==='insufficient_data')return '数据不足，暂不计算';
 if(item.evaluation?.status==='hit')return '已达标';
 const target=observationTarget(item);
 if(target==null || !Number.isFinite(target) || target<=0)return '目标未定义';
 const current=item.evaluation?.latest_close ?? item.contract.baseline;
 if(!Number.isFinite(current) || current<=0)return '数据不足';
 const distance=item.contract.outcome.success==='price_lte_target'?1-target/current:target/current-1;
 const source=item.evaluation?.latest_close==null?'（截至信号日）':item.contract.outcome.price_basis!=='close'?'（按最新收盘参考）':'';
 return `距目标 ${Math.max(0,distance*100).toFixed(2)}%${source}`;
}

export function observationTargetDescription(item: ForwardObservation) {
  const goal = item.contract.outcome;
  const target = observationTarget(item);
  if (target == null) return '目标未定义';
  const operator = goal.success === 'price_lte_target' ? '≤'
    : goal.success === 'price_gte_target' || goal.success === 'close_gte_target' ? '≥' : undefined;
  const price = { close: '收盘价', open: '开盘价', high: '最高价', low: '最低价' }[goal.price_basis || ''];
  if (!operator || !price) return '目标比较方式或价格类型未定义';
  return `${goal.horizon_days || '?'}个后续完整交易日内，${price}${operator} ${target.toFixed(4)}元（${((goal.target_return_pct || 0) * 100).toFixed(1)}%）`;
}
