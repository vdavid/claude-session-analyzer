<script lang="ts">
    /**
     * The pie's legend, as a table rather than as chart furniture.
     *
     * A legend that carries the duration, the share, and the row count answers most questions
     * without touching the chart, and it gives each kind room for the sentence that keeps it
     * honest: a `thinking` slice holds model latency, and a `stalled` one is a heuristic.
     *
     * Hovering a row lights up its slice, which is the whole reason the two sit side by side.
     */
    import { formatCount, formatDuration, formatShare } from '$lib/format'
    import { kindStyle } from '$lib/kinds'
    import type { KindSlice } from '$lib/transform/pie'

    interface Props {
        slices: KindSlice[]
        total: number
        onHover?: (index: number | null) => void
    }

    const { slices, total, onHover }: Props = $props()
</script>

<table class="w-full text-sm">
    <caption class="sr-only">Lane time by activity kind</caption>
    <thead>
        <tr class="border-b border-border-base text-left">
            <th scope="col" class="eyebrow pb-1.5 font-normal">Activity</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Lane time</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Share</th>
            <th scope="col" class="eyebrow pb-1.5 text-right font-normal">Rows</th>
        </tr>
    </thead>
    <tbody>
        {#each slices as slice, i (slice.kind)}
            <tr
                class="group border-b border-border-base last:border-0 hover:bg-sunken"
                onmouseenter={() => onHover?.(i)}
                onmouseleave={() => onHover?.(null)}
            >
                <th scope="row" class="py-1.5 pr-3 text-left font-normal">
                    <span class="flex items-start gap-2">
                        <span
                            class="mt-[5px] size-2.5 shrink-0 rounded-[3px]"
                            style:background-color={`var(${kindStyle(slice.kind).cssVar})`}
                        ></span>
                        <span>
                            <span class="text-ink">{slice.kind}</span>
                            <span class="block text-xs leading-snug text-ink-faint">
                                {kindStyle(slice.kind).description}
                            </span>
                        </span>
                    </span>
                </th>
                <td class="py-1.5 text-right align-top font-mono text-ink-muted">
                    {formatDuration(slice.seconds)}
                </td>
                <td class="py-1.5 pl-3 text-right align-top font-mono text-ink-muted">
                    {formatShare(slice.seconds, total)}
                </td>
                <td class="py-1.5 pl-3 text-right align-top font-mono text-ink-faint">
                    {formatCount(slice.rows)}
                </td>
            </tr>
        {/each}
    </tbody>
</table>
