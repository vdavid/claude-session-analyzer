/**
 * How many lanes were producing at once, over the session's span.
 *
 * This is the one number the pie and the swimlane can't show between them: the pie flattens time
 * away, and the swimlane spreads it over rows nobody can take in at a glance. A trace of "agents
 * producing" answers the shape question directly, and it's built from the same lanes and gaps the
 * swimlane draws, so the two can't disagree.
 *
 * A bucket holds the **average** number of lanes producing during it, not a peak: a lane busy for
 * a tenth of a bucket contributes a tenth. That keeps a sparse session from reading as a solid one
 * and keeps the area under the trace equal to the lane time it came from.
 */

import { busySegments } from './swimlane'
import { instantMs } from '../format'
import type { Lane } from '../types'

export interface TracePoint {
    /** The bucket's start, in milliseconds since the epoch. */
    t: number
    lanes: number
}

export interface Trace {
    from: number
    until: number
    bucketMs: number
    points: TracePoint[]
    /** The most lanes any one bucket averaged, for the axis and for the caption. */
    peak: number
}

const EMPTY: Trace = { from: 0, until: 0, bucketMs: 0, points: [], peak: 0 }

export function concurrencyTrace(
    lanes: readonly Lane[],
    buckets: number,
    span?: { from: number; until: number },
): Trace {
    const segments = lanes.flatMap(busySegments)
    // The span comes from the lanes' own ends, not from the busy segments inside them. Deriving it
    // from the segments would crop a session that opened or closed with everyone idle, which is
    // exactly the stretch a reader is looking for.
    const ends = lanes.flatMap((l) => [instantMs(l.from), instantMs(l.until)]).filter((v): v is number => v !== null)
    const from = span?.from ?? Math.min(...ends)
    const until = span?.until ?? Math.max(...ends)
    if (!Number.isFinite(from) || !Number.isFinite(until) || until <= from || buckets < 1) return EMPTY

    const bucketMs = (until - from) / buckets
    const covered = new Float64Array(buckets)

    for (const [a, b] of segments) {
        const start = Math.max(a, from)
        const end = Math.min(b, until)
        if (end <= start) continue
        const first = Math.min(Math.floor((start - from) / bucketMs), buckets - 1)
        const last = Math.min(Math.floor((end - from) / bucketMs), buckets - 1)
        for (let i = first; i <= last; i++) {
            const lo = Math.max(start, from + i * bucketMs)
            const hi = Math.min(end, from + (i + 1) * bucketMs)
            if (hi > lo) covered[i] += hi - lo
        }
    }

    const points: TracePoint[] = []
    let peak = 0
    for (let i = 0; i < buckets; i++) {
        const lanesHere = covered[i] / bucketMs
        peak = Math.max(peak, lanesHere)
        points.push({ t: from + i * bucketMs, lanes: lanesHere })
    }
    return { from, until, bucketMs, points, peak }
}
