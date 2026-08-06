<script lang="ts">
    /**
     * What the page says when it has nothing to show: a headline, what to do about it, and nothing
     * else. Never the words "error" or "failed", per the repo's voice.
     */
    import type { Snippet } from 'svelte'

    interface Props {
        headline: string
        detail?: string
        tone?: 'quiet' | 'trouble'
        children?: Snippet
    }

    const { headline, detail, tone = 'quiet', children }: Props = $props()
</script>

<div
    class="card px-5 py-6 text-center"
    class:border-band-trouble={tone === 'trouble'}
    role={tone === 'trouble' ? 'alert' : undefined}
>
    <p class="text-base font-medium text-ink">{headline}</p>
    {#if detail}
        <p class="mx-auto mt-1.5 max-w-prose text-sm leading-relaxed text-ink-muted">{detail}</p>
    {/if}
    {#if children}
        <div class="mt-4">{@render children()}</div>
    {/if}
</div>
