import { describe, expect, it } from 'vitest'
import { toolBreakdown } from '../src/lib/transform/tools'
import type { ToolGroupTotal } from '../src/lib/types'

const leaf = (leafName: string, calls: number): ToolGroupTotal['tools'][number] => ({
    tool: leafName,
    leaf: leafName,
    calls,
    seconds: calls,
    lanes: 1,
})

/** Deliberately out of order, the way a call-sorted API response arrives. */
const groups: ToolGroupTotal[] = [
    { group: 'Bash (search)', class: 'search', calls: 60, seconds: 120, lanes: 8, tools: [leaf('grep', 60)] },
    { group: 'Edit', class: 'file write', calls: 30, seconds: 10, lanes: 5, tools: [leaf('Edit', 30)] },
    { group: 'Read', class: 'file read', calls: 20, seconds: 5, lanes: 4, tools: [leaf('Read', 20)] },
    {
        group: 'codegraph (MCP)',
        class: 'mcp',
        calls: 8,
        seconds: 4,
        lanes: 2,
        tools: [leaf('codegraph_search', 5), leaf('codegraph_node', 3)],
    },
    {
        group: 'Bash (test)',
        class: 'test',
        calls: 2,
        seconds: 900,
        lanes: 1,
        errors: 1,
        tools: [leaf('cargo test', 2)],
    },
]

describe('toolBreakdown', () => {
    it('orders the slices by family rather than by size, so a colour means one thing on every session', () => {
        expect(toolBreakdown(groups).slices.map((s) => s.group)).toEqual([
            // Files first of the families present, and the bigger of its two groups opens the arc.
            'Edit',
            'Read',
            'Bash (test)',
            'Bash (search)',
            'codegraph (MCP)',
        ])
    })

    it('shares are of calls, not of seconds: a two-call test run took most of the time', () => {
        const { slices } = toolBreakdown(groups)
        expect(slices.find((s) => s.group === 'Bash (search)')?.share).toBeCloseTo(0.5)
        expect(slices.find((s) => s.group === 'Bash (test)')?.share).toBeCloseTo(2 / 120)
    })

    it('rolls the groups of one family into a contiguous arc, and says where it starts', () => {
        const { families } = toolBreakdown(groups)
        expect(families.map((f) => f.family)).toEqual(['files', 'gates', 'search', 'services'])

        const files = families[0]
        expect(files.calls).toBe(50)
        expect(files.groups).toBe(2)
        expect(files.firstIndex).toBe(0)
        expect(files.share).toBeCloseTo(50 / 120)
    })

    it('names the busiest group, which is the sentence a reader wants before the table', () => {
        expect(toolBreakdown(groups).busiest?.group).toBe('Bash (search)')
    })

    it('keeps a leaf list intact, so a legend can open a group without another request', () => {
        const codegraph = toolBreakdown(groups).slices.find((s) => s.group === 'codegraph (MCP)')
        expect(codegraph?.tools.map((t) => t.leaf)).toEqual(['codegraph_search', 'codegraph_node'])
        expect(codegraph?.lanes).toBe(2)
    })

    it('puts a class this page has no family for in the neutral bucket rather than dropping it', () => {
        const { slices } = toolBreakdown([
            { group: 'Telepathy', class: 'telepathy', calls: 3, seconds: 1, lanes: 1, tools: [leaf('Telepathy', 3)] },
        ])
        expect(slices[0].family).toBe('shell')
    })

    it('survives a session that never called a tool', () => {
        expect(toolBreakdown([])).toEqual({ slices: [], families: [], calls: 0, seconds: 0, busiest: null })
    })
})
