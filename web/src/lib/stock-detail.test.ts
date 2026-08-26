import { describe, expect, it } from 'vitest';

import { getSyncAgeDays, getSyncStatusPresentation } from './stock-detail';

describe('stock detail sync diagnostics', () => {
  it('does not treat a missing synchronization timestamp as the Unix epoch', () => {
    expect(getSyncAgeDays(undefined, Date.UTC(2026, 7, 3))).toBeNull();
    expect(getSyncAgeDays('', Date.UTC(2026, 7, 3))).toBeNull();
  });

  it('labels an absent synchronization record as unknown instead of failed or successful', () => {
    expect(getSyncStatusPresentation('unknown')).toEqual({ label: '未记录', color: 'orange' });
  });

  it('calculates age only for a real timestamp', () => {
    expect(getSyncAgeDays('2026-08-01T00:00:00Z', Date.UTC(2026, 7, 3))).toBe(2);
  });
});
