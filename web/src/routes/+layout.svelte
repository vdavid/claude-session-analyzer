<script lang="ts">
    import '../app.css'
    import { resolve } from '$app/paths'
    import { page } from '$app/state'
    import type { Snippet } from 'svelte'

    const { children }: { children: Snippet } = $props()

    const onSession = $derived(page.url.pathname.startsWith('/session/'))
</script>

<div class="min-h-screen">
    <header class="sticky top-0 z-20 border-b border-border-base bg-canvas/92 backdrop-blur-sm">
        <div class="mx-auto flex h-12 max-w-[92rem] items-center gap-4 px-5">
            <a href={resolve('/')} class="flex items-baseline gap-2 font-mono text-sm text-ink hover:text-accent">
                <span aria-hidden="true" class="flex items-center gap-[3px]">
                    <span class="block h-3 w-1 rounded-full" style:background="var(--csa-kind-thinking)"></span>
                    <span class="block h-3 w-1 rounded-full" style:background="var(--csa-kind-wait-person)"></span>
                    <span class="block h-3 w-1 rounded-full" style:background="var(--csa-kind-tool-call)"></span>
                </span>
                <span class="font-medium tracking-tight">Session analyzer</span>
            </a>
            {#if onSession}
                <a href={resolve('/')} class="text-sm text-ink-faint hover:text-accent">← All sessions</a>
            {/if}
            <span class="ml-auto hidden text-xs text-ink-faint sm:block"> Reading transcripts from this machine </span>
        </div>
    </header>

    <main class="mx-auto max-w-[92rem] px-5 pt-8 pb-24">
        {@render children()}
    </main>
</div>
