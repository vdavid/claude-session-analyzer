<script lang="ts">
    /**
     * One ECharts instance, kept alive against a Svelte lifecycle.
     *
     * Every chart on the page goes through here, so instance handling (create, resize, dispose)
     * exists once. A chart component's whole job is to build an option object; this owns the rest.
     *
     * `notMerge` is on for every update: the charts rebuild their series wholesale when the theme
     * or the data changes, and a merge would leave the previous series behind.
     */
    import { onMount } from 'svelte'
    import { echarts, type EChartsOption, type EChartsType } from './echarts'

    interface Props {
        option: EChartsOption
        /** CSS height. The chart fills its container's width on its own. */
        height: string
        /** What a screen reader is told this chart shows. Charts are images to them. */
        label: string
        /** Handlers by ECharts event name, wired once when the instance is created. */
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ECharts hands its handlers untyped payloads.
        events?: Record<string, (params: any) => void>
        /** Lights up one slice from outside the canvas, so the legend and the pie point at each other. */
        highlight?: number | null
    }

    const { option, height, label, events = {}, highlight = null }: Props = $props()

    let host: HTMLDivElement
    let chart: EChartsType | undefined

    onMount(() => {
        chart = echarts.init(host, undefined, { renderer: 'canvas' })
        for (const [name, handler] of Object.entries(events)) chart.on(name, handler)

        const observer = new ResizeObserver(() => chart?.resize())
        observer.observe(host)

        return () => {
            observer.disconnect()
            chart?.dispose()
            chart = undefined
        }
    })

    $effect(() => {
        chart?.setOption(option, { notMerge: true })
    })

    $effect(() => {
        if (!chart) return
        chart.dispatchAction({ type: 'downplay', seriesIndex: 0 })
        if (highlight !== null) chart.dispatchAction({ type: 'highlight', seriesIndex: 0, dataIndex: highlight })
    })
</script>

<div bind:this={host} style:height role="img" aria-label={label} class="w-full"></div>
