/**
 * How the eleven activity kinds are drawn and described.
 *
 * Look a kind up by name, never by position: the API lists only the kinds a session actually has
 * rows for, so indexing into an array would shift every colour between two sessions. An unknown
 * name (the engine grew a kind and this file didn't) falls back to a neutral entry rather than
 * throwing, so a new kind shows up as grey and unexplained instead of blanking the page.
 *
 * Colours live in `app.css` and are read from there, so light and dark stay in one place.
 */

export type KindFamily = 'work' | 'wait' | 'trouble' | 'overhead'

export interface KindStyle {
    /** The name the API sends. */
    kind: string
    /** The CSS custom property holding this kind's colour, in both themes. */
    cssVar: string
    family: KindFamily
    /** What the number means, in the legend's own tooltip. Honest over impressive. */
    description: string
}

/**
 * Legend order, matching `docs/api.md`. Work first, then the four waits as a family, then the two
 * ways a session loses time, then compaction.
 */
export const KIND_ORDER = [
    'thinking',
    'writing',
    'tool call',
    'tool execution',
    'waiting for a person',
    'waiting for a teammate',
    'waiting for a background task',
    'waiting, reason unknown',
    'API error',
    'stalled',
    'compacting',
] as const

const STYLES: Record<string, KindStyle> = {
    thinking: {
        kind: 'thinking',
        cssVar: '--csa-kind-thinking',
        family: 'work',
        description:
            'A thinking block. The span starts when the block before it finished streaming, so it holds model latency and prompt processing as well as reasoning.',
    },
    writing: {
        kind: 'writing',
        cssVar: '--csa-kind-writing',
        family: 'work',
        description: 'A text block: the agent composing prose for whoever asked.',
    },
    'tool call': {
        kind: 'tool call',
        cssVar: '--csa-kind-tool-call',
        family: 'work',
        description:
            'Composing the call. When no thinking block came first, this span carries the whole response latency too.',
    },
    'tool execution': {
        kind: 'tool execution',
        cssVar: '--csa-kind-tool-execution',
        family: 'work',
        description:
            'The tool running, from the call to its result. It counts whatever the tool itself waited on, a permission prompt included.',
    },
    'waiting for a person': {
        kind: 'waiting for a person',
        cssVar: '--csa-kind-wait-person',
        family: 'wait',
        description: 'The lane produced nothing until a person typed, queued a prompt, or answered a question.',
    },
    'waiting for a teammate': {
        kind: 'waiting for a teammate',
        cssVar: '--csa-kind-wait-teammate',
        family: 'wait',
        description: "Another agent's message closed the gap.",
    },
    'waiting for a background task': {
        kind: 'waiting for a background task',
        cssVar: '--csa-kind-wait-background',
        family: 'wait',
        description: "A background task's notification closed the gap.",
    },
    'waiting, reason unknown': {
        kind: 'waiting, reason unknown',
        cssVar: '--csa-kind-wait-unknown',
        family: 'wait',
        description: 'The lane went quiet and later produced something, with nothing in between to say why.',
    },
    'API error': {
        kind: 'API error',
        cssVar: '--csa-kind-api-error',
        family: 'trouble',
        description:
            "The API didn't answer: an outage, a rate limit, an expired login. The span is the harness retrying, capped at two hours because retries aren't written down.",
    },
    stalled: {
        kind: 'stalled',
        cssVar: '--csa-kind-stalled',
        family: 'trouble',
        description:
            'A result that arrived far too late for what the call was doing, so the agent was suspended rather than working. This one is a heuristic: the row carries the command and the line it was measured against.',
    },
    compacting: {
        kind: 'compacting',
        cssVar: '--csa-kind-compacting',
        family: 'overhead',
        description: 'The harness compacting the context. Real time, and neither the agent nor a tool.',
    },
}

const UNKNOWN: KindStyle = {
    kind: 'unknown',
    cssVar: '--csa-kind-compacting',
    family: 'overhead',
    description: "A kind this page has no description for yet. The engine grew one and the legend hasn't caught up.",
}

export function kindStyle(kind: string): KindStyle {
    return STYLES[kind] ?? { ...UNKNOWN, kind }
}

export function kindFamily(kind: string): KindFamily {
    return kindStyle(kind).family
}

/** Sorts kinds into the legend order above. Names the engine grew since fall to the end, in place. */
export function byLegendOrder<T extends { kind: string }>(items: readonly T[]): T[] {
    const rank = new Map<string, number>(KIND_ORDER.map((k, i) => [k, i]))
    return [...items].sort((a, b) => (rank.get(a.kind) ?? 99) - (rank.get(b.kind) ?? 99))
}
