import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());
function response(body: unknown) { return Promise.resolve(new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })); }

describe('AI-native core journeys consume backend truth', () => {
  it('starts stock or named-method research through the real agent endpoint', async () => {
    const fetcher = vi.fn(() => response({ response: '证据不足，暂不验证该规律' }));vi.stubGlobal('fetch', fetcher);
    const result = await api.agentChat('研究 000063 的上涨规律');
    expect(result.response).toContain('证据不足');expect(fetcher).toHaveBeenCalledWith('/api/agent/chat', expect.objectContaining({ method: 'POST' }));
  });
  it('loads today selection with immutable snapshot provenance', async () => {
    vi.stubGlobal('fetch', vi.fn(() => response({ id:'selection-real',snapshot_id:'snapshot-real',feature_snapshot_id:'features-real',snapshot_date:'2026-04-24',candidate_count:0,buy_count:0,candidates:[],exclusions:[{reason_code:'method_not_eligible',detail:'rejected'}] })));
    const run=await api.selectionToday();expect(run.snapshot_id).toBe('snapshot-real');expect(run.buy_count).toBe(0);expect(run.exclusions[0].reason_code).toBe('method_not_eligible');
  });
  it('loads sell decisions and preserves inferred and execution constraints', async () => {
    vi.stubGlobal('fetch', vi.fn(() => response({ id:'position-real',snapshot_id:'snapshot-real',snapshot_date:'2026-04-24',decisions:[{code:'000001',name:'平安银行',action:'exit',priority:'critical',deadline:'复牌后',inferred:true,executable:false,constraint:'停牌',return_pct:-.1,price_time:'2026-04-24',explanation:'止损'}] })));
    const run=await api.positionDecisionToday();expect(run.decisions[0]).toMatchObject({ action:'exit',inferred:true,executable:false,constraint:'停牌' });
  });
});
