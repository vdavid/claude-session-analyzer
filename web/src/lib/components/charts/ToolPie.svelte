<script lang="ts">
    /**
     * A donut of tool calls, by tool.
     *
     * It counts **calls**, not seconds. `pnpm check` costs minutes and a codegraph lookup costs a
     * second, so a pie of seconds would say what the machine was busy with rather than what the
     * agents reached for. The seconds are in the legend, where they read as what they are.
     *
     * Slices sit in the category order the API serves, so each category is one contiguous arc and a
     * colour means the same thing on every session (`src/lib/transform/tools.ts`). Groups inside a
     * category share its colour and are told apart by the 2px surface ring between them, by the legend
     * beside the chart, and by the highlight the two share.
     */
    import { formatCount, formatDuration, formatShare } from '$lib/format'
    import { theme } from '$lib/theme.svelte'
    import type { ToolSlice } from '$lib/transform/tools'
    import Chart from './Chart.svelte'
    import { baseOption, escapeHtml, type EChartsOption } from './echarts'

    interface Props {
        slices: ToolSlice[]
        calls: number
        height?: string
        /** Index of the legend row the pointer is on, or null. */
        highlight?: number | null
        /** A click on a slice, by group name. */
        onSelect?: (group: string) => void
    }

    const { slices, calls, height = '300px', highlight = null, onSelect }: Props = $props()

    const palette = $derived(theme.palette)

    const option: EChartsOption = $derived({
        ...baseOption(palette.chrome),
        animation: !theme.reducedMotion,
        animationDuration: 480,
        tooltip: {
            ...baseOption(palette.chrome).tooltip,
            trigger: 'item',
            formatter: (p: unknown) => {
                const slice = slices[(p as { dataIndex: number }).dataIndex]
                if (!slice) return ''
                const lanes = `${formatCount(slice.lanes)} ${slice.lanes === 1 ? 'lane' : 'lanes'}`
                return (
                    `<strong>${escapeHtml(slice.group)}</strong><br>` +
                    `${formatCount(slice.calls)} ${slice.calls === 1 ? 'call' : 'calls'}, ${formatShare(slice.calls, calls)}<br>` +
                    `${formatDuration(slice.seconds)} of tool time, from ${lanes}`
                )
            },
        },
        series: [
            {
                type: 'pie',
                radius: ['56%', '86%'],
                center: ['50%', '50%'],
                minAngle: 0.6,
                label: { show: false },
                labelLine: { show: false },
                itemStyle: { borderColor: palette.chrome.surface, borderWidth: 2 },
                emphasis: { scaleSize: 6, itemStyle: { shadowBlur: 12, shadowColor: 'rgba(0, 0, 0, 0.2)' } },
                cursor: onSelect ? 'pointer' : 'default',
                data: slices.map((s) => ({
                    name: s.group,
                    value: s.calls,
                    itemStyle: { color: palette.category[s.category] ?? palette.chrome.inkFaint },
                })),
            },
        ],
    })

    const events: Record<string, (params: { dataIndex: number }) => void> = {
        click: (p) => {
            const group = slices[p.dataIndex]?.group
            if (group) onSelect?.(group)
        },
    }
</script>

<div class="relative">
    <Chart
        {option}
        {height}
        {highlight}
        {events}
        label={`Tool calls by tool. ${slices
            .slice(0, 8)
            .map((s) => `${s.group}, ${formatShare(s.calls, calls)}`)
            .join('. ')}`}
    />
    <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
        <span class="eyebrow">Tool calls</span>
        <span class="tnum mt-0.5 font-mono text-2xl font-medium tracking-tight text-ink">
            {formatCount(calls)}
        </span>
    </div>
</div>
