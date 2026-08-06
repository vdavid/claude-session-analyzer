<script lang="ts">
    /**
     * Every row the engine derived, sortable and filterable.
     *
     * Two things keep it usable at 22,000 rows: TanStack Table owns sorting and filtering, and
     * TanStack Virtual draws only the rows on screen. Row height is fixed, which is what lets the
     * virtualizer measure nothing: long text is clipped to one line and carried in full on the
     * row's `title`.
     *
     * Sorting by an instant goes through a numeric accessor. RFC 3339 strings look sortable and
     * aren't: the engine trims trailing zeros, so `…16.703Z` sorts after `…16.7Z` as text.
     */
    import {
        createTable,
        getCoreRowModel,
        getFilteredRowModel,
        getSortedRowModel,
        type ColumnDef,
        type SortingState,
    } from '@tanstack/table-core'
    import {
        Virtualizer,
        elementScroll,
        observeElementOffset,
        observeElementRect,
        type VirtualItem,
    } from '@tanstack/virtual-core'
    import { formatCount, formatDuration, formatInstantSeconds, shortId } from '$lib/format'
    import { kindStyle } from '$lib/kinds'
    import type { Lane, TimelineRow } from '$lib/types'

    interface Props {
        rows: TimelineRow[]
        lanes: Lane[]
        kinds: string[]
        /**
         * The tool group to show, bound so the breakdown above can drive it: clicking a slice up
         * there is the same act as picking one from the dropdown down here.
         */
        toolFilter?: string
    }

    let { rows, lanes, kinds, toolFilter = $bindable('') }: Props = $props()

    const ROW_HEIGHT = 30
    const VIEWPORT_HEIGHT = 560

    let search = $state('')
    let debouncedSearch = $state('')
    let kindFilter = $state('')
    let laneFilter = $state('')
    let sorting = $state<SortingState>([{ id: 'from', desc: false }])

    // A keystroke rebuilds the row model over every row, so the box waits for a pause first.
    $effect(() => {
        const next = search
        const timer = setTimeout(() => {
            debouncedSearch = next
        }, 180)
        return () => clearTimeout(timer)
    })

    const classes = $derived(
        [...new Set(rows.map((r) => r.class).filter((c): c is string => Boolean(c)))].sort((a, b) =>
            a.localeCompare(b),
        ),
    )
    let classFilter = $state('')

    /** The groups this session actually has rows for, so the dropdown can't offer an empty result. */
    const toolGroups = $derived(
        [...new Set(rows.map((r) => r.toolGroup).filter((g): g is string => Boolean(g)))].sort((a, b) =>
            a.localeCompare(b),
        ),
    )

    /** Narrowed before the table sees them: four exact-match dropdowns are cheaper as a filter. */
    const visibleRows = $derived(
        rows.filter(
            (r) =>
                (!kindFilter || r.kind === kindFilter) &&
                (!laneFilter || r.laneId === laneFilter) &&
                (!classFilter || r.class === classFilter) &&
                (!toolFilter || r.toolGroup === toolFilter),
        ),
    )

    const columns: ColumnDef<TimelineRow>[] = [
        { id: 'from', header: 'Started', accessorFn: (r) => Date.parse(r.from), size: 162 },
        { id: 'seconds', header: 'Length', accessorFn: (r) => r.seconds, size: 88 },
        { id: 'agent', header: 'Agent', accessorFn: (r) => r.agent, size: 148 },
        { id: 'kind', header: 'Activity', accessorFn: (r) => r.kind, size: 170 },
        { id: 'tool', header: 'Tool', accessorFn: (r) => r.tool ?? '', size: 132 },
        { id: 'class', header: 'Class', accessorFn: (r) => r.class ?? '', size: 96 },
        { id: 'info', header: 'Extra info', accessorFn: (r) => r.info, size: 0 },
        { id: 'line', header: 'Line', accessorFn: (r) => r.line ?? 0, size: 72 },
    ]

    /**
     * `table.getState()` hands back `options.state` verbatim, so a partial one leaves every feature
     * this sheet doesn't use (column pinning, sizing, pagination) undefined and the first header
     * read throws. The full default state only exists once the table has been built, so it's merged
     * in straight after.
     */
    const table = $derived.by(() => {
        const instance = createTable<TimelineRow>({
            data: visibleRows,
            columns,
            state: {},
            globalFilterFn: 'includesString',
            onStateChange: () => {},
            renderFallbackValue: null,
            getCoreRowModel: getCoreRowModel(),
            getSortedRowModel: getSortedRowModel(),
            getFilteredRowModel: getFilteredRowModel(),
            onSortingChange: (updater) => {
                sorting = typeof updater === 'function' ? updater(sorting) : updater
            },
            onGlobalFilterChange: (updater) => {
                debouncedSearch = typeof updater === 'function' ? updater(debouncedSearch) : updater
            },
        })
        instance.setOptions((previous) => ({
            ...previous,
            state: { ...instance.initialState, sorting, globalFilter: debouncedSearch },
        }))
        return instance
    })

    const modelRows = $derived(table.getRowModel().rows)

    let scroller = $state<HTMLDivElement | null>(null)
    let virtualItems = $state<VirtualItem[]>([])
    let totalSize = $state(0)

    $effect(() => {
        const element = scroller
        const count = modelRows.length
        if (!element) return
        const virtualizer = new Virtualizer<HTMLDivElement, Element>({
            count,
            getScrollElement: () => element,
            estimateSize: () => ROW_HEIGHT,
            overscan: 14,
            scrollToFn: elementScroll,
            observeElementOffset,
            observeElementRect,
            onChange: (instance) => {
                virtualItems = instance.getVirtualItems()
                totalSize = instance.getTotalSize()
            },
        })
        const cleanup = virtualizer._didMount()
        virtualizer._willUpdate()
        virtualItems = virtualizer.getVirtualItems()
        totalSize = virtualizer.getTotalSize()
        return cleanup
    })

    // A narrower result set has to start at the top, or the view sits past the end of it.
    $effect(() => {
        void debouncedSearch
        void kindFilter
        void laneFilter
        void classFilter
        void toolFilter
        scroller?.scrollTo({ top: 0 })
    })

    function sortIndicator(id: string): string {
        const entry = sorting.find((s) => s.id === id)
        if (!entry) return ''
        return entry.desc ? '↓' : '↑'
    }

    function clearFilters() {
        search = ''
        debouncedSearch = ''
        kindFilter = ''
        laneFilter = ''
        classFilter = ''
        toolFilter = ''
    }

    const filtered = $derived(modelRows.length !== rows.length)
    const laneName = $derived(new Map(lanes.map((l) => [l.id, l.name])))
</script>

<div class="flex flex-wrap items-end gap-3">
    <label class="flex-1 basis-56">
        <span class="eyebrow block pb-1">Search</span>
        <input
            type="search"
            bind:value={search}
            placeholder="A tool, a command, an agent, a lane id"
            class="w-full rounded-md border border-border-base bg-surface px-2.5 py-1.5 text-sm text-ink placeholder:text-ink-faint"
        />
    </label>

    <label>
        <span class="eyebrow block pb-1">Activity</span>
        <select
            bind:value={kindFilter}
            class="w-44 rounded-md border border-border-base bg-surface px-2 py-1.5 text-sm text-ink"
        >
            <option value="">Every kind</option>
            {#each kinds as kind (kind)}
                <option value={kind}>{kind}</option>
            {/each}
        </select>
    </label>

    <label>
        <span class="eyebrow block pb-1">Tool</span>
        <select
            bind:value={toolFilter}
            class="w-44 rounded-md border border-border-base bg-surface px-2 py-1.5 text-sm text-ink"
        >
            <option value="">Every tool</option>
            {#each toolGroups as group (group)}
                <option value={group}>{group}</option>
            {/each}
        </select>
    </label>

    <label>
        <span class="eyebrow block pb-1">Class</span>
        <select
            bind:value={classFilter}
            class="w-32 rounded-md border border-border-base bg-surface px-2 py-1.5 text-sm text-ink"
        >
            <option value="">Every class</option>
            {#each classes as klass (klass)}
                <option value={klass}>{klass}</option>
            {/each}
        </select>
    </label>

    <label>
        <span class="eyebrow block pb-1">Lane</span>
        <select
            bind:value={laneFilter}
            class="w-52 rounded-md border border-border-base bg-surface px-2 py-1.5 text-sm text-ink"
        >
            <option value="">Every lane</option>
            {#each lanes as lane (lane.id)}
                <option value={lane.id}>{lane.name} · {shortId(lane.id)}</option>
            {/each}
        </select>
    </label>

    <p class="ml-auto pb-1.5 text-sm text-ink-muted">
        <span class="tnum font-mono text-ink">{formatCount(modelRows.length)}</span>
        {modelRows.length === 1 ? 'row' : 'rows'}
        {#if filtered}
            <span class="text-ink-faint">of {formatCount(rows.length)}</span>
            <button type="button" class="ml-2 text-accent underline underline-offset-2" onclick={clearFilters}>
                Show them all
            </button>
        {/if}
    </p>
</div>

<!--
    The scroll container has to be reachable by keyboard, or the only way through 22,000 rows is a
    pointer (WCAG 2.1.1). Svelte's a11y rule doesn't know a scrollable region is the exception.
-->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div
    bind:this={scroller}
    class="mt-3 overflow-auto rounded-lg border border-border-base"
    style:height={`${VIEWPORT_HEIGHT}px`}
    role="region"
    aria-label="Timeline rows"
    tabindex="0"
>
    <table class="grid w-full text-sm">
        <thead class="sticky top-0 z-10 grid bg-sunken">
            <tr class="flex border-b border-border-base">
                {#each table.getHeaderGroups()[0].headers as header (header.id)}
                    <th
                        scope="col"
                        class="flex-1 px-2.5 py-2 text-left"
                        style:flex={header.column.columnDef.size ? `0 0 ${header.column.columnDef.size}px` : '1 1 0%'}
                        aria-sort={sorting.find((s) => s.id === header.id)
                            ? sorting.find((s) => s.id === header.id)?.desc
                                ? 'descending'
                                : 'ascending'
                            : 'none'}
                    >
                        <button
                            type="button"
                            class="eyebrow flex w-full items-center gap-1 hover:text-ink"
                            class:justify-end={header.id === 'seconds' || header.id === 'line'}
                            onclick={header.column.getToggleSortingHandler()}
                        >
                            {header.column.columnDef.header}
                            <span aria-hidden="true" class="text-accent">{sortIndicator(header.id)}</span>
                        </button>
                    </th>
                {/each}
            </tr>
        </thead>

        <tbody class="relative block">
            <tr aria-hidden="true" class="block" style:height={`${totalSize}px`}></tr>
            {#each virtualItems as item (item.key)}
                {@const row = modelRows[item.index]?.original}
                {#if row}
                    <tr
                        class="absolute top-0 left-0 flex w-full items-center border-b border-border-base last:border-0 hover:bg-sunken"
                        style:height={`${ROW_HEIGHT}px`}
                        style:transform={`translateY(${item.start}px)`}
                    >
                        <td
                            class="shrink-0 px-2.5 font-mono text-xs whitespace-nowrap text-ink-muted"
                            style:flex="0 0 162px"
                        >
                            {formatInstantSeconds(row.from)}
                        </td>
                        <td class="shrink-0 px-2.5 text-right font-mono text-xs text-ink" style:flex="0 0 88px">
                            {formatDuration(row.seconds)}
                        </td>
                        <td
                            class="shrink-0 truncate px-2.5 text-ink-muted"
                            style:flex="0 0 148px"
                            title={`${row.agent} · lane ${row.laneId}`}
                        >
                            {laneName.get(row.laneId) ?? row.agent}
                        </td>
                        <td class="shrink-0 truncate px-2.5" style:flex="0 0 170px">
                            <span class="flex items-center gap-1.5">
                                <span
                                    class="size-2 shrink-0 rounded-[2px]"
                                    style:background-color={`var(${kindStyle(row.kind).cssVar})`}
                                ></span>
                                <span class="truncate text-ink-muted">{row.kind}</span>
                            </span>
                        </td>
                        <td class="shrink-0 truncate px-2.5 font-mono text-xs text-ink-muted" style:flex="0 0 132px">
                            {row.tool ?? ''}
                        </td>
                        <td class="shrink-0 truncate px-2.5 text-xs text-ink-faint" style:flex="0 0 96px">
                            {row.class ?? ''}
                        </td>
                        <td class="min-w-0 flex-1 truncate px-2.5 text-ink-muted" title={row.info}>
                            {row.info}
                            {#if row.timedOut}<span class="ml-1 text-xs text-ink-faint">(hit its limit)</span>{/if}
                            {#if row.overlapped}<span class="ml-1 text-xs text-ink-faint">(ran in parallel)</span>{/if}
                        </td>
                        <td class="shrink-0 px-2.5 text-right font-mono text-xs text-ink-faint" style:flex="0 0 72px">
                            {row.line ?? ''}
                        </td>
                    </tr>
                {/if}
            {/each}
        </tbody>
    </table>
</div>

{#if modelRows.length === 0}
    <p class="mt-3 text-sm text-ink-muted">
        Nothing matches those filters. <button
            type="button"
            class="text-accent underline underline-offset-2"
            onclick={clearFilters}>Show every row</button
        >.
    </p>
{/if}
