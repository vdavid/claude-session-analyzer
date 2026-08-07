<script lang="ts">
    /**
     * One session, analysed on load.
     *
     * Two fetches, in order. The aggregates (`?rows=false`) are 364 KB even on the largest session,
     * so the charts are on screen before the rows are asked for; the sheet's own copy is megabytes
     * and arrives behind them. Asking for both at once would have the server parsing the same
     * transcript twice at the same time, which is exactly the wrong way round.
     */
    import { page } from '$app/state'
    import { SvelteSet } from 'svelte/reactivity'
    import { describeApiError, fetchAggregates, fetchTimelineRows } from '$lib/api'
    import BandBar from '$lib/components/BandBar.svelte'
    import DataSheet from '$lib/components/DataSheet.svelte'
    import KindLegend from '$lib/components/KindLegend.svelte'
    import Notice from '$lib/components/Notice.svelte'
    import StatRail from '$lib/components/StatRail.svelte'
    import TimeLadder from '$lib/components/TimeLadder.svelte'
    import ToolClockBars from '$lib/components/ToolClockBars.svelte'
    import ToolLegend from '$lib/components/ToolLegend.svelte'
    import ConcurrencyTrace from '$lib/components/charts/ConcurrencyTrace.svelte'
    import KindPie from '$lib/components/charts/KindPie.svelte'
    import Swimlane from '$lib/components/charts/Swimlane.svelte'
    import ToolPie from '$lib/components/charts/ToolPie.svelte'
    import { categoryVar } from '$lib/categories'
    import {
        formatBytes,
        formatCount,
        formatDuration,
        formatInstant,
        formatShare,
        instantMs,
        shortId,
    } from '$lib/format'
    import { theme } from '$lib/theme.svelte'
    import { concurrencyTrace } from '$lib/transform/concurrency'
    import { timeLadder } from '$lib/transform/ladder'
    import { bandTotals, kindSlices } from '$lib/transform/pie'
    import { buildSwimlane } from '$lib/transform/swimlane'
    import { toolBreakdown, toolClockBars } from '$lib/transform/tools'
    import type { TimelineResponse } from '$lib/types'

    let aggregates = $state<TimelineResponse | null>(null)
    let full = $state<TimelineResponse | null>(null)
    let failure = $state<unknown>(null)
    let rowsFailure = $state<unknown>(null)
    let rowsLoading = $state(false)
    const expanded = new SvelteSet<string>()

    const aborted = (cause: unknown) => cause instanceof DOMException && cause.name === 'AbortError'

    $effect(() => {
        const id = page.params.id
        if (!id) return
        const controller = new AbortController()
        let cancelled = false

        aggregates = null
        full = null
        failure = null
        rowsFailure = null
        rowsLoading = false
        expanded.clear()

        void (async () => {
            try {
                const summary = await fetchAggregates(id, controller.signal)
                if (cancelled) return
                aggregates = summary
            } catch (cause) {
                if (!cancelled && !aborted(cause)) failure = cause
                return
            }

            rowsLoading = true
            try {
                const everything = await fetchTimelineRows(id, controller.signal)
                if (!cancelled) full = everything
            } catch (cause) {
                if (!cancelled && !aborted(cause)) rowsFailure = cause
            } finally {
                if (!cancelled) rowsLoading = false
            }
        })()

        return () => {
            cancelled = true
            controller.abort()
        }
    })

    const session = $derived(aggregates?.session)
    const totals = $derived(aggregates?.totals)
    const lanes = $derived(aggregates?.lanes ?? [])

    const span = $derived.by(() => {
        const from = instantMs(totals?.from)
        const until = instantMs(totals?.until)
        return from !== null && until !== null && until > from ? { from, until } : null
    })

    const slices = $derived(kindSlices(totals?.byKind ?? []))
    const bands = $derived(bandTotals(totals?.byKind ?? []))
    const swimlaneRows = $derived(buildSwimlane(lanes, { expanded }))
    const trace = $derived(span ? concurrencyTrace(lanes, 420, span) : null)

    const workflows = $derived.by(() => {
        const counts: Record<string, number> = {}
        for (const lane of lanes) {
            if (!lane.workflowId) continue
            counts[lane.workflowId] = (counts[lane.workflowId] ?? 0) + 1
        }
        return Object.entries(counts).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    })

    const hiddenLanes = $derived(swimlaneRows.reduce((sum, row) => sum + row.hiddenLanes, 0))
    /** A session whose records carry no timestamp derives no rows, so there's nothing to place on a clock. */
    const derivable = $derived(Boolean(totals?.rows))
    const kinds = $derived(slices.map((s) => s.kind))
    let highlight = $state<number | null>(null)

    const ladder = $derived(
        timeLadder(totals ?? { wallClockSeconds: 0, laneTimeSeconds: 0, netSeconds: 0, activeSeconds: 0 }),
    )

    const tools = $derived(toolBreakdown(totals?.byTool ?? [], aggregates?.toolCategories ?? []))
    const clocks = $derived(toolClockBars(tools.slices))
    /** What each category holds, as the API described it, for the strip's tooltips. */
    const descriptions = $derived(new Map((aggregates?.toolCategories ?? []).map((c) => [c.category, c.description])))
    let toolHighlight = $state<number | null>(null)
    /** The group the sheet is filtered to. Set by a click on a slice or a legend row. */
    let toolFilter = $state('')
    let sheet = $state<HTMLElement | null>(null)

    function toggleWorkflow(id: string) {
        if (expanded.has(id)) expanded.delete(id)
        else expanded.add(id)
    }

    /**
     * Filtering the sheet from up here only helps if the sheet is where the reader ends up, and it's
     * two screens down. Clicking the group that's already showing clears it, so the same click undoes
     * itself.
     */
    function showTool(group: string) {
        toolFilter = toolFilter === group ? '' : group
        if (toolFilter) {
            sheet?.scrollIntoView({ behavior: theme.reducedMotion ? 'auto' : 'smooth', block: 'start' })
        }
    }

    const stats = $derived(
        totals
            ? [
                  {
                      label: 'Elapsed',
                      value: formatDuration(totals.wallClockSeconds),
                      note: 'First record to last, across every lane',
                      title: `${formatInstant(totals.from)} to ${formatInstant(totals.until)}`,
                  },
                  {
                      label: 'Lane time',
                      value: formatDuration(totals.laneTimeSeconds),
                      note: 'Every lane added up, so parallel work counts once each',
                  },
                  {
                      label: 'Net agent time',
                      value: formatDuration(totals.netSeconds),
                      note: 'Lane time minus waiting on a person or a teammate',
                      title: `Lane time minus the two waits whose clock belongs to somebody else. A wait on a teammate is already that teammate's own lane time, and a wait on a person was never agent time.`,
                  },
                  {
                      label: 'Lanes',
                      value: formatCount(totals.lanes),
                      note: totals.lanes
                          ? `The lead and ${formatCount(totals.lanes - 1)} subagents`
                          : 'Nothing here carries a timestamp',
                  },
                  {
                      label: 'Rows',
                      value: formatCount(totals.rows),
                      note: "One per stretch of one lane's clock",
                  },
              ]
            : [],
    )
</script>

<svelte:head>
    <title>{session?.title || 'Session'} · Claude session analyzer</title>
</svelte:head>

{#if failure}
    {@const described = describeApiError(failure)}
    <div class="max-w-2xl">
        <Notice headline={described.headline} detail={described.detail} tone="trouble" />
    </div>
{:else if !aggregates || !totals || !session}
    <p class="text-sm text-ink-faint">Reading the transcript and deriving the timeline…</p>
{:else}
    <header class="rise max-w-4xl">
        <p class="eyebrow">
            {session.projectName || 'Project unknown'}
            <span class="text-ink-faint"> · </span>
            <span class="normal-case">{shortId(session.id)}</span>
        </p>
        <h1 class="mt-2 text-3xl leading-tight font-semibold tracking-tight text-balance text-ink sm:text-4xl">
            {session.title || 'Untitled session'}
        </h1>
        <p class="mt-2.5 text-sm text-ink-faint">
            {#if session.projectPath}<code class="text-ink-muted">{session.projectPath}</code>{/if}
            {#if session.projectPath}<span class="px-1.5">·</span>{/if}
            {#if totals.from && totals.until}
                {formatInstant(totals.from)} to {formatInstant(totals.until)}
                <span class="px-1.5">·</span>
            {/if}
            {formatBytes(session.bytes)} of transcript
        </p>
    </header>

    {#if trace && trace.points.length}
        <section class="rise mt-7" style:--rise-delay="60ms" aria-labelledby="trace-heading">
            <div class="flex items-baseline justify-between gap-4">
                <h2 id="trace-heading" class="eyebrow">Agents producing, over the session</h2>
                <p class="text-xs text-ink-faint">
                    Peak <span class="tnum font-mono text-ink-muted">{Math.round(trace.peak)}</span> at once
                </p>
            </div>
            <div class="mt-1.5 border-b border-border-base">
                <ConcurrencyTrace {trace} />
            </div>
            <div class="mt-1.5 flex justify-between font-mono text-[11px] text-ink-faint">
                <span>{formatInstant(totals.from)}</span>
                <span>{formatInstant(totals.until)}</span>
            </div>
        </section>
    {/if}

    <section class="rise mt-8" style:--rise-delay="100ms">
        <StatRail {stats} />
    </section>

    {#if !derivable}
        <section class="rise mt-8 max-w-2xl" style:--rise-delay="140ms">
            <Notice
                headline="There's no timeline to draw here"
                detail="Not one record in this transcript carries a timestamp, so there's nothing to place on a clock. That's usual for a session that ended within its first few lines."
            />
        </section>
    {:else}
        <section class="rise mt-8" style:--rise-delay="140ms" aria-labelledby="split-heading">
            <h2 id="split-heading" class="text-xl font-semibold tracking-tight text-ink">Where lane time went</h2>
            <p class="mt-1.5 max-w-3xl text-sm leading-relaxed text-ink-muted">
                A breakdown of <strong class="font-medium text-ink">lane time</strong>, {formatDuration(
                    totals.laneTimeSeconds,
                )}, not of the {formatDuration(totals.wallClockSeconds)} the session took. Lanes running side by side each
                count their own time, which is why the two differ. Take the waiting out of it and
                <strong class="font-medium text-ink">net agent time</strong>, {formatDuration(totals.netSeconds)}, is
                what the session actually cost.
            </p>

            <div class="card mt-4 p-5">
                <BandBar {bands} />

                <div class="mt-6 border-t border-border-base pt-5">
                    <h3 class="eyebrow">Lane time, net agent time, active time</h3>
                    <p class="mt-1.5 mb-4 max-w-3xl text-xs leading-relaxed text-ink-faint">
                        Three durations over the same rows, each one the rung above minus something named. They aren't
                        rivals: pick the one that answers your question, and the subtraction is what stops one being
                        read as another.
                    </p>
                    <TimeLadder {ladder} />
                </div>

                <div
                    class="mt-6 grid gap-6 border-t border-border-base pt-5 lg:grid-cols-[minmax(0,340px)_minmax(0,1fr)] lg:gap-8"
                >
                    <div class="lg:sticky lg:top-20 lg:self-start">
                        <KindPie {slices} {highlight} height="300px" />
                    </div>
                    <KindLegend {slices} total={bands.total} onHover={(index) => (highlight = index)} />
                </div>
            </div>
        </section>

        <section class="rise mt-10" style:--rise-delay="180ms" aria-labelledby="lanes-heading">
            <h2 id="lanes-heading" class="text-xl font-semibold tracking-tight text-ink">Who was alive, and when</h2>
            <p class="mt-1.5 max-w-3xl text-sm leading-relaxed text-ink-muted">
                A thick bar is a stretch the lane produced something. A thin one is a stretch it didn't, coloured by
                what it was waiting on. Drag to pan, ctrl and scroll to zoom, or use the slider underneath.
            </p>

            {#if workflows.length}
                <div class="mt-4 flex flex-wrap items-center gap-2">
                    <span class="eyebrow">
                        {formatCount(workflows.length)}
                        {workflows.length === 1 ? 'workflow' : 'workflows'}, one row each
                    </span>
                    {#each workflows as [id, count] (id)}
                        <button
                            type="button"
                            onclick={() => toggleWorkflow(id)}
                            aria-pressed={expanded.has(id)}
                            class="rounded-full border px-2.5 py-1 font-mono text-xs transition-colors"
                            class:border-accent={expanded.has(id)}
                            class:text-accent={expanded.has(id)}
                            class:bg-accent-soft={expanded.has(id)}
                            class:border-border-base={!expanded.has(id)}
                            class:text-ink-muted={!expanded.has(id)}
                        >
                            {expanded.has(id) ? '▾' : '▸'}
                            {id}
                            <span class="text-ink-faint">· {formatCount(count)}</span>
                        </button>
                    {/each}
                </div>
                <p class="mt-2 text-xs text-ink-faint">
                    A workflow's row is the union of its lanes, so it's filled wherever at least one of them was
                    producing. Open one to see its lanes; the chart draws the first 150 of them.
                </p>
            {/if}

            {#if span && swimlaneRows.length}
                <div class="card mt-4 py-3">
                    <Swimlane
                        rows={swimlaneRows}
                        from={span.from}
                        until={span.until}
                        onToggleWorkflow={toggleWorkflow}
                    />
                </div>
                <p class="mt-2 text-xs text-ink-faint">
                    {formatCount(swimlaneRows.length)}
                    {swimlaneRows.length === 1 ? 'row' : 'rows'} drawn, standing for {formatCount(totals.lanes)}
                    {totals.lanes === 1 ? 'lane' : 'lanes'}.
                    {#if hiddenLanes}
                        {formatCount(hiddenLanes)} lanes of the workflows you opened aren't drawn.
                    {/if}
                </p>
            {:else}
                <div class="mt-4 max-w-2xl">
                    <Notice
                        headline="No lane here carries a readable span"
                        detail="Every record in this session is missing its timestamp, so no lane has a span to draw."
                    />
                </div>
            {/if}
        </section>

        {#if tools.slices.length}
            <section class="rise mt-10" style:--rise-delay="200ms" aria-labelledby="tools-heading">
                <h2 id="tools-heading" class="text-xl font-semibold tracking-tight text-ink">Tools</h2>
                <p class="mt-1.5 max-w-3xl text-sm leading-relaxed text-ink-muted">
                    {formatCount(tools.calls)} tool
                    {tools.calls === 1 ? 'call' : 'calls'} across every lane.
                    {#if tools.busiest}
                        The busiest is <strong class="font-medium text-ink">{tools.busiest.group}</strong>, at
                        {formatCount(tools.busiest.calls)}
                        {tools.busiest.calls === 1 ? 'call' : 'calls'} ({formatShare(
                            tools.busiest.calls,
                            tools.calls,
                        )}).
                    {/if}
                    Every call leaves two stretches of a lane's clock, and the page keeps them apart: the agent
                    <strong class="font-medium text-ink">composing</strong> the call, and the tool
                    <strong class="font-medium text-ink">running</strong> it. A third,
                    <strong class="font-medium text-ink">stalled</strong>, turns up when a result came back far too late
                    to have been running. So how often a tool was reached for and how much time went through it are two
                    questions, and they get a chart each. Pick a tool anywhere here to filter the sheet below to its
                    rows.
                </p>

                <div class="card mt-4 p-5">
                    <h3 class="eyebrow">Which tools, by call count</h3>
                    <p class="mt-1.5 max-w-3xl text-xs leading-relaxed text-ink-faint">
                        Counted as calls rather than as time: a checker run costs minutes and a lookup costs a second,
                        so seconds here would say what the machine was busy with rather than what the agents reached
                        for. Colour is the category, and a category is one contiguous arc.
                    </p>

                    <ul class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5">
                        {#each tools.categories as category (category.category)}
                            {@const described = descriptions.get(category.category)}
                            <li class="flex items-center gap-1.5 text-xs" title={described}>
                                <span
                                    class="size-2.5 shrink-0 rounded-[3px]"
                                    style:background-color={`var(${categoryVar(category.category)})`}
                                ></span>
                                <span class="text-ink-muted">{category.label}</span>
                                <span class="tnum font-mono text-ink-faint">
                                    {formatShare(category.calls, tools.calls)}
                                </span>
                            </li>
                        {/each}
                    </ul>

                    <div class="mt-4 grid gap-6 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)] lg:gap-8">
                        <div class="lg:sticky lg:top-20 lg:self-start">
                            <ToolPie
                                slices={tools.slices}
                                calls={tools.calls}
                                highlight={toolHighlight}
                                onSelect={showTool}
                            />
                        </div>

                        <div>
                            <h3 class="eyebrow">Where each tool's time went</h3>
                            <p class="mt-1.5 mb-3 text-xs leading-relaxed text-ink-faint">
                                The split inverts per tool, and that's the finding: the model streams a whole diff as an
                                <code class="text-ink-muted">Edit</code>'s arguments, so the writing is the cost and the
                                write itself is instant, while a checker is the other way round. Bars are ranked by the
                                time that arrived under each tool's name.
                            </p>
                            <ToolClockBars
                                chart={clocks}
                                selected={toolFilter}
                                onHover={(index) => (toolHighlight = index)}
                                onSelect={showTool}
                            />
                        </div>
                    </div>

                    <div class="mt-6 border-t border-border-base pt-5">
                        <h3 class="eyebrow">Every tool, with all three clocks</h3>
                        <p class="mt-1.5 mb-3 max-w-3xl text-xs leading-relaxed text-ink-faint">
                            Open a group to see the exact tools inside it: an MCP server's methods, or the programs a
                            <code class="text-ink-muted">Bash</code> group ran. The lane count is who reached for it.
                        </p>
                        <div class="overflow-x-auto">
                            <ToolLegend
                                slices={tools.slices}
                                calls={tools.calls}
                                categories={tools.categories}
                                selected={toolFilter}
                                onHover={(index) => (toolHighlight = index)}
                                onSelect={showTool}
                            />
                        </div>
                    </div>
                </div>
            </section>
        {/if}

        <section class="rise mt-10" style:--rise-delay="220ms" aria-labelledby="sheet-heading" bind:this={sheet}>
            <h2 id="sheet-heading" class="text-xl font-semibold tracking-tight text-ink">Every row</h2>
            <p class="mt-1.5 max-w-3xl text-sm leading-relaxed text-ink-muted">
                One row per stretch of one lane's clock, tiling that lane end to end. The lead's row is the same shape
                as a subagent's, so sorting by length across the whole session tells you what actually took the time.
            </p>

            <div class="mt-4">
                {#if full?.rows}
                    <DataSheet rows={full.rows} {lanes} {kinds} bind:toolFilter />
                {:else if rowsFailure}
                    {@const described = describeApiError(rowsFailure)}
                    <div class="max-w-2xl">
                        <Notice headline={described.headline} detail={described.detail} tone="trouble" />
                    </div>
                {:else if rowsLoading}
                    <p class="text-sm text-ink-faint">
                        Fetching {formatCount(totals.rows)} rows. The charts above already have everything they need.
                    </p>
                {/if}
            </div>
        </section>

        <p class="mt-12 max-w-3xl text-xs leading-relaxed text-ink-faint">
            Waiting is {formatShare(bands.wait.seconds, bands.total)} of this session's lane time. Thinking spans hold model
            latency as well as reasoning, and a stall is a heuristic each row shows its own working for.
        </p>
    {/if}
{/if}
