# components/: the display layer

Thin by design. Business logic belongs in the Go engine, shaping in `src/lib/transform/`; these render what those two
hand over.

## Must-knows

- **`DataSheet.svelte` builds its TanStack table with a full state.** `table.getState()` returns `options.state`
  verbatim, so a partial one leaves column pinning undefined and the first header read throws. Default state only exists
  once the table is built, hence the `setOptions` right after `createTable`.
- **Row height is fixed**, which lets the virtualizer measure nothing. Long text is clipped to one line, carried in full
  on the row's `title`. Anything that can wrap breaks row alignment.
- **Sort an instant through a numeric accessor.** RFC 3339 strings look sortable and aren't: the engine trims trailing
  zeros, so `…16.703Z` sorts after `…16.7Z` as text.
- **Scroll container carries `tabindex="0"`** with an a11y suppression: a scrollable region has to be reachable by
  keyboard, and 22,000 rows behind a pointer-only scroll is not usable.
- Search box is debounced because a keystroke rebuilds the row model over every row.
- **A stacked bar's 2px separator is a flex `gap`, never a border inside the segments.** A border swallows any segment
  narrower than itself, and `SendMessage`'s 1m49s of composing is about a pixel on a 10-hour axis. Costs up to 4px of
  overstated length on a three-segment bar, which beats losing a value.
- **`ToolClockBars` prints one clock per row, named, and no total.** The three clocks are three measurements that happen
  to share a name; a figure over all three reports a tool as costing what the agent and a suspension cost. Same reason
  `ToolLegend` has three columns and no total column, and writes `–` rather than `0s` where nothing stalled.
- **A clock bar's whole row is one button** carrying every number in its `aria-label`, because the bars are where a
  sighted reader gets the comparison and a screen reader gets nothing from a `<div>` of widths.

## Module map

- `DataSheet.svelte`: the virtualized sheet. Sorting and filtering through TanStack Table, windowing through TanStack
  Virtual, plus four exact-match dropdowns applied before the table sees the rows. `toolFilter` is `$bindable`, so the
  tool breakdown above drives it.
- `KindLegend.svelte`: the pie's legend as a table, each kind's caveat with it. Hovering a row lights its slice.
- `ToolLegend.svelte`: the breakdown as a table and the legend for both charts above it, and where the question actually
  gets answered: calls, share, composing, running, stalled, and the lane count saying who reached for each tool. A group
  with several tools in it opens to show them.
- `ToolClockBars.svelte`: one ranked horizontal stacked bar per tool group, split into the three clocks. Hue is the
  category, treatment is the clock, position is when it happened. Hover lights the donut's arc, click filters the sheet.
  Encoding decision: `docs/frontend.md` § Three clocks, three treatments, not three hues.
- `TimeLadder.svelte`: lane time, net agent time, active time as nested neutral bars, each printing what it lost.
  Elapsed appears under it as the thing that isn't a rung.
- `BandBar.svelte`: the eleven kinds rolled into working, waiting, lost, compacting.
- `StatRail.svelte`: numbers with their caveat under them rather than in a tooltip. Column count follows the stat count.
- `Notice.svelte`: empty and unreachable states. Never says "error" or "failed".
- `charts/`: see `web/src/lib/components/charts/CLAUDE.md`.
