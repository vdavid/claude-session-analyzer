/**
 * The JSON shapes `internal/api` serves. The contract is `docs/api.md`; when a field changes there,
 * it changes here. Fields the server leaves out when they don't apply are optional, never empty.
 */

/** One of the eleven activity kinds. The engine owns the list; `kinds.ts` owns how it's drawn. */
export type KindName = string

export interface KindTotal {
    kind: KindName
    seconds: number
    rows: number
}

export interface SessionSummary {
    id: string
    projectSlug: string
    /** Empty for a session whose records carry no `cwd`, which is 99 of the 725 on this machine. */
    projectPath: string
    projectName: string
    /** Empty for a session that never got a title. */
    title: string
    /** Null for a session whose records carry no timestamps. Never a zero date. */
    start: string | null
    end: string | null
    seconds: number
    modified: string
    /** Lanes the session spawned. One less than a timeline's `totals.lanes`, which counts the lead. */
    subagents: number
    bytes: number
}

export interface SessionListResponse {
    root: string
    sessions: SessionSummary[]
    totals: { sessions: number; subagents: number; bytes: number }
}

export interface Gap {
    from: string
    until: string
    seconds: number
    kind: KindName
    info: string
}

export interface Lane {
    id: string
    name: string
    isLead: boolean
    model?: string
    /** What the terminal used. Missing far more often than not, so the UI keeps its own palette. */
    color?: string
    /** The workflow that spawned this lane, absent for one the session spawned directly. */
    workflowId?: string
    from: string
    until: string
    seconds: number
    rows: number
    byKind: KindTotal[]
    /** Every stretch the lane produced nothing, in time order. A swimlane draws these as holes. */
    gaps: Gap[]
}

export interface TimelineRow {
    from: string
    until: string
    seconds: number
    /** The grouping key. Never group by `agent`: 977 lanes in one session share one name. */
    laneId: string
    agent: string
    kind: KindName
    info: string
    tool?: string
    class?: string
    /** Which slice of the tool breakdown the row belongs to. Only there alongside `tool`. */
    toolGroup?: string
    overlapped?: boolean
    timedOut?: boolean
    isError?: boolean
    /** A line inside one transcript file, so it's a pointer for tracing rather than a key. */
    line?: number
}

export interface TimelineTotals {
    /** Null on a session whose records carry no timestamp, same as the session's own `start`. */
    from: string | null
    until: string | null
    /** How long the session took. */
    wallClockSeconds: number
    /** Every lane's rows added up, so it's larger whenever lanes ran at the same time. */
    laneTimeSeconds: number
    rows: number
    lanes: number
    /** Only the kinds with rows behind them, in the order a legend should show them. */
    byKind: KindTotal[]
    /** The tool breakdown, most calls first. Empty on a session that never called a tool. */
    byTool: ToolGroupTotal[]
}

/**
 * One slice of the tool breakdown: everything a session did with one tool, at the level a reader
 * asks about. `Bash` arrives split by what the command was doing and an MCP server's methods arrive
 * collapsed into the server, so the groups are `Bash (git)`, `codegraph (MCP)`, `Read`.
 *
 * The counts are calls, not rows: every call leaves a row for composing it and a row for running it,
 * and only the second is counted.
 */
export interface ToolGroupTotal {
    group: string
    /** What kind of work the group does. `classes.ts` turns it into the colour and the family. */
    class: string
    calls: number
    seconds: number
    /** How many lanes reached for it, which is the answer to "who used this". */
    lanes: number
    errors?: number
    timedOut?: number
    /** The exact tools inside the group, most calls first. */
    tools: ToolTotal[]
}

export interface ToolTotal {
    /** The raw name the harness used, so a reader can grep a transcript for it. */
    tool: string
    /** The part that varies inside the group: an MCP method, or the program a Bash call ran. */
    leaf: string
    calls: number
    seconds: number
    lanes: number
    errors?: number
    timedOut?: number
}

export interface TimelineResponse {
    session: SessionSummary
    totals: TimelineTotals
    lanes: Lane[]
    /** Absent when the timeline was fetched with `?rows=false`. */
    rows?: TimelineRow[]
}

export interface ApiErrorBody {
    error: { code: string; message: string; matches?: string[] }
}
