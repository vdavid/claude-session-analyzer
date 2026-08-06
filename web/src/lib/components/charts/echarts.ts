/**
 * ECharts, assembled from its parts.
 *
 * Importing `echarts` whole pulls in every chart type and every component. Registering only what
 * the three charts here use keeps the bundle to what's on screen, and it means a new chart has to
 * say what it needs rather than inheriting everything by accident.
 *
 * The canvas renderer, not SVG: the swimlane draws a few hundred rects and the trace a few hundred
 * bars, which canvas composites in one layer.
 */

import * as echarts from 'echarts/core'
import { CustomChart, LineChart, PieChart } from 'echarts/charts'
import { DataZoomComponent, GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ComposeOption } from 'echarts/core'
import type { CustomSeriesOption, LineSeriesOption, PieSeriesOption } from 'echarts/charts'
import type { DataZoomComponentOption, GridComponentOption, TooltipComponentOption } from 'echarts/components'

echarts.use([PieChart, CustomChart, LineChart, TooltipComponent, GridComponent, DataZoomComponent, CanvasRenderer])

export { echarts }
export type { EChartsType } from 'echarts/core'

export type EChartsOption = ComposeOption<
    | PieSeriesOption
    | CustomSeriesOption
    | LineSeriesOption
    | TooltipComponentOption
    | GridComponentOption
    | DataZoomComponentOption
>

interface BaseOption {
    textStyle: { fontFamily: string; color: string }
    tooltip: TooltipComponentOption
}

/**
 * The look every chart on the page shares: no chart-drawn title, tooltips as small cards on the
 * page's own surface, and text in the page's own faces. Merged under each chart's own option.
 */
export function baseOption(chrome: Record<string, string>): BaseOption {
    return {
        textStyle: {
            fontFamily:
                'ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, "Helvetica Neue", sans-serif',
            color: chrome.inkMuted,
        },
        tooltip: {
            backgroundColor: chrome.surface,
            borderColor: chrome.border,
            borderWidth: 1,
            padding: [8, 10],
            textStyle: { color: chrome.ink, fontSize: 12 },
            extraCssText: 'box-shadow: 0 4px 16px rgb(0 0 0 / 0.14); border-radius: 8px; max-width: 380px;',
        },
    }
}

/** Escapes text going into a tooltip, which ECharts renders as HTML. */
export function escapeHtml(text: string): string {
    return text.replace(
        /[&<>"']/g,
        (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string,
    )
}
