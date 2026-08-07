# agg/: one cube every total rolls up from

Sums a timeline once, keyed by lane, agent, kind, class, group, leaf, tool, and local day. `RollUp` keeps whichever
dimensions a caller asks for. API pie, API tool breakdown, cached digest, and a `stats` query are all this same sum.

## Must-knows

- **Call `ToolRuns` before reporting on a tool, and before rolling the kind dimension away.** Every call leaves two
  rows: agent composing it, and tool running. Both carry the tool's name. Take both and a tool reads as costing about
  double, and nothing in the output looks wrong. `ToolRuns` drops the `tool call` cells, which is why a tool query
  grouped by kind shows `tool execution` and `stalled` but never `tool call`.
- **A row crossing local midnight splits its seconds across days, counts on the day it started.** Rolling the day
  dimension away has to give the row back whole. Counting it once per midnight crossed would inflate `Rows` on a
  three-day session, and that's the one property every downstream total rests on.
- **`Lanes` is a distinct count, not a sum.** One lane calling two of a server's methods is one lane for the server and
  one for each method. `RollUp` recomputes it from contributing cells, so never add two cells' `Lanes` by hand. A cell
  that arrived already rolled up past the lane dimension carries a count instead of a lane; `RollUp` takes the larger,
  which is exact because a cell can't have both.
- **Cells carry `time.Duration`, not seconds.** Rounding happens once, at the edge, in `report.Seconds`. Round earlier
  and a hundred thousand rows drift away from the totals they were added from.
- **`RollUp` sorts by key**, so two runs over the same session agree and a golden file stays stable.
- Zone defaults to UTC. Local is what day-bucketed questions want ("how much in July"), and the caller passes it:
  `internal/cache` does, `internal/report` doesn't because it rolls the day away.

## Module map

- `agg.go`: `Dim` bitmask, `Key`, `Cell`, `Build`, `Cube.RollUp`, `RollUp` over loose cells (what a cached digest rolls
  up again), `ToolRuns`, `Sum`, `splitByDay`.
