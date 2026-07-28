import { describe, expect, it } from 'vitest'

import { TongStockAPIError } from '../api/client'
import { classifyDetailStatus } from './useStockDetail'

describe('useStockDetail error classification', () => {
  it.each(['not_found', 'cache_miss', 'multiple_matches'])('maps %s to not_found', (code) => {
    expect(classifyDetailStatus(new TongStockAPIError(code, 'localized text may change'))).toBe('not_found')
  })

  it('does not classify infrastructure failures as not_found', () => {
    expect(classifyDetailStatus(new TongStockAPIError('upstream_unavailable', 'TDX unavailable'))).toBe('no_data')
  })
})
