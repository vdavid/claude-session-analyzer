import { describe, expect, it } from 'vitest'
import { toolBreakdown, toolClockBars } from '../src/lib/transform/tools'
import type { ToolCategory, ToolGroupTotal } from '../src/lib/types'

const leaf = (leafName: string, calls: number): ToolGroupTotal['tools'][number] => ({
    tool: leafName,
    leaf: leafName,
    calls,
    seconds: calls,
    composingSeconds: calls,
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
        composingSeconds: 300,
        lanes: 8,
        tools: [leaf('grep', 60)],
    },
    {
        // The inversion the chart exists for: the model streams the whole diff, so the write is instant.
        group: 'Edit',
        class: 'file write',
        category: 'write',
        calls: 30,
        seconds: 10,
        composingSeconds: 600,
        lanes: 5,
        tools: [leaf('Edit', 30)],
    },
    {
        group: 'Read',
        class: 'file read',
        category: 'read',
        calls: 20,
        seconds: 5,
        composingSeconds: 40,
        lanes: 4,
        tools: [leaf('Read', 20)],
    },
    {
        group: 'codegraph (MCP)',
        class: 'mcp',
        // The engine's override: codegraph is how the agents read a codebase, so it isn't a service bucket.
        category: 'read',
        calls: 8,
        seconds: 4,
        composingSeconds: 7,
        lanes: 2,
        tools: [leaf('codegraph_search', 5), leaf('codegraph_node', 3)],
    },
    {
        group: 'Bash (test)',
        class: 'test',
        category: 'checks',
        calls: 2,
        seconds: 900,
        composingSeconds: 30,
        lanes: 1,
        errors: 1,
        tools: [leaf('cargo test', 2)],
    },
    {
        // One suspended `rm`, the shape of the reference session's `Bash (file write)`.
        group: 'Bash (file write)',
        class: 'file write',
        category: 'write',
        calls: 4,
        seconds: 12,
        composingSeconds: 8,
        stalledSeconds: 2000,
        lanes: 3,
        tools: [leaf('rm', 4)],
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
            'Bash (file write)',
            'Bash (test)',
        ])
    })

    it('shares are of calls, not of seconds: a two-call test run took most of the time', () => {
        const { slices, calls } = toolBreakdown(groups, categories)
        expect(calls).toBe(124)
        expect(slices.find((s) => s.group === 'Bash (search)')?.share).toBeCloseTo(60 / 124)
        expect(slices.find((s) => s.group === 'Bash (test)')?.share).toBeCloseTo(2 / 124)
    })

    it('carries the three clocks apart, because one number holding all three reports a tool as costing what a suspension cost', () => {
        const write = toolBreakdown(groups, categories).slices.find((s) => s.group === 'Bash (file write)')
        expect(write).toMatchObject({
            runningSeconds: 12,
            composingSeconds: 8,
            stalledSeconds: 2000,
        })
        // The sum is every second that arrived under the name, which is what a bar's length can honestly be.
        expect(write?.attributedSeconds).toBe(2020)
    })

    it('reads a missing stalled clock as zero, since the API leaves the field out when nothing stalled', () => {
        const edit = toolBreakdown(groups, categories).slices.find((s) => s.group === 'Edit')
        expect(edit?.stalledSeconds).toBe(0)
        expect(edit?.attributedSeconds).toBe(610)
    })

    it('totals each clock separately, and never as one number', () => {
        const { runningSeconds, composingSeconds, stalledSeconds } = toolBreakdown(groups, categories)
        expect(runningSeconds).toBe(1051)
        expect(composingSeconds).toBe(985)
        expect(stalledSeconds).toBe(2000)
    })

    it('rolls the groups of one category into a contiguous arc, and says where it starts', () => {
        const { categories: rolled } = toolBreakdown(groups, categories)
        expect(rolled.map((c) => c.category)).toEqual(['read', 'write', 'checks'])

        const read = rolled[0]
        expect(read.label).toBe('Read')
        expect(read.calls).toBe(88)
        expect(read.groups).toBe(3)
        expect(read.firstIndex).toBe(0)
        expect(read.share).toBeCloseTo(88 / 124)
        expect(read.runningSeconds).toBe(129)
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
                {
                    group: 'Telepathy',
                    class: 'telepathy',
                    category: 'esp',
                    calls: 3,
                    seconds: 1,
                    composingSeconds: 1,
                    lanes: 1,
                    tools: [],
                },
                {
                    group: 'Read',
                    class: 'file read',
                    category: 'read',
                    calls: 1,
                    seconds: 1,
                    composingSeconds: 1,
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
            runningSeconds: 0,
            composingSeconds: 0,
            stalledSeconds: 0,
            busiest: null,
        })
    })
})

describe('toolClockBars', () => {
    const { slices } = toolBreakdown(groups, categories)
    const chart = toolClockBars(slices)

    it('ranks the bars by every second under the tool name, so the split and the ranking read together', () => {
        expect(chart.rows.map((r) => r.group)).toEqual([
            'Bash (file write)', // 2,020, almost all of it one suspension
            'Bash (test)', // 930, almost all of it running
            'Edit', // 610, almost all of it composing
            'Bash (search)', // 420
            'Read', // 45
            'codegraph (MCP)', // 11
        ])
        expect(chart.max).toBe(2020)
    })

    it('stacks the segments in the order the time happened, and drops a clock that holds nothing', () => {
        const edit = chart.rows.find((r) => r.group === 'Edit')
        expect(edit?.segments.map((s) => s.clock)).toEqual(['composing', 'running'])

        const write = chart.rows.find((r) => r.group === 'Bash (file write)')
        // Stalled sits last: it stands in for running rather than following it, and it has to be the end a
        // reader's eye lands on.
        expect(write?.segments.map((s) => s.clock)).toEqual(['composing', 'running', 'stalled'])
    })

    it('measures every bar against the axis, so two rows are comparable and the marks line up', () => {
        // 2,020 s rounds up to three quarter-hours, so the widest bar stops a little short of the end.
        expect(chart.bound).toBe(2700)
        const [widest] = chart.rows
        expect(widest.segments.reduce((sum, s) => sum + s.widthShare, 0)).toBeCloseTo(2020 / 2700, 6)

        const edit = chart.rows.find((r) => r.group === 'Edit')
        expect(edit?.segments.find((s) => s.clock === 'composing')?.widthShare).toBeCloseTo(600 / 2700, 6)
        expect(edit?.segments.find((s) => s.clock === 'running')?.widthShare).toBeCloseTo(10 / 2700, 6)
    })

    it('labels the clock holding most of a row, which is where the inversion shows', () => {
        expect(chart.rows.find((r) => r.group === 'Edit')?.labelled).toEqual(['composing'])
        expect(chart.rows.find((r) => r.group === 'Bash (test)')?.labelled).toEqual(['running'])
        expect(chart.rows.find((r) => r.group === 'Bash (file write)')?.labelled).toEqual(['stalled'])
    })

    it('labels a stall even when it is not the biggest clock, because a stall is the anomaly', () => {
        const withSmallStall = toolBreakdown(
            [
                {
                    group: 'Bash (git)',
                    class: 'git',
                    category: 'management',
                    calls: 10,
                    seconds: 600,
                    composingSeconds: 20,
                    stalledSeconds: 30,
                    lanes: 2,
                    tools: [leaf('git commit', 10)],
                },
            ],
            categories,
        ).slices
        expect(toolClockBars(withSmallStall).rows[0].labelled).toEqual(['running', 'stalled'])
    })

    it('names the rows holding stalled time, so the chart can attribute it to a suspension', () => {
        expect(chart.stalled.map((r) => r.group)).toEqual(['Bash (file write)'])
    })

    it('totals each clock across the chart, kept apart the same way a row keeps them apart', () => {
        expect(chart.totals).toEqual({ composing: 985, running: 1051, stalled: 2000 })
    })

    it('points a row back at its slice in the pie, so hovering a bar can light the arc', () => {
        const edit = chart.rows.find((r) => r.group === 'Edit')
        expect(edit).toBeDefined()
        expect(slices[edit?.sliceIndex ?? -1].group).toBe('Edit')
    })

    it('puts round marks across the axis, ending exactly on the axis rather than past it', () => {
        // Marks a clock has, not a decimal one: quarter-hours here rather than 505-second steps.
        expect(chart.ticks).toEqual([0, 900, 1800, 2700])
        expect(chart.ticks.at(-1)).toBe(chart.bound)
        expect(chart.bound).toBeGreaterThanOrEqual(chart.max)
    })

    it('steps up to hours on a session whose widest bar is hours long', () => {
        // The reference session's `Bash (checker)`: 7h51m running plus 24m composing, rounded to 10h.
        const hours = toolBreakdown(
            [
                {
                    group: 'Bash (checker)',
                    class: 'checker',
                    category: 'checks',
                    calls: 344,
                    seconds: 28309,
                    composingSeconds: 1454,
                    lanes: 21,
                    tools: [leaf('pnpm check', 344)],
                },
            ],
            categories,
        ).slices
        const chartOfHours = toolClockBars(hours)
        expect(chartOfHours.ticks).toEqual([0, 7200, 14400, 21600, 28800, 36000])
        expect(chartOfHours.bound).toBe(36000)
    })

    it('breaks ties on the group name, so the order is the same on every render', () => {
        const tied = toolBreakdown(
            [
                {
                    group: 'Zed',
                    class: 'other',
                    category: 'other',
                    calls: 1,
                    seconds: 5,
                    composingSeconds: 5,
                    lanes: 1,
                    tools: [],
                },
                {
                    group: 'Ada',
                    class: 'other',
                    category: 'other',
                    calls: 1,
                    seconds: 5,
                    composingSeconds: 5,
                    lanes: 1,
                    tools: [],
                },
            ],
            categories,
        ).slices
        expect(toolClockBars(tied).rows.map((r) => r.group)).toEqual(['Ada', 'Zed'])
    })

    it('survives a session that never called a tool, and one whose calls took no measurable time', () => {
        expect(toolClockBars([])).toEqual({
            rows: [],
            max: 0,
            bound: 0,
            ticks: [0],
            totals: { composing: 0, running: 0, stalled: 0 },
            stalled: [],
        })

        const instant = toolBreakdown(
            [
                {
                    group: 'Skill',
                    class: 'other',
                    category: 'other',
                    calls: 1,
                    seconds: 0,
                    composingSeconds: 0,
                    lanes: 1,
                    tools: [],
                },
            ],
            categories,
        ).slices
        const drawn = toolClockBars(instant)
        expect(drawn.rows[0].segments).toEqual([])
        expect(drawn.rows[0].labelled).toEqual([])
        expect(drawn.max).toBe(0)
    })
})
