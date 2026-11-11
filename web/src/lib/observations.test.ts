import {describe,it,expect} from 'vitest';
import type {ForwardObservation} from '../api/client';
import {distanceToObservationTarget,observationTarget,observationTargetDescription} from './observations';
const item=(success='price_gte_target',target=.2):ForwardObservation=>({contract:{baseline:10,outcome:{horizon_days:2,target_return_pct:target,price_basis:'close',success}}} as ForwardObservation);
describe('frozen observation display',()=>{
 it('shows upward and downward distances from actual latest close',()=>{
  const up=item();up.evaluation={latest_close:11} as ForwardObservation['evaluation'];expect(observationTarget(up)).toBe(12);expect(distanceToObservationTarget(up)).toBe('距目标 9.09%');
  const down=item('price_lte_target',-.2);down.evaluation={latest_close:9} as ForwardObservation['evaluation'];expect(distanceToObservationTarget(down)).toBe('距目标 11.11%');
 });
 it('labels the frozen signal baseline when no future bar exists, and does not invent missing targets',()=>{
  expect(distanceToObservationTarget(item())).toContain('20.00%（截至信号日）');
  const missing=item();missing.contract.outcome.target_return_pct=undefined;expect(observationTarget(missing)).toBeUndefined();expect(distanceToObservationTarget(missing)).toBe('目标未定义');
 });
 it('shows achieved status instead of a negative remaining distance',()=>{
  const achieved=item();achieved.evaluation={status:'hit',latest_close:13} as ForwardObservation['evaluation'];expect(distanceToObservationTarget(achieved)).toBe('已达标');
 });
 it('shows precise frozen thresholds and does not invent unknown comparisons',()=>{
  const exact=item();exact.contract.baseline=5.27;expect(observationTargetDescription(exact)).toContain('6.3240');
  exact.contract.outcome.success='unknown';expect(observationTargetDescription(exact)).toContain('未定义');
 });
 it('does not calculate distance with unverified data and labels non-close reference distance',()=>{
  const missing=item();missing.evaluation={status:'insufficient_data',latest_close:11} as ForwardObservation['evaluation'];expect(distanceToObservationTarget(missing)).toBe('数据不足，暂不计算');
  const high=item();high.contract.outcome.price_basis='high';high.evaluation={status:'ongoing',latest_close:11} as ForwardObservation['evaluation'];expect(distanceToObservationTarget(high)).toContain('按最新收盘参考');
 });

});
