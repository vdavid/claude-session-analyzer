import { describe, expect, it } from 'vitest'
import { bandTotals, kindSlices } from '../src/lib/transform/pie'
import type { KindTotal } from '../src/lib/types'

const totals: KindTotal[] = [
    { kind: 'tool execution', seconds: 300, rows: 30 },
    { kind: 'thinking', seconds: 100, rows: 10 },
    { kind: 'waiting for a person', seconds: 500, rows: 5 },
    { kind: 'stalled', seconds: 100, rows: 1 },
]

describe('kindSlices', () => {
    it('puts the slices in legend order rather than by size', () => {
        expect(kindSlices(totals).map((s) => s.kind)).toEqual([
            'thinking',
            'tool execution',
            'waiting for a person',
            'stalled',
        ])
    })

    it('carries each slice’s share of lane time, not of elapsed time', () => {
        const slices = kindSlices(totals)
        expect(slices.find((s) => s.kind === 'waiting for a person')?.share).toBeCloseTo(0.5)
    })

    it('drops a kind with no time behind it, so the legend holds no empty rows', () => {
        const slices = kindSlices([...totals, { kind: 'compacting', seconds: 0, rows: 0 }])
        expect(slices.map((s) => s.kind)).not.toContain('compacting')
    })

    it('survives a session with no rows at all', () => {
        expect(kindSlices([])).toEqual([])
    })

    it('keeps a kind the engine grew that this page has no entry for', () => {
        const slices = kindSlices([{ kind: 'daydreaming', seconds: 10, rows: 1 }])
        expect(slices.map((s) => s.kind)).toEqual(['daydreaming'])
    })
})

describe('bandTotals', () => {
    it('rolls the eleven kinds up into working, waiting, and lost', () => {
        const bands = bandTotals(totals)
        expect(bands.work.seconds).toBe(400)
        expect(bands.wait.seconds).toBe(500)
        expect(bands.trouble.seconds).toBe(100)
        expect(bands.total).toBe(1000)
    })

    it('counts compaction as overhead rather than as work or as waiting', () => {
        const bands = bandTotals([{ kind: 'compacting', seconds: 60, rows: 1 }])
        expect(bands.overhead.seconds).toBe(60)
        expect(bands.work.seconds).toBe(0)
    })
})
