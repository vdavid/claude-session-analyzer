# charts/: the ECharts layer

One component per chart, all over `Chart.svelte`. A chart component's whole job is building an option object; instance
handling lives in one place.

## Must-knows

- **Register what you use in `echarts.ts`.** Library is assembled from parts, so a chart type or component not in that
  `echarts.use([…])` call silently renders nothing.
- **Colours are literals, not `var(…)`.** Canvas can't read custom properties, so charts take them from `theme.palette`,
  which re-reads the stylesheet when `prefers-color-scheme` flips. Never hardcode a hex here.
- **`api.style()` is gone in ECharts 6.** A custom series carries its fill as an extra dimension of each data point and
  `renderItem` reads it back with `api.value(n)`.
- **Tooltips are HTML**, so anything from a transcript goes through `escapeHtml`. Row info holds shell commands.
- `Chart.svelte` sets options with `notMerge: true`: charts rebuild their series wholesale when theme or data changes,
  and a merge would leave the old series behind.
- Chart animation is a JS option, so it reads `theme.reducedMotion` rather than the CSS media query. Swimlane has
  animation off outright: a few hundred rects tweening is noise, not motion design.

## Module map

- `Chart.svelte`: create, resize (`ResizeObserver`), dispose, event wiring, outside-in highlight.
- `KindPie.svelte`: donut of lane time. Waits carry a diagonal hatch and the two ways a session loses time carry dots,
  so the chart survives being read without hue.
- `ToolPie.svelte`: donut of tool calls, coloured by tool category rather than group. Slices of one category are
  adjacent and share a colour by design; the 2px surface ring, the legend, and the shared highlight tell them apart.
- `Swimlane.svelte`: two custom series, thin bars for gaps and thick for producing. Rows scroll through a fixed window
  (`MAX_VISIBLE_ROWS`) rather than shrinking, and a click on a workflow's bar opens it.
- `ConcurrencyTrace.svelte`: stepped area strip under the session header.
- `echarts.ts`: assembled library, composed option type, the look every chart shares.
