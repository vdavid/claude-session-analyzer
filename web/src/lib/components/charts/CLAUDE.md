# charts/: the ECharts layer

One component per chart, all of them over `Chart.svelte`. A chart component's whole job is to build an option object;
instance handling lives in one place.

## Must-knows

- **Register what you use in `echarts.ts`.** The library is assembled from parts, so a chart type or component that
  isn't in that `echarts.use([…])` call silently renders nothing.
- **Colours are literals, not `var(…)`.** Canvas can't read custom properties, so charts take them from `theme.palette`,
  which re-reads the stylesheet when `prefers-color-scheme` flips. Never hardcode a hex here.
- **`api.style()` is gone in ECharts 6.** A custom series carries its fill as an extra dimension of each data point and
  `renderItem` reads it back with `api.value(n)`.
- **Tooltips are HTML**, so anything from a transcript goes through `escapeHtml`. Row info holds shell commands.
- `Chart.svelte` sets options with `notMerge: true`: charts rebuild their series wholesale when the theme or the data
  changes, and a merge would leave the old series behind.
- Chart animation is a JS option, so it reads `theme.reducedMotion` rather than relying on the CSS media query. The
  swimlane has animation off outright: a few hundred rects tweening is noise, not motion design.

## Module map

- `Chart.svelte`: create, resize (`ResizeObserver`), dispose, event wiring, and outside-in highlight.
- `KindPie.svelte`: the donut. Waits carry a diagonal hatch and the two ways a session loses time carry dots, so the
  chart survives being read without hue.
- `Swimlane.svelte`: two custom series, thin bars for gaps and thick for producing. Rows scroll through a fixed window
  (`MAX_VISIBLE_ROWS`) rather than shrinking, and a click on a workflow's bar opens it.
- `ConcurrencyTrace.svelte`: the stepped area strip under the session header.
- `echarts.ts`: the assembled library, the composed option type, and the look every chart shares.
