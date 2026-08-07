/**
 * The three durations over the same rows, shaped as rungs.
 *
 * Lane time, net agent time, active time. Each rung is the one above minus something, and the
 * subtraction is the part that matters: it's what stops one of them being quoted as another. A
 * reader picks the rung that answers their question rather than choosing between rivals.
 * `docs/api.md` § The ladder is the definition; this only measures.
 *
 * Wall clock isn't a rung. It's how long the session took whatever ran at once, so it isn't lane
 * time minus anything, and it rides alongside rather than in the list.
 */

import type { TimelineTotals } from '../types'

export type RungKey = 'lane' | 'net' | 'active'

export interface Rung {
    key: RungKey
    seconds: number
    /** Of lane time, 0 to 1, so a rung nests inside the one above it. */
    share: number
    /** What the rung above lost to get here. Zero on the top rung, and never negative. */
    subtracted: number
}

export interface TimeLadder {
    rungs: Rung[]
    laneSeconds: number
    /** Beside the ladder, never a rung. */
    wallClockSeconds: number
}

type Totals = Pick<TimelineTotals, 'wallClockSeconds' | 'laneTimeSeconds' | 'netSeconds' | 'activeSeconds'>

/**
 * The rungs widest first, each carrying what it lost.
 *
 * A rung arriving larger than the one above it can't come out of the engine, so it's clamped rather
 * than handled: a bad response should degrade to a page that reads, not to a bar drawn past its own
 * track.
 */
export function timeLadder(totals: Totals): TimeLadder {
    const lane = Math.max(totals.laneTimeSeconds, 0)
    const net = clamp(totals.netSeconds, lane)
    const active = clamp(totals.activeSeconds, net)

    const rung = (key: RungKey, seconds: number, above: number): Rung => ({
        key,
        seconds,
        share: lane ? seconds / lane : 0,
        subtracted: Math.max(above - seconds, 0),
    })

    return {
        rungs: [rung('lane', lane, lane), rung('net', net, lane), rung('active', active, net)],
        laneSeconds: lane,
        wallClockSeconds: Math.max(totals.wallClockSeconds, 0),
    }
}

function clamp(seconds: number, ceiling: number): number {
    return Math.min(Math.max(seconds, 0), ceiling)
}
