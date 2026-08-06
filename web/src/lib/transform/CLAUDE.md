# transform/ — API JSON into chart series

The only layer here with tests, because it's the only one where being wrong is invisible. A button either works or it
doesn't; a swimlane bar drawn one hour too wide looks exactly like a correct one.

## Must-knows

- **`busySegments` is the honesty rule.** A lane's bar is its span minus its gaps, never the span itself. The reference
  session's lead would otherwise claim it was busy for 71 of its 73 hours.
- **A workflow collapses to one row** whose bar is the union of its members'. A hole in that row means the whole
  workflow was quiet, which is a real finding rather than a rendering shortcut. Expanded groups are capped
  (`DEFAULT_MAX_LANES_PER_GROUP`) and report what they held back in `hiddenLanes`.
- **Members of a workflow are labelled by lane id when their names collide**, which is the normal case: 848 lanes all
  called `workflow-subagent`. The real name moves to `sublabel`.
- **`concurrencyTrace` spans the lanes' own ends, not their busy segments.** Deriving the span from the segments would
  crop a session that opened or closed with everyone idle, which is the stretch a reader is looking for. A bucket holds
  the average lanes producing during it, so the area under the trace equals the lane time behind it.
- **`kindSlices` keeps legend order, not size order**, so a colour means the same thing on every session and the four
  waits stay adjacent.

## Module map

- `swimlane.ts`: `busySegments` (span minus gaps) and `buildSwimlane` (lead, then workflows, then direct lanes).
- `concurrency.ts`: lanes producing at once, bucketed over the session's span.
- `pie.ts`: `kindSlices` for the donut and its legend, `bandTotals` for the working / waiting / lost / compacting bar.

Everything takes milliseconds since the epoch, which is what ECharts' time axis reads. Instants stay strings until they
cross into here.
