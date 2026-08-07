<script lang="ts">
    /**
     * A donut of lane time by activity kind.
     *
     * Slices keep legend order rather than size order, so the four waits sit together and a colour
     * means the same thing on every session. Waits carry a diagonal hatch and the two ways a
     * session loses time carry dots, which is a second channel for anyone who can't lean on hue and
     * a reminder that those slices aren't work.
     */
    import { formatDuration, formatShare } from '$lib/format'
    import { theme } from '$lib/theme.svelte'
    import type { KindSlice } from '$lib/transform/pie'
    import Chart from './Chart.svelte'
    import { baseOption, escapeHtml, type EChartsOption } from './echarts'

    interface Props {
        slices: KindSlice[]
        height?: string
        /** Index of the legend row the pointer is on, or null. */
        highlight?: number | null
    }

    const { slices, height = '320px', highlight = null }: Props = $props()

    const palette = $derived(theme.palette)

    /**
     * Ink over a saturated fill in light mode, shadow over a bright one in dark. It's a token rather
     * than a literal here because the tool clock bars draw their own texture in the same ink, and two
     * definitions of one colour drift.
     */
    const decalInk = $derived(palette.chrome.decalInk)

    const hatch = $derived({
        color: decalInk,
        symbol: 'rect',
        symbolSize: 1,
        dashArrayX: [1, 5],
        dashArrayY: [6, 0],
        rotation: Math.PI / 4,
    })

    const dots = $derived({
        color: decalInk,
        symbol: 'circle',
        symbolSize: 0.4,
        dashArrayX: [
            [4, 4],
            [0, 4, 4, 0],
        ],
        dashArrayY: [3, 3],
    })

    const total = $derived(slices.reduce((sum, s) => sum + s.seconds, 0))

    const option: EChartsOption = $derived({
        ...baseOption(palette.chrome),
        animation: !theme.reducedMotion,
        animationDuration: 480,
        tooltip: {
            ...baseOption(palette.chrome).tooltip,
            trigger: 'item',
            formatter: (p: unknown) => {
                const params = p as { dataIndex: number }
                const slice = slices[params.dataIndex]
                if (!slice) return ''
                return `<strong>${escapeHtml(slice.kind)}</strong><br>${formatDuration(slice.seconds)} of lane time, ${formatShare(slice.seconds, total)}`
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
                data: slices.map((s) => ({
                    name: s.kind,
                    value: s.seconds,
                    itemStyle: {
                        color: palette.kind[s.kind] ?? palette.chrome.inkFaint,
                        decal: s.family === 'wait' ? hatch : s.family === 'trouble' ? dots : undefined,
                    },
                })),
            },
        ],
    })
</script>

<div class="relative">
    <Chart
        {option}
        {height}
        {highlight}
        label={`Lane time by activity kind. ${slices.map((s) => `${s.kind}, ${formatShare(s.seconds, total)}`).join('. ')}`}
    />
    <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
        <span class="eyebrow">Lane time</span>
        <span class="tnum mt-0.5 font-mono text-2xl font-medium tracking-tight text-ink">
            {formatDuration(total)}
        </span>
    </div>
</div>
