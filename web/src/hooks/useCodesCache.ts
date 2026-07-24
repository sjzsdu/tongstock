import { useState, useCallback } from 'react';
import { api } from '../api/client';
import type { CodesCacheEntry } from '../types/screen';
import { CACHE_EXPIRY } from '../types/screen';

interface UseCodesCacheReturn {
  codesCache: Record<string, CodesCacheEntry>;
  preloadCodesCache: () => Promise<Record<string, CodesCacheEntry>>;
}

export function useCodesCache(): UseCodesCacheReturn {
  const [codesCache, setCodesCache] = useState<Record<string, CodesCacheEntry>>({});

  const preloadCodesCache = useCallback(async (): Promise<Record<string, CodesCacheEntry>> => {
    const exchanges = ['sz', 'sh', 'bj'] as const;
    const merged: Record<string, CodesCacheEntry> = { ...codesCache };
    await Promise.all(
      exchanges.map(async (exchange) => {
        if (!merged[exchange] || Date.now() - merged[exchange].timestamp >= CACHE_EXPIRY) {
          try {
            const codesList = await api.codes(exchange);
            merged[exchange] = { list: codesList, timestamp: Date.now() };
          } catch {
            return;
          }
        }
      }),
    );
    setCodesCache(merged);
    return merged;
  }, [codesCache]);

  return { codesCache, preloadCodesCache };
}