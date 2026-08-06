/**
 * The one place that talks to the Go server. Paths are relative, because Vite proxies `/api` to the
 * backend port out of the repo root's `.env` (see `vite.config.ts`), so the app is same-origin with
 * its API and never carries a base URL.
 *
 * Every failure arrives as an `ApiError` carrying the server's `code`, which is what callers branch
 * on. Never branch on the message: it's written for a person and it's free to change.
 */

import type { ApiErrorBody, SessionListResponse, SessionSummary, TimelineResponse } from './types'

export class ApiError extends Error {
    readonly code: string
    readonly status: number
    readonly matches: string[]

    constructor(code: string, message: string, status: number, matches: string[] = []) {
        super(message)
        this.name = 'ApiError'
        this.code = code
        this.status = status
        this.matches = matches
    }
}

/** What went wrong, said the way this repo says things: conversational, and pointing somewhere. */
export function describeApiError(err: unknown): { headline: string; detail: string } {
    if (err instanceof ApiError) {
        switch (err.code) {
            case 'unreachable':
                return {
                    headline: "The analyzer isn't answering",
                    detail: 'Start it with `pnpm dev` at the repo root, which brings up the Go server alongside this page.',
                }
            case 'not_found':
                return { headline: 'No session with that id', detail: err.message }
            case 'ambiguous_id':
                return {
                    headline: 'That id matches several sessions',
                    detail: `${err.message} Try a longer prefix: ${err.matches.join(', ')}.`,
                }
            case 'no_transcripts':
                return { headline: 'No transcripts on this machine', detail: err.message }
            default:
                return { headline: 'The analyzer turned that request down', detail: err.message }
        }
    }
    return { headline: 'Something went wrong', detail: err instanceof Error ? err.message : String(err) }
}

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
    let res: Response
    try {
        res = await fetch(path, { signal, headers: { accept: 'application/json' } })
    } catch (cause) {
        if (cause instanceof DOMException && cause.name === 'AbortError') throw cause
        throw new ApiError('unreachable', 'The page could not reach the analyzer on this machine.', 0)
    }
    if (!res.ok) {
        const body = (await res.json().catch(() => null)) as ApiErrorBody | null
        const error = body?.error
        throw new ApiError(
            error?.code ?? 'internal',
            error?.message ?? `The analyzer answered ${res.status}.`,
            res.status,
            error?.matches ?? [],
        )
    }
    return (await res.json()) as T
}

/** Every session under the transcript root, newest first. 273 KB and 140 ms over 725 of them. */
export function fetchSessions(signal?: AbortSignal): Promise<SessionListResponse> {
    return get<SessionListResponse>('/api/sessions', signal)
}

export function fetchSession(id: string, signal?: AbortSignal): Promise<{ session: SessionSummary }> {
    return get<{ session: SessionSummary }>(`/api/sessions/${encodeURIComponent(id)}`, signal)
}

/**
 * The aggregates alone: totals, per-kind sums, and one lane entry per lane with its gaps. 364 KB on
 * the largest session, against 7.7 MB with the rows, so the charts render off this and the sheet
 * fetches its own copy afterwards.
 */
export function fetchAggregates(id: string, signal?: AbortSignal): Promise<TimelineResponse> {
    return get<TimelineResponse>(`/api/sessions/${encodeURIComponent(id)}/timeline?rows=false`, signal)
}

/** The same timeline with every row. Megabytes, and a second parse on the server, so ask once. */
export function fetchTimelineRows(id: string, signal?: AbortSignal): Promise<TimelineResponse> {
    return get<TimelineResponse>(`/api/sessions/${encodeURIComponent(id)}/timeline`, signal)
}
