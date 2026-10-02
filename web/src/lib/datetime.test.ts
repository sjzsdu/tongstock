import { describe, it, expect } from 'vitest'
import { formatDate, formatShortDate, formatTime, formatDateTime, formatTdxDate, beijingNowParts } from './datetime'

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

    it('matches local clock when environment is already UTC+8', () => {
      const d = new Date(2026, 9, 1, 14, 59, 0)
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
})
