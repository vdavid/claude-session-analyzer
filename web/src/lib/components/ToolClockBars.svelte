<script lang="ts">
    /**
     * One horizontal bar per tool, split into the three clocks a call's time goes to.
     *
     * The chart exists because the split **inverts** per tool, and the inversion is the finding: `Edit`
     * is 1,032 calls, 2h14m of an agent writing diffs and 1m50s of writing them to disk, while
     * `Bash (checker)` is 344 calls, 24m composing against 7h51m running. Three pies couldn't say that:
     * pie-to-pie is the weakest comparison there is, and the question is inherently comparative.
     *
     * **The three clocks are not addable, and nothing here adds them for a reader.** A bar's length is
     * every second the grouping rule filed under the tool's name, which is what a ranking can honestly
     * sort on; the number printed beside a bar is one clock, named. A single figure over all three
     * would report a tool as costing what the agent and a suspension cost.
     *
     * Encoding: hue is the tool's category (the same vocabulary the pie beside it uses, so an arc and a
     * bar are the same tool), treatment is the clock, and position is when it happened. No fourth
     * palette. `docs/frontend.md` § Three clocks, three treatments, not three hues.
     *
     * Drawn in HTML rather than on canvas, the way `BandBar.svelte` is: 28 rows of divs cost nothing,
     * the labels stay real text, and the colours come straight from the stylesheet in both themes.
     */
    import { categoryVar } from '$lib/categories'
    import { formatCount, formatDuration } from '$lib/format'
    import type { ClockName, ToolClockChart } from '$lib/transform/tools'

    interface Props {
        chart: ToolClockChart
        /** The group the sheet below is filtered to, so the chart can show which row is doing it. */
        selected?: string
        /** The pie's slice index, so hovering a bar lights the same tool's arc. */
        onHover?: (sliceIndex: number | null) => void
        onSelect?: (group: string) => void
    }

    const { chart, selected = '', onHover, onSelect }: Props = $props()

    /** What each clock is called and what it does and doesn't include. The legend prints all three. */
    const CLOCKS: Record<ClockName, { label: string; note: string }> = {
        composing: {
            label: 'Composing',
            note: "the agent writing the call. Its own clock, not the tool's",
        },
        running: {
            label: 'Running',
            note: 'the tool, from the call to its result. It counts whatever the tool waited on, a permission prompt included',
        },
        stalled: {
            label: 'Stalled',
            note: 'a result back far too late to have been running, so a suspended agent rather than a slow tool',
        },
    }

    /** One template for the rows and the axis under them, so a tick lands where a bar ends. */
    const COLUMNS = 'minmax(4.5rem, 11rem) minmax(0, 1fr) 6.75rem'

    /** The interior marks. Zero is the baseline and the last one is the axis end, so neither is drawn. */
    const interiorTicks = $derived(chart.ticks.slice(1, -1))

    /**
     * A mark's own label. Every step is a whole number of seconds, so the tenth `formatDuration` prints
     * between 10 s and a minute is noise on an axis: it puts `5s` beside `10.0s` and the pair reads as
     * two different grains of the same scale.
     */
    function tickLabel(seconds: number): string {
        if (seconds === 0) return '0'
        return seconds < 60 ? `${seconds}s` : formatDuration(seconds)
    }

    function spoken(row: ToolClockChart['rows'][number]): string {
        const clocks = [
            `${formatDuration(row.composingSeconds)} composing`,
            `${formatDuration(row.runningSeconds)} running`,
            ...(row.stalledSeconds > 0 ? [`${formatDuration(row.stalledSeconds)} stalled`] : []),
        ].join(', ')
        const lanes = `${formatCount(row.lanes)} ${row.lanes === 1 ? 'lane' : 'lanes'}`
        return `${row.group}: ${clocks}. ${formatCount(row.calls)} ${row.calls === 1 ? 'call' : 'calls'} from ${lanes}. Show only this group's rows.`
    }
</script>

<div>
    <ul class="flex flex-wrap gap-x-5 gap-y-1.5">
        {#each ['composing', 'running', 'stalled'] as const as clock (clock)}
            <li class="flex items-baseline gap-1.5 text-xs">
                <!-- The swatches wear ink rather than a category hue: they stand for the treatment, and
                     the hue is legended by the categories above the pie. -->
                <span
                    class="clock-fill mt-[3px] size-2.5 shrink-0 rounded-[2px]"
                    class:clock-fill-composing={clock === 'composing'}
                    class:clock-fill-stalled={clock === 'stalled'}
                ></span>
                <span class="text-ink">{CLOCKS[clock].label}</span>
                <span class="text-ink-faint">{CLOCKS[clock].note}</span>
            </li>
        {/each}
    </ul>

    <p class="mt-2 text-xs leading-relaxed text-ink-faint">
        Three separate clocks, never one total. A bar's length is every second that arrived under the tool's name, which
        is what the ranking sorts on; adding the three together would report a tool as costing what the agent and a
        suspension cost. The number beside each bar is the clock holding most of that row.
    </p>

    <!-- The three clocks ride on each row's `title`, which becomes its accessible description, rather than
         on an `aria-label`. An `aria-label` would replace the row's visible text with a sentence that
         doesn't start with it, which is the "label in name" failure: someone driving the page by voice says
         what they can see, so the name has to contain it. Name is the tool plus its printed clock; the other
         two clocks, the calls, and the lanes are the description, and the table below carries all of it. -->
    <ul class="mt-4">
        {#each chart.rows as row (row.group)}
            <li>
                <button
                    type="button"
                    class="grid w-full items-center gap-x-3 rounded-[4px] py-[3px] text-left hover:bg-sunken"
                    class:bg-accent-soft={selected === row.group}
                    style:grid-template-columns={COLUMNS}
                    style:--clock-hue={`var(${categoryVar(row.category)})`}
                    title={spoken(row)}
                    onmouseenter={() => onHover?.(row.sliceIndex)}
                    onmouseleave={() => onHover?.(null)}
                    onfocus={() => onHover?.(row.sliceIndex)}
                    onblur={() => onHover?.(null)}
                    onclick={() => onSelect?.(row.group)}
                >
                    <span class="flex min-w-0 items-center gap-2">
                        <span class="clock-fill size-2.5 shrink-0 rounded-[3px]"></span>
                        <span class="truncate text-sm text-ink">{row.group}</span>
                    </span>

                    <span class="relative block h-2.5" aria-hidden="true">
                        {#each interiorTicks as tick (tick)}
                            <span
                                class="absolute inset-y-0 w-px bg-border-base"
                                style:left={`${(tick / chart.bound) * 100}%`}
                            ></span>
                        {/each}
                        <!-- The segments sit over the marks, and a 2px surface gap between them does the
                             separating: a stroke around a mark would add ink that isn't data.

                             The gap is real space rather than a 2px border inside each segment, because a
                             border wide enough to see swallows a segment narrower than itself: 1m49s of
                             `SendMessage` composing is about a pixel on this axis, and it drew as nothing
                             at all. A bar with three segments now reads up to 4px long instead, which is
                             a rounding error next to losing a value. -->
                        <span class="relative flex h-full gap-x-[2px]">
                            {#each row.segments as segment (segment.clock)}
                                <span
                                    class="clock-fill h-full last:rounded-r-[4px]"
                                    class:clock-fill-composing={segment.clock === 'composing'}
                                    class:clock-fill-stalled={segment.clock === 'stalled'}
                                    style:width={`${segment.widthShare * 100}%`}
                                ></span>
                            {/each}
                        </span>
                    </span>

                    <span class="text-[11px] leading-tight">
                        {#each row.labelled as clock (clock)}
                            <span class="block truncate">
                                <span class="tnum font-mono text-ink">
                                    {formatDuration(
                                        clock === 'composing'
                                            ? row.composingSeconds
                                            : clock === 'running'
                                              ? row.runningSeconds
                                              : row.stalledSeconds,
                                    )}
                                </span>
                                <span class={clock === 'stalled' ? 'text-ink-muted' : 'text-ink-faint'}>{clock}</span>
                            </span>
                        {/each}
                    </span>
                </button>
            </li>
        {/each}
    </ul>

    <div class="mt-1.5 grid gap-x-3" style:grid-template-columns={COLUMNS}>
        <span></span>
        <span class="relative block h-4">
            {#each chart.ticks as tick, index (tick)}
                <span
                    class="tnum absolute top-0 font-mono text-[10px] text-ink-faint"
                    style:left={`${(tick / (chart.bound || 1)) * 100}%`}
                    style:transform={index === 0
                        ? 'none'
                        : index === chart.ticks.length - 1
                          ? 'translateX(-100%)'
                          : 'translateX(-50%)'}
                >
                    {tickLabel(tick)}
                </span>
            {/each}
        </span>
        <span></span>
    </div>

    {#each chart.stalled as row (row.group)}
        <p class="mt-2.5 text-xs leading-relaxed text-ink-muted">
            <strong class="font-medium text-ink">{row.group}</strong>
            carries {formatDuration(row.stalledSeconds)} of stalled time beside {formatDuration(row.runningSeconds)} of running
            time. That's one suspended agent rather than a slow tool, which is why the two are drawn apart: read together
            they'd make the tool look pathological.
        </p>
    {/each}
</div>
