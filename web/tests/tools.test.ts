import { describe, expect, it } from 'vitest'
import { toolBreakdown } from '../src/lib/transform/tools'
import type { ToolCategory, ToolGroupTotal } from '../src/lib/types'

const leaf = (leafName: string, calls: number): ToolGroupTotal['tools'][number] => ({
    tool: leafName,
    leaf: leafName,
    calls,
    seconds: calls,
    lanes: 1,
})

/** What the API serves on every session: the categories in legend order, which is the pie's order. */
const categories: ToolCategory[] = [
    { category: 'management', label: 'Management', description: 'Teammates, version control, questions.' },
    { category: 'read', label: 'Read', description: 'Reading and searching.' },
    { category: 'write', label: 'Write', description: 'Editing files.' },
    { category: 'build', label: 'Build', description: 'Compilers and dev servers.' },
    { category: 'checks', label: 'Checks', description: 'Tests, linters, the gate.' },
    { category: 'qa', label: 'QA', description: 'Driving the product.' },
    { category: 'other', label: 'Other', description: 'Everything else.' },
]

/** Deliberately out of order, the way a call-sorted API response arrives. */
const groups: ToolGroupTotal[] = [
    {
        group: 'Bash (search)',
        class: 'search',
        category: 'read',
        calls: 60,
        seconds: 120,
        lanes: 8,
        tools: [leaf('grep', 60)],
    },
    {
        group: 'Edit',
        class: 'file write',
        category: 'write',
        calls: 30,
        seconds: 10,
        lanes: 5,
        tools: [leaf('Edit', 30)],
    },
    { group: 'Read', class: 'file read', category: 'read', calls: 20, seconds: 5, lanes: 4, tools: [leaf('Read', 20)] },
    {
        group: 'codegraph (MCP)',
        class: 'mcp',
        // The engine's override: codegraph is how the agents read a codebase, so it isn't a service bucket.
        category: 'read',
        calls: 8,
        seconds: 4,
        lanes: 2,
        tools: [leaf('codegraph_search', 5), leaf('codegraph_node', 3)],
    },
    {
        group: 'Bash (test)',
        class: 'test',
        category: 'checks',
        calls: 2,
        seconds: 900,
        lanes: 1,
        errors: 1,
        tools: [leaf('cargo test', 2)],
    },
]

describe('toolBreakdown', () => {
    it('orders the slices by category rather than by size, so a colour means one thing on every session', () => {
        expect(toolBreakdown(groups, categories).slices.map((s) => s.group)).toEqual([
            // Read comes first of the categories present, and the biggest of its three groups opens the arc.
            'Bash (search)',
            'Read',
            'codegraph (MCP)',
            'Edit',
            'Bash (test)',
        ])
    })

    it('shares are of calls, not of seconds: a two-call test run took most of the time', () => {
        const { slices } = toolBreakdown(groups, categories)
        expect(slices.find((s) => s.group === 'Bash (search)')?.share).toBeCloseTo(0.5)
        expect(slices.find((s) => s.group === 'Bash (test)')?.share).toBeCloseTo(2 / 120)
    })

    it('rolls the groups of one category into a contiguous arc, and says where it starts', () => {
        const { categories: rolled } = toolBreakdown(groups, categories)
        expect(rolled.map((c) => c.category)).toEqual(['read', 'write', 'checks'])

        const read = rolled[0]
        expect(read.label).toBe('Read')
        expect(read.calls).toBe(88)
        expect(read.groups).toBe(3)
        expect(read.firstIndex).toBe(0)
        expect(read.share).toBeCloseTo(88 / 120)
    })

    it('names the busiest group, which is the sentence a reader wants before the table', () => {
        expect(toolBreakdown(groups, categories).busiest?.group).toBe('Bash (search)')
    })

    it('keeps a leaf list intact, so a legend can open a group without another request', () => {
        const codegraph = toolBreakdown(groups, categories).slices.find((s) => s.group === 'codegraph (MCP)')
        expect(codegraph?.tools.map((t) => t.leaf)).toEqual(['codegraph_search', 'codegraph_node'])
        expect(codegraph?.lanes).toBe(2)
    })

    it('sorts a category the API never listed to the end rather than dropping its groups', () => {
        const { slices, categories: rolled } = toolBreakdown(
            [
                { group: 'Telepathy', class: 'telepathy', category: 'esp', calls: 3, seconds: 1, lanes: 1, tools: [] },
                {
                    group: 'Read',
                    class: 'file read',
                    category: 'read',
                    calls: 1,
                    seconds: 1,
                    lanes: 1,
                    tools: [leaf('Read', 1)],
                },
            ],
            categories,
        )
        expect(slices.map((s) => s.group)).toEqual(['Read', 'Telepathy'])
        // With no label served for it, the raw name is what the legend gets: better than an empty row.
        expect(rolled[1]).toMatchObject({ category: 'esp', label: 'esp' })
    })

    it('survives a session that never called a tool', () => {
        expect(toolBreakdown([], categories)).toEqual({
            slices: [],
            categories: [],
            calls: 0,
            seconds: 0,
            busiest: null,
        })
    })
})
