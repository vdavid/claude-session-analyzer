/**
 * The tool breakdown, shaped for the pie and its legend.
 *
 * Everything here counts **calls**, not time. The question this chart answers is "which tools did
 * this session reach for, and who reached for them", and time answers a different one: a `pnpm check`
 * costs minutes and a codegraph lookup costs a second, so a pie of seconds says nothing about what
 * the agents actually did. The seconds ride along in the legend, where they can be read for what
 * they are.
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
    seconds: number
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
    seconds: number
    share: number
    /** Where this category's slices start in the pie, so a legend can point at the right arc. */
    firstIndex: number
    groups: number
}

export interface ToolBreakdown {
    slices: ToolSlice[]
    categories: CategorySlice[]
    calls: number
    seconds: number
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
    const seconds = groups.reduce((sum, g) => sum + g.seconds, 0)

    const rank = new Map(categories.map((c, i) => [c.category, i]))
    const labels = new Map(categories.map((c) => [c.category, c.label]))

    const slices: ToolSlice[] = groups
        .map((g) => ({
            group: g.group,
            category: g.category,
            className: g.class,
            calls: g.calls,
            seconds: g.seconds,
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
            last.seconds += slice.seconds
            last.share += slice.share
            last.groups++
            return
        }
        rolled.push({
            category: slice.category,
            label: labels.get(slice.category) ?? slice.category,
            calls: slice.calls,
            seconds: slice.seconds,
            share: slice.share,
            firstIndex: index,
            groups: 1,
        })
    })

    const busiest = slices.reduce<ToolSlice | null>((best, s) => (!best || s.calls > best.calls ? s : best), null)
    return { slices, categories: rolled, calls, seconds, busiest }
}
