/**
 * Turning the API's numbers into text a person reads. Every duration is seconds and every instant
 * is RFC 3339 in UTC, so these are the only two conversions the UI does.
 *
 * Instants render in the machine's own zone, because "when did this happen" means the clock on the
 * wall the session ran against, not UTC.
 */

const HOUR = 3600
const MINUTE = 60

/**
 * A duration at the grain a reader can hold: `6h15m`, `9m21s`, `1.997s`, `194ms`. Sub-second rows
 * are real (the engine keeps them), so they can't round to `0s`.
 */
export function formatDuration(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds < 0) return '?'
    if (seconds === 0) return '0s'
    if (seconds < 1) return `${Math.round(seconds * 1000)}ms`
    if (seconds < 10) return `${trimZeros(seconds.toFixed(3))}s`
    if (seconds < MINUTE) return `${seconds.toFixed(1)}s`
    if (seconds < HOUR) {
        const m = Math.floor(seconds / MINUTE)
        return `${m}m${pad(Math.floor(seconds % MINUTE))}s`
    }
    const h = Math.floor(seconds / HOUR)
    const m = Math.floor((seconds % HOUR) / MINUTE)
    return `${formatCount(h)}h${pad(m)}m`
}

/** The same duration spelled out, for a tooltip or a title attribute: `6 hours, 15 minutes`. */
export function formatDurationLong(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds < 0) return 'unknown'
    if (seconds < MINUTE) return `${trimZeros(seconds.toFixed(3))} seconds`
    const parts: string[] = []
    const h = Math.floor(seconds / HOUR)
    const m = Math.floor((seconds % HOUR) / MINUTE)
    const s = Math.floor(seconds % MINUTE)
    if (h) parts.push(`${formatCount(h)} ${h === 1 ? 'hour' : 'hours'}`)
    if (m) parts.push(`${m} ${m === 1 ? 'minute' : 'minutes'}`)
    if (s && !h) parts.push(`${s} ${s === 1 ? 'second' : 'seconds'}`)
    return parts.join(', ')
}

/** A whole number with thousands separators, the way every user-facing count is written. */
export function formatCount(n: number): string {
    return new Intl.NumberFormat('en-US').format(Math.round(n))
}

/** A share of a whole, as a percentage. Anything above zero but below 0.1% says `<0.1%`. */
export function formatShare(part: number, whole: number): string {
    if (!whole) return '0%'
    const pct = (part / whole) * 100
    if (pct > 0 && pct < 0.1) return '<0.1%'
    return `${pct >= 10 ? pct.toFixed(0) : pct.toFixed(1)}%`
}

const DATE_TIME = new Intl.DateTimeFormat('sv-SE', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
})

const DATE_TIME_SECONDS = new Intl.DateTimeFormat('sv-SE', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
})

const TIME_ONLY = new Intl.DateTimeFormat('sv-SE', { hour: '2-digit', minute: '2-digit', hour12: false })

/**
 * `2026-08-03 08:42` in the machine's zone. The `sv-SE` locale is the shortest way to an ISO-shaped
 * date out of `Intl`, which is the format this repo writes dates in.
 */
export function formatInstant(iso: string | null | undefined): string {
    const d = parseInstant(iso)
    return d ? DATE_TIME.format(d) : 'unknown'
}

/** The same, down to the second, for a row that lasted under a minute. */
export function formatInstantSeconds(iso: string | null | undefined): string {
    const d = parseInstant(iso)
    return d ? DATE_TIME_SECONDS.format(d) : 'unknown'
}

export function formatTimeOfDay(ms: number): string {
    return TIME_ONLY.format(new Date(ms))
}

/**
 * The floor exists because a zero date is a valid RFC 3339 string. The API nulls its own, but anything upstream that
 * grows a new one shouldn't be able to put "1-01-01" in front of a reader: no Claude Code transcript predates 2024.
 */
const EARLIEST_PLAUSIBLE = Date.UTC(2024, 0, 1)

export function parseInstant(iso: string | null | undefined): Date | null {
    if (!iso) return null
    const d = new Date(iso)
    const ms = d.getTime()
    return Number.isNaN(ms) || ms < EARLIEST_PLAUSIBLE ? null : d
}

/** Milliseconds since the epoch, or null when the instant is missing or unparseable. */
export function instantMs(iso: string | null | undefined): number | null {
    return parseInstant(iso)?.getTime() ?? null
}

/**
 * Bytes the way the operating system writes them, in powers of a thousand: `67.0 MB`, `2.8 KB`.
 * Matches `humanBytes` in `internal/cli/format.go`, so the CLI and this page never disagree about
 * the size of the same transcript.
 */
export function formatBytes(bytes: number): string {
    if (bytes < 1000) return `${bytes} B`
    const units = ['KB', 'MB', 'GB', 'TB']
    let value = bytes / 1000
    let unit = 0
    while (value >= 1000 && unit < units.length - 1) {
        value /= 1000
        unit += 1
    }
    return `${value.toFixed(1)} ${units[unit]}`
}

/** How long ago, for a listing: `4 minutes ago`, `3 days ago`. */
export function formatRelative(iso: string | null | undefined, now = Date.now()): string {
    const ms = instantMs(iso)
    if (ms === null) return 'unknown'
    const seconds = (ms - now) / 1000
    const abs = Math.abs(seconds)
    const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
    if (abs < MINUTE) return rtf.format(Math.round(seconds), 'second')
    if (abs < HOUR) return rtf.format(Math.round(seconds / MINUTE), 'minute')
    if (abs < 24 * HOUR) return rtf.format(Math.round(seconds / HOUR), 'hour')
    if (abs < 30 * 24 * HOUR) return rtf.format(Math.round(seconds / (24 * HOUR)), 'day')
    if (abs < 365 * 24 * HOUR) return rtf.format(Math.round(seconds / (30 * 24 * HOUR)), 'month')
    return rtf.format(Math.round(seconds / (365 * 24 * HOUR)), 'year')
}

/** The first eight characters of a session id, which is what the CLI accepts as a prefix. */
export function shortId(id: string): string {
    return id.slice(0, 8)
}

function pad(n: number): string {
    return n < 10 ? `0${n}` : `${n}`
}

function trimZeros(s: string): string {
    return s.includes('.') ? s.replace(/\.?0+$/, '') : s
}
