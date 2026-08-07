<script lang="ts">
    /**
     * Lane time, net agent time, and active time, as three nested bars.
     *
     * The subtraction is the content. Each rung is the one above minus something named, which is what
     * stops one of the three being quoted as another: "119 hours" and "45 hours" are both true about
     * this session and they answer different questions. `docs/api.md` § The ladder is the definition.
     *
     * The bars are neutral on purpose. Every saturated pixel on this page stands for an activity kind
     * or a tool category, and these three are totals over all of them, so a colour here would read as
     * a kind that isn't in any legend.
     */
    import { formatDuration, formatDurationLong, formatShare } from '$lib/format'
    import type { RungKey, TimeLadder } from '$lib/transform/ladder'

    const { ladder }: { ladder: TimeLadder } = $props()

    const WORDS: Record<RungKey, { label: string; lost: string; note: string }> = {
        lane: {
            label: 'Lane time',
            lost: '',
            note: "Every lane's clock added up, so two agents working side by side for an hour is two hours",
        },
        net: {
            label: 'Net agent time',
            lost: 'waiting on a person or on a teammate',
            note: "What the session cost in agent time: a teammate's wait is already that teammate's own time, and a person's was never agent time",
        },
        active: {
            label: 'Active time',
            lost: 'of stalls, API errors, and waits on a background task',
            note: 'How much of it was producing something. It answers a different question from net rather than replacing it',
        },
    }

    const rungs = $derived(ladder.rungs.map((rung) => ({ ...rung, words: WORDS[rung.key] })))
</script>

<dl class="grid gap-x-4 gap-y-2.5 sm:grid-cols-[minmax(0,9rem)_minmax(0,1fr)]">
    {#each rungs as rung (rung.key)}
        <dt class="self-center text-sm text-ink-muted sm:text-right">{rung.words.label}</dt>
        <dd class="min-w-0">
            <div class="flex items-center gap-3">
                <div class="h-2 min-w-0 flex-1 overflow-hidden rounded-full bg-sunken">
                    <div
                        class="h-full rounded-r-[4px] bg-border-strong"
                        style:width={`${rung.share * 100}%`}
                        title={`${formatShare(rung.seconds, ladder.laneSeconds)} of lane time`}
                    ></div>
                </div>
                <span
                    class="tnum shrink-0 font-mono text-sm text-ink"
                    title={formatDurationLong(rung.seconds)}
                    aria-label={formatDurationLong(rung.seconds)}
                >
                    {formatDuration(rung.seconds)}
                </span>
            </div>
            <p class="mt-1 text-xs leading-snug text-ink-faint">
                {#if rung.subtracted > 0}
                    <span class="text-ink-muted">
                        minus {formatDuration(rung.subtracted)}
                        {rung.words.lost}.
                    </span>
                {/if}
                {rung.words.note}.
            </p>
        </dd>
    {/each}
</dl>

<p class="mt-3 text-xs leading-relaxed text-ink-faint">
    Elapsed time, {formatDuration(ladder.wallClockSeconds)}, isn't a rung on this ladder: it's how long the session took
    from first record to last, whatever ran at once. Lane time is the larger of the two whenever lanes overlapped.
</p>
