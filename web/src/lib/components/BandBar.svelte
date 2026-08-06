<script lang="ts">
    /**
     * The eleven kinds rolled into the four things a reader wants first: working, waiting, lost, and
     * compaction. It's a share of lane time, same as the pie, and it sits above the pie because
     * "how much of this was waiting" is the question nobody has to be taught to ask.
     */
    import { formatDuration, formatShare } from '$lib/format'
    import type { Bands } from '$lib/transform/pie'

    const { bands }: { bands: Bands } = $props()

    const segments = $derived(
        [
            { key: 'working', band: bands.work, color: 'var(--csa-band-work)' },
            { key: 'waiting', band: bands.wait, color: 'var(--csa-band-wait)' },
            { key: 'lost', band: bands.trouble, color: 'var(--csa-band-trouble)' },
            { key: 'compacting', band: bands.overhead, color: 'var(--csa-kind-compacting)' },
        ].filter((s) => s.band.seconds > 0),
    )
</script>

<div>
    <div
        class="flex h-2.5 overflow-hidden rounded-full bg-sunken"
        role="img"
        aria-label={segments
            .map((s) => `${s.key}, ${formatShare(s.band.seconds, bands.total)} of lane time`)
            .join('. ')}
    >
        {#each segments as segment (segment.key)}
            <span class="h-full" style:width={`${segment.band.share * 100}%`} style:background-color={segment.color}
            ></span>
        {/each}
    </div>
    <ul class="mt-2.5 flex flex-wrap gap-x-5 gap-y-1.5 text-sm">
        {#each segments as segment (segment.key)}
            <li class="flex items-baseline gap-1.5">
                <span class="size-2 shrink-0 rounded-full" style:background-color={segment.color}></span>
                <span class="text-ink-muted">{segment.key}</span>
                <span class="tnum font-mono text-ink">{formatShare(segment.band.seconds, bands.total)}</span>
                <span class="tnum font-mono text-xs text-ink-faint">{formatDuration(segment.band.seconds)}</span>
            </li>
        {/each}
    </ul>
</div>
