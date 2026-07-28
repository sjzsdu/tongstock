import { describe, it, expect } from 'vitest'
import { formatDate, formatShortDate, formatTime, formatDateTime, formatTdxDate } from './datetime'

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
