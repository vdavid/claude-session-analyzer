<script lang="ts">
    /**
     * Which agents were alive when, and where the holes are.
     *
     * A row's thick bar is the stretches it produced something; the thin bars are the stretches it
     * didn't, coloured by what it was waiting on. Thickness is the channel that carries "working",
     * so a row that's mostly thin reads as mostly idle from across the room, which is the finding
     * the reference session exists to show: its lead was idle for 71 of its 73 hours.
     *
     * Rows scroll rather than shrink. A session with 983 lanes would otherwise be a grey smear, so
     * the chart draws a fixed number of full-height rows and moves a window over the rest.
     */
    import { formatDuration, formatInstant } from '$lib/format'
    import { theme } from '$lib/theme.svelte'
    import type { SwimlaneRow } from '$lib/transform/swimlane'
    import Chart from './Chart.svelte'
    import { baseOption, escapeHtml, type EChartsOption } from './echarts'

    interface Props {
        rows: SwimlaneRow[]
        from: number
        until: number
        onToggleWorkflow?: (workflowId: string) => void
    }

    const { rows, from, until, onToggleWorkflow }: Props = $props()

    const ROW_HEIGHT = 26
    const MAX_VISIBLE_ROWS = 22
    /** Room for the x axis, its slider, and the grid's own top padding. */
    const CHART_CHROME = 86

    const palette = $derived(theme.palette)

    /**
     * The time window the reader scrubbed to, kept outside the option.
     *
     * Opening a workflow changes the rows, which rebuilds the option, and `notMerge` would otherwise
     * throw the zoom away: scrub to an interesting hour, open the workflow that ran in it, and you're
     * back at three days wide. ECharts doesn't emit `dataZoom` for a programmatic `setOption`, so
     * feeding this back in can't loop.
     */
    let window = $state({ start: 0, end: 100 })
    const visibleRows = $derived(Math.min(Math.max(rows.length, 1), MAX_VISIBLE_ROWS))
    const scrolls = $derived(rows.length > MAX_VISIBLE_ROWS)
    const height = $derived(`${visibleRows * ROW_HEIGHT + CHART_CHROME}px`)

    /** `[rowIndex, startMs, endMs, fill, opacity]`. The last two are read by `renderItem`. */
    type BarValue = [number, number, number, string, number]

    interface Bar {
        value: BarValue
        row: SwimlaneRow
        kind: string | null
    }

    const busyBars: Bar[] = $derived(
        rows.flatMap((row, i) =>
            row.busy.map((s) => ({
                value: [
                    i,
                    s[0],
                    s[1],
                    row.kind === 'lead' ? palette.chrome.accent : palette.chrome.work,
                    1,
                ] as BarValue,
                row,
                kind: null,
            })),
        ),
    )

    const gapBars: Bar[] = $derived(
        rows.flatMap((row, i) =>
            row.gaps.map((g) => ({
                value: [i, g.from, g.until, palette.kind[g.kind] ?? palette.chrome.inkFaint, 0.85] as BarValue,
                row,
                kind: g.kind,
            })),
        ),
    )

    /** `↳` marks a lane revealed inside a workflow, `▾`/`▸` a workflow that opens. */
    function axisLabel(row: SwimlaneRow): string {
        const marker = row.expandable ? (row.expanded ? '▾ ' : '▸ ') : row.depth > 0 ? '  ↳ ' : ''
        const label = row.label.length > 20 ? `${row.label.slice(0, 19)}…` : row.label
        return marker + label
    }

    /**
     * A bar's colour rides along as a fourth dimension of its data point. ECharts 6 deprecated
     * `api.style()`, and reading the fill off the point is the replacement it asks for.
     */
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ECharts hands `renderItem` untyped coordinate helpers.
    function renderBar(thickness: number): any {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        return (params: any, api: any) => {
            const rowIndex = api.value(0)
            const start = api.coord([api.value(1), rowIndex])
            const end = api.coord([api.value(2), rowIndex])
            const band = api.size([0, 1])[1]
            const barHeight = Math.max(band * thickness, 2)
            const grid = params.coordSys
            const x = Math.max(start[0], grid.x)
            const right = Math.min(Math.max(end[0], start[0] + 1.5), grid.x + grid.width)
            if (right <= x) return
            return {
                type: 'rect',
                shape: { x, y: start[1] - barHeight / 2, width: right - x, height: barHeight, r: 1 },
                style: { fill: api.value(3), opacity: api.value(4) },
            }
        }
    }

    function tooltip(bar: Bar): string {
        const [, start, end] = bar.value
        const span = `${formatInstant(new Date(start).toISOString())} to ${formatInstant(new Date(end).toISOString())}`
        const name = bar.row.sublabel ? `${bar.row.label} (${bar.row.sublabel})` : bar.row.label
        const heading = bar.kind
            ? `<strong>${escapeHtml(bar.kind)}</strong>`
            : `<strong>${escapeHtml(name)}</strong> was producing`
        const trailer = bar.row.expandable ? '<br><em>Click to open this workflow.</em>' : ''
        return `${heading}<br>${escapeHtml(name)} · ${formatDuration((end - start) / 1000)}<br>${escapeHtml(span)}${trailer}`
    }

    const option: EChartsOption = $derived({
        ...baseOption(palette.chrome),
        animation: false,
        tooltip: {
            ...baseOption(palette.chrome).tooltip,
            trigger: 'item',
            formatter: (p: unknown) => {
                const params = p as { seriesIndex: number; dataIndex: number }
                const bar = (params.seriesIndex === 0 ? gapBars : busyBars)[params.dataIndex]
                return bar ? tooltip(bar) : ''
            },
        },
        grid: { left: 156, right: scrolls ? 34 : 18, top: 10, bottom: 62 },
        xAxis: {
            type: 'time',
            min: from,
            max: until,
            axisLine: { lineStyle: { color: palette.chrome.border } },
            axisTick: { lineStyle: { color: palette.chrome.border } },
            axisLabel: { color: palette.chrome.inkFaint, fontSize: 11, hideOverlap: true },
            splitLine: { show: true, lineStyle: { color: palette.chrome.border, type: 'dashed', opacity: 0.6 } },
        },
        yAxis: {
            type: 'category',
            inverse: true,
            data: rows.map(axisLabel),
            axisLine: { show: false },
            axisTick: { show: false },
            axisLabel: {
                interval: 0,
                color: palette.chrome.inkMuted,
                fontSize: 11,
                fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                margin: 12,
            },
            splitArea: { show: true, areaStyle: { color: ['transparent', palette.chrome.sunken] } },
        },
        dataZoom: [
            {
                type: 'inside',
                xAxisIndex: 0,
                filterMode: 'none',
                start: window.start,
                end: window.end,
                // Plain wheel keeps scrolling the page. Ctrl and pinch zoom time, drag pans it.
                zoomOnMouseWheel: 'ctrl',
                moveOnMouseWheel: false,
                moveOnMouseMove: true,
            },
            {
                type: 'slider',
                xAxisIndex: 0,
                filterMode: 'none',
                start: window.start,
                end: window.end,
                height: 22,
                bottom: 14,
                borderColor: palette.chrome.border,
                backgroundColor: palette.chrome.sunken,
                fillerColor: palette.dark ? 'rgba(122, 167, 232, 0.16)' : 'rgba(44, 95, 168, 0.12)',
                handleStyle: { color: palette.chrome.surface, borderColor: palette.chrome.borderStrong },
                moveHandleStyle: { color: palette.chrome.borderStrong },
                dataBackground: { lineStyle: { opacity: 0 }, areaStyle: { opacity: 0 } },
                selectedDataBackground: { lineStyle: { opacity: 0 }, areaStyle: { opacity: 0 } },
                textStyle: { color: palette.chrome.inkFaint, fontSize: 10 },
                labelFormatter: (value: number) => formatInstant(new Date(value).toISOString()),
            },
            ...(scrolls
                ? [
                      {
                          type: 'slider' as const,
                          yAxisIndex: 0,
                          filterMode: 'none' as const,
                          right: 6,
                          width: 14,
                          top: 10,
                          bottom: 62,
                          startValue: 0,
                          endValue: MAX_VISIBLE_ROWS - 1,
                          zoomLock: true,
                          brushSelect: false,
                          showDetail: false,
                          borderColor: 'transparent',
                          backgroundColor: palette.chrome.sunken,
                          fillerColor: palette.dark ? 'rgba(122, 167, 232, 0.22)' : 'rgba(44, 95, 168, 0.16)',
                          handleStyle: { opacity: 0 },
                          moveHandleStyle: { opacity: 0 },
                          dataBackground: { lineStyle: { opacity: 0 }, areaStyle: { opacity: 0 } },
                          selectedDataBackground: { lineStyle: { opacity: 0 }, areaStyle: { opacity: 0 } },
                      },
                      {
                          type: 'inside' as const,
                          yAxisIndex: 0,
                          filterMode: 'none' as const,
                          startValue: 0,
                          endValue: MAX_VISIBLE_ROWS - 1,
                          zoomLock: true,
                          zoomOnMouseWheel: false,
                          moveOnMouseWheel: false,
                          moveOnMouseMove: false,
                      },
                  ]
                : []),
        ],
        series: [
            {
                name: 'idle',
                type: 'custom',
                clip: true,
                renderItem: renderBar(0.34),
                encode: { x: [1, 2], y: 0 },
                data: gapBars.map((b) => ({ value: b.value })),
            },
            {
                name: 'producing',
                type: 'custom',
                clip: true,
                renderItem: renderBar(0.66),
                encode: { x: [1, 2], y: 0 },
                data: busyBars.map((b) => ({ value: b.value })),
            },
        ],
    })

    const events = {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the click payload is ECharts'.
        click: (params: any) => {
            const bar = (params.seriesIndex === 0 ? gapBars : busyBars)[params.dataIndex]
            if (bar?.row.expandable && bar.row.workflowId) onToggleWorkflow?.(bar.row.workflowId)
        },
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- so is this one.
        dataZoom: (params: any) => {
            // A wheel or a drag reports the window inline; the slider reports it in `batch`.
            const moved = params.batch?.[0] ?? params
            if (typeof moved.start === 'number' && typeof moved.end === 'number') {
                window = { start: moved.start, end: moved.end }
            }
        },
    }
</script>

<Chart
    {option}
    {height}
    {events}
    label={`Agent liveness over the session. ${rows.length} rows, from ${formatInstant(new Date(from).toISOString())} to ${formatInstant(new Date(until).toISOString())}.`}
/>
