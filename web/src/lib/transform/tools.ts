/**
 * The tool breakdown, shaped for the pie, the clock bars, and the table under both.
 *
 * The **pie** counts calls, not time. The question it answers is "which tools did this session reach
 * for, and who reached for them", and time answers a different one: a `pnpm check` costs minutes and
 * a codegraph lookup costs a second, so a pie of seconds says nothing about what the agents actually
 * did.
 *
 * Time is the **clock bars'** question, and there it's three numbers rather than one, because all
 * three arrive under one tool's name: the agent composing the call, the tool running, and a call that
 * came back far too late to have been running. They're carried apart the whole way through, and
 * nothing here ever sums them into a "cost": `attributedSeconds` is named for what it is, every
 * second the grouping rule filed under the name.
 *
 * Slices keep **category order**, not size order, for the same reason the kind pie does: a colour has
 * to mean the same thing on every session, and the API's order is the adjacency the palette was
 * validated against (`categories.ts`). It also makes the chart readable as categories, so a
 * contiguous blue arc is "this session was mostly reading" rather than five unrelated slices that
 * happen to be adjacent.
 *
 * The categories come in as an argument rather than being known here, because the taxonomy is the
 * engine's: `docs/api.md` § The tool breakdown.
 */

import type { ToolCategory, ToolGroupTotal, ToolTotal } from '../types'

export interface ToolSlice {
    /** The group name the API sends, which is also what filtering a row matches on. */
    group: string
    /** The category the group falls in, straight off the API. */
    category: string
    className: string
    calls: number
    /** The tool running, call to result. The API's `seconds`, renamed so it can't read as a cost. */
    runningSeconds: number
    /** The agent writing the calls. Most of what an `Edit` costs, a rounding error on a checker. */
    composingSeconds: number
    /** A call back far too late to have been running. Zero when the API left the field out. */
    stalledSeconds: number
    /**
     * Every second the grouping rule filed under this name, which is the three clocks added. It's what
     * a bar's length can honestly be and what a ranking can honestly sort on, and it is **not** a cost:
     * one suspended `rm` puts six hours in here that neither the agent nor the tool spent working.
     */
    attributedSeconds: number
    lanes: number
    errors: number
    /** Of the session's tool calls, in the range 0 to 1. */
    share: number
    /** The exact tools inside the group, most calls first. */
    tools: ToolTotal[]
}

export interface CategorySlice {
    category: string
    /** What the legend calls it, as the API named it. */
    label: string
    calls: number
    /** The tool running, for every group in the category. The other two clocks stay per group. */
    runningSeconds: number
    share: number
    /** Where this category's slices start in the pie, so a legend can point at the right arc. */
    firstIndex: number
    groups: number
}

export interface ToolBreakdown {
    slices: ToolSlice[]
    categories: CategorySlice[]
    calls: number
    /** The three clocks over the whole session, kept apart the same way a group keeps them apart. */
    runningSeconds: number
    composingSeconds: number
    stalledSeconds: number
    /** The busiest group, which is the one sentence a reader wants before the table. */
    busiest: ToolSlice | null
}

/**
 * Orders the API's groups for the pie: by category in the order the API served them, and by calls
 * inside each category, so the biggest slice of a category opens its arc.
 *
 * A group whose category isn't in the served list sorts last rather than being dropped, which is what
 * happens if the engine grows a category this page hasn't been told about yet.
 */
export function toolBreakdown(
    groups: readonly ToolGroupTotal[],
    categories: readonly ToolCategory[] = [],
): ToolBreakdown {
    const calls = groups.reduce((sum, g) => sum + g.calls, 0)

    const rank = new Map(categories.map((c, i) => [c.category, i]))
    const labels = new Map(categories.map((c) => [c.category, c.label]))

    const slices: ToolSlice[] = groups
        .map((g) => ({
            group: g.group,
            category: g.category,
            className: g.class,
            calls: g.calls,
            runningSeconds: g.seconds,
            composingSeconds: g.composingSeconds,
            stalledSeconds: g.stalledSeconds ?? 0,
            attributedSeconds: g.seconds + g.composingSeconds + (g.stalledSeconds ?? 0),
            lanes: g.lanes,
            errors: g.errors ?? 0,
            share: calls ? g.calls / calls : 0,
            tools: g.tools,
        }))
        .sort(
            (a, b) =>
                (rank.get(a.category) ?? Number.MAX_SAFE_INTEGER) - (rank.get(b.category) ?? Number.MAX_SAFE_INTEGER) ||
                b.calls - a.calls ||
                a.group.localeCompare(b.group),
        )

    const rolled: CategorySlice[] = []
    slices.forEach((slice, index) => {
        const last = rolled[rolled.length - 1]
        if (last?.category === slice.category) {
            last.calls += slice.calls
            last.runningSeconds += slice.runningSeconds
            last.share += slice.share
            last.groups++
            return
        }
        rolled.push({
            category: slice.category,
            label: labels.get(slice.category) ?? slice.category,
            calls: slice.calls,
            runningSeconds: slice.runningSeconds,
            share: slice.share,
            firstIndex: index,
            groups: 1,
        })
    })

    const busiest = slices.reduce<ToolSlice | null>((best, s) => (!best || s.calls > best.calls ? s : best), null)
    return {
        slices,
        categories: rolled,
        calls,
        runningSeconds: sum(slices, (s) => s.runningSeconds),
        composingSeconds: sum(slices, (s) => s.composingSeconds),
        stalledSeconds: sum(slices, (s) => s.stalledSeconds),
        busiest,
    }
}

/** One of the three clocks a call's time goes to. Also the order a bar stacks them in. */
export type ClockName = 'composing' | 'running' | 'stalled'

/**
 * Left to right, the order the time happened: the agent writes the call, then the tool runs, and a
 * stall stands in for running rather than following it. Position is half the encoding, so it's fixed
 * here rather than decided per row, and a stall is always the end a reader's eye lands on.
 */
export const CLOCK_ORDER: readonly ClockName[] = ['composing', 'running', 'stalled']

export interface ClockSegment {
    clock: ClockName
    seconds: number
    /** Of the axis, so a row is comparable with every other row and with the marks under them. */
    widthShare: number
}

export interface ToolClockRow {
    group: string
    category: string
    className: string
    calls: number
    lanes: number
    segments: ClockSegment[]
    /** The three clocks added. Read `ToolSlice.attributedSeconds` for what it is and isn't. */
    attributedSeconds: number
    composingSeconds: number
    runningSeconds: number
    stalledSeconds: number
    /**
     * The clocks that get a direct label: the one holding most of the row, plus a stall whenever there
     * is one. Labelling the biggest is what makes the inversion legible at a glance (`Edit` says
     * composing, `Bash (checker)` says running); labelling a stall regardless is because it's the
     * anomaly, and an anomaly nobody labelled is an anomaly nobody sees.
     */
    labelled: ClockName[]
    /** Where the group sits in the pie, so hovering a bar can light the same arc. */
    sliceIndex: number
}

export interface ToolClockChart {
    rows: ToolClockRow[]
    /** The widest bar. */
    max: number
    /**
     * Where the axis ends: `max` rounded up to the next round mark. Widths are shares of this rather
     * than of `max`, so the marks land on the track instead of past its right edge, and the widest bar
     * stops a little short the way a bar chart's does.
     */
    bound: number
    /** Round marks across the axis, in seconds, from zero to `bound`. */
    ticks: number[]
    totals: Record<ClockName, number>
    /** The rows holding stalled time, so the chart can say a suspension rather than a slow tool. */
    stalled: ToolClockRow[]
}

/**
 * One horizontal bar per tool, ranked by every second filed under its name.
 *
 * Ranking on that sum rather than on running time is what keeps the finding on screen: sorted by the
 * tool's own clock, `Edit` sinks to the bottom and the reader never learns that 1,032 calls cost two
 * hours of an agent writing diffs. The sum is a length and a rank here, never printed as a number.
 */
export function toolClockBars(slices: readonly ToolSlice[]): ToolClockChart {
    const ranked = slices
        .map((slice, sliceIndex) => ({ slice, sliceIndex }))
        .sort(
            (a, b) =>
                b.slice.attributedSeconds - a.slice.attributedSeconds || a.slice.group.localeCompare(b.slice.group),
        )

    const max = ranked.reduce((most, { slice }) => Math.max(most, slice.attributedSeconds), 0)
    const { bound, ticks } = niceAxis(max)

    const rows: ToolClockRow[] = ranked.map(({ slice, sliceIndex }) => {
        const held: Record<ClockName, number> = {
            composing: slice.composingSeconds,
            running: slice.runningSeconds,
            stalled: slice.stalledSeconds,
        }
        const segments = CLOCK_ORDER.filter((clock) => held[clock] > 0).map((clock) => ({
            clock,
            seconds: held[clock],
            widthShare: bound ? held[clock] / bound : 0,
        }))

        const biggest = segments.reduce<ClockSegment | null>(
            (best, segment) => (!best || segment.seconds > best.seconds ? segment : best),
            null,
        )
        const labelled: ClockName[] = biggest ? [biggest.clock] : []
        if (held.stalled > 0 && biggest?.clock !== 'stalled') labelled.push('stalled')

        return {
            group: slice.group,
            category: slice.category,
            className: slice.className,
            calls: slice.calls,
            lanes: slice.lanes,
            segments,
            attributedSeconds: slice.attributedSeconds,
            composingSeconds: slice.composingSeconds,
            runningSeconds: slice.runningSeconds,
            stalledSeconds: slice.stalledSeconds,
            labelled,
            sliceIndex,
        }
    })

    return {
        rows,
        max,
        bound,
        ticks,
        totals: {
            composing: sum(rows, (r) => r.composingSeconds),
            running: sum(rows, (r) => r.runningSeconds),
            stalled: sum(rows, (r) => r.stalledSeconds),
        },
        stalled: rows.filter((r) => r.stalledSeconds > 0),
    }
}

/**
 * Steps a reader already knows how to read: seconds, minutes, then hours, never 2,020. Durations
 * don't divide by ten, so the candidates are the ones a clock actually has.
 */
const TICK_STEPS = [1, 5, 15, 30, 60, 300, 900, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 604800]

/** The axis: the smallest round step that covers `max` in at most `most` intervals, and its marks. */
function niceAxis(max: number, most = 5): { bound: number; ticks: number[] } {
    if (max <= 0) return { bound: 0, ticks: [0] }
    const step = TICK_STEPS.find((candidate) => candidate * most >= max) ?? TICK_STEPS[TICK_STEPS.length - 1]
    const intervals = Math.max(Math.ceil(max / step), 1)
    return { bound: step * intervals, ticks: Array.from({ length: intervals + 1 }, (_, i) => i * step) }
}

function sum<T>(items: readonly T[], of: (item: T) => number): number {
    return items.reduce((total, item) => total + of(item), 0)
}
