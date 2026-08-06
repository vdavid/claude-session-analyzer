<script lang="ts">
    /**
     * What this tool is, and every session on the machine.
     *
     * The whole list arrives in one 273 KB response, so filtering and sorting happen here with no
     * round trip. 725 rows render fine as plain DOM; the sheet on a session page is the one that
     * needs virtualizing.
     */
    import { resolve } from '$app/paths'
    import { describeApiError, fetchSessions } from '$lib/api'
    import Notice from '$lib/components/Notice.svelte'
    import { formatBytes, formatCount, formatDuration, formatInstant, formatRelative } from '$lib/format'
    import type { SessionListResponse, SessionSummary } from '$lib/types'

    let data = $state<SessionListResponse | null>(null)
    let failure = $state<unknown>(null)
    let query = $state('')
    let sortKey = $state<'start' | 'seconds' | 'subagents' | 'bytes' | 'title'>('start')
    let descending = $state(true)

    $effect(() => {
        const controller = new AbortController()
        fetchSessions(controller.signal)
            .then((result) => {
                data = result
                failure = null
            })
            .catch((cause) => {
                if (cause instanceof DOMException && cause.name === 'AbortError') return
                failure = cause
            })
        return () => controller.abort()
    })

    const sessions = $derived(data?.sessions ?? [])

    const matching = $derived.by(() => {
        const needle = query.trim().toLowerCase()
        if (!needle) return sessions
        return sessions.filter((s) =>
            `${s.title} ${s.projectName} ${s.projectPath} ${s.id}`.toLowerCase().includes(needle),
        )
    })

    const sorted = $derived.by(() => {
        const direction = descending ? -1 : 1
        const value = (s: SessionSummary): number | string => {
            switch (sortKey) {
                case 'start':
                    return s.start ? Date.parse(s.start) : 0
                case 'seconds':
                    return s.seconds
                case 'subagents':
                    return s.subagents
                case 'bytes':
                    return s.bytes
                case 'title':
                    return (s.title || s.projectName).toLowerCase()
            }
        }
        return [...matching].sort((a, b) => {
            const left = value(a)
            const right = value(b)
            if (typeof left === 'string' && typeof right === 'string') return direction * left.localeCompare(right)
            return direction * ((left as number) - (right as number))
        })
    })

    function sortBy(key: typeof sortKey) {
        if (sortKey === key) descending = !descending
        else {
            sortKey = key
            descending = key !== 'title'
        }
    }

    function ariaSort(key: typeof sortKey): 'ascending' | 'descending' | 'none' {
        if (sortKey !== key) return 'none'
        return descending ? 'descending' : 'ascending'
    }

    const columns: { key: typeof sortKey; label: string; align?: 'right' }[] = [
        { key: 'title', label: 'Session' },
        { key: 'start', label: 'Started' },
        { key: 'seconds', label: 'Elapsed', align: 'right' },
        { key: 'subagents', label: 'Subagents', align: 'right' },
        { key: 'bytes', label: 'Transcript', align: 'right' },
    ]
</script>

<svelte:head><title>Claude session analyzer</title></svelte:head>

<section class="rise grid gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-16">
    <div class="max-w-2xl">
        <p class="eyebrow">Claude Code, measured</p>
        <h1 class="mt-2 text-4xl leading-[1.1] font-semibold tracking-tight text-balance text-ink sm:text-5xl">
            Where the time went
        </h1>
        <p class="mt-4 text-lg leading-relaxed text-ink-muted">
            This reads session transcripts off this machine and reconstructs what every agent was doing, second by
            second, from the first prompt to the last tool result. Open a session to see the split: thinking, tools
            running, and the long stretches where somebody was waiting.
        </p>
        <p class="mt-3 text-base leading-relaxed text-ink-faint">
            Nothing is stored and nothing is cached. Every number is derived from the transcript at the moment you ask
            for it.
        </p>
    </div>

    {#if data}
        <div class="lg:w-64 lg:border-l lg:border-border-base lg:pl-8">
            <dl>
                <div
                    class="flex items-baseline justify-between gap-4 border-b border-border-base py-2 lg:block lg:py-0"
                >
                    <dt class="eyebrow">Sessions</dt>
                    <dd class="tnum font-mono text-2xl leading-tight text-ink lg:mt-0.5">
                        {formatCount(data.totals.sessions)}
                    </dd>
                </div>
                <div
                    class="flex items-baseline justify-between gap-4 border-b border-border-base py-2 lg:mt-4 lg:block lg:py-0"
                >
                    <dt class="eyebrow">Subagent lanes</dt>
                    <dd class="tnum font-mono text-2xl leading-tight text-ink lg:mt-0.5">
                        {formatCount(data.totals.subagents)}
                    </dd>
                </div>
                <div class="flex items-baseline justify-between gap-4 py-2 lg:mt-4 lg:block lg:py-0">
                    <dt class="eyebrow">On disk</dt>
                    <dd class="tnum font-mono text-2xl leading-tight text-ink lg:mt-0.5">
                        {formatBytes(data.totals.bytes)}
                    </dd>
                </div>
            </dl>
            <p class="mt-3 font-mono text-[11px] leading-snug break-all text-ink-faint">
                Read from {data.root}
            </p>
        </div>
    {/if}
</section>

{#if failure}
    {@const described = describeApiError(failure)}
    <div class="mt-8 max-w-2xl">
        <Notice headline={described.headline} detail={described.detail} tone="trouble" />
    </div>
{:else if !data}
    <p class="mt-10 text-sm text-ink-faint">Reading the transcript directory…</p>
{:else}
    <section class="rise mt-10" style:--rise-delay="60ms">
        <div class="flex flex-wrap items-end justify-between gap-4">
            <p class="text-sm text-ink-muted">
                Showing <span class="tnum font-mono text-ink">{formatCount(sorted.length)}</span>
                {sorted.length === 1 ? 'session' : 'sessions'}, newest first unless you sort them otherwise.
            </p>
            <label class="w-full sm:w-72">
                <span class="sr-only">Filter sessions</span>
                <input
                    type="search"
                    bind:value={query}
                    placeholder="A title, a project, a session id"
                    class="w-full rounded-md border border-border-base bg-surface px-3 py-1.5 text-sm text-ink placeholder:text-ink-faint"
                />
            </label>
        </div>

        <div class="mt-4 overflow-hidden rounded-lg border border-border-base">
            <table class="w-full text-sm">
                <caption class="sr-only">Every session on this machine</caption>
                <thead class="bg-sunken">
                    <tr class="border-b border-border-base">
                        {#each columns as column (column.key)}
                            <th
                                scope="col"
                                class="px-3 py-2"
                                class:text-right={column.align === 'right'}
                                class:text-left={column.align !== 'right'}
                                aria-sort={ariaSort(column.key)}
                            >
                                <button
                                    type="button"
                                    class="eyebrow inline-flex items-center gap-1 hover:text-ink"
                                    onclick={() => sortBy(column.key)}
                                >
                                    {column.label}
                                    <span aria-hidden="true" class="text-accent">
                                        {sortKey === column.key ? (descending ? '↓' : '↑') : ''}
                                    </span>
                                </button>
                            </th>
                        {/each}
                    </tr>
                </thead>
                <tbody>
                    {#each sorted as session (session.id)}
                        <tr class="border-b border-border-base last:border-0 hover:bg-sunken">
                            <td class="px-3 py-2">
                                <a
                                    href={resolve('/session/[id]', { id: session.id })}
                                    class="block max-w-[46ch] truncate font-medium text-ink hover:text-accent hover:underline"
                                >
                                    {session.title || 'Untitled session'}
                                </a>
                                <span class="block truncate text-xs text-ink-faint">
                                    {session.projectName || 'project unknown'}
                                    <span class="text-ink-faint">·</span>
                                    <span class="font-mono">{session.id.slice(0, 8)}</span>
                                </span>
                            </td>
                            <td class="px-3 py-2 font-mono text-xs whitespace-nowrap text-ink-muted">
                                <span title={formatRelative(session.start)}>{formatInstant(session.start)}</span>
                            </td>
                            <td class="px-3 py-2 text-right font-mono whitespace-nowrap text-ink-muted">
                                {session.start ? formatDuration(session.seconds) : '–'}
                            </td>
                            <td class="px-3 py-2 text-right font-mono whitespace-nowrap text-ink-muted">
                                {session.subagents ? formatCount(session.subagents) : '–'}
                            </td>
                            <td class="px-3 py-2 text-right font-mono text-xs whitespace-nowrap text-ink-faint">
                                {formatBytes(session.bytes)}
                            </td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>

        {#if sorted.length === 0}
            <p class="mt-4 text-sm text-ink-muted">
                No session matches “{query}”. Try a project name, or the first few characters of a session id.
            </p>
        {/if}
    </section>

    <details class="rise mt-10 max-w-2xl text-sm" style:--rise-delay="120ms">
        <summary class="cursor-pointer text-ink-muted hover:text-ink">What these numbers don't say</summary>
        <ul class="mt-3 space-y-2 pl-4 text-ink-faint">
            <li class="list-disc">
                A <strong class="font-medium text-ink-muted">thinking</strong> span starts when the block before it finished
                streaming, so it holds model latency and prompt processing as well as reasoning.
            </li>
            <li class="list-disc">
                <strong class="font-medium text-ink-muted">Lane time</strong> adds every lane up, so two agents working for
                an hour side by side is two hours of lane time and one hour of elapsed time. Both numbers are on every session
                page, labelled.
            </li>
            <li class="list-disc">
                <strong class="font-medium text-ink-muted">Stalled</strong> is a heuristic: it says a result came back far
                later than the call could plausibly have taken. Each of those rows carries the command and the threshold it
                was measured against, so you can overrule it.
            </li>
            <li class="list-disc">
                Thinking text is stripped from almost every transcript, so a thinking row borrows its subject from
                whatever the agent did next and says “before” to mark the guess.
            </li>
        </ul>
    </details>
{/if}
