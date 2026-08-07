# transform/: API JSON into chart series

Only layer here with tests, because it's the only one where being wrong is invisible. A button either works or it
doesn't; a swimlane bar drawn one hour too wide looks exactly like a correct one.

## Must-knows

- **`busySegments` is the honesty rule.** A lane's bar is its span minus its gaps, never the span itself. Reference
  session's lead would otherwise claim it was busy 71 of its 73 hours.
- **A workflow collapses to one row** whose bar is the union of its members'. A hole in that row means the whole
  workflow was quiet, a real finding rather than a rendering shortcut. Expanded groups are capped
  (`DEFAULT_MAX_LANES_PER_GROUP`) and report what they held back in `hiddenLanes`.
- **Members of a workflow are labelled by lane id when their names collide**, the normal case: 848 lanes all called
  `workflow-subagent`. Real name moves to `sublabel`.
- **`concurrencyTrace` spans the lanes' own ends, not their busy segments.** Deriving span from segments would crop a
  session that opened or closed with everyone idle, the stretch a reader is looking for. A bucket holds average lanes
  producing during it, so area under the trace equals the lane time behind it.
- **`kindSlices` keeps legend order, not size order**, so a colour means the same thing on every session and the four
  waits stay adjacent.
- **`toolBreakdown` counts calls, never seconds**, and orders by family for the same reason. A pie of tool seconds says
  what the machine was busy with; a `pnpm check` is minutes and a codegraph lookup is a second. Family order also makes
  each family one contiguous arc, the fixed adjacency the palette was validated against.

## Module map

- `swimlane.ts`: `busySegments` (span minus gaps), `buildSwimlane` (lead, then workflows, then direct lanes).
- `concurrency.ts`: lanes producing at once, bucketed over the session's span.
- `pie.ts`: `kindSlices` for the donut and its legend, `bandTotals` for the working / waiting / lost / compacting bar.
- `tools.ts`: `toolBreakdown`, the API's tool groups ordered for the tool donut, family arcs over them.

Everything takes milliseconds since the epoch, what ECharts' time axis reads. Instants stay strings until they cross
into here.
