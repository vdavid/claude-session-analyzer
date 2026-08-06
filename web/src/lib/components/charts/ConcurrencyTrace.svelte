<script lang="ts">
    /**
     * How many agents were producing at once, across the whole session.
     *
     * The pie flattens time away and the swimlane spreads it over more rows than anyone can take in
     * at once. This strip is the shape of the session in one line: where the parallel waves were,
     * and how much of the span had nobody working at all.
     *
     * No axes. It sits under the session header as a trace, and the numbers that matter are spelled
     * out in the caption beside it.
     */
    import { formatTimeOfDay } from '$lib/format'
    import { theme } from '$lib/theme.svelte'
    import type { Trace } from '$lib/transform/concurrency'
    import Chart from './Chart.svelte'
    import { baseOption, type EChartsOption } from './echarts'

    interface Props {
        trace: Trace
        height?: string
    }

    const { trace, height = '76px' }: Props = $props()

    const palette = $derived(theme.palette)

    const option: EChartsOption = $derived({
        ...baseOption(palette.chrome),
        animation: !theme.reducedMotion,
        animationDuration: 600,
        grid: { left: 0, right: 0, top: 6, bottom: 0 },
        tooltip: {
            ...baseOption(palette.chrome).tooltip,
            trigger: 'axis',
            axisPointer: { type: 'line', lineStyle: { color: palette.chrome.borderStrong, width: 1 } },
            formatter: (p: unknown) => {
                const points = p as { value: [number, number] }[]
                const point = points[0]
                if (!point) return ''
                const lanes = point.value[1]
                const rounded = lanes < 1 && lanes > 0 ? lanes.toFixed(2) : Math.round(lanes).toString()
                return `${formatTimeOfDay(point.value[0])} · ${rounded} ${lanes >= 1.5 ? 'agents' : 'agent'} producing`
            },
        },
        xAxis: { type: 'time', min: trace.from, max: trace.until, show: false },
        yAxis: { type: 'value', min: 0, max: Math.max(trace.peak, 1), show: false },
        series: [
            {
                type: 'line',
                step: 'end',
                showSymbol: false,
                lineStyle: { width: 1, color: palette.chrome.work },
                areaStyle: { color: palette.chrome.work, opacity: palette.dark ? 0.34 : 0.22 },
                data: trace.points.map((p) => [p.t, p.lanes]),
            },
        ],
    })
</script>

{#if trace.points.length}
    <Chart {option} {height} label={`Agents producing over time. At most ${Math.round(trace.peak)} at once.`} />
{/if}
