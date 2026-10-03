import { describe, it, expect } from 'vitest'
import { formatDate, formatShortDate, formatTime, formatDateTime, formatTdxDate, beijingNowParts, recentWeekdayDates, formatAPIDate } from './datetime'

describe('datetime', () => {
  describe('formatDate', () => {
    it('formats a Date object', () => {
      const d = new Date(2024, 0, 15)
      expect(formatDate(d)).toBe('2024-01-15')
    })

    it('formats YYYYMMDD string', () => {
      expect(formatDate('20240115')).toBe('2024-01-15')
    })

    it('returns fallback for invalid input', () => {
      expect(formatDate('')).toBe('-')
      expect(formatDate(null)).toBe('-')
      expect(formatDate(undefined)).toBe('-')
      expect(formatDate('not-a-date', 'NA')).toBe('NA')
    })
  })

  describe('formatShortDate', () => {
    it('formats with weekday', () => {
      const d = new Date(2024, 0, 15) // Mon
      expect(formatShortDate(d)).toBe('01-15 周一')
    })

    it('returns fallback for invalid input', () => {
      expect(formatShortDate('')).toBe('-')
    })
  })

  describe('beijingNowParts', () => {
    it('converts a UTC epoch to fixed UTC+8 regardless of environment timezone', () => {
      // UTC 2026-10-01 14:59 → 北京时间 22:59
      const utcEpoch = Date.UTC(2026, 9, 1, 14, 59, 0)
      expect(beijingNowParts(new Date(utcEpoch)).text).toBe('22:59')
    })

    it('matches the clock when the instant is constructed UTC+8-explicit', () => {
      // 北京时间 2026-10-01 14:59 的瞬时时刻：UTC 明确构造（UTC 06:59），
      // 不依赖运行环境的本地时区（GitHub runner 是 UTC）。
      const d = new Date(Date.UTC(2026, 9, 1, 14, 59, 0) - 8 * 60 * 60 * 1000)
      expect(beijingNowParts(d).text).toBe('14:59')
    })

    it('handles day rollover and hour boundary', () => {
      // UTC 2026-10-01 16:30 → 北京时间次日 00:30
      expect(beijingNowParts(new Date(Date.UTC(2026, 9, 1, 16, 30))).text).toBe('00:30')
      // UTC 2026-10-01 06:29 → 北京时间 14:29（恰好不到 14:30）
      const parts = beijingNowParts(new Date(Date.UTC(2026, 9, 1, 6, 29)))
      expect(parts.text).toBe('14:29')
      expect(parts.hour).toBe(14)
      expect(parts.minute).toBe(29)
    })
  })

  describe('formatTime', () => {
    it('formats HH:MM from string', () => {
      expect(formatTime('14:05')).toBe('14:05')
    })

    it('returns fallback for invalid', () => {
      expect(formatTime(null)).toBe('-')
    })
  })

  describe('formatDateTime', () => {
    it('combines date and time', () => {
      expect(formatDateTime('20240115 14:05')).toBe('2024-01-15 14:05')
    })

    it('returns fallback', () => {
      expect(formatDateTime('')).toBe('-')
    })
  })

  describe('formatTdxDate', () => {
    it('formats tdx date string', () => {
      expect(formatTdxDate('2024-01-15 14:05:00')).toBe('2024-01-15')
    })

    it('returns fallback', () => {
      expect(formatTdxDate(null)).toBe('-')
    })
  })

  describe('recentWeekdayDates', () => {
    // 国庆假期场景（本次 bug）：2026-10-03 是周六且处长假，
    // 候选必须从周五继续往前扫到真实交易日 9/30
    it('walks back from holiday Saturday over holidays to previous trading days', () => {
      const dates = recentWeekdayDates(3, new Date(2026, 9, 3))
      expect(dates.map(d => formatDate(d))).toEqual(['2026-10-02', '2026-10-01', '2026-09-30'])
    })

    it('skips weekends when walking back from Monday', () => {
      const dates = recentWeekdayDates(2, new Date(2026, 9, 5))
      expect(dates.map(d => formatDate(d))).toEqual(['2026-10-02', '2026-10-01'])
    })

    it('crosses year boundary with local date components', () => {
      const dates = recentWeekdayDates(3, new Date(2026, 0, 1))
      expect(dates.map(d => formatDate(d))).toEqual(['2025-12-31', '2025-12-30', '2025-12-29'])
    })
  })

  describe('formatAPIDate', () => {
    it('formats local date as YYYYMMDD', () => {
      expect(formatAPIDate(new Date(2026, 8, 30))).toBe('20260930')
    })
  })
})
