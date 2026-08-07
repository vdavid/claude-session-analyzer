<script lang="ts">
    /**
     * The tool breakdown as a table, and the legend for both charts above it.
     *
     * This is where the question actually gets answered. The charts show the shape; the table carries
     * the counts, all three clocks, and how many lanes reached for each tool, which is what "who used
     * codegraph" comes down to. Opening a group shows the exact tools inside it: an MCP server's
     * methods, or the programs a `Bash (git)` group ran.
     *
     * **The three clocks get three columns and no total column.** They're three different measurements
     * that happen to arrive under one name, so a "time" column holding their sum would report a tool as
     * costing what the agent and a suspension cost. Stalled is left blank rather than written as `0s`
     * wherever nothing stalled, so the column reads as the exception it is.
     *
     * A row is a button rather than a link: clicking one filters the sheet below to that group's
     * rows, and hovering lights its slice.
     */
    import { categoryVar } from '$lib/categories'
    import { formatCount, formatDuration, formatShare } from '$lib/format'
    import type { CategorySlice, ToolSlice } from '$lib/transform/tools'

    interface Props {
        slices: ToolSlice[]
        calls: number
        /** The categories present, so a row can name and describe the bucket it sits in. */
        categories: CategorySlice[]
        /** The group the sheet below is filtered to, so the table can show which row is doing it. */
        selected?: string
        onHover?: (index: number | null) => void
        onSelect?: (group: string) => void
    }

    const { slices, calls, categories, selected = '', onHover, onSelect }: Props = $props()

    let opened = $state<string>('')

    const labels = $derived(new Map(categories.map((c) => [c.category, c.label])))

    function toggle(group: string) {
        opened = opened === group ? '' : group
    }
</script>

<table class="w-full min-w-[42rem] text-sm">
    <caption class="sr-only">
        Every tool group: calls, share of the session's calls, the three clocks its time went to, and how many lanes
        reached for it
    </caption>
    <thead>
        <tr class="border-b border-border-base text-left">
            <th scope="col" class="eyebrow pb-1.5 font-normal">Tool</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Calls</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Share</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal" title="The agent writing the calls">
                Composing
            </th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal" title="The tool, from the call to its result">
                Running
            </th>
            <th
                scope="col"
                class="eyebrow pb-1.5 text-right font-normal"
                title="Calls back far too late to have been running: a suspended agent, not a slow tool"
            >
                Stalled
            </th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Lanes</th>
        </tr>
    </thead>
    <tbody>
        {#each slices as slice, i (slice.group)}
            {@const label = labels.get(slice.category) ?? slice.category}
            {@const isOpen = opened === slice.group}
            <tr
                class="border-b border-border-base last:border-0 hover:bg-sunken"
                class:bg-accent-soft={selected === slice.group}
                onmouseenter={() => onHover?.(i)}
                onmouseleave={() => onHover?.(null)}
            >
                <th scope="row" class="py-1.5 pr-3 text-left font-normal">
                    <span class="flex items-center gap-2">
                        <span
                            class="size-2.5 shrink-0 rounded-[3px]"
                            style:background-color={`var(${categoryVar(slice.category)})`}
                        ></span>
                        <button
                            type="button"
                            class="truncate text-left text-ink hover:text-accent hover:underline hover:underline-offset-2"
                            title={`Show only this group's rows. ${label}, ${slice.className}.`}
                            onclick={() => onSelect?.(slice.group)}
                        >
                            {slice.group}
                        </button>
                        {#if slice.tools.length > 1}
                            <button
                                type="button"
                                class="shrink-0 font-mono text-xs text-ink-faint hover:text-accent"
                                aria-expanded={isOpen}
                                onclick={() => toggle(slice.group)}
                            >
                                {isOpen ? '▾' : '▸'}
                                {formatCount(slice.tools.length)}
                            </button>
                        {/if}
                        {#if slice.errors}
                            <span class="shrink-0 text-xs text-ink-faint" title="Calls the tool reported as failures">
                                {formatCount(slice.errors)} failed
                            </span>
                        {/if}
                    </span>
                </th>
                <td class="py-1.5 text-right font-mono text-ink-muted">{formatCount(slice.calls)}</td>
                <td class="py-1.5 pl-3 text-right font-mono text-ink-muted">{formatShare(slice.calls, calls)}</td>
                <td class="py-1.5 pl-3 text-right font-mono text-ink-muted">
                    {formatDuration(slice.composingSeconds)}
                </td>
                <td class="py-1.5 pl-3 text-right font-mono text-ink-muted">{formatDuration(slice.runningSeconds)}</td>
                <td class="py-1.5 pl-3 text-right font-mono" class:text-ink={slice.stalledSeconds > 0}>
                    {#if slice.stalledSeconds > 0}
                        {formatDuration(slice.stalledSeconds)}
                    {:else}
                        <span class="text-ink-faint" aria-label="nothing stalled">–</span>
                    {/if}
                </td>
                <td class="py-1.5 pl-3 text-right font-mono text-ink-faint">{formatCount(slice.lanes)}</td>
            </tr>

            {#if isOpen}
                {#each slice.tools as tool (tool.tool + tool.leaf)}
                    <tr class="border-b border-border-base last:border-0 bg-sunken/60">
                        <th scope="row" class="py-1 pr-3 pl-[26px] text-left font-normal">
                            <span class="truncate font-mono text-xs text-ink-muted" title={tool.tool}>
                                {tool.leaf}
                            </span>
                        </th>
                        <td class="py-1 text-right font-mono text-xs text-ink-muted">{formatCount(tool.calls)}</td>
                        <td
                            class="py-1 pl-3 text-right font-mono text-xs text-ink-faint"
                            title={`Of ${slice.group}'s calls. The group's own share is of the whole session.`}
                        >
                            {formatShare(tool.calls, slice.calls)}
                        </td>
                        <td class="py-1 pl-3 text-right font-mono text-xs text-ink-muted">
                            {formatDuration(tool.composingSeconds)}
                        </td>
                        <td class="py-1 pl-3 text-right font-mono text-xs text-ink-muted">
                            {formatDuration(tool.seconds)}
                        </td>
                        <td class="py-1 pl-3 text-right font-mono text-xs" class:text-ink={tool.stalledSeconds}>
                            {#if tool.stalledSeconds}
                                {formatDuration(tool.stalledSeconds)}
                            {:else}
                                <span class="text-ink-faint" aria-label="nothing stalled">–</span>
                            {/if}
                        </td>
                        <td class="py-1 pl-3 text-right font-mono text-xs text-ink-faint">
                            {formatCount(tool.lanes)}
                        </td>
                    </tr>
                {/each}
            {/if}
        {/each}
    </tbody>
</table>
