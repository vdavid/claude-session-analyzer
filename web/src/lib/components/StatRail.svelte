<script lang="ts">
    /**
     * A row of numbers with the caveat attached. Every stat here has one, and a number whose caveat
     * lives somewhere else gets read wrong, so `note` sits under the value rather than in a tooltip.
     */
    interface Stat {
        label: string
        value: string
        note?: string
        /** Shown on hover and to screen readers, for the longer version of the caveat. */
        title?: string
    }

    const { stats }: { stats: Stat[] } = $props()
</script>

<dl class="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-border-base bg-border-base sm:grid-cols-4">
    {#each stats as stat (stat.label)}
        <!-- The note lives inside the `dd`: a group in a `dl` may hold only a `dt` and its `dd`, and
             the caveat is part of what the number means rather than a sibling remark. -->
        <div class="bg-surface px-4 py-3" title={stat.title}>
            <dt class="eyebrow">{stat.label}</dt>
            <dd class="mt-1">
                <span class="tnum block font-mono text-xl leading-tight font-medium tracking-tight text-ink">
                    {stat.value}
                </span>
                {#if stat.note}
                    <span class="mt-1 block text-xs leading-snug text-ink-faint">{stat.note}</span>
                {/if}
            </dd>
        </div>
    {/each}
</dl>
