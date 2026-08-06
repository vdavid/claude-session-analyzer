import { describe, expect, it } from 'vitest'
import { formatBytes, formatCount, formatDuration, formatInstant, formatShare, instantMs } from '../src/lib/format'

describe('formatDuration', () => {
    it('keeps a sub-second row visible instead of rounding it to nothing', () => {
        expect(formatDuration(0.194)).toBe('194ms')
        expect(formatDuration(0.032)).toBe('32ms')
    })

    it('reads at the grain the number deserves', () => {
        expect(formatDuration(0)).toBe('0s')
        expect(formatDuration(1.997)).toBe('1.997s')
        expect(formatDuration(45.2)).toBe('45.2s')
        expect(formatDuration(561)).toBe('9m21s')
        expect(formatDuration(22536.871)).toBe('6h15m')
    })

    it('separates thousands once an hour count runs long', () => {
        expect(formatDuration(2287 * 3600 + 120)).toBe('2,287h02m')
    })

    it('says so rather than inventing a number', () => {
        expect(formatDuration(Number.NaN)).toBe('?')
        expect(formatDuration(-1)).toBe('?')
    })
})

describe('formatShare', () => {
    it('never rounds a real slice down to zero', () => {
        expect(formatShare(1, 100000)).toBe('<0.1%')
        expect(formatShare(0, 100)).toBe('0.0%')
        expect(formatShare(1, 2)).toBe('50%')
        expect(formatShare(1, 20)).toBe('5.0%')
    })
})

describe('formatCount and formatBytes', () => {
    it('separates thousands', () => {
        expect(formatCount(21964)).toBe('21,964')
    })

    it('sizes a transcript the way the CLI does, so the two surfaces agree', () => {
        expect(formatBytes(146)).toBe('146 B')
        expect(formatBytes(2768)).toBe('2.8 KB')
        expect(formatBytes(66984138)).toBe('67.0 MB')
        expect(formatBytes(3781937698)).toBe('3.8 GB')
    })
})

describe('parseInstant', () => {
    it('refuses a zero date, which is a valid string and a lie', () => {
        expect(instantMs('0001-01-01T00:00:00Z')).toBeNull()
        expect(instantMs('1970-01-01T00:00:00Z')).toBeNull()
    })

    it('takes a real one, trailing zeros trimmed or not', () => {
        expect(instantMs('2026-08-03T08:44:16.7Z')).toBe(Date.parse('2026-08-03T08:44:16.700Z'))
    })

    it('says nothing rather than guessing', () => {
        expect(instantMs(null)).toBeNull()
        expect(instantMs('not a date')).toBeNull()
        expect(formatInstant(null)).toBe('unknown')
    })
})
