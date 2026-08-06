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
        <div class="bg-surface px-4 py-3" title={stat.title}>
            <dt class="eyebrow">{stat.label}</dt>
            <dd class="tnum mt-1 font-mono text-xl leading-tight font-medium tracking-tight text-ink">
                {stat.value}
            </dd>
            {#if stat.note}
                <p class="mt-1 text-xs leading-snug text-ink-faint">{stat.note}</p>
            {/if}
        </div>
    {/each}
</dl>
