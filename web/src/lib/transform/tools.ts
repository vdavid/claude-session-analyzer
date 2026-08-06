/**
 * The tool breakdown, shaped for the pie and its legend.
 *
 * Everything here counts **calls**, not time. The question this chart answers is "which tools did
 * this session reach for, and who reached for them", and time answers a different one: a `pnpm check`
 * costs minutes and a codegraph lookup costs a second, so a pie of seconds says nothing about what
 * the agents actually did. The seconds ride along in the legend, where they can be read for what
 * they are.
 *
 * Slices keep **family order**, not size order, for the same reason the kind pie does: a colour has
 * to mean the same thing on every session, and the fixed order is what the palette was validated
 * against (`classes.ts`). It also makes the chart readable as families, so a contiguous blue arc is
 * "this session was mostly file work" rather than five unrelated slices that happen to be adjacent.
 */

import { classFamily, FAMILY_ORDER, type ToolFamily } from '../classes'
import type { ToolGroupTotal, ToolTotal } from '../types'

export interface ToolSlice {
    /** The group name the API sends, which is also what filtering a row matches on. */
    group: string
    family: ToolFamily
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

export interface FamilySlice {
    family: ToolFamily
    calls: number
    seconds: number
    share: number
    /** Where this family's slices start in the pie, so a legend can point at the right arc. */
    firstIndex: number
    groups: number
}

export interface ToolBreakdown {
    slices: ToolSlice[]
    families: FamilySlice[]
    calls: number
    seconds: number
    /** The busiest group, which is the one sentence a reader wants before the table. */
    busiest: ToolSlice | null
}

/**
 * Orders the API's groups for the pie: by family in the fixed order, and by calls inside each
 * family, so the biggest slice of a family opens its arc.
 */
export function toolBreakdown(groups: readonly ToolGroupTotal[]): ToolBreakdown {
    const calls = groups.reduce((sum, g) => sum + g.calls, 0)
    const seconds = groups.reduce((sum, g) => sum + g.seconds, 0)

    const rank = new Map(FAMILY_ORDER.map((f, i) => [f, i]))
    const slices: ToolSlice[] = groups
        .map((g) => ({
            group: g.group,
            family: classFamily(g.class),
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
                (rank.get(a.family) ?? 99) - (rank.get(b.family) ?? 99) ||
                b.calls - a.calls ||
                a.group.localeCompare(b.group),
        )

    const families: FamilySlice[] = []
    slices.forEach((slice, index) => {
        const last = families[families.length - 1]
        if (last?.family === slice.family) {
            last.calls += slice.calls
            last.seconds += slice.seconds
            last.share += slice.share
            last.groups++
            return
        }
        families.push({
            family: slice.family,
            calls: slice.calls,
            seconds: slice.seconds,
            share: slice.share,
            firstIndex: index,
            groups: 1,
        })
    })

    const busiest = slices.reduce<ToolSlice | null>((best, s) => (!best || s.calls > best.calls ? s : best), null)
    return { slices, families, calls, seconds, busiest }
}
