/**
 * The per-kind totals, shaped for the pie and its legend.
 *
 * Everything here is a share of **lane time**: every lane's rows added up, which is larger than the
 * session's elapsed time whenever lanes ran at once. The UI has to say so wherever these numbers
 * appear, because reading them as a breakdown of elapsed time is the one mistake the API is shaped
 * to prevent (`docs/api.md` § The two numbers that aren't the same).
 */

import { byLegendOrder, kindFamily, type KindFamily } from '../kinds'
import type { KindTotal } from '../types'

export interface KindSlice {
    kind: string
    seconds: number
    rows: number
    /** Of lane time, in the range 0 to 1. */
    share: number
    family: KindFamily
}

/**
 * Slices in legend order, not in size order. The API only sends kinds that have rows, so the four
 * waits stay adjacent whichever of them a session happens to have, and a colour can't drift from
 * one session to the next.
 */
export function kindSlices(totals: readonly KindTotal[]): KindSlice[] {
    const withTime = totals.filter((t) => t.seconds > 0)
    const whole = withTime.reduce((sum, t) => sum + t.seconds, 0)
    return byLegendOrder(withTime).map((t) => ({
        kind: t.kind,
        seconds: t.seconds,
        rows: t.rows,
        share: whole ? t.seconds / whole : 0,
        family: kindFamily(t.kind),
    }))
}

export interface Band {
    seconds: number
    share: number
}

export interface Bands {
    work: Band
    wait: Band
    trouble: Band
    overhead: Band
    total: number
}

/**
 * The four kind families rolled up, which is the answer to the question everyone asks first: how
 * much of this was waiting? `trouble` is the time a session lost through no fault of an agent (a
 * refused request, a suspended one), and `overhead` is compaction.
 */
export function bandTotals(totals: readonly KindTotal[]): Bands {
    const sums: Record<KindFamily, number> = { work: 0, wait: 0, trouble: 0, overhead: 0 }
    for (const t of totals) sums[kindFamily(t.kind)] += t.seconds
    const total = sums.work + sums.wait + sums.trouble + sums.overhead
    const band = (seconds: number): Band => ({ seconds, share: total ? seconds / total : 0 })
    return {
        work: band(sums.work),
        wait: band(sums.wait),
        trouble: band(sums.trouble),
        overhead: band(sums.overhead),
        total,
    }
}
