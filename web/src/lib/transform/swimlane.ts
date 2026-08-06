/**
 * Lanes and their gaps, shaped into swimlane rows.
 *
 * Two things this exists to get right:
 *
 * 1. **A bar is never solid across idle hours.** The API hands back each lane's span plus every
 *    stretch it produced nothing, so a row's bar is the span minus those gaps. Drawn solid, the
 *    reference session's lead would claim it was busy for 71 of its 73 hours.
 * 2. **A thousand lanes is a real session, not a hypothetical.** One session on this machine has
 *    983 lanes, 977 of them spawned by 12 workflows and all 977 named `workflow-subagent`. Drawing
 *    983 rows would be unreadable, so a workflow collapses to one row carrying the union of its
 *    lanes' bars, and opens on click.
 *
 * Lanes are keyed by `id` throughout. Names collide by the hundred.
 */

import { instantMs } from '../format'
import type { Lane } from '../types'

/** `[start, end]` in milliseconds since the epoch, which is what ECharts' time axis takes. */
export type Segment = [number, number]

export interface GapSegment {
    from: number
    until: number
    kind: string
}

export type RowKind = 'lead' | 'lane' | 'workflow'

export interface SwimlaneRow {
    /** Stable across re-renders and unique, so expanding one group can't reorder another. */
    key: string
    label: string
    /** The model, a lane count, or whatever else distinguishes this row from the one above. */
    sublabel: string
    kind: RowKind
    /** 0 for a top-level row, 1 for a lane revealed inside an expanded workflow. */
    depth: number
    workflowId?: string
    expandable: boolean
    expanded: boolean
    /** Lanes behind this row: 1 for a lane, the member count for a workflow. */
    laneCount: number
    from: number
    until: number
    /** The stretches something was actually produced. */
    busy: Segment[]
    /** The stretches nothing was, each carrying what was waited on. Empty on a workflow row. */
    gaps: GapSegment[]
    /** Lane time behind this row, in seconds. */
    seconds: number
    /** Members an expanded workflow held back, so the UI can say how many aren't drawn. */
    hiddenLanes: number
}

export interface SwimlaneOptions {
    /** Workflow ids the reader has opened. */
    expanded: ReadonlySet<string>
    /** How many of a workflow's lanes an expanded group will draw. */
    maxLanesPerGroup?: number
}

const DEFAULT_MAX_LANES_PER_GROUP = 150

/**
 * The stretches a lane produced something: its span with every gap cut out. Gaps are merged and
 * clamped first, because the engine emits them per row and two can touch or run past the span.
 */
export function busySegments(lane: Lane): Segment[] {
    const from = instantMs(lane.from)
    const until = instantMs(lane.until)
    if (from === null || until === null || until < from) return []

    const holes = merge(
        lane.gaps
            .map((g): Segment => [instantMs(g.from) ?? from, instantMs(g.until) ?? from])
            .map(([a, b]): Segment => [clamp(a, from, until), clamp(b, from, until)])
            .filter(([a, b]) => b > a),
    )

    const busy: Segment[] = []
    let cursor = from
    for (const [a, b] of holes) {
        if (a > cursor) busy.push([cursor, a])
        cursor = Math.max(cursor, b)
    }
    if (cursor < until) busy.push([cursor, until])
    return busy
}

/**
 * Rows in reading order: the lead, then one row per workflow, then the lanes the session spawned
 * itself. Workflows come before direct lanes because a workflow row stands for hundreds of lanes
 * and burying it under a list of singletons hides the bulk of the session.
 */
export function buildSwimlane(lanes: readonly Lane[], options: SwimlaneOptions): SwimlaneRow[] {
    const maxPerGroup = options.maxLanesPerGroup ?? DEFAULT_MAX_LANES_PER_GROUP
    const usable = lanes.filter((l) => instantMs(l.from) !== null && instantMs(l.until) !== null)

    const lead = usable.filter((l) => l.isLead)
    const direct = usable.filter((l) => !l.isLead && !l.workflowId)
    const grouped = new Map<string, Lane[]>()
    for (const l of usable) {
        if (l.isLead || !l.workflowId) continue
        const members = grouped.get(l.workflowId)
        if (members) members.push(l)
        else grouped.set(l.workflowId, [l])
    }

    const rows: SwimlaneRow[] = []
    for (const l of byStart(lead)) rows.push(laneRow(l, 0))

    const workflows = [...grouped.entries()].sort((a, b) => earliest(a[1]) - earliest(b[1]) || a[0].localeCompare(b[0]))
    for (const [workflowId, members] of workflows) {
        const expanded = options.expanded.has(workflowId)
        const drawn = expanded ? byStart(members).slice(0, maxPerGroup) : []
        const named = distinctNames(members)
        rows.push(workflowRow(workflowId, members, expanded, members.length - drawn.length))
        for (const l of drawn) rows.push(laneRow(l, 1, workflowId, named ? undefined : l.id.slice(0, 8)))
    }

    for (const l of byStart(direct)) rows.push(laneRow(l, 0))
    return rows
}

/**
 * Whether a group's lanes can be told apart by name. A workflow's 848 lanes are all called
 * `workflow-subagent`, and a column of that name 150 times says nothing, so those rows fall back to
 * the front of their lane id instead.
 */
function distinctNames(lanes: readonly Lane[]): boolean {
    return new Set(lanes.map((l) => l.name)).size === lanes.length
}

function laneRow(lane: Lane, depth: number, workflowId?: string, labelOverride?: string): SwimlaneRow {
    const from = instantMs(lane.from) as number
    const until = instantMs(lane.until) as number
    return {
        key: lane.isLead ? `lead:${lane.id}` : `lane:${lane.id}`,
        label: labelOverride || lane.name || lane.id,
        sublabel: lane.isLead ? 'the session itself' : labelOverride ? lane.name : (lane.model ?? ''),
        kind: lane.isLead ? 'lead' : 'lane',
        depth,
        workflowId,
        expandable: false,
        expanded: false,
        laneCount: 1,
        from,
        until,
        busy: busySegments(lane),
        gaps: lane.gaps
            .map((g) => ({ from: instantMs(g.from), until: instantMs(g.until), kind: g.kind }))
            .filter((g): g is GapSegment => g.from !== null && g.until !== null && g.until > g.from),
        seconds: lane.seconds,
        hiddenLanes: 0,
    }
}

/**
 * One row standing for every lane a workflow spawned. Its bar is the union of the members' bars, so
 * a stretch is drawn only when at least one member was producing: the hole in a workflow row means
 * the whole workflow was quiet, which is worth seeing on its own.
 */
function workflowRow(workflowId: string, members: Lane[], expanded: boolean, hidden: number): SwimlaneRow {
    const busy = merge(members.flatMap(busySegments))
    return {
        key: `workflow:${workflowId}`,
        label: workflowId,
        sublabel: `${members.length} ${members.length === 1 ? 'lane' : 'lanes'}`,
        kind: 'workflow',
        depth: 0,
        workflowId,
        expandable: true,
        expanded,
        laneCount: members.length,
        from: Math.min(...members.map((l) => instantMs(l.from) as number)),
        until: Math.max(...members.map((l) => instantMs(l.until) as number)),
        busy,
        gaps: [],
        seconds: members.reduce((sum, l) => sum + l.seconds, 0),
        hiddenLanes: expanded ? hidden : 0,
    }
}

function byStart(lanes: readonly Lane[]): Lane[] {
    return [...lanes].sort((a, b) => (instantMs(a.from) ?? 0) - (instantMs(b.from) ?? 0) || a.id.localeCompare(b.id))
}

function earliest(lanes: readonly Lane[]): number {
    return Math.min(...lanes.map((l) => instantMs(l.from) ?? Number.POSITIVE_INFINITY))
}

/** Sorts segments and folds together any that overlap or touch. */
function merge(segments: readonly Segment[]): Segment[] {
    const sorted = [...segments].sort((a, b) => a[0] - b[0])
    const out: Segment[] = []
    for (const [a, b] of sorted) {
        const last = out[out.length - 1]
        if (last && a <= last[1]) last[1] = Math.max(last[1], b)
        else out.push([a, b])
    }
    return out
}

function clamp(value: number, low: number, high: number): number {
    return Math.min(Math.max(value, low), high)
}
