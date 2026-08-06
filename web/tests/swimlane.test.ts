import { describe, expect, it } from 'vitest'
import { buildSwimlane, busySegments } from '../src/lib/transform/swimlane'
import type { Gap, Lane } from '../src/lib/types'

const t = (iso: string) => new Date(iso).getTime()

function lane(partial: Partial<Lane> & Pick<Lane, 'id' | 'from' | 'until'>): Lane {
    return {
        name: partial.id,
        isLead: false,
        seconds: 0,
        rows: 0,
        byKind: [],
        gaps: [],
        ...partial,
    }
}

function gap(from: string, until: string, kind = 'waiting for a person'): Gap {
    return { from, until, seconds: (t(until) - t(from)) / 1000, kind, info: '' }
}

describe('busySegments', () => {
    it('gives one segment for a lane that never went quiet', () => {
        const l = lane({ id: 'a', from: '2026-01-01T00:00:00Z', until: '2026-01-01T01:00:00Z' })
        expect(busySegments(l)).toEqual([[t('2026-01-01T00:00:00Z'), t('2026-01-01T01:00:00Z')]])
    })

    it('cuts a hole for every gap rather than drawing the bar solid', () => {
        const l = lane({
            id: 'a',
            from: '2026-01-01T00:00:00Z',
            until: '2026-01-01T04:00:00Z',
            gaps: [gap('2026-01-01T01:00:00Z', '2026-01-01T02:00:00Z')],
        })
        expect(busySegments(l)).toEqual([
            [t('2026-01-01T00:00:00Z'), t('2026-01-01T01:00:00Z')],
            [t('2026-01-01T02:00:00Z'), t('2026-01-01T04:00:00Z')],
        ])
    })

    it('leaves nothing behind when the lane was idle end to end', () => {
        const l = lane({
            id: 'a',
            from: '2026-01-01T00:00:00Z',
            until: '2026-01-01T01:00:00Z',
            gaps: [gap('2026-01-01T00:00:00Z', '2026-01-01T01:00:00Z')],
        })
        expect(busySegments(l)).toEqual([])
    })

    it('merges gaps that overlap or touch, and clamps them to the lane', () => {
        const l = lane({
            id: 'a',
            from: '2026-01-01T01:00:00Z',
            until: '2026-01-01T05:00:00Z',
            gaps: [
                gap('2026-01-01T00:00:00Z', '2026-01-01T02:00:00Z'),
                gap('2026-01-01T02:00:00Z', '2026-01-01T03:00:00Z'),
                gap('2026-01-01T04:00:00Z', '2026-01-01T06:00:00Z'),
            ],
        })
        expect(busySegments(l)).toEqual([[t('2026-01-01T03:00:00Z'), t('2026-01-01T04:00:00Z')]])
    })
})

describe('buildSwimlane', () => {
    const lead = lane({
        id: 'lead',
        name: 'lead',
        isLead: true,
        from: '2026-01-01T00:00:00Z',
        until: '2026-01-01T10:00:00Z',
    })
    const direct = lane({ id: 'a1', name: 'reviewer', from: '2026-01-01T02:00:00Z', until: '2026-01-01T03:00:00Z' })
    const wf = (id: string, n: number) =>
        Array.from({ length: n }, (_, i) =>
            lane({
                id: `${id}-${i}`,
                name: 'workflow-subagent',
                workflowId: id,
                from: `2026-01-01T0${4 + (i % 2)}:00:00Z`,
                until: `2026-01-01T0${5 + (i % 2)}:00:00Z`,
            }),
        )

    it('puts the lead first, then workflows, then the lanes the session spawned itself', () => {
        const rows = buildSwimlane([direct, ...wf('wf_a', 3), lead], { expanded: new Set() })
        expect(rows.map((r) => r.kind)).toEqual(['lead', 'workflow', 'lane'])
        expect(rows[1].label).toContain('wf_a')
        expect(rows[1].laneCount).toBe(3)
    })

    it('collapses a workflow into one row whose bar is the union of its lanes', () => {
        const rows = buildSwimlane(wf('wf_a', 4), { expanded: new Set() })
        expect(rows).toHaveLength(1)
        expect(rows[0].busy).toEqual([[t('2026-01-01T04:00:00Z'), t('2026-01-01T06:00:00Z')]])
    })

    it('reveals a workflow’s lanes when it is expanded, and marks them as children', () => {
        const rows = buildSwimlane(wf('wf_a', 3), { expanded: new Set(['wf_a']) })
        expect(rows).toHaveLength(4)
        expect(rows[0].expanded).toBe(true)
        expect(rows.slice(1).every((r) => r.depth === 1 && r.workflowId === 'wf_a')).toBe(true)
    })

    it('caps how many lanes an expanded workflow draws, and says how many it held back', () => {
        const rows = buildSwimlane(wf('wf_a', 50), { expanded: new Set(['wf_a']), maxLanesPerGroup: 10 })
        expect(rows).toHaveLength(11)
        expect(rows[0].hiddenLanes).toBe(40)
    })

    it('groups by lane id, so 977 lanes sharing one name stay 977 lanes', () => {
        const same = [
            lane({ id: 'x1', name: 'general-purpose', from: '2026-01-01T01:00:00Z', until: '2026-01-01T02:00:00Z' }),
            lane({ id: 'x2', name: 'general-purpose', from: '2026-01-01T01:00:00Z', until: '2026-01-01T02:00:00Z' }),
        ]
        expect(buildSwimlane(same, { expanded: new Set() })).toHaveLength(2)
    })

    it('keeps a gap’s kind on the row, so the four waits can be drawn apart', () => {
        const l = lane({
            id: 'a',
            from: '2026-01-01T00:00:00Z',
            until: '2026-01-01T03:00:00Z',
            gaps: [gap('2026-01-01T01:00:00Z', '2026-01-01T02:00:00Z', 'waiting for a teammate')],
        })
        const [row] = buildSwimlane([l], { expanded: new Set() })
        expect(row.gaps).toEqual([
            { from: t('2026-01-01T01:00:00Z'), until: t('2026-01-01T02:00:00Z'), kind: 'waiting for a teammate' },
        ])
    })

    it('labels a workflow’s lanes by id when they all share one name', () => {
        const rows = buildSwimlane(wf('wf_a', 3), { expanded: new Set(['wf_a']) })
        // Sorted by start, and the two that began at 04:00 come before the one that began at 05:00.
        expect(rows.slice(1).map((r) => r.label)).toEqual(['wf_a-0', 'wf_a-2', 'wf_a-1'])
        expect(rows[1].sublabel).toBe('workflow-subagent')
    })

    it('keeps the names when a workflow’s lanes can be told apart by them', () => {
        const named = ['scout', 'builder'].map((name, i) =>
            lane({
                id: `wf_b-${i}`,
                name,
                workflowId: 'wf_b',
                from: '2026-01-01T04:00:00Z',
                until: '2026-01-01T05:00:00Z',
            }),
        )
        const rows = buildSwimlane(named, { expanded: new Set(['wf_b']) })
        expect(rows.slice(1).map((r) => r.label)).toEqual(['scout', 'builder'])
    })

    it('skips a lane whose span is unreadable rather than drawing it at the epoch', () => {
        const broken = lane({ id: 'b', from: 'not a date', until: 'not a date' })
        expect(buildSwimlane([broken, direct], { expanded: new Set() }).map((r) => r.key)).toEqual(['lane:a1'])
    })
})
