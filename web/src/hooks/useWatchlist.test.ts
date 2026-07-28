import { describe, expect, it } from 'vitest'

import { mergeWatchlist, resolveWatchlistAdditions } from './useWatchlist'

describe('useWatchlist state transitions', () => {
  it('merges remote entries without duplicating locally cached stocks', () => {
    expect(mergeWatchlist(
      [{ code: '600000', name: '浦发银行' }],
      [{ code: '600000', name: '重复' }, { code: '000001', name: '平安银行' }],
    )).toEqual([
      { code: '600000', name: '浦发银行' },
      { code: '000001', name: '平安银行' },
    ])
  })

  it('resolves additions by exchange and removes duplicate requests', () => {
    const cache = {
      sh: { list: [{ Code: '600000', Name: '浦发银行' }] },
      sz: { list: [{ Code: '000001', Name: '平安银行' }] },
      bj: { list: [{ Code: '800001', Name: '北交样本' }] },
    }
    expect(resolveWatchlistAdditions(['600000', '000001', '800001', '600000'], cache)).toEqual([
      { code: '000001', name: '平安银行' },
      { code: '600000', name: '浦发银行' },
      { code: '800001', name: '北交样本' },
    ])
  })
})
