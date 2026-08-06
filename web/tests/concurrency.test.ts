import { describe, expect, it } from 'vitest'
import { concurrencyTrace } from '../src/lib/transform/concurrency'
import type { Lane } from '../src/lib/types'

function lane(id: string, from: string, until: string, gaps: Lane['gaps'] = []): Lane {
    return { id, name: id, isLead: false, from, until, seconds: 0, rows: 0, byKind: [], gaps }
}

describe('concurrencyTrace', () => {
    it('counts one lane as one across the stretch it produced something', () => {
        const trace = concurrencyTrace([lane('a', '2026-01-01T00:00:00Z', '2026-01-01T10:00:00Z')], 10)
        expect(trace.points).toHaveLength(10)
        expect(trace.points.every((p) => p.lanes === 1)).toBe(true)
    })

    it('adds lanes that were producing at the same time', () => {
        const trace = concurrencyTrace(
            [
                lane('a', '2026-01-01T00:00:00Z', '2026-01-01T10:00:00Z'),
                lane('b', '2026-01-01T05:00:00Z', '2026-01-01T10:00:00Z'),
            ],
            10,
        )
        expect(trace.points[0].lanes).toBe(1)
        expect(trace.points[9].lanes).toBe(2)
    })

    it('leaves a lane’s idle stretch out, so waiting doesn’t read as working', () => {
        const trace = concurrencyTrace(
            [
                lane('a', '2026-01-01T00:00:00Z', '2026-01-01T10:00:00Z', [
                    { from: '2026-01-01T00:00:00Z', until: '2026-01-01T05:00:00Z', seconds: 18000, kind: 'waiting for a person', info: '' },
                ]),
            ],
            10,
        )
        expect(trace.points[0].lanes).toBe(0)
        expect(trace.points[9].lanes).toBe(1)
    })

    it('reports a fraction when a lane was producing for part of a bucket', () => {
        const trace = concurrencyTrace([lane('a', '2026-01-01T00:00:00Z', '2026-01-01T00:15:00Z')], 2, {
            from: new Date('2026-01-01T00:00:00Z').getTime(),
            until: new Date('2026-01-01T01:00:00Z').getTime(),
        })
        expect(trace.points[0].lanes).toBeCloseTo(0.5)
        expect(trace.points[1].lanes).toBeCloseTo(0)
    })

    it('gives back nothing when no lane carries a readable span', () => {
        expect(concurrencyTrace([lane('a', 'nope', 'nope')], 10).points).toEqual([])
    })
})
